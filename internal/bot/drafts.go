package bot

import (
	"context"
	"time"

	"github.com/igor/shelfy/internal/domain"
	"github.com/igor/shelfy/internal/observability"
	"github.com/igor/shelfy/internal/telegram"
)

func (s *Service) enterDraftEditMode(ctx context.Context, draft domain.DraftSession, chatID int64, status domain.DraftStatus, text string) error {
	if draft.EditPromptMessageID != nil {
		s.deleteMessagesNow(ctx, chatID, *draft.EditPromptMessageID)
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
	return nil
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
		3*time.Second,
		confirmationMessageID,
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
	s.deleteMessagesNow(ctx, chatID,
		ptrValue(draft.DraftMessageID),
		ptrValue(draft.EditPromptMessageID),
		ptrValue(draft.SourceMessageID),
		ptrValue(draft.FeedbackMessageID),
	)
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
