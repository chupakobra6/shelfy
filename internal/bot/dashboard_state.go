package bot

import (
	"context"

	"github.com/igor/shelfy/internal/telegram"
)

type dashboardView string

const (
	dashboardViewHome     dashboardView = "home"
	dashboardViewList     dashboardView = "list"
	dashboardViewSoon     dashboardView = "soon"
	dashboardViewStats    dashboardView = "stats"
	dashboardViewSettings dashboardView = "settings"
	dashboardViewProduct  dashboardView = "product"
)

type dashboardState struct {
	View      dashboardView
	Page      int
	ProductID *int64
	Origin    dashboardListOrigin
}

func homeDashboardState() dashboardState {
	return normalizeDashboardState(dashboardState{View: dashboardViewHome})
}

func listDashboardState(page int) dashboardState {
	return normalizeDashboardState(dashboardState{View: dashboardViewList, Page: page})
}

func soonDashboardState(page int) dashboardState {
	return normalizeDashboardState(dashboardState{View: dashboardViewSoon, Page: page})
}

func statsDashboardState() dashboardState {
	return normalizeDashboardState(dashboardState{View: dashboardViewStats})
}

func settingsDashboardState() dashboardState {
	return normalizeDashboardState(dashboardState{View: dashboardViewSettings})
}

func productDashboardState(productID int64, origin dashboardListOrigin) dashboardState {
	return normalizeDashboardState(dashboardState{
		View:      dashboardViewProduct,
		ProductID: &productID,
		Origin: dashboardListOrigin{
			mode: normalizeDashboardListMode(origin.mode),
			page: origin.page,
		},
	})
}

func normalizeDashboardState(state dashboardState) dashboardState {
	if state.View == "" {
		state.View = dashboardViewHome
	}
	if state.Page < 0 {
		state.Page = 0
	}
	state.Origin.mode = normalizeDashboardListMode(state.Origin.mode)
	if state.Origin.page < 0 {
		state.Origin.page = 0
	}
	if state.View != dashboardViewProduct {
		state.ProductID = nil
		state.Origin = dashboardListOrigin{mode: "list", page: 0}
	}
	return state
}

func dashboardStateForMode(mode string, page int) dashboardState {
	if normalizeDashboardListMode(mode) == "soon" {
		return soonDashboardState(page)
	}
	return listDashboardState(page)
}

func (s *Service) renderDashboardState(ctx context.Context, userID int64, state dashboardState) (string, *telegram.InlineKeyboardMarkup, dashboardState, error) {
	state = normalizeDashboardState(state)
	now, err := s.currentNow(ctx)
	if err != nil {
		return "", nil, homeDashboardState(), err
	}

	switch state.View {
	case dashboardViewHome:
		stats, err := s.store.DashboardStats(ctx, userID, now)
		if err != nil {
			return "", nil, homeDashboardState(), err
		}
		text, markup, err := s.ui.DashboardHome(stats)
		return text, markup, homeDashboardState(), err
	case dashboardViewList:
		text, markup, page, _, err := s.renderDashboardListPage(ctx, userID, "list", state.Page)
		effective := listDashboardState(page)
		return text, markup, effective, err
	case dashboardViewSoon:
		text, markup, page, _, err := s.renderDashboardListPage(ctx, userID, "soon", state.Page)
		effective := soonDashboardState(page)
		return text, markup, effective, err
	case dashboardViewStats:
		stats, err := s.store.DashboardStats(ctx, userID, now)
		if err != nil {
			return "", nil, statsDashboardState(), err
		}
		text, markup, err := s.ui.DashboardStats(stats)
		return text, markup, statsDashboardState(), err
	case dashboardViewSettings:
		settings, err := s.store.GetUserSettings(ctx, userID)
		if err != nil {
			return "", nil, settingsDashboardState(), err
		}
		text, markup, err := s.ui.SettingsCard(settings.Timezone, settings.DigestLocalTime)
		return text, markup, settingsDashboardState(), err
	case dashboardViewProduct:
		if state.ProductID == nil {
			return s.renderDashboardState(ctx, userID, homeDashboardState())
		}
		product, err := s.store.GetProduct(ctx, *state.ProductID)
		if err != nil {
			return s.renderDashboardState(ctx, userID, dashboardStateForMode(state.Origin.mode, state.Origin.page))
		}
		text, markup, err := s.ui.ProductCard(product, state.Origin.mode, state.Origin.page)
		return text, markup, productDashboardState(product.ID, state.Origin), err
	default:
		return s.renderDashboardState(ctx, userID, homeDashboardState())
	}
}
