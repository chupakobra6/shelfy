package bot

import (
	"context"
	"strconv"
	"strings"

	"github.com/igor/shelfy/internal/domain"
	"github.com/igor/shelfy/internal/telegram"
)

const dashboardListPageSize = 8

type dashboardListOrigin struct {
	mode string
	page int
}

func parseDashboardPage(parts []string) int {
	if len(parts) < 4 || parts[2] != "page" {
		return 0
	}
	page, err := strconv.Atoi(parts[3])
	if err != nil || page < 0 {
		return 0
	}
	return page
}

func normalizeDashboardListMode(mode string) string {
	switch strings.TrimSpace(mode) {
	case "soon":
		return "soon"
	default:
		return "list"
	}
}

func parseProductOrigin(parts []string) dashboardListOrigin {
	if len(parts) < 5 {
		return dashboardListOrigin{mode: "list", page: 0}
	}
	origin := dashboardListOrigin{
		mode: normalizeDashboardListMode(parts[3]),
		page: 0,
	}
	page, err := strconv.Atoi(parts[4])
	if err == nil && page >= 0 {
		origin.page = page
	}
	return origin
}

func clampDashboardPage(page, totalCount, pageSize int) int {
	if page < 0 {
		return 0
	}
	if totalCount <= 0 || pageSize <= 0 {
		return 0
	}
	lastPage := (totalCount - 1) / pageSize
	if page > lastPage {
		return lastPage
	}
	return page
}

func storeModeForOrigin(origin dashboardListOrigin) string {
	if origin.mode == "soon" {
		return "soon"
	}
	return "active"
}

func (s *Service) renderDashboardListPage(ctx context.Context, userID int64, mode string, page int) (string, *telegram.InlineKeyboardMarkup, int, int, error) {
	now, err := s.currentNow(ctx)
	if err != nil {
		return "", nil, 0, 0, err
	}
	storeMode := "active"
	if mode == "soon" {
		storeMode = "soon"
	}
	offset := page * dashboardListPageSize
	products, totalCount, err := s.store.ListVisibleProductsPage(ctx, userID, storeMode, now, dashboardListPageSize, offset)
	if err != nil {
		return "", nil, 0, 0, err
	}
	clampedPage := clampDashboardPage(page, totalCount, dashboardListPageSize)
	if clampedPage != page {
		offset = clampedPage * dashboardListPageSize
		products, totalCount, err = s.store.ListVisibleProductsPage(ctx, userID, storeMode, now, dashboardListPageSize, offset)
		if err != nil {
			return "", nil, 0, 0, err
		}
	}
	text, markup, err := s.ui.DashboardList(products, mode, clampedPage, totalCount, dashboardListPageSize)
	if err != nil {
		return "", nil, 0, 0, err
	}
	return text, markup, clampedPage, totalCount, nil
}

func dashboardPageCount(totalCount, pageSize int) int {
	if totalCount <= 0 || pageSize <= 0 {
		return 0
	}
	return (totalCount-1)/pageSize + 1
}

func sliceProductNames(products []domain.Product) []string {
	names := make([]string, 0, len(products))
	for _, product := range products {
		names = append(names, product.Name)
	}
	return names
}
