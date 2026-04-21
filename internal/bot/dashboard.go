package bot

import (
	"context"
	"errors"

	"github.com/igor/shelfy/internal/storage/postgres"
	"github.com/igor/shelfy/internal/telegram"
	"github.com/jackc/pgx/v5"
)

func (s *Service) RefreshDashboardHome(ctx context.Context, userID, chatID int64) error {
	settings, err := s.store.GetUserSettings(ctx, userID)
	if err != nil {
		return err
	}
	if settings.DashboardMessageID == nil {
		return nil
	}
	_, err = s.ops.ApplyDashboard(ctx, userID, chatID, *settings.DashboardMessageID, homeDashboardState())
	return err
}

func (s *Service) ensureUserSettings(ctx context.Context, userID, chatID int64) (postgres.UserSettings, error) {
	settings, err := s.store.GetUserSettings(ctx, userID)
	switch {
	case err == nil:
		if settings.ChatID == chatID {
			return settings, nil
		}
		settings.ChatID = chatID
		if err := s.store.UpsertUserSettings(ctx, settings); err != nil {
			return postgres.UserSettings{}, err
		}
		return settings, nil
	case errors.Is(err, pgx.ErrNoRows):
		settings = postgres.UserSettings{
			UserID:          userID,
			ChatID:          chatID,
			Timezone:        s.defaultTimezone,
			DigestLocalTime: s.digestLocalTime,
		}
		if err := s.store.UpsertUserSettings(ctx, settings); err != nil {
			return postgres.UserSettings{}, err
		}
		return settings, nil
	default:
		return postgres.UserSettings{}, err
	}
}

func (s *Service) createHomeDashboard(ctx context.Context, userID, chatID int64) (telegram.Message, error) {
	message, _, err := s.ops.CreateDashboard(ctx, userID, chatID, homeDashboardState())
	return message, err
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
