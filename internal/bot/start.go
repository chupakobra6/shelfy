package bot

import (
	"context"

	"github.com/igor/shelfy/internal/observability"
	"github.com/igor/shelfy/internal/storage/postgres"
	"github.com/igor/shelfy/internal/telegram"
)

func (s *Service) HandleStart(ctx context.Context, userID, chatID, startMessageID int64) error {
	now, err := s.currentNow(ctx)
	if err != nil {
		return err
	}
	previousSettings, prevErr := s.store.GetUserSettings(ctx, userID)
	if err := s.store.UpsertUserSettings(ctx, postgres.UserSettings{
		UserID:          userID,
		ChatID:          chatID,
		Timezone:        s.defaultTimezone,
		DigestLocalTime: s.digestLocalTime,
	}); err != nil {
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
	message, err := s.tg.SendMessage(ctx, telegram.SendMessageRequest{
		ChatID:      chatID,
		Text:        text,
		ParseMode:   "HTML",
		ReplyMarkup: markup,
	})
	if err != nil {
		return err
	}
	if err := s.store.SetDashboardMessageID(ctx, userID, message.MessageID); err != nil {
		return err
	}
	if err := s.tg.PinMessage(ctx, chatID, message.MessageID); err != nil {
		return err
	}
	if prevErr == nil && previousSettings.DashboardMessageID != nil && *previousSettings.DashboardMessageID != message.MessageID {
		if err := s.tg.DeleteMessage(ctx, chatID, *previousSettings.DashboardMessageID); err != nil {
			s.logger.WarnContext(ctx, "delete_previous_dashboard_failed", observability.LogAttrs(ctx,
				"user_id", userID,
				"message_id", *previousSettings.DashboardMessageID,
				"error", err,
			)...)
		}
	}
	s.deleteMessagesNow(ctx, chatID, startMessageID)
	s.logger.InfoContext(ctx, "start_dashboard_created", observability.LogAttrs(ctx,
		"user_id", userID,
		"chat_id", chatID,
		"start_message_id", startMessageID,
		"dashboard_message_id", message.MessageID,
	)...)
	return nil
}
