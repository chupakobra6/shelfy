package bot

import (
	"context"
	"strconv"

	"github.com/igor/shelfy/internal/domain"
	"github.com/igor/shelfy/internal/observability"
	"github.com/igor/shelfy/internal/telegram"
)

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
		origin := parseProductOrigin(parts)
		nextState := productDashboardState(productID, origin)
		effectiveState, err := s.ops.ApplyDashboard(ctx, callback.From.ID, callback.Message.Chat.ID, callback.Message.MessageID, nextState)
		if err != nil {
			return err
		}
		s.logger.InfoContext(ctx, "dashboard_product_opened", observability.LogAttrs(ctx,
			"product_id", productID,
			"origin_mode", origin.mode,
			"origin_page", origin.page,
			"state_to", effectiveState.View,
			"page_to", effectiveState.Page,
		)...)
		return nil
	case "set":
		if len(parts) < 4 {
			return nil
		}
		status := domain.ProductStatus(parts[3])
		origin := parseProductOrigin(parts[1:])
		if err := s.store.UpdateProductStatus(ctx, productID, status); err != nil {
			return err
		}
		nextState := dashboardStateForMode(origin.mode, origin.page)
		effectiveState, err := s.ops.ApplyDashboard(ctx, callback.From.ID, callback.Message.Chat.ID, callback.Message.MessageID, nextState)
		if err != nil {
			return err
		}
		s.logger.InfoContext(ctx, "product_status_changed", observability.LogAttrs(ctx,
			"product_id", productID,
			"status", status,
			"origin_mode", origin.mode,
			"origin_page", origin.page,
			"state_to", effectiveState.View,
			"page_to", effectiveState.Page,
		)...)
		return nil
	default:
		return nil
	}
}
