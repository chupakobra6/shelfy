package ingest

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/igor/shelfy/internal/domain"
	"github.com/igor/shelfy/internal/jobs"
	"github.com/igor/shelfy/internal/observability"
	"github.com/igor/shelfy/internal/telegram"
)

const ingestFailureCleanupTTL = 8 * time.Second

func (s *Service) handleText(ctx context.Context, payload jobs.IngestPayload, now time.Time) error {
	draft, err := s.parseTextDraft(ctx, payload.Text, now)
	if err != nil {
		return s.sendFailure(ctx, payload, err)
	}
	return s.createDraftCard(ctx, payload, draft)
}

func (s *Service) handlePhoto(ctx context.Context, payload jobs.IngestPayload, now time.Time) error {
	return s.withPipelineWorkspace(ctx, "photo-*", "photo_pipeline_started", func(dir string) error {
		imagePath := filepath.Join(dir, "input.jpg")
		if err := s.tg.DownloadFile(ctx, payload.FileID, imagePath); err != nil {
			return s.sendFailure(ctx, payload, err)
		}
		ocrText, err := s.runTesseract(ctx, imagePath)
		if err != nil {
			s.logger.WarnContext(ctx, "tesseract_failed", observability.LogAttrs(ctx, "error", err)...)
		}
		draft, err := s.parsePhotoDraft(ctx, payload.Caption, ocrText, now, imagePath)
		if err != nil {
			return s.sendFailure(ctx, payload, err)
		}
		return s.createDraftCard(ctx, payload, draft)
	})
}

func (s *Service) handleAudio(ctx context.Context, payload jobs.IngestPayload, now time.Time) error {
	return s.withPipelineWorkspace(ctx, "audio-*", "audio_pipeline_started", func(dir string) error {
		inputPath := filepath.Join(dir, "input")
		if err := s.tg.DownloadFile(ctx, payload.FileID, inputPath); err != nil {
			return s.sendFailure(ctx, payload, err)
		}
		wavPath := filepath.Join(dir, "input.wav")
		if err := s.runFFmpeg(ctx, inputPath, wavPath); err != nil {
			return s.sendFailure(ctx, payload, err)
		}
		transcript, err := s.runVosk(ctx, wavPath)
		if err != nil {
			return s.sendFailure(ctx, payload, err)
		}
		draft, err := s.parseTextDraft(ctx, transcript, now)
		if err != nil {
			return s.sendFailure(ctx, payload, err)
		}
		return s.createDraftCard(ctx, payload, draft)
	})
}

func (s *Service) createDraftCard(ctx context.Context, payload jobs.IngestPayload, parsed parsedDraft) error {
	var draftID int64
	if existing, ok, err := s.store.FindDraftSessionByTraceID(ctx, payload.TraceID); err != nil {
		return err
	} else if ok {
		draftID = existing.ID
		if err := s.store.UpdateDraftFields(ctx, draftID, parsed.Name, parsed.ExpiresOn, parsed.RawDeadlinePhrase, domain.DraftStatusReady); err != nil {
			return err
		}
		if existing.DraftMessageID != nil {
			s.logger.InfoContext(ctx, "draft_card_reused_existing", observability.LogAttrs(ctx,
				"draft_id", existing.ID,
				"draft_message_id", *existing.DraftMessageID,
			)...)
			return s.finishDraftReady(ctx, payload, "draft reused")
		}
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
			DraftPayload: map[string]any{
				"confidence": parsed.Confidence,
				"source":     parsed.Source,
			},
		}
		var err error
		draftID, err = s.store.CreateDraftSession(ctx, session)
		if err != nil {
			return err
		}
	}
	ctx = observability.WithDraftID(ctx, draftID)
	draft, err := s.store.GetDraftSession(ctx, draftID)
	if err != nil {
		return err
	}
	text, markup, err := s.ui.DraftCard(draft)
	if err != nil {
		return err
	}
	message, err := s.tg.SendMessage(ctx, telegram.SendMessageRequest{
		ChatID:      payload.ChatID,
		Text:        text,
		ParseMode:   "HTML",
		ReplyMarkup: markup,
	})
	if err != nil {
		return err
	}
	if err := s.store.SetDraftMessageID(ctx, draftID, message.MessageID); err != nil {
		return err
	}
	s.logger.InfoContext(ctx, "draft_card_created", observability.LogAttrs(ctx,
		"draft_message_id", message.MessageID,
		"source_kind", payload.Kind,
		"confidence", parsed.Confidence,
		"source", parsed.Source,
	)...)
	return s.finishDraftReady(ctx, payload, "draft created")
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
