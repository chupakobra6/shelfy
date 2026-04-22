package bot

import (
	"context"
	"strings"

	"github.com/igor/shelfy/internal/observability"
	"github.com/igor/shelfy/internal/telegram"
)

func (s *Service) HandleCallback(ctx context.Context, callback telegram.CallbackQuery) error {
	if callback.Message == nil {
		return nil
	}
	ctx = observability.WithUserID(ctx, callback.From.ID)
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

func (s *Service) applyDashboardCallbackState(ctx context.Context, callback telegram.CallbackQuery, nextState dashboardState, event string, attrs ...any) error {
	if callback.Message == nil {
		return nil
	}
	effectiveState, err := s.ops.ApplyDashboard(ctx, callback.From.ID, callback.Message.Chat.ID, callback.Message.MessageID, nextState)
	if err != nil {
		return err
	}
	attrs = append(attrs,
		"state_to", effectiveState.View,
		"page_to", effectiveState.Page,
	)
	s.logger.InfoContext(ctx, event, observability.LogAttrs(ctx, attrs...)...)
	return nil
}
