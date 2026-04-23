package ingest

import (
	"context"
	"os"
	"strings"
	"time"

	"github.com/igor/shelfy/internal/domain"
	"github.com/igor/shelfy/internal/jobs"
	"github.com/igor/shelfy/internal/observability"
	"github.com/igor/shelfy/internal/telegram"
)

const ingestFailureCleanupTTL = 8 * time.Second

func (s *Service) handleText(ctx context.Context, payload jobs.IngestPayload, now time.Time) error {
	result, err := buildInitialDraft(payload.Text, now)
	if err != nil {
		return s.sendFailure(ctx, payload, err)
	}
	draft, err := s.persistReadyDraft(ctx, payload, result)
	if err != nil {
		return err
	}
	return s.enqueueBackgroundCleaner(ctx, payload, draft, result.Trace.NormalizedInput)
}

func (s *Service) handleAudio(ctx context.Context, payload jobs.IngestPayload, now time.Time) error {
	return s.withPipelineWorkspace(ctx, "audio-*", "audio_pipeline_started", func(dir string) error {
		inputPath := dir + "/input"
		if err := s.tg.DownloadFile(ctx, payload.FileID, inputPath); err != nil {
			return s.sendFailure(ctx, payload, err)
		}
		wavPath := dir + "/input.wav"
		if err := s.runFFmpeg(ctx, inputPath, wavPath); err != nil {
			return s.sendFailure(ctx, payload, err)
		}
		transcript, err := s.runVosk(ctx, wavPath)
		if err != nil {
			return s.sendFailure(ctx, payload, err)
		}
		normalizedTranscript := normalizeVoiceTranscript(transcript)
		result, err := buildInitialDraft(normalizedTranscript, now)
		if err != nil {
			return s.sendFailure(ctx, payload, err)
		}
		draft, err := s.persistReadyDraft(ctx, payload, result)
		if err != nil {
			return err
		}
		return s.enqueueBackgroundCleaner(ctx, payload, draft, result.Trace.NormalizedInput)
	})
}

func (s *Service) handleCleanDraft(ctx context.Context, payload jobs.CleanDraftPayload) error {
	draft, err := s.store.GetDraftSession(ctx, payload.DraftID)
	if err != nil {
		return err
	}
	if draft.Status != domain.DraftStatusReady {
		s.logger.InfoContext(ctx, "cleaner_skipped_draft_not_ready", observability.LogAttrs(ctx,
			"draft_id", draft.ID,
			"status", draft.Status,
		)...)
		return nil
	}
	localNow, err := s.currentLocalNow(ctx, payload.UserID)
	if err != nil {
		return err
	}
	trace, candidate, apply := s.runCleanerPass(ctx, payload.Kind, payload.NormalizedInput, draft, localNow)
	if !apply {
		meta := mergeDraftPayload(draft.DraftPayload, withCleanerPending(buildDraftTracePayload(trace), false))
		if err := s.store.UpdateDraftPayload(ctx, draft.ID, meta); err != nil {
			return err
		}
		return s.publishDraftCard(ctx, draft.ID)
	}
	meta := mergeDraftPayload(draft.DraftPayload, withCleanerPending(buildDraftTracePayload(trace), false))
	updated, err := s.store.ApplyCleanerUpdateIfReady(ctx, draft.ID, candidate.Name, candidate.ExpiresOn, candidate.RawDeadlinePhrase, meta)
	if err != nil {
		return err
	}
	if !updated {
		s.logger.InfoContext(ctx, "cleaner_update_skipped", observability.LogAttrs(ctx,
			"draft_id", draft.ID,
			"reason", "not_ready_anymore",
		)...)
		return nil
	}
	return s.publishDraftCard(ctx, draft.ID)
}

func (s *Service) enqueueBackgroundCleaner(ctx context.Context, payload jobs.IngestPayload, draft domain.DraftSession, normalizedInput string) error {
	normalizedInput = strings.TrimSpace(normalizedInput)
	if draft.Status != domain.DraftStatusReady || normalizedInput == "" {
		return nil
	}
	now, err := s.currentNow(ctx)
	if err != nil {
		return err
	}
	key := payload.TraceID + ":cleaner"
	return s.store.EnqueueJob(ctx, payload.TraceID, domain.JobTypeCleanDraft, jobs.CleanDraftPayload{
		TraceID:         payload.TraceID,
		DraftID:         draft.ID,
		UserID:          payload.UserID,
		ChatID:          payload.ChatID,
		Kind:            payload.Kind,
		NormalizedInput: normalizedInput,
	}, now, &key)
}

func (s *Service) persistReadyDraft(ctx context.Context, payload jobs.IngestPayload, result draftBuildResult) (domain.DraftSession, error) {
	meta := withCleanerPending(buildDraftTracePayload(result.Trace), true)
	draft, err := s.upsertDraftCard(ctx, payload, result.Draft, meta)
	if err != nil {
		return domain.DraftSession{}, err
	}
	if err := s.finishDraftReady(ctx, payload, "draft created"); err != nil {
		return domain.DraftSession{}, err
	}
	return draft, nil
}

func (s *Service) upsertDraftCard(ctx context.Context, payload jobs.IngestPayload, parsed parsedDraft, payloadMeta map[string]any) (domain.DraftSession, error) {
	if existing, ok, err := s.store.FindDraftSessionByTraceID(ctx, payload.TraceID); err != nil {
		return domain.DraftSession{}, err
	} else if ok {
		ctx = observability.WithDraftID(ctx, existing.ID)
		if err := s.store.UpdateDraftFields(ctx, existing.ID, parsed.Name, parsed.ExpiresOn, parsed.RawDeadlinePhrase, domain.DraftStatusReady); err != nil {
			return domain.DraftSession{}, err
		}
		mergedPayload := mergeDraftPayload(existing.DraftPayload, payloadMeta)
		if err := s.store.UpdateDraftPayload(ctx, existing.ID, mergedPayload); err != nil {
			return domain.DraftSession{}, err
		}
		if err := s.publishDraftCard(ctx, existing.ID); err != nil {
			return domain.DraftSession{}, err
		}
		draft, err := s.store.GetDraftSession(ctx, existing.ID)
		if err != nil {
			return domain.DraftSession{}, err
		}
		s.logger.InfoContext(ctx, "draft_card_upserted_existing", observability.LogAttrs(ctx,
			"draft_id", existing.ID,
			"draft_message_id", ptrValue(draft.DraftMessageID),
			"source_kind", payload.Kind,
			"confidence", parsed.Confidence,
			"source", parsed.Source,
		)...)
		return draft, nil
	} else {
		session := domain.DraftSession{
			TraceID:           payload.TraceID,
			UserID:            payload.UserID,
			ChatID:            payload.ChatID,
			SourceKind:        payload.Kind,
			Status:            domain.DraftStatusReady,
			SourceMessageID:   &payload.MessageID,
			FeedbackMessageID: int64Ptr(payload.FeedbackMessageID),
			DraftName:         parsed.Name,
			DraftExpiresOn:    parsed.ExpiresOn,
			RawDeadlinePhrase: parsed.RawDeadlinePhrase,
			DraftPayload:      payloadMeta,
		}
		draftID, err := s.store.CreateDraftSession(ctx, session)
		if err != nil {
			return domain.DraftSession{}, err
		}
		ctx = observability.WithDraftID(ctx, draftID)
		if err := s.publishDraftCard(ctx, draftID); err != nil {
			return domain.DraftSession{}, err
		}
		draft, err := s.store.GetDraftSession(ctx, draftID)
		if err != nil {
			return domain.DraftSession{}, err
		}
		s.logger.InfoContext(ctx, "draft_card_created", observability.LogAttrs(ctx,
			"draft_message_id", ptrValue(draft.DraftMessageID),
			"source_kind", payload.Kind,
			"confidence", parsed.Confidence,
			"source", parsed.Source,
		)...)
		return draft, nil
	}
}

func (s *Service) publishDraftCard(ctx context.Context, draftID int64) error {
	draft, err := s.store.GetDraftSession(ctx, draftID)
	if err != nil {
		return err
	}
	text, markup, err := s.ui.DraftCard(draft)
	if err != nil {
		return err
	}
	if draft.DraftMessageID == nil {
		message, err := s.tg.SendMessage(ctx, telegram.SendMessageRequest{
			ChatID:      draft.ChatID,
			Text:        text,
			ParseMode:   "HTML",
			ReplyMarkup: markup,
		})
		if err != nil {
			return err
		}
		return s.store.SetDraftMessageID(ctx, draftID, message.MessageID)
	}
	err = s.tg.EditMessageText(ctx, telegram.EditMessageTextRequest{
		ChatID:      draft.ChatID,
		MessageID:   *draft.DraftMessageID,
		Text:        text,
		ParseMode:   "HTML",
		ReplyMarkup: markup,
	})
	if err == nil {
		return nil
	}
	if !telegram.IsMissingMessageTargetError(err) {
		return err
	}
	message, sendErr := s.tg.SendMessage(ctx, telegram.SendMessageRequest{
		ChatID:      draft.ChatID,
		Text:        text,
		ParseMode:   "HTML",
		ReplyMarkup: markup,
	})
	if sendErr != nil {
		return sendErr
	}
	return s.store.SetDraftMessageID(ctx, draftID, message.MessageID)
}

func (s *Service) sendFailure(ctx context.Context, payload jobs.IngestPayload, originalErr error) error {
	s.logger.ErrorContext(ctx, "ingest_failed", observability.LogAttrs(ctx, "error", originalErr)...)
	text, err := s.ui.IngestFailed()
	if err != nil {
		return err
	}
	message, err := s.tg.SendMessage(ctx, telegram.SendMessageRequest{
		ChatID:    payload.ChatID,
		Text:      text,
		ParseMode: "HTML",
	})
	if err != nil {
		return err
	}
	s.deleteFeedbackMessage(ctx, payload.ChatID, payload.FeedbackMessageID)
	if err := s.scheduleFailureCleanup(ctx, payload, message.MessageID, ingestFailureCleanupTTL); err != nil {
		return err
	}
	return s.store.UpdateIngestStatus(ctx, payload.TraceID, "failed", originalErr.Error())
}

func (s *Service) finishDraftReady(ctx context.Context, payload jobs.IngestPayload, summary string) error {
	s.deleteFeedbackMessage(ctx, payload.ChatID, payload.FeedbackMessageID)
	return s.store.UpdateIngestStatus(ctx, payload.TraceID, "draft_ready", summary)
}

func (s *Service) deleteFeedbackMessage(ctx context.Context, chatID, messageID int64) {
	if messageID == 0 {
		return
	}
	s.tg.DeleteMessage(ctx, chatID, messageID)
}

func (s *Service) scheduleFailureCleanup(ctx context.Context, payload jobs.IngestPayload, feedbackMessageID int64, delay time.Duration) error {
	now, err := s.currentNow(ctx)
	if err != nil {
		return err
	}
	cleanup := jobs.DeleteMessagesPayload{
		TraceID:    payload.TraceID,
		ChatID:     payload.ChatID,
		MessageIDs: jobs.CompactMessageIDs(payload.MessageID, feedbackMessageID),
	}
	return s.store.EnqueueJob(ctx, payload.TraceID, domain.JobTypeDeleteMessages, cleanup, now.Add(delay), nil)
}

func (s *Service) withPipelineWorkspace(ctx context.Context, pattern, startedEvent string, fn func(string) error) error {
	dir, err := os.MkdirTemp(s.tmpDir, pattern)
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	s.logger.InfoContext(ctx, startedEvent, observability.LogAttrs(ctx, "tmp_dir", dir)...)
	return fn(dir)
}
