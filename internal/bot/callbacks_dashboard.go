package bot

import (
	"context"

	"github.com/igor/shelfy/internal/observability"
	"github.com/igor/shelfy/internal/telegram"
)

func (s *Service) handleDashboardCallback(ctx context.Context, callback telegram.CallbackQuery, parts []string) error {
	if callback.Message == nil {
		return nil
	}
	var nextState dashboardState
	trigger := callback.Data
	switch {
	case len(parts) >= 2 && parts[1] == "home":
		nextState = homeDashboardState()
	case len(parts) >= 2 && parts[1] == "list":
		nextState = listDashboardState(parseDashboardPage(parts))
	case len(parts) >= 2 && parts[1] == "soon":
		nextState = soonDashboardState(parseDashboardPage(parts))
	case len(parts) >= 2 && parts[1] == "stats":
		nextState = statsDashboardState()
	case len(parts) >= 2 && parts[1] == "settings":
		nextState = settingsDashboardState()
	default:
		return nil
	}
	effectiveState, err := s.ops.ApplyDashboard(ctx, callback.From.ID, callback.Message.Chat.ID, callback.Message.MessageID, nextState)
	if err != nil {
		return err
	}
	s.logger.InfoContext(ctx, "dashboard_transition_applied", observability.LogAttrs(ctx,
		"trigger", trigger,
		"state_to", effectiveState.View,
		"page_to", effectiveState.Page,
	)...)
	return nil
}
