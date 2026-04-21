package bot

import (
	"context"
	"errors"

	"github.com/igor/shelfy/internal/observability"
	"github.com/igor/shelfy/internal/telegram"
	"github.com/jackc/pgx/v5"
)

func shouldDeletePinnedDashboardServiceMessage(currentDashboardMessageID *int64, msg telegram.Message) bool {
	if msg.Chat.Type != "private" || msg.PinnedMessage == nil || currentDashboardMessageID == nil {
		return false
	}
	return *currentDashboardMessageID == msg.PinnedMessage.MessageID
}

func (s *Service) handleServiceMessage(ctx context.Context, msg telegram.Message) (bool, error) {
	if msg.Chat.Type != "private" || msg.PinnedMessage == nil {
		return false, nil
	}

	settings, err := s.store.GetUserSettings(ctx, msg.Chat.ID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			s.logger.DebugContext(ctx, "pinned_service_message_ignored_missing_user_settings", observability.LogAttrs(ctx,
				"chat_id", msg.Chat.ID,
				"message_id", msg.MessageID,
				"pinned_target_message_id", msg.PinnedMessage.MessageID,
			)...)
			return true, nil
		}
		return true, err
	}

	if !shouldDeletePinnedDashboardServiceMessage(settings.DashboardMessageID, msg) {
		s.logger.DebugContext(ctx, "pinned_service_message_ignored_unrelated", observability.LogAttrs(ctx,
			"chat_id", msg.Chat.ID,
			"message_id", msg.MessageID,
			"pinned_target_message_id", msg.PinnedMessage.MessageID,
			"current_dashboard_message_id", ptrValue(settings.DashboardMessageID),
		)...)
		return true, nil
	}

	s.logger.InfoContext(ctx, "dashboard_pin_service_message_deleted", observability.LogAttrs(ctx,
		"chat_id", msg.Chat.ID,
		"message_id", msg.MessageID,
		"dashboard_message_id", msg.PinnedMessage.MessageID,
	)...)
	traceID := observability.TraceID(observability.EnsureTraceID(ctx))
	if err := s.deleteMessagesReliably(ctx, traceID, "pin_service", msg.Chat.ID, 0, msg.MessageID); err != nil {
		return true, err
	}
	return true, nil
}
