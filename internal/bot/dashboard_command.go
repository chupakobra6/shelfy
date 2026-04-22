package bot

import (
	"context"
	"time"

	"github.com/igor/shelfy/internal/observability"
)

const dashboardRecoveryPromptTTL = 20 * time.Second

func (s *Service) HandleDashboardCommand(ctx context.Context, userID, chatID, commandMessageID int64) error {
	traceID := observability.TraceID(observability.EnsureTraceID(ctx))
	settings, err := s.ensureUserSettings(ctx, userID, chatID)
	if err != nil {
		return err
	}

	previousDashboardMessageID := ptrValue(settings.DashboardMessageID)
	message, err := s.ops.CreateDashboard(ctx, userID, chatID, homeDashboardState())
	if err != nil {
		return err
	}
	if previousDashboardMessageID != 0 && previousDashboardMessageID != message.MessageID {
		if err := s.deleteMessagesReliably(ctx, traceID, "dashboard_replaced", chatID, 0, previousDashboardMessageID); err != nil {
			return err
		}
	}
	if err := s.deleteMessagesReliably(ctx, traceID, "dashboard_command", chatID, 0, commandMessageID); err != nil {
		return err
	}
	event := "dashboard_command_created"
	if previousDashboardMessageID != 0 {
		event = "dashboard_command_recreated_explicit"
	}
	s.logger.InfoContext(ctx, event, observability.LogAttrs(ctx,
		"user_id", userID,
		"chat_id", chatID,
		"command_message_id", commandMessageID,
		"state_to", dashboardViewHome,
		"previous_dashboard_message_id", previousDashboardMessageID,
		"dashboard_message_id", message.MessageID,
	)...)
	return nil
}

func (s *Service) promptDashboardRecovery(ctx context.Context, chatID int64) error {
	text, err := s.ui.DashboardRecoverHint()
	if err != nil {
		return err
	}
	return s.ops.SendTransientFeedback(ctx, chatID, text, dashboardRecoveryPromptTTL)
}
