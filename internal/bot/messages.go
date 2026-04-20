package bot

import (
	"context"
	"strings"
	"time"

	"github.com/igor/shelfy/internal/domain"
	"github.com/igor/shelfy/internal/jobs"
	"github.com/igor/shelfy/internal/observability"
	"github.com/igor/shelfy/internal/storage/postgres"
	"github.com/igor/shelfy/internal/telegram"
)

func (s *Service) HandleMessage(ctx context.Context, msg telegram.Message) error {
	if msg.Chat.Type != "private" || msg.From == nil {
		return nil
	}
	if msg.From.IsBot {
		s.logger.InfoContext(ctx, "telegram_message_ignored_from_bot", observability.LogAttrs(ctx,
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
		return s.handleDraftEditMessage(observability.WithDraftID(ctx, editable.ID), editable, msg)
	}

	kind, fileID := classifyMessage(msg)
	s.logger.InfoContext(ctx, "telegram_message_classified", observability.LogAttrs(ctx,
		"message_id", msg.MessageID,
		"kind", kind,
		"has_text", strings.TrimSpace(msg.Text) != "",
		"has_file", fileID != "",
	)...)
	if kind == domain.MessageKindUnsupported {
		return s.handleUnsupportedMessage(ctx, msg)
	}
	if err := s.store.UpsertUserSettings(ctx, postgres.UserSettings{
		UserID:          msg.From.ID,
		ChatID:          msg.Chat.ID,
		Timezone:        s.defaultTimezone,
		DigestLocalTime: s.digestLocalTime,
	}); err != nil {
		return err
	}
	traceID := observability.TraceID(observability.EnsureTraceID(ctx))
	if err := s.store.SaveIngestEvent(ctx, traceID, msg.From.ID, msg.Chat.ID, msg.MessageID, kind, "queued", "accepted for background processing", map[string]any{
		"has_text": strings.TrimSpace(msg.Text) != "",
		"file_id":  fileID,
	}); err != nil {
		return err
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
	payload := jobs.IngestPayload{
		TraceID:           traceID,
		UserID:            msg.From.ID,
		ChatID:            msg.Chat.ID,
		MessageID:         msg.MessageID,
		FeedbackMessageID: feedback.MessageID,
		FileID:            fileID,
		Text:              msg.Text,
		Kind:              kind,
	}
	jobType := jobTypeForMessage(kind)
	if err := s.enqueueJobNow(ctx, traceID, jobType, payload, nil); err != nil {
		return err
	}
	return s.scheduleDeleteMessages(ctx, traceID, msg.Chat.ID, 45*time.Second, feedback.MessageID)
}

func (s *Service) handleDraftEditMessage(ctx context.Context, draft domain.DraftSession, msg telegram.Message) error {
	settings, err := s.store.GetUserSettings(ctx, draft.UserID)
	if err != nil {
		return err
	}
	now, err := s.currentNow(ctx)
	if err != nil {
		return err
	}
	location, err := time.LoadLocation(settings.Timezone)
	if err != nil {
		location = time.UTC
	}
	switch draft.Status {
	case domain.DraftStatusEditingName:
		name := strings.TrimSpace(msg.Text)
		if name == "" {
			return s.handleInvalidDraftEditMessage(ctx, draft, msg, false)
		}
		if err := s.store.UpdateDraftFields(ctx, draft.ID, name, draft.DraftExpiresOn, draft.RawDeadlinePhrase, domain.DraftStatusReady); err != nil {
			return err
		}
	case domain.DraftStatusEditingDate:
		resolved := domain.ResolveRelativeDate(msg.Text, now.In(location))
		if resolved.Value == nil {
			return s.handleInvalidDraftEditMessage(ctx, draft, msg, true)
		}
		if err := s.store.UpdateDraftFields(ctx, draft.ID, draft.DraftName, resolved.Value, msg.Text, domain.DraftStatusReady); err != nil {
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
	if err := s.cleanupDraftEditMessages(ctx, updated, msg.Chat.ID, msg.MessageID); err != nil {
		return err
	}
	return nil
}

func (s *Service) handleInvalidDraftEditMessage(ctx context.Context, draft domain.DraftSession, msg telegram.Message, dateMode bool) error {
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
	return s.scheduleDeleteMessages(ctx, draft.TraceID, msg.Chat.ID, 15*time.Second, msg.MessageID, feedback.MessageID)
}

func (s *Service) cleanupDraftEditMessages(ctx context.Context, draft domain.DraftSession, chatID, userMessageID int64) error {
	messageIDs := jobs.CompactMessageIDs(userMessageID, ptrValue(draft.EditPromptMessageID))
	for _, messageID := range messageIDs {
		if err := s.tg.DeleteMessage(ctx, chatID, messageID); err != nil {
			s.logger.WarnContext(ctx, "draft_edit_message_delete_failed", observability.LogAttrs(ctx,
				"chat_id", chatID,
				"message_id", messageID,
				"error", err,
			)...)
		}
	}
	return nil
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
	if err := s.store.SaveIngestEvent(ctx, traceID, msg.From.ID, msg.Chat.ID, msg.MessageID, domain.MessageKindUnsupported, "unsupported", "unsupported message type", nil); err != nil {
		return err
	}
	return s.scheduleDeleteMessages(ctx, traceID, msg.Chat.ID, 20*time.Second, msg.MessageID, feedback.MessageID)
}
