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
		product, err := s.store.GetProduct(ctx, productID)
		if err != nil {
			return err
		}
		text, markup, err := s.ui.ProductCard(product)
		if err != nil {
			return err
		}
		return s.editDashboardMessage(ctx, callback.Message.Chat.ID, callback.Message.MessageID, text, markup)
	case "set":
		if len(parts) < 4 {
			return nil
		}
		status := domain.ProductStatus(parts[3])
		if err := s.store.UpdateProductStatus(ctx, productID, status); err != nil {
			return err
		}
		return s.RefreshDashboardHome(ctx, callback.From.ID, callback.Message.Chat.ID)
	default:
		return nil
	}
}
