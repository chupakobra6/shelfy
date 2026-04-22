package bot

import (
	"context"
	"strconv"

	"github.com/igor/shelfy/internal/domain"
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
		return s.applyDashboardCallbackState(ctx, callback, nextState, "dashboard_product_opened",
			"product_id", productID,
			"origin_mode", origin.mode,
			"origin_page", origin.page,
		)
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
		return s.applyDashboardCallbackState(ctx, callback, nextState, "product_status_changed",
			"product_id", productID,
			"status", status,
			"origin_mode", origin.mode,
			"origin_page", origin.page,
		)
	default:
		return nil
	}
}
