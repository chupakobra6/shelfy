package bot

import (
	"context"

	"github.com/igor/shelfy/internal/observability"
	"github.com/igor/shelfy/internal/telegram"
)

func (s *Service) HandleStart(ctx context.Context, userID, chatID, startMessageID int64) error {
	traceID := observability.TraceID(observability.EnsureTraceID(ctx))
	settings, err := s.ensureUserSettings(ctx, userID, chatID)
	if err != nil {
		return err
	}

	if settings.DashboardMessageID == nil {
		message, _, err := s.ops.CreateDashboard(ctx, userID, chatID, homeDashboardState())
		if err != nil {
			return err
		}
		if err := s.deleteMessagesReliably(ctx, traceID, "start_command", chatID, 0, startMessageID); err != nil {
			return err
		}
		s.logger.InfoContext(ctx, "start_dashboard_created", observability.LogAttrs(ctx,
			"user_id", userID,
			"chat_id", chatID,
			"start_message_id", startMessageID,
			"state_to", dashboardViewHome,
			"dashboard_message_id", message.MessageID,
		)...)
		return nil
	}

	if err := s.tg.PinMessage(ctx, chatID, *settings.DashboardMessageID); err != nil {
		if telegram.IsMissingMessageTargetError(err) {
			if err := s.promptDashboardRecovery(ctx, chatID); err != nil {
				return err
			}
			s.logger.InfoContext(ctx, "start_dashboard_missing_prompted", observability.LogAttrs(ctx,
				"user_id", userID,
				"chat_id", chatID,
				"start_message_id", startMessageID,
				"dashboard_message_id", *settings.DashboardMessageID,
			)...)
		} else {
			s.logger.WarnContext(ctx, "start_dashboard_repin_failed", observability.LogAttrs(ctx,
				"user_id", userID,
				"chat_id", chatID,
				"start_message_id", startMessageID,
				"dashboard_message_id", *settings.DashboardMessageID,
				"error", err,
			)...)
		}
	} else {
		s.logger.InfoContext(ctx, "start_dashboard_repinned", observability.LogAttrs(ctx,
			"user_id", userID,
			"chat_id", chatID,
			"start_message_id", startMessageID,
			"dashboard_message_id", *settings.DashboardMessageID,
		)...)
	}

	return s.deleteMessagesReliably(ctx, traceID, "start_command", chatID, 0, startMessageID)
}
