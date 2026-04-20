package bot

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/igor/shelfy/internal/domain"
	"github.com/igor/shelfy/internal/observability"
	"github.com/igor/shelfy/internal/telegram"
)

func (s *Service) handleDraftCallback(ctx context.Context, callback telegram.CallbackQuery, parts []string) error {
	if len(parts) < 3 {
		return nil
	}
	draftID, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil {
		return nil
	}
	ctx = observability.WithDraftID(ctx, draftID)
	draft, err := s.store.GetDraftSession(ctx, draftID)
	if err != nil {
		return err
	}
	if isDraftTerminal(draft.Status) {
		s.logger.InfoContext(ctx, "draft_callback_ignored_terminal", observability.LogAttrs(ctx,
			"draft_id", draftID,
			"draft_status", draft.Status,
			"action", parts[1],
		)...)
		text, err := s.ui.DraftAlreadyProcessed()
		if err != nil {
			return err
		}
		if err := s.sendTransientFeedback(ctx, callback.Message.Chat.ID, text, 20*time.Second); err != nil {
			return err
		}
		if draft.DraftMessageID != nil {
			if err := s.scheduleDeleteMessages(ctx, draft.TraceID, draft.ChatID, 5*time.Second, *draft.DraftMessageID); err != nil {
				return err
			}
		}
		return nil
	}
	switch parts[1] {
	case "confirm":
		if draft.DraftExpiresOn == nil || strings.TrimSpace(draft.DraftName) == "" {
			s.logger.InfoContext(ctx, "draft_confirm_blocked_incomplete", observability.LogAttrs(ctx,
				"draft_id", draftID,
				"has_name", strings.TrimSpace(draft.DraftName) != "",
				"has_expiry", draft.DraftExpiresOn != nil,
			)...)
			text, err := s.ui.DraftIncomplete()
			if err != nil {
				return err
			}
			return s.sendTransientFeedback(ctx, callback.Message.Chat.ID, text, 20*time.Second)
		}
		product, err := s.store.CreateProductFromDraft(ctx, draftID)
		if err != nil {
			return err
		}
		s.logger.InfoContext(ctx, "draft_confirmed", observability.LogAttrs(ctx,
			"draft_id", draftID,
			"product_id", product.ID,
			"product_name", product.Name,
		)...)
		text, err := s.ui.DraftConfirmed(product.Name, product.ExpiresOn)
		if err != nil {
			return err
		}
		return s.finishDraftAction(ctx, draft, callback.Message.Chat.ID, text)
	case "cancel":
		return s.closeDraftWithStatus(ctx, draftID, draft, callback.Message.Chat.ID, domain.DraftStatusCanceled)
	case "delete":
		return s.closeDraftWithStatus(ctx, draftID, draft, callback.Message.Chat.ID, domain.DraftStatusDeleted)
	case "edit_name":
		text, err := s.ui.DraftEditNamePrompt()
		if err != nil {
			return err
		}
		s.logger.InfoContext(ctx, "draft_edit_requested", observability.LogAttrs(ctx,
			"draft_id", draftID,
			"mode", "name",
		)...)
		return s.enterDraftEditMode(ctx, draft, callback.Message.Chat.ID, domain.DraftStatusEditingName, text)
	case "edit_date":
		text, err := s.ui.DraftEditDatePrompt()
		if err != nil {
			return err
		}
		s.logger.InfoContext(ctx, "draft_edit_requested", observability.LogAttrs(ctx,
			"draft_id", draftID,
			"mode", "date",
		)...)
		return s.enterDraftEditMode(ctx, draft, callback.Message.Chat.ID, domain.DraftStatusEditingDate, text)
	default:
		return nil
	}
}

func (s *Service) closeDraftWithStatus(ctx context.Context, draftID int64, draft domain.DraftSession, chatID int64, status domain.DraftStatus) error {
	if err := s.store.UpdateDraftStatus(ctx, draftID, status); err != nil {
		return err
	}
	s.logger.InfoContext(ctx, "draft_closed", observability.LogAttrs(ctx,
		"draft_id", draftID,
		"status", status,
	)...)
	text, err := s.ui.DraftCanceled()
	if err != nil {
		return err
	}
	return s.finishDraftAction(ctx, draft, chatID, text)
}
