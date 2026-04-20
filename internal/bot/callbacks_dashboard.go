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
	now, err := s.currentNow(ctx)
	if err != nil {
		return err
	}
	switch {
	case len(parts) >= 2 && parts[1] == "home":
		s.logger.InfoContext(ctx, "dashboard_view_opened", observability.LogAttrs(ctx, "view", "home")...)
		return s.RefreshDashboardHome(ctx, callback.From.ID, callback.Message.Chat.ID)
	case len(parts) >= 2 && parts[1] == "list":
		products, err := s.store.ListVisibleProducts(ctx, callback.From.ID, "active", now)
		if err != nil {
			return err
		}
		text, markup, err := s.ui.DashboardList(products, "active")
		if err != nil {
			return err
		}
		s.logger.InfoContext(ctx, "dashboard_view_opened", observability.LogAttrs(ctx, "view", "list", "product_count", len(products))...)
		return s.editDashboardMessage(ctx, callback.Message.Chat.ID, callback.Message.MessageID, text, markup)
	case len(parts) >= 2 && parts[1] == "soon":
		products, err := s.store.ListVisibleProducts(ctx, callback.From.ID, "soon", now)
		if err != nil {
			return err
		}
		text, markup, err := s.ui.DashboardList(products, "soon")
		if err != nil {
			return err
		}
		s.logger.InfoContext(ctx, "dashboard_view_opened", observability.LogAttrs(ctx, "view", "soon", "product_count", len(products))...)
		return s.editDashboardMessage(ctx, callback.Message.Chat.ID, callback.Message.MessageID, text, markup)
	case len(parts) >= 2 && parts[1] == "stats":
		stats, err := s.store.DashboardStats(ctx, callback.From.ID, now)
		if err != nil {
			return err
		}
		text, markup, err := s.ui.DashboardStats(stats)
		if err != nil {
			return err
		}
		s.logger.InfoContext(ctx, "dashboard_view_opened", observability.LogAttrs(ctx,
			"view", "stats",
			"active_count", stats.ActiveCount,
			"soon_count", stats.SoonCount,
			"expired_count", stats.ExpiredCount,
		)...)
		return s.editDashboardMessage(ctx, callback.Message.Chat.ID, callback.Message.MessageID, text, markup)
	case len(parts) >= 2 && parts[1] == "settings":
		settings, err := s.store.GetUserSettings(ctx, callback.From.ID)
		if err != nil {
			return err
		}
		text, markup, err := s.ui.SettingsCard(settings.Timezone, settings.DigestLocalTime)
		if err != nil {
			return err
		}
		s.logger.InfoContext(ctx, "dashboard_view_opened", observability.LogAttrs(ctx,
			"view", "settings",
			"timezone", settings.Timezone,
			"digest_local_time", settings.DigestLocalTime,
		)...)
		return s.editDashboardMessage(ctx, callback.Message.Chat.ID, callback.Message.MessageID, text, markup)
	default:
		return nil
	}
}
