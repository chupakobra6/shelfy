package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"time"

	"github.com/igor/shelfy/internal/ingest"
)

type countedRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
}

type proxySnapshot struct {
	TextCalls int
	Calls     []modelCall `json:"calls"`
}

type modelCall struct {
	Model    string `json:"model"`
	Mode     string `json:"mode"`
	Prompt   string `json:"prompt"`
	Response string `json:"response"`
	Status   int    `json:"status"`
}

type countingOllamaProxy struct {
	target string
	client *http.Client
	server *httptest.Server

	mu      sync.Mutex
	current proxySnapshot
}

type variantSummary struct {
	Family              string         `json:"family"`
	Variant             string         `json:"variant"`
	Total               int            `json:"total"`
	Exact               int            `json:"exact"`
	Failed              int            `json:"failed"`
	FirstExact          int            `json:"first_exact,omitempty"`
	TextCalls           int            `json:"text_calls"`
	Duration            time.Duration  `json:"duration_ns"`
	Timeouts            int            `json:"timeouts"`
	AssetErrors         int            `json:"asset_errors"`
	ReviewApplied       int            `json:"review_applied,omitempty"`
	ImprovedByReview    int            `json:"improved_by_review,omitempty"`
	ReviewEligible      int            `json:"review_eligible,omitempty"`
	CleanerReturned     int            `json:"cleaner_returned,omitempty"`
	ReviewHelped        int            `json:"review_helped,omitempty"`
	ReviewHurt          int            `json:"review_hurt,omitempty"`
	NoChangeAfterReview int            `json:"no_change_after_review,omitempty"`
	FailureKinds        map[string]int `json:"failure_kinds,omitempty"`
	Cases               []caseResult   `json:"cases"`
}

type caseResult struct {
	ID                string      `json:"id"`
	Family            string      `json:"family"`
	Variant           string      `json:"variant"`
	Dataset           string      `json:"dataset,omitempty"`
	SourceID          string      `json:"source_id,omitempty"`
	SourcePage        string      `json:"source_page,omitempty"`
	License           string      `json:"license,omitempty"`
	Difficulty        string      `json:"difficulty,omitempty"`
	Tags              []string    `json:"tags,omitempty"`
	Note              string      `json:"note,omitempty"`
	Exact             bool        `json:"exact"`
	FailureKind       string      `json:"failure_kind,omitempty"`
	WantState         string      `json:"want_state,omitempty"`
	GotState          string      `json:"got_state,omitempty"`
	FirstState        string      `json:"first_state,omitempty"`
	WantSummary       string      `json:"want_summary,omitempty"`
	GotSummary        string      `json:"got_summary,omitempty"`
	FirstSummary      string      `json:"first_summary,omitempty"`
	Error             string      `json:"error,omitempty"`
	TextCalls         int         `json:"text_calls"`
	DurationMillis    int64       `json:"duration_ms"`
	TimeToFirstMillis int64       `json:"time_to_first_ms,omitempty"`
	TimeToFinalMillis int64       `json:"time_to_final_ms,omitempty"`
	ModelCalls        []modelCall `json:"model_calls,omitempty"`
	InputText         string      `json:"input_text,omitempty"`
	NormalizedText    string      `json:"normalized_text,omitempty"`
	TranscriptRef     string      `json:"transcript_ref,omitempty"`
	Transcript        string      `json:"transcript,omitempty"`
	TranscriptCER     float64     `json:"transcript_cer,omitempty"`
	ParsedSummary     string      `json:"parsed_summary,omitempty"`
	AudioPath         string      `json:"audio_path,omitempty"`
	ReviewEligible    bool        `json:"review_eligible,omitempty"`
	CleanerReturned   bool        `json:"cleaner_returned,omitempty"`
	ReviewApplied     bool        `json:"review_applied,omitempty"`
	ReviewHelped      bool        `json:"review_helped,omitempty"`
	ReviewHurt        bool        `json:"review_hurt,omitempty"`
	ReviewNoChange    bool        `json:"review_no_change,omitempty"`
	ReviewCleanedText string      `json:"review_cleaned_text,omitempty"`
	ReviewReasonCode  string      `json:"review_reason_code,omitempty"`
	ReviewApplyReason string      `json:"review_apply_reason,omitempty"`
}

type voiceVariantSpec struct {
	Family string
	Name   string
}

func newCountingOllamaProxy(target string) *countingOllamaProxy {
	p := &countingOllamaProxy{
		target: strings.TrimRight(target, "/"),
		client: &http.Client{Timeout: 2 * time.Minute},
	}
	p.server = httptest.NewServer(http.HandlerFunc(p.handle))
	return p
}

func (p *countingOllamaProxy) URL() string {
	return p.server.URL
}

func (p *countingOllamaProxy) Close() {
	p.server.Close()
}

func (p *countingOllamaProxy) Reset() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.current = proxySnapshot{}
}

func (p *countingOllamaProxy) Snapshot() proxySnapshot {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := p.current
	out.Calls = append([]modelCall(nil), p.current.Calls...)
	return out
}

func (p *countingOllamaProxy) handle(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	call := modelCall{}
	var req countedRequest
	if err := json.Unmarshal(body, &req); err == nil {
		call.Model = req.Model
		call.Prompt = truncate(req.Prompt, 2400)
		call.Mode = "text"
	}

	targetURL := p.target + r.URL.Path
	if r.URL.RawQuery != "" {
		targetURL += "?" + r.URL.RawQuery
	}
	proxyReq, err := http.NewRequestWithContext(r.Context(), r.Method, targetURL, bytes.NewReader(body))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	proxyReq.Header = r.Header.Clone()

	resp, err := p.client.Do(proxyReq)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	call.Status = resp.StatusCode
	call.Response = parseProxyResponse(responseBody)

	p.mu.Lock()
	p.current.TextCalls++
	p.current.Calls = append(p.current.Calls, call)
	p.mu.Unlock()

	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(responseBody)
}

func parseProxyResponse(body []byte) string {
	var payload struct {
		Response string `json:"response"`
	}
	if err := json.Unmarshal(body, &payload); err == nil && strings.TrimSpace(payload.Response) != "" {
		return truncate(payload.Response, 2400)
	}
	return truncate(string(body), 2400)
}

func runTextVariant(family, variant string, cases []textCase, proxy *countingOllamaProxy, eval func(context.Context, textCase) (ingest.EvalResult, error)) variantSummary {
	summary := variantSummary{
		Family:  family,
		Variant: variant,
		Total:   len(cases),
	}
	startedAt := time.Now()
	for _, tc := range cases {
		proxy.Reset()
		caseStarted := time.Now()
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		result, err := eval(ctx, tc)
		cancel()
		snapshot := proxy.Snapshot()
		record := assessTextCase(family, variant, tc, result, err, snapshot, time.Since(caseStarted))
		summary.Cases = append(summary.Cases, record)
		accumulateSummary(&summary, record)
	}
	summary.Duration = time.Since(startedAt)
	return summary
}

func runTextReviewVariant(family, variant string, cases []textCase, proxy *countingOllamaProxy, eval func(context.Context, textCase) ingest.PipelineEvalResult) variantSummary {
	summary := variantSummary{
		Family:  family,
		Variant: variant,
		Total:   len(cases),
	}
	startedAt := time.Now()
	for _, tc := range cases {
		proxy.Reset()
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		pipeline := eval(ctx, tc)
		cancel()
		snapshot := proxy.Snapshot()
		record := assessTextPipelineCase(family, variant, tc, pipeline, snapshot)
		summary.Cases = append(summary.Cases, record)
		accumulateSummary(&summary, record)
	}
	summary.Duration = time.Since(startedAt)
	return summary
}

func runVoiceVariant(cases []voiceCase, spec voiceVariantSpec, proxy *countingOllamaProxy, runtime *benchRuntime, evaluator *ingest.Evaluator, cfg runConfig, now time.Time) variantSummary {
	summary := variantSummary{
		Family:       spec.Family,
		Variant:      spec.Name,
		Total:        len(cases),
		FailureKinds: map[string]int{},
	}
	startedAt := time.Now()
	for _, vc := range cases {
		proxy.Reset()
		caseStarted := time.Now()
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		audioPath, wavPath, err := runtime.resolveVoicePaths(ctx, vc)
		rawTranscript := ""
		normalizedTranscript := ""
		parsedSummary := ""
		parseResult := ingest.EvalResult{}
		parseErr := err
		if err == nil {
			rawTranscript, err = runtime.runDockerVosk(ctx, wavPath, cfg.voskGrammarPath)
			normalizedTranscript = ingest.NormalizeVoiceTranscriptForBenchmark(rawTranscript)
			if err == nil {
				parseResult, parseErr = evaluator.FirstVoiceTranscriptCard(ctx, rawTranscript, now)
				if parseErr == nil {
					parsedSummary = describeOutcome(classifyState(parseResult, nil), normalizeName(parseResult.Name), formatDate(parseResult.ExpiresOn))
				}
			}
		}
		cancel()
		snapshot := proxy.Snapshot()
		record := assessVoiceCase(spec, vc, audioPath, rawTranscript, normalizedTranscript, parsedSummary, parseResult, parseErr, snapshot, time.Since(caseStarted))
		summary.Cases = append(summary.Cases, record)
		accumulateSummary(&summary, record)
	}
	summary.Duration = time.Since(startedAt)
	return summary
}

func runVoiceReviewVariant(cases []voiceCase, spec voiceVariantSpec, proxy *countingOllamaProxy, runtime *benchRuntime, evaluator *ingest.Evaluator, cfg runConfig, now time.Time) variantSummary {
	summary := variantSummary{
		Family:       spec.Family,
		Variant:      spec.Name,
		Total:        len(cases),
		FailureKinds: map[string]int{},
	}
	startedAt := time.Now()
	for _, vc := range cases {
		proxy.Reset()
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		audioPath, wavPath, err := runtime.resolveVoicePaths(ctx, vc)
		rawTranscript := ""
		normalizedTranscript := ""
		pipeline := ingest.PipelineEvalResult{FirstErr: err, FinalErr: err}
		if err == nil {
			rawTranscript, err = runtime.runDockerVosk(ctx, wavPath, cfg.voskGrammarPath)
			normalizedTranscript = ingest.NormalizeVoiceTranscriptForBenchmark(rawTranscript)
			if err == nil {
				pipeline = evaluator.VoiceTranscriptPipeline(ctx, rawTranscript, now)
			} else {
				pipeline = ingest.PipelineEvalResult{FirstErr: err, FinalErr: err}
			}
		}
		cancel()
		snapshot := proxy.Snapshot()
		record := assessVoicePipelineCase(spec, vc, audioPath, rawTranscript, normalizedTranscript, pipeline, snapshot)
		summary.Cases = append(summary.Cases, record)
		accumulateSummary(&summary, record)
	}
	summary.Duration = time.Since(startedAt)
	return summary
}

func accumulateSummary(summary *variantSummary, record caseResult) {
	if record.Exact {
		summary.Exact++
	} else {
		summary.Failed++
	}
	if record.FirstSummary != "" && record.FirstSummary == record.WantSummary {
		summary.FirstExact++
	}
	summary.TextCalls += record.TextCalls
	if record.ReviewEligible {
		summary.ReviewEligible++
	}
	if record.CleanerReturned {
		summary.CleanerReturned++
	}
	if record.ReviewApplied {
		summary.ReviewApplied++
	}
	if record.ReviewHelped {
		summary.ImprovedByReview++
		summary.ReviewHelped++
	}
	if record.ReviewHurt {
		summary.ReviewHurt++
	}
	if record.ReviewNoChange {
		summary.NoChangeAfterReview++
	}
	if record.FailureKind != "" {
		if summary.FailureKinds == nil {
			summary.FailureKinds = map[string]int{}
		}
		summary.FailureKinds[record.FailureKind]++
	}
	switch record.FailureKind {
	case "voice_timeout":
		summary.Timeouts++
	case "asset_error":
		summary.AssetErrors++
	}
}

func assessTextCase(family, variant string, tc textCase, result ingest.EvalResult, err error, snapshot proxySnapshot, duration time.Duration) caseResult {
	record := caseResult{
		ID:             tc.ID,
		Family:         family,
		Variant:        variant,
		Difficulty:     tc.Difficulty,
		Tags:           tc.Tags,
		Note:           tc.Note,
		WantState:      tc.WantState,
		InputText:      tc.Input,
		NormalizedText: ingest.NormalizeFreeTextForBenchmark(tc.Input),
		TextCalls:      snapshot.TextCalls,
		ModelCalls:     snapshot.Calls,
		DurationMillis: duration.Milliseconds(),
	}
	record.GotState = classifyState(result, err)
	record.Exact, record.FailureKind, record.WantSummary, record.GotSummary, record.Error = assessProductOutcome(family, tc.WantState, tc.WantName, tc.WantDate, result, err)
	record.FirstState = record.GotState
	record.FirstSummary = record.GotSummary
	record.TimeToFirstMillis = duration.Milliseconds()
	record.TimeToFinalMillis = duration.Milliseconds()
	return record
}

func assessTextPipelineCase(family, variant string, tc textCase, pipeline ingest.PipelineEvalResult, snapshot proxySnapshot) caseResult {
	record := caseResult{
		ID:                tc.ID,
		Family:            family,
		Variant:           variant,
		Difficulty:        tc.Difficulty,
		Tags:              tc.Tags,
		Note:              tc.Note,
		WantState:         tc.WantState,
		InputText:         tc.Input,
		NormalizedText:    ingest.NormalizeFreeTextForBenchmark(tc.Input),
		TextCalls:         snapshot.TextCalls,
		ModelCalls:        snapshot.Calls,
		DurationMillis:    pipeline.TimeToFinal.Milliseconds(),
		TimeToFirstMillis: pipeline.TimeToFirst.Milliseconds(),
		TimeToFinalMillis: pipeline.TimeToFinal.Milliseconds(),
		ReviewEligible:    pipeline.ReviewEligible,
		CleanerReturned:   pipeline.CleanerReturned,
		ReviewApplied:     pipeline.ReviewApplied,
		ReviewNoChange:    pipeline.ReviewNoChange,
		ReviewCleanedText: pipeline.ReviewCleanedText,
		ReviewReasonCode:  pipeline.ReviewReasonCode,
		ReviewApplyReason: pipeline.ReviewApplyReason,
	}
	record.FirstState = classifyState(pipeline.First, pipeline.FirstErr)
	record.FirstSummary = describeOutcome(record.FirstState, normalizeName(pipeline.First.Name), formatDate(pipeline.First.ExpiresOn))
	record.Exact, record.FailureKind, record.WantSummary, record.GotSummary, record.Error = assessProductOutcome(family, tc.WantState, tc.WantName, tc.WantDate, pipeline.Final, pipeline.FinalErr)
	record.GotState = classifyState(pipeline.Final, pipeline.FinalErr)
	record.ReviewHelped = record.ReviewApplied && record.FirstSummary != record.WantSummary && record.GotSummary == record.WantSummary
	record.ReviewHurt = record.ReviewApplied && record.FirstSummary == record.WantSummary && record.GotSummary != record.WantSummary
	if record.Error == "" && pipeline.ReviewError != nil {
		record.Error = pipeline.ReviewError.Error()
	}
	return record
}

func assessVoiceCase(spec voiceVariantSpec, vc voiceCase, audioPath, rawTranscript, normalizedTranscript, parsedSummary string, result ingest.EvalResult, err error, snapshot proxySnapshot, duration time.Duration) caseResult {
	ref := normalizeSpeechText(vc.TranscriptRef)
	got := normalizeSpeechText(normalizedTranscript)
	exact, failureKind, wantSummary, gotSummary, errString := assessProductOutcome(spec.Family, vc.WantState, vc.WantName, vc.WantDate, result, err)
	record := caseResult{
		ID:                vc.ID,
		Family:            spec.Family,
		Variant:           spec.Name,
		Dataset:           vc.Dataset,
		SourceID:          vc.SourceID,
		SourcePage:        vc.SourcePage,
		License:           vc.License,
		Difficulty:        vc.Difficulty,
		Tags:              vc.Tags,
		Note:              vc.Note,
		Exact:             exact,
		FailureKind:       failureKind,
		WantState:         vc.WantState,
		GotState:          classifyState(result, err),
		WantSummary:       wantSummary,
		GotSummary:        gotSummary,
		Error:             errString,
		TextCalls:         snapshot.TextCalls,
		ModelCalls:        snapshot.Calls,
		DurationMillis:    duration.Milliseconds(),
		TimeToFirstMillis: duration.Milliseconds(),
		TimeToFinalMillis: duration.Milliseconds(),
		TranscriptRef:     vc.TranscriptRef,
		Transcript:        rawTranscript,
		NormalizedText:    normalizedTranscript,
		TranscriptCER:     cer(ref, got),
		ParsedSummary:     parsedSummary,
		AudioPath:         audioPath,
	}
	record.FirstState = record.GotState
	record.FirstSummary = record.GotSummary
	return record
}

func assessVoicePipelineCase(spec voiceVariantSpec, vc voiceCase, audioPath, rawTranscript, normalizedTranscript string, pipeline ingest.PipelineEvalResult, snapshot proxySnapshot) caseResult {
	ref := normalizeSpeechText(vc.TranscriptRef)
	got := normalizeSpeechText(normalizedTranscript)
	record := caseResult{
		ID:                vc.ID,
		Family:            spec.Family,
		Variant:           spec.Name,
		Dataset:           vc.Dataset,
		SourceID:          vc.SourceID,
		SourcePage:        vc.SourcePage,
		License:           vc.License,
		Difficulty:        vc.Difficulty,
		Tags:              vc.Tags,
		Note:              vc.Note,
		WantState:         vc.WantState,
		TextCalls:         snapshot.TextCalls,
		ModelCalls:        snapshot.Calls,
		DurationMillis:    pipeline.TimeToFinal.Milliseconds(),
		TimeToFirstMillis: pipeline.TimeToFirst.Milliseconds(),
		TimeToFinalMillis: pipeline.TimeToFinal.Milliseconds(),
		TranscriptRef:     vc.TranscriptRef,
		Transcript:        rawTranscript,
		NormalizedText:    normalizedTranscript,
		TranscriptCER:     cer(ref, got),
		AudioPath:         audioPath,
		ReviewEligible:    pipeline.ReviewEligible,
		CleanerReturned:   pipeline.CleanerReturned,
		ReviewApplied:     pipeline.ReviewApplied,
		ReviewNoChange:    pipeline.ReviewNoChange,
		ReviewCleanedText: pipeline.ReviewCleanedText,
		ReviewReasonCode:  pipeline.ReviewReasonCode,
		ReviewApplyReason: pipeline.ReviewApplyReason,
	}
	record.FirstState = classifyState(pipeline.First, pipeline.FirstErr)
	record.FirstSummary = describeOutcome(record.FirstState, normalizeName(pipeline.First.Name), formatDate(pipeline.First.ExpiresOn))
	record.Exact, record.FailureKind, record.WantSummary, record.GotSummary, record.Error = assessProductOutcome(spec.Family, vc.WantState, vc.WantName, vc.WantDate, pipeline.Final, pipeline.FinalErr)
	record.GotState = classifyState(pipeline.Final, pipeline.FinalErr)
	record.ParsedSummary = record.GotSummary
	record.ReviewHelped = record.ReviewApplied && record.FirstSummary != record.WantSummary && record.GotSummary == record.WantSummary
	record.ReviewHurt = record.ReviewApplied && record.FirstSummary == record.WantSummary && record.GotSummary != record.WantSummary
	if record.Error == "" && pipeline.ReviewError != nil {
		record.Error = pipeline.ReviewError.Error()
	}
	return record
}

func assessProductOutcome(family, wantState, wantName, wantDate string, result ingest.EvalResult, err error) (bool, string, string, string, string) {
	gotState := classifyState(result, err)
	gotName := normalizeName(result.Name)
	wantName = normalizeName(wantName)
	gotDate := formatDate(result.ExpiresOn)
	wantDate = strings.TrimSpace(wantDate)

	wantSummary := describeOutcome(wantState, wantName, wantDate)
	gotSummary := describeOutcome(gotState, gotName, gotDate)

	switch wantState {
	case "reject":
		if gotState == "reject" {
			return true, "", wantSummary, gotSummary, ""
		}
		if err != nil {
			return false, classifyRuntimeError(err, family), wantSummary, "", err.Error()
		}
		return false, "wrong_state", wantSummary, gotSummary, ""
	case "needs_name":
		if err != nil {
			return false, classifyRuntimeError(err, family), wantSummary, "", err.Error()
		}
		if wantDate == gotDate && (gotState == "needs_name" || gotState == "ready") {
			return true, "", wantSummary, gotSummary, ""
		}
		return false, "wrong_state", wantSummary, gotSummary, ""
	case "needs_expiry":
		if err != nil {
			return false, classifyRuntimeError(err, family), wantSummary, "", err.Error()
		}
		if gotState != "needs_expiry" {
			return false, "wrong_state", wantSummary, gotSummary, ""
		}
		if !productNamesEquivalent(wantName, gotName) {
			return false, "wrong_name", wantSummary, gotSummary, ""
		}
		return true, "", wantSummary, gotSummary, ""
	case "ready":
		if err != nil {
			return false, classifyRuntimeError(err, family), wantSummary, "", err.Error()
		}
		if gotState != "ready" {
			return false, "wrong_state", wantSummary, gotSummary, ""
		}
		if !productNamesEquivalent(wantName, gotName) {
			return false, "wrong_name", wantSummary, gotSummary, ""
		}
		if wantDate != gotDate {
			return false, "wrong_date", wantSummary, gotSummary, ""
		}
		return true, "", wantSummary, gotSummary, ""
	default:
		return false, "unknown_want_state", wantSummary, gotSummary, ""
	}
}

func classifyRuntimeError(err error, family string) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, context.DeadlineExceeded), strings.Contains(strings.ToLower(err.Error()), "context deadline exceeded"):
		if family == "voice" {
			return "voice_timeout"
		}
		return "timeout"
	case strings.Contains(strings.ToLower(err.Error()), "sha256 mismatch"),
		strings.Contains(strings.ToLower(err.Error()), "download "),
		strings.Contains(strings.ToLower(err.Error()), "has no download_url"),
		strings.Contains(strings.ToLower(err.Error()), "asset path"),
		strings.Contains(strings.ToLower(err.Error()), "ffmpeg voice convert"):
		return "asset_error"
	default:
		if family == "voice" {
			return "voice_error"
		}
		return "runtime_error"
	}
}

func classifyState(result ingest.EvalResult, err error) string {
	if err != nil {
		return "reject"
	}
	hasName := strings.TrimSpace(result.Name) != ""
	hasDate := result.ExpiresOn != nil
	switch {
	case hasName && hasDate:
		return "ready"
	case hasName:
		return "needs_expiry"
	case hasDate:
		return "needs_name"
	default:
		return "reject"
	}
}

func normalizeName(value string) string {
	value = ingest.NormalizeDraftNameForBenchmark(value)
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, "ё", "е")
	return strings.Join(strings.Fields(value), " ")
}

func productNamesEquivalent(want, got string) bool {
	if want == got {
		return true
	}
	wantTokens := strings.Fields(want)
	gotTokens := strings.Fields(got)
	if len(wantTokens) != len(gotTokens) {
		return false
	}
	for i := range wantTokens {
		if canonicalProductToken(wantTokens[i]) != canonicalProductToken(gotTokens[i]) {
			return false
		}
	}
	return true
}

func canonicalProductToken(token string) string {
	for _, rewrite := range [][2]string{
		{"ицу", "ица"},
		{"ку", "ка"},
		{"гу", "га"},
		{"ху", "ха"},
		{"жу", "жа"},
		{"шу", "ша"},
		{"чу", "ча"},
		{"щу", "ща"},
		{"ю", "я"},
	} {
		if strings.HasSuffix(token, rewrite[0]) {
			return strings.TrimSuffix(token, rewrite[0]) + rewrite[1]
		}
	}
	return token
}

func normalizeSpeechText(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, "ё", "е")
	value = strings.ReplaceAll(value, "́", "")
	value = strings.Map(func(r rune) rune {
		switch {
		case r >= '0' && r <= '9':
			return r
		case r >= 'a' && r <= 'z':
			return r
		case r >= 'а' && r <= 'я':
			return r
		case r == ' ':
			return r
		default:
			return ' '
		}
	}, value)
	return strings.Join(strings.Fields(value), " ")
}

func formatDate(value *time.Time) string {
	if value == nil {
		return ""
	}
	return value.Format("2006-01-02")
}

func describeOutcome(state, name, date string) string {
	parts := []string{"state=" + state}
	if name != "" {
		parts = append(parts, "name="+name)
	}
	if date != "" {
		parts = append(parts, "date="+date)
	}
	return strings.Join(parts, ", ")
}

func cer(want, got string) float64 {
	wantRunes := []rune(want)
	gotRunes := []rune(got)
	if len(wantRunes) == 0 {
		if len(gotRunes) == 0 {
			return 0
		}
		return 1
	}
	return float64(levenshtein(wantRunes, gotRunes)) / float64(len(wantRunes))
}

func levenshtein(a, b []rune) int {
	if len(a) == 0 {
		return len(b)
	}
	if len(b) == 0 {
		return len(a)
	}
	prev := make([]int, len(b)+1)
	curr := make([]int, len(b)+1)
	for j := 0; j <= len(b); j++ {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		curr[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 0
			if a[i-1] != b[j-1] {
				cost = 1
			}
			curr[j] = min3(
				prev[j]+1,
				curr[j-1]+1,
				prev[j-1]+cost,
			)
		}
		prev, curr = curr, prev
	}
	return prev[len(b)]
}

func min3(a, b, c int) int {
	if a <= b && a <= c {
		return a
	}
	if b <= c {
		return b
	}
	return c
}

func truncate(value string, limit int) string {
	value = strings.TrimSpace(value)
	if len(value) <= limit {
		return value
	}
	return value[:limit] + "..."
}
