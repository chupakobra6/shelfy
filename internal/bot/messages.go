package bot

import (
	"context"
	"strings"
	"time"

	"github.com/igor/shelfy/internal/domain"
	"github.com/igor/shelfy/internal/jobs"
	"github.com/igor/shelfy/internal/observability"
	"github.com/igor/shelfy/internal/telegram"
)

const transientInputCleanupTTL = 6 * time.Second

func (s *Service) HandleMessage(ctx context.Context, msg telegram.Message) error {
	if msg.Chat.Type != "private" {
		return nil
	}
	if handled, err := s.handleServiceMessage(ctx, msg); err != nil {
		return err
	} else if handled {
		return nil
	}
	if msg.From == nil {
		return nil
	}
	if msg.From.IsBot {
		s.logger.DebugContext(ctx, "telegram_message_ignored_from_bot", observability.LogAttrs(ctx,
			"message_id", msg.MessageID,
			"user_id", msg.From.ID,
			"username", msg.From.Username,
		)...)
		return nil
	}
	ctx = observability.WithUserID(ctx, msg.From.ID)
	if editable, ok, err := s.store.FindEditableDraft(ctx, msg.From.ID); err != nil {
		return err
	} else if ok && msg.Text != "" {
		s.logger.InfoContext(ctx, "draft_edit_input_received", observability.LogAttrs(ctx,
			"draft_id", editable.ID,
			"message_id", msg.MessageID,
			"draft_status", editable.Status,
		)...)
		return s.handleDraftEditMessage(observability.WithDraftID(ctx, editable.ID), editable, msg)
	}

	kind, fileID := classifyMessage(msg)
	s.logger.InfoContext(ctx, "telegram_message_classified", observability.LogAttrs(ctx,
		"message_id", msg.MessageID,
		"kind", kind,
		"has_text", strings.TrimSpace(msg.Text) != "",
		"has_caption", strings.TrimSpace(msg.Caption) != "",
		"has_file", fileID != "",
	)...)
	if kind == domain.MessageKindUnsupported {
		return s.handleUnsupportedMessage(ctx, msg)
	}
	if _, err := s.ensureUserSettings(ctx, msg.From.ID, msg.Chat.ID); err != nil {
		return err
	}
	traceID := observability.TraceID(observability.EnsureTraceID(ctx))
	if err := s.store.SaveIngestEvent(ctx, traceID, msg.From.ID, msg.Chat.ID, msg.MessageID, kind, "queued", "accepted for background processing", map[string]any{
		"has_text": strings.TrimSpace(msg.Text) != "",
		"file_id":  fileID,
	}); err != nil {
		return err
	}
	payload := jobs.IngestPayload{
		TraceID:   traceID,
		UserID:    msg.From.ID,
		ChatID:    msg.Chat.ID,
		MessageID: msg.MessageID,
		FileID:    fileID,
		Text:      msg.Text,
		Caption:   msg.Caption,
		Kind:      kind,
	}

	if handled, err := s.tryHandleTextFast(ctx, payload); err != nil {
		return err
	} else if handled {
		return nil
	}

	processing, err := s.ui.ProcessingMessage()
	if err != nil {
		return err
	}
	feedback, err := s.tg.SendMessage(ctx, telegram.SendMessageRequest{
		ChatID:    msg.Chat.ID,
		Text:      processing,
		ParseMode: "HTML",
	})
	if err != nil {
		return err
	}
	payload.FeedbackMessageID = feedback.MessageID
	jobType := jobTypeForMessage(kind)
	fallbackCleanupAt := 2 * time.Minute
	if err := s.enqueueJobNow(ctx, traceID, jobType, payload, nil); err != nil {
		return err
	}
	s.logger.InfoContext(ctx, "message_accepted_async", observability.LogAttrs(ctx,
		"message_id", msg.MessageID,
		"kind", kind,
		"job_type", jobType,
		"feedback_message_id", feedback.MessageID,
	)...)
	return s.scheduleDeleteMessages(ctx, traceID, msg.Chat.ID, fallbackCleanupAt, feedback.MessageID)
}

func (s *Service) tryHandleTextFast(ctx context.Context, payload jobs.IngestPayload) (bool, error) {
	if payload.Kind != domain.MessageKindText || s.textFastPath == nil {
		return false, nil
	}
	handled, err := s.textFastPath.TryHandleTextFast(ctx, payload)
	if err != nil {
		return false, err
	}
	if !handled {
		return false, nil
	}
	s.logger.InfoContext(ctx, "message_processed_fast_path", observability.LogAttrs(ctx,
		"message_id", payload.MessageID,
		"kind", payload.Kind,
	)...)
	return true, nil
}

func (s *Service) handleDraftEditMessage(ctx context.Context, draft domain.DraftSession, msg telegram.Message) error {
	promptMessageID := draft.EditPromptMessageID
	settings, err := s.store.GetUserSettings(ctx, draft.UserID)
	if err != nil {
		return err
	}
	now, err := s.currentNow(ctx)
	if err != nil {
		return err
	}
	switch draft.Status {
	case domain.DraftStatusEditingName:
		name := strings.TrimSpace(msg.Text)
		if name == "" {
			return s.handleInvalidDraftEditMessage(ctx, draft, msg, false)
		}
		nextStatus, err := applyDraftEditTransition(draft.Status)
		if err != nil {
			return err
		}
		if err := s.store.UpdateDraftFields(ctx, draft.ID, name, draft.DraftExpiresOn, draft.RawDeadlinePhrase, nextStatus); err != nil {
			return err
		}
	case domain.DraftStatusEditingDate:
		resolved := domain.ResolveRelativeDate(msg.Text, domain.LocalizeTime(now, settings.Timezone))
		if resolved.Value == nil {
			return s.handleInvalidDraftEditMessage(ctx, draft, msg, true)
		}
		nextStatus, err := applyDraftEditTransition(draft.Status)
		if err != nil {
			return err
		}
		if err := s.store.UpdateDraftFields(ctx, draft.ID, draft.DraftName, resolved.Value, msg.Text, nextStatus); err != nil {
			return err
		}
	default:
		return nil
	}
	updated, err := s.store.GetDraftSession(ctx, draft.ID)
	if err != nil {
		return err
	}
	text, markup, err := s.ui.DraftCard(updated)
	if err != nil {
		return err
	}
	if updated.DraftMessageID != nil {
		if err := s.tg.EditMessageText(ctx, telegram.EditMessageTextRequest{
			ChatID:      msg.Chat.ID,
			MessageID:   *updated.DraftMessageID,
			Text:        text,
			ParseMode:   "HTML",
			ReplyMarkup: markup,
		}); err != nil {
			return err
		}
	}
	s.logger.InfoContext(ctx, "draft_edit_applied", observability.LogAttrs(ctx,
		"draft_id", draft.ID,
		"state_from", draft.Status,
		"state_to", updated.Status,
		"message_id", msg.MessageID,
		"has_name", strings.TrimSpace(updated.DraftName) != "",
		"has_expiry", updated.DraftExpiresOn != nil,
	)...)
	return s.cleanupDraftEditMessages(ctx, draft.TraceID, msg.Chat.ID, msg.MessageID, ptrValue(promptMessageID))
}

func (s *Service) handleInvalidDraftEditMessage(ctx context.Context, draft domain.DraftSession, msg telegram.Message, dateMode bool) error {
	if _, err := invalidDraftEditTransition(draft.Status); err != nil {
		return err
	}
	var (
		text string
		err  error
	)
	if dateMode {
		text, err = s.ui.DraftEditDateInvalid()
	} else {
		text, err = s.ui.DraftEditNamePrompt()
	}
	if err != nil {
		return err
	}
	feedback, err := s.tg.SendMessage(ctx, telegram.SendMessageRequest{
		ChatID:    msg.Chat.ID,
		Text:      text,
		ParseMode: "HTML",
	})
	if err != nil {
		return err
	}
	s.logger.InfoContext(ctx, "draft_edit_invalid", observability.LogAttrs(ctx,
		"draft_id", draft.ID,
		"draft_status", draft.Status,
		"message_id", msg.MessageID,
		"date_mode", dateMode,
	)...)
	return s.cleanupSourceMessageWithFeedback(ctx, draft.TraceID, "invalid_input", msg.Chat.ID, msg.MessageID, feedback.MessageID, transientInputCleanupTTL)
}

func (s *Service) cleanupDraftEditMessages(ctx context.Context, traceID string, chatID, userMessageID, promptMessageID int64) error {
	return s.deleteMessagesReliably(ctx, traceID, "draft_edit_cleanup", chatID, 0, userMessageID, promptMessageID)
}

func (s *Service) handleUnsupportedMessage(ctx context.Context, msg telegram.Message) error {
	text, err := s.ui.UnsupportedMessage()
	if err != nil {
		return err
	}
	feedback, err := s.tg.SendMessage(ctx, telegram.SendMessageRequest{
		ChatID:    msg.Chat.ID,
		Text:      text,
		ParseMode: "HTML",
	})
	if err != nil {
		return err
	}
	traceID := observability.TraceID(observability.EnsureTraceID(ctx))
	s.logger.InfoContext(ctx, "unsupported_message_handled", observability.LogAttrs(ctx,
		"message_id", msg.MessageID,
		"kind", "unsupported",
	)...)
	if err := s.store.SaveIngestEvent(ctx, traceID, msg.From.ID, msg.Chat.ID, msg.MessageID, domain.MessageKindUnsupported, "unsupported", "unsupported message type", nil); err != nil {
		return err
	}
	return s.cleanupSourceMessageWithFeedback(ctx, traceID, "unsupported_input", msg.Chat.ID, msg.MessageID, feedback.MessageID, transientInputCleanupTTL)
}

func (s *Service) cleanupSourceMessageWithFeedback(ctx context.Context, traceID, origin string, chatID, sourceMessageID, feedbackMessageID int64, feedbackTTL time.Duration) error {
	if err := s.deleteMessagesReliably(ctx, traceID, origin, chatID, 0, sourceMessageID); err != nil {
		return err
	}
	return s.scheduleDeleteMessagesWithOrigin(ctx, traceID, origin, chatID, feedbackTTL, feedbackMessageID)
}
