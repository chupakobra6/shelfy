package ingest

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/igor/shelfy/internal/domain"
	"github.com/igor/shelfy/internal/jobs"
	"github.com/igor/shelfy/internal/observability"
)

func (s *Service) handleRefineDraftAI(ctx context.Context, rawPayload []byte, now time.Time) error {
	var payload jobs.RefineDraftAIPayload
	if err := json.Unmarshal(rawPayload, &payload); err != nil {
		return err
	}
	ctx = observability.WithUserID(ctx, payload.UserID)

	draft, ok, err := s.store.FindDraftSessionByTraceID(ctx, payload.TraceID)
	if err != nil {
		return err
	}
	if !ok {
		s.logger.InfoContext(ctx, "smart_review_skipped_missing_draft", observability.LogAttrs(ctx, "trace_id", payload.TraceID)...)
		return nil
	}
	ctx = observability.WithDraftID(ctx, draft.ID)
	if shouldSkipReviewForDraft(draft) {
		s.logger.InfoContext(ctx, "smart_review_skipped_inactive_draft", observability.LogAttrs(ctx,
			"draft_id", draft.ID,
			"draft_status", draft.Status,
		)...)
		return nil
	}

	review, err := s.runSmartReview(ctx, payload, now)
	if err != nil {
		s.logger.WarnContext(ctx, "smart_review_model_failed", observability.LogAttrs(ctx,
			"draft_id", draft.ID,
			"error", err,
		)...)
		return renderReviewStatus(ctx, s, draft.ID, "", "")
	}
	current, err := s.store.GetDraftSession(ctx, draft.ID)
	if err != nil {
		return err
	}
	if shouldSkipReviewForDraft(current) {
		s.logger.InfoContext(ctx, "smart_review_skipped_after_refresh", observability.LogAttrs(ctx,
			"draft_id", current.ID,
			"draft_status", current.Status,
		)...)
		return nil
	}
	shouldApply := false
	applyReason := "candidate_invalid"
	if review.CandidateOK {
		shouldApply, applyReason = reviewApplyDecision(current, review.Candidate)
	}
	if !review.CandidateOK || !shouldApply {
		meta := reviewMetadata(current.DraftPayload, review.Cleaner, review.CleanedText, false, applyReason)
		if updateErr := s.store.UpdateDraftPayload(ctx, current.ID, meta); updateErr != nil {
			return updateErr
		}
		s.logger.InfoContext(ctx, "smart_review_no_change", observability.LogAttrs(ctx,
			"draft_id", current.ID,
			"reason", applyReason,
			"cleaner_reason_code", review.Cleaner.ReasonCode,
			"cleaned_excerpt", excerptForLog(review.CleanedText, 240),
		)...)
		return renderReviewStatus(ctx, s, current.ID, "", "")
	}

	meta := reviewMetadata(current.DraftPayload, review.Cleaner, review.CleanedText, true, applyReason)
	meta[domain.DraftPayloadKeyAIReviewStatus] = domain.AIReviewStatusImproved

	applied, err := s.store.ApplyDraftAIReviewIfReady(ctx, current.ID, review.Candidate.Name, review.Candidate.ExpiresOn, review.Candidate.RawDeadlinePhrase, meta)
	if err != nil {
		return err
	}
	if !applied {
		s.logger.InfoContext(ctx, "smart_review_skipped_lost_ready_state", observability.LogAttrs(ctx, "draft_id", current.ID)...)
		return nil
	}
	if err := s.publishDraftCard(ctx, current.ID); err != nil {
		return err
	}
	s.logger.InfoContext(ctx, "smart_review_applied", observability.LogAttrs(ctx,
		"draft_id", current.ID,
		"reason", applyReason,
		"cleaner_reason_code", review.Cleaner.ReasonCode,
		"has_name", strings.TrimSpace(review.Candidate.Name) != "",
		"has_expiry", review.Candidate.ExpiresOn != nil,
		"cleaned_excerpt", excerptForLog(review.CleanedText, 240),
	)...)
	return nil
}

func (s *Service) runSmartReview(ctx context.Context, payload jobs.RefineDraftAIPayload, now time.Time) (reviewRun, error) {
	var cleaner reviewCleaner
	var err error
	switch payload.SourceKind {
	case domain.MessageKindVoice, domain.MessageKindAudio:
		cleaner, err = s.callOllamaVoiceCleaner(ctx, reviewModelInput(payload))
	default:
		cleaner, err = s.callOllamaTextCleaner(ctx, reviewModelInput(payload))
	}
	if err != nil {
		return reviewRun{}, err
	}
	cleanedText := reviewCleanedInput(cleaner, reviewModelInput(payload))
	candidate, _, candidateErr := buildCleanerCandidate(now, cleaner, reviewModelInput(payload))
	return reviewRun{
		Cleaner:     cleaner,
		CleanedText: cleanedText,
		Candidate:   candidate,
		CandidateOK: candidateErr == nil,
	}, nil
}
