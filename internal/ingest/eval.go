package ingest

import (
	"context"
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
	First               EvalResult
	FirstErr            error
	Final               EvalResult
	FinalErr            error
	TimeToFirst         time.Duration
	TimeToFinal         time.Duration
	CleanerEligible     bool
	CleanerCalled       bool
	CleanerChangedInput bool
	CandidateValid      bool
	CleanerApplied      bool
	CleanerNoChange     bool
	CleanedInput        string
	CleanerReasonCode   string
	SelectionReason     string
	ChosenSource        string
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

func (e *Evaluator) FirstTextCard(ctx context.Context, input string, now time.Time) (EvalResult, error) {
	draft, err := parseFastDraft(input, now)
	return toEvalResult(draft), err
}

func (e *Evaluator) TextPipeline(ctx context.Context, input string, now time.Time) PipelineEvalResult {
	startedAt := time.Now()
	firstBuild, firstErr := buildInitialDraft(input, now)
	firstDuration := time.Since(startedAt)

	result := newPipelineEvalResult(firstBuild.Draft, firstErr, firstDuration)
	if firstErr != nil {
		return result
	}
	trace, candidate, applied := e.service.runCleanerPass(ctx, domain.MessageKindText, firstBuild.Trace.NormalizedInput, draftSessionFromParsed(firstBuild.Draft, domain.MessageKindText), now)
	return finalizePipelineEvalResult(result, firstBuild.Draft, trace, candidate, applied, time.Since(startedAt))
}

func (e *Evaluator) FirstVoiceTranscriptCard(ctx context.Context, transcript string, now time.Time) (EvalResult, error) {
	normalizedTranscript := normalizeVoiceTranscript(transcript)
	draft, err := parseFastDraft(normalizedTranscript, now)
	return toEvalResult(draft), err
}

func (e *Evaluator) VoiceTranscriptPipeline(ctx context.Context, transcript string, now time.Time) PipelineEvalResult {
	startedAt := time.Now()
	normalizedTranscript := normalizeVoiceTranscript(transcript)
	firstBuild, firstErr := buildInitialDraft(normalizedTranscript, now)
	firstDuration := time.Since(startedAt)

	result := newPipelineEvalResult(firstBuild.Draft, firstErr, firstDuration)
	if firstErr != nil {
		return result
	}
	trace, candidate, applied := e.service.runCleanerPass(ctx, domain.MessageKindVoice, firstBuild.Trace.NormalizedInput, draftSessionFromParsed(firstBuild.Draft, domain.MessageKindVoice), now)
	return finalizePipelineEvalResult(result, firstBuild.Draft, trace, candidate, applied, time.Since(startedAt))
}

func newPipelineEvalResult(firstDraft parsedDraft, firstErr error, firstDuration time.Duration) PipelineEvalResult {
	result := PipelineEvalResult{
		FirstErr:    firstErr,
		TimeToFirst: firstDuration,
		TimeToFinal: firstDuration,
	}
	if firstErr == nil {
		result.First = toEvalResult(firstDraft)
	}
	return result
}

func finalizePipelineEvalResult(result PipelineEvalResult, baseline parsedDraft, trace draftBuildTrace, candidate parsedDraft, applied bool, finalDuration time.Duration) PipelineEvalResult {
	result.FinalErr = nil
	result.TimeToFinal = finalDuration
	result.CleanerEligible = true
	result.CleanerCalled = trace.CleanerCalled
	result.CleanerChangedInput = trace.CleanerChangedInput
	result.CandidateValid = trace.CandidateValid
	result.CleanerApplied = applied
	result.CleanerNoChange = result.CleanerCalled && !result.CleanerApplied
	result.CleanedInput = trace.CleanedInput
	result.CleanerReasonCode = trace.CleanerReasonCode
	result.SelectionReason = trace.SelectionReason
	result.ChosenSource = trace.ChosenSource
	if applied {
		result.Final = toEvalResult(candidate)
	} else {
		result.Final = toEvalResult(baseline)
	}
	return result
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

func draftSessionFromParsed(draft parsedDraft, sourceKind domain.MessageKind) domain.DraftSession {
	return domain.DraftSession{
		SourceKind:        sourceKind,
		Status:            domain.DraftStatusReady,
		DraftName:         draft.Name,
		DraftExpiresOn:    draft.ExpiresOn,
		RawDeadlinePhrase: draft.RawDeadlinePhrase,
	}
}
