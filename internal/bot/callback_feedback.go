package bot

import (
	"context"
	"errors"
	"time"

	"github.com/igor/shelfy/internal/domain"
	"github.com/igor/shelfy/internal/observability"
	"github.com/igor/shelfy/internal/telegram"
	"github.com/jackc/pgx/v5"
)

func (s *Service) sendTransientFeedback(ctx context.Context, chatID int64, text string, delay time.Duration) error {
	message, err := s.tg.SendMessage(ctx, telegram.SendMessageRequest{
		ChatID:    chatID,
		Text:      text,
		ParseMode: "HTML",
	})
	if err != nil {
		return err
	}
	traceID := observability.TraceID(observability.EnsureTraceID(ctx))
	return s.scheduleDeleteMessages(ctx, traceID, chatID, delay, message.MessageID)
}

func (s *Service) ensureCurrentDashboardCallback(ctx context.Context, userID int64, messageID int64, chatID int64) (bool, error) {
	settings, err := s.store.GetUserSettings(ctx, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	if settings.DashboardMessageID != nil && *settings.DashboardMessageID == messageID {
		return true, nil
	}
	s.logger.InfoContext(ctx, "dashboard_callback_ignored_stale", observability.LogAttrs(ctx,
		"message_id", messageID,
		"chat_id", chatID,
		"current_dashboard_message_id", ptrValue(settings.DashboardMessageID),
	)...)
	text, err := s.ui.DashboardStale()
	if err != nil {
		return false, err
	}
	if err := s.sendTransientFeedback(ctx, chatID, text, 20*time.Second); err != nil {
		return false, err
	}
	return false, nil
}

func isDraftTerminal(status domain.DraftStatus) bool {
	switch status {
	case domain.DraftStatusConfirmed, domain.DraftStatusCanceled, domain.DraftStatusDeleted, domain.DraftStatusFailed:
		return true
	default:
		return false
	}
}
