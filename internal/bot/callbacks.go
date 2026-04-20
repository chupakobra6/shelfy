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

func (s *Service) HandleCallback(ctx context.Context, callback telegram.CallbackQuery) error {
	if callback.Message == nil {
		return nil
	}
	ctx = observability.WithUserID(ctx, callback.From.ID)
	if err := s.tg.AnswerCallbackQuery(ctx, telegram.AnswerCallbackQueryRequest{
		CallbackQueryID: callback.ID,
	}); err != nil {
		s.logger.WarnContext(ctx, "answer_callback_failed", observability.LogAttrs(ctx, "error", err)...)
	}
	parts := strings.Split(callback.Data, ":")
	if len(parts) == 0 {
		return nil
	}
	s.logger.InfoContext(ctx, "callback_received", observability.LogAttrs(ctx, "callback_data", callback.Data)...)
	if parts[0] == "dashboard" || parts[0] == "product" || parts[0] == "settings" {
		current, err := s.ensureCurrentDashboardCallback(ctx, callback.From.ID, callback.Message.MessageID, callback.Message.Chat.ID)
		if err != nil {
			return err
		}
		if !current {
			return nil
		}
	}
	switch parts[0] {
	case "dashboard":
		return s.handleDashboardCallback(ctx, callback, parts)
	case "draft":
		return s.handleDraftCallback(ctx, callback, parts)
	case "product":
		return s.handleProductCallback(ctx, callback, parts)
	case "settings":
		return s.handleSettingsCallback(ctx, callback, parts)
	default:
		return nil
	}
}

func (s *Service) handleDashboardCallback(ctx context.Context, callback telegram.CallbackQuery, parts []string) error {
	if callback.Message == nil {
		return nil
	}
	now, err := s.currentNow(ctx)
	if err != nil {
		return err
	}
	switch {
	case len(parts) >= 2 && parts[1] == "home":
		return s.RefreshDashboardHome(ctx, callback.From.ID, callback.Message.Chat.ID)
	case len(parts) >= 2 && parts[1] == "list":
		products, err := s.store.ListVisibleProducts(ctx, callback.From.ID, "active", now)
		if err != nil {
			return err
		}
		text, markup, err := s.ui.DashboardList(products, "active")
		if err != nil {
			return err
		}
		return s.tg.EditMessageText(ctx, telegram.EditMessageTextRequest{
			ChatID:      callback.Message.Chat.ID,
			MessageID:   callback.Message.MessageID,
			Text:        text,
			ParseMode:   "HTML",
			ReplyMarkup: markup,
		})
	case len(parts) >= 2 && parts[1] == "soon":
		products, err := s.store.ListVisibleProducts(ctx, callback.From.ID, "soon", now)
		if err != nil {
			return err
		}
		text, markup, err := s.ui.DashboardList(products, "soon")
		if err != nil {
			return err
		}
		return s.tg.EditMessageText(ctx, telegram.EditMessageTextRequest{
			ChatID:      callback.Message.Chat.ID,
			MessageID:   callback.Message.MessageID,
			Text:        text,
			ParseMode:   "HTML",
			ReplyMarkup: markup,
		})
	case len(parts) >= 2 && parts[1] == "stats":
		stats, err := s.store.DashboardStats(ctx, callback.From.ID, now)
		if err != nil {
			return err
		}
		text, markup, err := s.ui.DashboardStats(stats)
		if err != nil {
			return err
		}
		return s.tg.EditMessageText(ctx, telegram.EditMessageTextRequest{
			ChatID:      callback.Message.Chat.ID,
			MessageID:   callback.Message.MessageID,
			Text:        text,
			ParseMode:   "HTML",
			ReplyMarkup: markup,
		})
	case len(parts) >= 2 && parts[1] == "settings":
		settings, err := s.store.GetUserSettings(ctx, callback.From.ID)
		if err != nil {
			return err
		}
		text, markup, err := s.ui.SettingsCard(settings.Timezone, settings.DigestLocalTime)
		if err != nil {
			return err
		}
		return s.tg.EditMessageText(ctx, telegram.EditMessageTextRequest{
			ChatID:      callback.Message.Chat.ID,
			MessageID:   callback.Message.MessageID,
			Text:        text,
			ParseMode:   "HTML",
			ReplyMarkup: markup,
		})
	default:
		return nil
	}
}

func (s *Service) handleSettingsCallback(ctx context.Context, callback telegram.CallbackQuery, parts []string) error {
	if callback.Message == nil || len(parts) < 2 {
		return nil
	}
	settings, err := s.store.GetUserSettings(ctx, callback.From.ID)
	if err != nil {
		return err
	}
	switch parts[1] {
	case "digest":
		if len(parts) < 3 {
			return nil
		}
		updated, err := adjustDigestTime(settings.DigestLocalTime, parts[2])
		if err != nil {
			return err
		}
		if err := s.store.UpdateUserDigestLocalTime(ctx, callback.From.ID, updated); err != nil {
			return err
		}
		settings.DigestLocalTime = updated
	case "timezone":
		if len(parts) < 3 {
			return nil
		}
		if !isAllowedTimezone(parts[2]) {
			return nil
		}
		if err := s.store.UpdateUserTimezone(ctx, callback.From.ID, parts[2]); err != nil {
			return err
		}
		settings.Timezone = parts[2]
	default:
		return nil
	}
	text, markup, err := s.ui.SettingsCard(settings.Timezone, settings.DigestLocalTime)
	if err != nil {
		return err
	}
	return s.tg.EditMessageText(ctx, telegram.EditMessageTextRequest{
		ChatID:      callback.Message.Chat.ID,
		MessageID:   callback.Message.MessageID,
		Text:        text,
		ParseMode:   "HTML",
		ReplyMarkup: markup,
	})
}

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
		text, err := s.ui.DraftConfirmed(product.Name, product.ExpiresOn)
		if err != nil {
			return err
		}
		return s.finishDraftAction(ctx, draft, callback.Message.Chat.ID, text)
	case "cancel":
		if err := s.store.UpdateDraftStatus(ctx, draftID, domain.DraftStatusCanceled); err != nil {
			return err
		}
		text, err := s.ui.DraftCanceled()
		if err != nil {
			return err
		}
		return s.finishDraftAction(ctx, draft, callback.Message.Chat.ID, text)
	case "delete":
		if err := s.store.UpdateDraftStatus(ctx, draftID, domain.DraftStatusDeleted); err != nil {
			return err
		}
		text, err := s.ui.DraftCanceled()
		if err != nil {
			return err
		}
		return s.finishDraftAction(ctx, draft, callback.Message.Chat.ID, text)
	case "edit_name":
		text, err := s.ui.DraftEditNamePrompt()
		if err != nil {
			return err
		}
		return s.enterDraftEditMode(ctx, draft, callback.Message.Chat.ID, domain.DraftStatusEditingName, text)
	case "edit_date":
		text, err := s.ui.DraftEditDatePrompt()
		if err != nil {
			return err
		}
		return s.enterDraftEditMode(ctx, draft, callback.Message.Chat.ID, domain.DraftStatusEditingDate, text)
	default:
		return nil
	}
}

func (s *Service) handleProductCallback(ctx context.Context, callback telegram.CallbackQuery, parts []string) error {
	if len(parts) < 3 {
		return nil
	}
	productID, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil {
		return nil
	}
	switch parts[1] {
	case "open":
		product, err := s.store.GetProduct(ctx, productID)
		if err != nil {
			return err
		}
		text, markup, err := s.ui.ProductCard(product)
		if err != nil {
			return err
		}
		return s.tg.EditMessageText(ctx, telegram.EditMessageTextRequest{
			ChatID:      callback.Message.Chat.ID,
			MessageID:   callback.Message.MessageID,
			Text:        text,
			ParseMode:   "HTML",
			ReplyMarkup: markup,
		})
	case "set":
		if len(parts) < 4 {
			return nil
		}
		status := domain.ProductStatus(parts[3])
		if err := s.store.UpdateProductStatus(ctx, productID, status); err != nil {
			return err
		}
		return s.RefreshDashboardHome(ctx, callback.From.ID, callback.Message.Chat.ID)
	default:
		return nil
	}
}
