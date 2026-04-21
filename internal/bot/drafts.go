package bot

import (
	"context"
	"time"

	"github.com/igor/shelfy/internal/domain"
	"github.com/igor/shelfy/internal/observability"
	"github.com/igor/shelfy/internal/telegram"
)

func (s *Service) enterDraftEditMode(ctx context.Context, draft domain.DraftSession, chatID int64, status domain.DraftStatus, text string) error {
	if shouldReuseDraftEditPrompt(draft, status) {
		s.logger.InfoContext(ctx, "draft_edit_request_ignored_duplicate", observability.LogAttrs(ctx,
			"chat_id", chatID,
			"status", status,
			"prompt_message_id", ptrValue(draft.EditPromptMessageID),
		)...)
		return nil
	}
	nextStatus, err := enterDraftEditTransition(draft.Status, status)
	if err != nil {
		return err
	}

	var previousPromptMessageID *int64
	if draft.EditPromptMessageID != nil {
		previousPromptMessageID = draft.EditPromptMessageID
	}
	if err := s.store.UpdateDraftStatus(ctx, draft.ID, nextStatus); err != nil {
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
		"state_from", draft.Status,
		"state_to", nextStatus,
	)...)
	if err := s.store.SetDraftEditPromptMessageID(ctx, draft.ID, &message.MessageID); err != nil {
		return err
	}
	if previousPromptMessageID != nil && *previousPromptMessageID != message.MessageID {
		if err := s.scheduleDeleteMessages(ctx, draft.TraceID, chatID, 0, *previousPromptMessageID); err != nil {
			return err
		}
	}
	return nil
}

func shouldReuseDraftEditPrompt(draft domain.DraftSession, status domain.DraftStatus) bool {
	return draft.Status == status && draft.EditPromptMessageID != nil
}

func (s *Service) scheduleDraftCleanup(ctx context.Context, draft domain.DraftSession, chatID, confirmationMessageID int64) error {
	s.logger.InfoContext(ctx, "draft_cleanup_scheduled", observability.LogAttrs(ctx,
		"chat_id", chatID,
		"confirmation_message_id", confirmationMessageID,
	)...)
	return s.scheduleDeleteMessagesWithOrigin(
		ctx,
		draft.TraceID,
		"draft_finish",
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
	if err := s.deleteMessagesReliably(ctx, draft.TraceID, "draft_finish", chatID, 0,
		ptrValue(draft.DraftMessageID),
		ptrValue(draft.EditPromptMessageID),
		ptrValue(draft.SourceMessageID),
		ptrValue(draft.FeedbackMessageID),
	); err != nil {
		return err
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
