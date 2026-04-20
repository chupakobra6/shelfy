package bot

import (
	"context"

	"github.com/igor/shelfy/internal/telegram"
)

func (s *Service) RefreshDashboardHome(ctx context.Context, userID, chatID int64) error {
	settings, err := s.store.GetUserSettings(ctx, userID)
	if err != nil {
		return err
	}
	if settings.DashboardMessageID == nil {
		return nil
	}
	now, err := s.currentNow(ctx)
	if err != nil {
		return err
	}
	stats, err := s.store.DashboardStats(ctx, userID, now)
	if err != nil {
		return err
	}
	text, markup, err := s.ui.DashboardHome(stats)
	if err != nil {
		return err
	}
	return s.tg.EditMessageText(ctx, telegram.EditMessageTextRequest{
		ChatID:      chatID,
		MessageID:   *settings.DashboardMessageID,
		Text:        text,
		ParseMode:   "HTML",
		ReplyMarkup: markup,
	})
}

func (s *Service) editDashboardMessage(ctx context.Context, chatID, messageID int64, text string, markup *telegram.InlineKeyboardMarkup) error {
	return s.tg.EditMessageText(ctx, telegram.EditMessageTextRequest{
		ChatID:      chatID,
		MessageID:   messageID,
		Text:        text,
		ParseMode:   "HTML",
		ReplyMarkup: markup,
	})
}
