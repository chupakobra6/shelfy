package ingest

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/igor/shelfy/internal/domain"
)

type EvalResult struct {
	Name              string
	ExpiresOn         *time.Time
	RawDeadlinePhrase string
	Confidence        string
	Source            string
}

type PipelineEvalResult struct {
	First             EvalResult
	FirstErr          error
	Final             EvalResult
	FinalErr          error
	TimeToFirst       time.Duration
	TimeToFinal       time.Duration
	ReviewEligible    bool
	CleanerReturned   bool
	ReviewApplied     bool
	ReviewNoChange    bool
	ReviewCleanedText string
	ReviewReasonCode  string
	ReviewApplyReason string
	ReviewError       error
}

type Evaluator struct {
	service *Service
}

func NewEvaluator(ollamaBaseURL, ollamaModel string, logger *slog.Logger) *Evaluator {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &Evaluator{
		service: &Service{
			logger:        logger,
			ollamaBaseURL: strings.TrimRight(ollamaBaseURL, "/"),
			ollamaModel:   ollamaModel,
		},
	}
}

func (e *Evaluator) CurrentText(ctx context.Context, input string, now time.Time) (EvalResult, error) {
	draft, err := e.currentText(ctx, input, now)
	return toEvalResult(draft), err
}

func (e *Evaluator) FirstTextCard(ctx context.Context, input string, now time.Time) (EvalResult, error) {
	draft, err := e.firstText(ctx, input, now)
	return toEvalResult(draft), err
}

func (e *Evaluator) TextPipeline(ctx context.Context, input string, now time.Time) PipelineEvalResult {
	startedAt := time.Now()
	firstDraft, firstErr := e.firstText(ctx, input, now)
	firstDuration := time.Since(startedAt)
	result := PipelineEvalResult{
		FirstErr:    firstErr,
		TimeToFirst: firstDuration,
		TimeToFinal: firstDuration,
	}
	if firstErr != nil {
		result.FinalErr = firstErr
		return result
	}
	result.First = toEvalResult(firstDraft)
	result.Final = result.First
	result.FinalErr = nil

	review, applied, cleanedText, reviewErr := e.reviewText(ctx, input, firstDraft, now)
	result.ReviewError = reviewErr
	result.ReviewEligible = true
	result.CleanerReturned = review.Cleaner != (reviewCleaner{})
	result.ReviewApplied = applied
	result.ReviewNoChange = result.CleanerReturned && !applied
	result.ReviewCleanedText = cleanedText
	result.ReviewReasonCode = strings.TrimSpace(review.Cleaner.ReasonCode)
	result.ReviewApplyReason = strings.TrimSpace(reviewApplyReason(firstDraft, review))
	if applied {
		result.Final = toEvalResult(reviewToParsedDraft(review, firstDraft, cleanedText, now))
	}
	result.TimeToFinal = time.Since(startedAt)
	return result
}

func (e *Evaluator) LLMFirstText(ctx context.Context, input string, now time.Time) (EvalResult, error) {
	draft, err := e.llmFirstText(ctx, input, now)
	return toEvalResult(draft), err
}

func (e *Evaluator) CurrentVoiceTranscript(ctx context.Context, transcript string, now time.Time) (EvalResult, error) {
	draft, err := e.service.parseTextDraft(ctx, normalizeVoiceTranscript(transcript), now)
	return toEvalResult(draft), err
}

func (e *Evaluator) FirstVoiceTranscriptCard(ctx context.Context, transcript string, now time.Time) (EvalResult, error) {
	draft, err := e.firstVoiceTranscript(ctx, normalizeVoiceTranscript(transcript), now)
	return toEvalResult(draft), err
}

func (e *Evaluator) VoiceTranscriptPipeline(ctx context.Context, transcript string, now time.Time) PipelineEvalResult {
	startedAt := time.Now()
	normalized := normalizeVoiceTranscript(transcript)
	firstDraft, firstErr := e.firstVoiceTranscript(ctx, normalized, now)
	firstDuration := time.Since(startedAt)
	result := PipelineEvalResult{
		FirstErr:    firstErr,
		TimeToFirst: firstDuration,
		TimeToFinal: firstDuration,
	}
	if firstErr != nil {
		result.FinalErr = firstErr
		return result
	}
	result.First = toEvalResult(firstDraft)
	result.Final = result.First
	result.FinalErr = nil

	review, applied, cleanedText, reviewErr := e.reviewVoice(ctx, transcript, normalized, firstDraft, now)
	result.ReviewError = reviewErr
	result.ReviewEligible = true
	result.CleanerReturned = review.Cleaner != (reviewCleaner{})
	result.ReviewApplied = applied
	result.ReviewNoChange = result.CleanerReturned && !applied
	result.ReviewCleanedText = cleanedText
	result.ReviewReasonCode = strings.TrimSpace(review.Cleaner.ReasonCode)
	result.ReviewApplyReason = strings.TrimSpace(reviewApplyReason(firstDraft, review))
	if applied {
		result.Final = toEvalResult(reviewToParsedDraft(review, firstDraft, cleanedText, now))
	}
	result.TimeToFinal = time.Since(startedAt)
	return result
}

func (e *Evaluator) LLMFirstVoiceTranscript(ctx context.Context, transcript string, now time.Time) (EvalResult, error) {
	draft, err := e.llmFirstText(ctx, normalizeVoiceTranscript(transcript), now)
	return toEvalResult(draft), err
}

func (e *Evaluator) RepairVoiceTranscript(ctx context.Context, transcript string) (string, error) {
	return e.service.callOllamaTranscriptRepair(ctx, transcript)
}

func toEvalResult(draft parsedDraft) EvalResult {
	return EvalResult{
		Name:              draft.Name,
		ExpiresOn:         draft.ExpiresOn,
		RawDeadlinePhrase: draft.RawDeadlinePhrase,
		Confidence:        draft.Confidence,
		Source:            draft.Source,
	}
}

func (e *Evaluator) currentText(ctx context.Context, input string, now time.Time) (parsedDraft, error) {
	cleaned := normalizeFreeText(input)
	result := heuristicParse(cleaned, now)
	if result.Name != "" || result.ExpiresOn != nil {
		if !shouldTryTextModel(cleaned, result) {
			return finalizeParsedTextDraft(cleaned, result)
		}
	}
	return e.service.parseTextDraft(ctx, input, now)
}

func (e *Evaluator) llmFirstText(ctx context.Context, input string, now time.Time) (parsedDraft, error) {
	cleaned := normalizeFreeText(input)
	if cleaned == "" {
		return parsedDraft{}, fmt.Errorf("unable to extract any draft fields")
	}

	result := parsedDraft{}
	if structured, err := e.service.callOllamaText(ctx, cleaned); err == nil {
		if structured.Name != "" {
			result.Name = structured.Name
		}
		if structured.RawDeadlinePhrase != "" {
			resolvedPhrase, resolved := resolveDraftDeadlinePhrase(structured.RawDeadlinePhrase, now)
			result.RawDeadlinePhrase = resolvedPhrase
			result.ExpiresOn = resolved.Value
			result.LockedExpiry = resolved.Absolute
		}
		if result.Name != "" || result.ExpiresOn != nil {
			result.Confidence = "model"
			result.Source = "ollama-text"
		}
	} else {
		e.service.logger.WarnContext(ctx, "ollama_text_failed", "error", err)
	}

	heuristic := heuristicParse(cleaned, now)
	if result.Name == "" && heuristic.Name != "" {
		result.Name = heuristic.Name
	}
	if result.RawDeadlinePhrase == "" && heuristic.RawDeadlinePhrase != "" {
		result.RawDeadlinePhrase = heuristic.RawDeadlinePhrase
	}
	if result.ExpiresOn == nil && heuristic.ExpiresOn != nil {
		result.ExpiresOn = heuristic.ExpiresOn
	}
	if result.Source == "" && (heuristic.Name != "" || heuristic.ExpiresOn != nil) {
		result.Source = heuristic.Source
		result.Confidence = heuristic.Confidence
	}

	return finalizeParsedTextDraft(cleaned, result)
}

func (e *Evaluator) firstText(ctx context.Context, input string, now time.Time) (parsedDraft, error) {
	if draft, err := parseFastDraft(input, now); err == nil {
		return draft, nil
	}
	return e.service.parseTextDraft(ctx, input, now)
}

func (e *Evaluator) firstVoiceTranscript(ctx context.Context, normalizedTranscript string, now time.Time) (parsedDraft, error) {
	if draft, err := parseFastDraft(normalizedTranscript, now); err == nil {
		return draft, nil
	}
	return e.service.parseTextDraft(ctx, normalizedTranscript, now)
}

func (e *Evaluator) reviewText(ctx context.Context, originalText string, current parsedDraft, now time.Time) (reviewRun, bool, string, error) {
	cleaner, err := e.service.callOllamaTextCleaner(ctx, normalizeFreeText(originalText))
	if err != nil {
		return reviewRun{}, false, "", err
	}
	cleanedText := reviewCleanedInput(cleaner, normalizeFreeText(originalText))
	candidate, cleanedText := reviewCandidateFromResult(cleaner, current, normalizeFreeText(originalText), now)
	candidateErr := reviewCandidateError(candidate, current, cleanedText, normalizeFreeText(originalText))
	run := reviewRun{Cleaner: cleaner, CleanedText: cleanedText, Candidate: candidate, CandidateOK: candidateErr == nil}
	if !run.CandidateOK {
		return run, false, cleanedText, nil
	}
	if applied, _ := reviewApplyDecision(draftSessionFromParsed(current, domain.MessageKindText), candidate); !applied {
		return run, false, cleanedText, nil
	}
	return run, true, cleanedText, nil
}

func (e *Evaluator) reviewVoice(ctx context.Context, rawTranscript, normalizedTranscript string, current parsedDraft, now time.Time) (reviewRun, bool, string, error) {
	cleaner, err := e.service.callOllamaVoiceCleaner(ctx, normalizeFreeText(normalizedTranscript))
	if err != nil {
		return reviewRun{}, false, "", err
	}
	cleanedText := reviewCleanedInput(cleaner, normalizedTranscript)
	candidate, cleanedText := reviewCandidateFromResult(cleaner, current, normalizeFreeText(normalizedTranscript), now)
	candidateErr := reviewCandidateError(candidate, current, cleanedText, normalizedTranscript)
	run := reviewRun{Cleaner: cleaner, CleanedText: cleanedText, Candidate: candidate, CandidateOK: candidateErr == nil}
	if !run.CandidateOK {
		return run, false, cleanedText, nil
	}
	if applied, _ := reviewApplyDecision(draftSessionFromParsed(current, domain.MessageKindVoice), candidate); !applied {
		return run, false, cleanedText, nil
	}
	return run, true, cleanedText, nil
}

func reviewCandidateFromResult(cleaner reviewCleaner, current parsedDraft, fallbackText string, now time.Time) (parsedDraft, string) {
	cleanedText := reviewCleanedInput(cleaner, fallbackText)
	candidate, cleanedText, err := buildCleanerCandidate(now, cleaner, fallbackText)
	if err != nil {
		return current, cleanedText
	}
	return candidate, cleanedText
}

func reviewToParsedDraft(review reviewRun, fallback parsedDraft, cleanedText string, now time.Time) parsedDraft {
	if review.CandidateOK {
		return review.Candidate
	}
	candidate, _, err := buildCleanerCandidate(now, review.Cleaner, cleanedText)
	if err != nil {
		return fallback
	}
	return candidate
}

func reviewApplyReason(current parsedDraft, review reviewRun) string {
	if !review.CandidateOK {
		return "candidate_invalid"
	}
	_, reason := reviewApplyDecision(draftSessionFromParsed(current, domain.MessageKindText), review.Candidate)
	return reason
}

func reviewCandidateError(candidate, current parsedDraft, cleanedText, fallbackText string) error {
	if strings.TrimSpace(cleanedText) == "" {
		return fmt.Errorf("candidate_invalid")
	}
	if candidate == current && normalizeFreeText(cleanedText) == normalizeFreeText(strings.TrimSpace(fallbackText)) {
		return nil
	}
	if strings.TrimSpace(candidate.Name) == "" && candidate.ExpiresOn == nil {
		return fmt.Errorf("candidate_invalid")
	}
	return nil
}

func draftSessionFromParsed(draft parsedDraft, sourceKind domain.MessageKind) domain.DraftSession {
	return domain.DraftSession{
		SourceKind:        sourceKind,
		Status:            domain.DraftStatusReady,
		DraftName:         draft.Name,
		DraftExpiresOn:    draft.ExpiresOn,
		RawDeadlinePhrase: draft.RawDeadlinePhrase,
		DraftPayload: map[string]any{
			domain.DraftPayloadKeyFastSource:     draft.Source,
			domain.DraftPayloadKeyFastConfidence: draft.Confidence,
		},
	}
}
