package bot

import (
	"context"
	"time"

	"github.com/igor/shelfy/internal/domain"
	"github.com/igor/shelfy/internal/jobs"
	"github.com/igor/shelfy/internal/observability"
	"github.com/igor/shelfy/internal/telegram"
)

func (s *Service) enterDraftEditMode(ctx context.Context, draft domain.DraftSession, chatID int64, status domain.DraftStatus, text string) error {
	if draft.EditPromptMessageID != nil {
		if err := s.tg.DeleteMessage(ctx, chatID, *draft.EditPromptMessageID); err != nil {
			s.logger.WarnContext(ctx, "delete_previous_edit_prompt_failed", observability.LogAttrs(ctx,
				"chat_id", chatID,
				"message_id", *draft.EditPromptMessageID,
				"error", err,
			)...)
		}
	}
	if err := s.store.UpdateDraftStatus(ctx, draft.ID, status); err != nil {
		return err
	}
	message, err := s.tg.SendMessage(ctx, telegram.SendMessageRequest{
		ChatID:    chatID,
		Text:      text,
		ParseMode: "HTML",
	})
	if err != nil {
		return err
	}
	s.logger.InfoContext(ctx, "draft_edit_mode_entered", observability.LogAttrs(ctx,
		"chat_id", chatID,
		"prompt_message_id", message.MessageID,
		"status", status,
	)...)
	if err := s.store.SetDraftEditPromptMessageID(ctx, draft.ID, &message.MessageID); err != nil {
		return err
	}
	return s.scheduleDeleteMessages(ctx, draft.TraceID, chatID, 30*time.Second, message.MessageID)
}

func (s *Service) scheduleDraftCleanup(ctx context.Context, draft domain.DraftSession, chatID, confirmationMessageID int64) error {
	s.logger.InfoContext(ctx, "draft_cleanup_scheduled", observability.LogAttrs(ctx,
		"chat_id", chatID,
		"confirmation_message_id", confirmationMessageID,
	)...)
	return s.scheduleDeleteMessages(
		ctx,
		draft.TraceID,
		chatID,
		5*time.Second,
		jobs.CompactMessageIDs(
			confirmationMessageID,
			ptrValue(draft.SourceMessageID),
			ptrValue(draft.DraftMessageID),
			ptrValue(draft.FeedbackMessageID),
			ptrValue(draft.EditPromptMessageID),
		)...,
	)
}

func (s *Service) finishDraftAction(ctx context.Context, draft domain.DraftSession, chatID int64, text string) error {
	confirm, err := s.tg.SendMessage(ctx, telegram.SendMessageRequest{
		ChatID:    chatID,
		Text:      text,
		ParseMode: "HTML",
	})
	if err != nil {
		return err
	}
	for _, messageID := range jobs.CompactMessageIDs(ptrValue(draft.DraftMessageID), ptrValue(draft.EditPromptMessageID)) {
		if err := s.tg.DeleteMessage(ctx, chatID, messageID); err != nil {
			s.logger.WarnContext(ctx, "draft_terminal_immediate_delete_failed", observability.LogAttrs(ctx,
				"chat_id", chatID,
				"message_id", messageID,
				"error", err,
			)...)
		}
	}
	if err := s.scheduleDraftCleanup(ctx, draft, chatID, confirm.MessageID); err != nil {
		return err
	}
	return s.RefreshDashboardHome(ctx, draft.UserID, draft.ChatID)
}

func ptrValue(v *int64) int64 {
	if v == nil {
		return 0
	}
	return *v
}
