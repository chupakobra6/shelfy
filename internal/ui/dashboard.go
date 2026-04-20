package ui

import (
	"fmt"
	"strings"

	"github.com/igor/shelfy/internal/domain"
	"github.com/igor/shelfy/internal/telegram"
)

func (r *Renderer) DashboardHome(stats domain.DashboardStats) (string, *telegram.InlineKeyboardMarkup, error) {
	text, err := r.copy.Render("dashboard.home", map[string]any{
		"active_count":  stats.ActiveCount,
		"soon_count":    stats.SoonCount,
		"expired_count": stats.ExpiredCount,
	})
	if err != nil {
		return "", nil, err
	}
	listLabel, err := r.copy.Label("dashboard.button.list")
	if err != nil {
		return "", nil, err
	}
	soonLabel, err := r.copy.Label("dashboard.button.soon")
	if err != nil {
		return "", nil, err
	}
	statsLabel, err := r.copy.Label("dashboard.button.stats")
	if err != nil {
		return "", nil, err
	}
	settingsLabel, err := r.copy.Label("dashboard.button.settings")
	if err != nil {
		return "", nil, err
	}
	return text, &telegram.InlineKeyboardMarkup{
		InlineKeyboard: [][]telegram.InlineKeyboardButton{
			{{Text: listLabel, CallbackData: "dashboard:list"}, {Text: soonLabel, CallbackData: "dashboard:soon"}},
			{{Text: statsLabel, CallbackData: "dashboard:stats"}, {Text: settingsLabel, CallbackData: "dashboard:settings"}},
		},
	}, nil
}

func (r *Renderer) DashboardList(products []domain.Product, mode string) (string, *telegram.InlineKeyboardMarkup, error) {
	if len(products) == 0 {
		text, err := r.copy.Render("dashboard.list_empty", nil)
		if err != nil {
			return "", nil, err
		}
		keyboard, err := r.dashboardBackKeyboard()
		return text, keyboard, err
	}
	titleMessageID := "dashboard.list.title.active"
	if mode == "soon" {
		titleMessageID = "dashboard.list.title.soon"
	}
	title, err := r.copy.Render(titleMessageID, nil)
	if err != nil {
		return "", nil, err
	}
	header, err := r.copy.Render("dashboard.list.header", map[string]any{"title": title})
	if err != nil {
		return "", nil, err
	}
	openPrefix, err := r.copy.Label("product.button.open_prefix")
	if err != nil {
		return "", nil, err
	}
	backLabel, err := r.copy.Label("dashboard.button.back")
	if err != nil {
		return "", nil, err
	}
	lines := []string{header, ""}
	keyboard := make([][]telegram.InlineKeyboardButton, 0, len(products)+1)
	for _, product := range products {
		line, err := r.copy.Render("dashboard.list.item", map[string]any{
			"name":       escapeHTML(product.Name),
			"expires_on": product.ExpiresOn.Format("2006-01-02"),
		})
		if err != nil {
			return "", nil, err
		}
		lines = append(lines, line)
		keyboard = append(keyboard, []telegram.InlineKeyboardButton{
			{Text: openPrefix + short(product.Name, 18), CallbackData: fmt.Sprintf("product:open:%d", product.ID)},
		})
	}
	keyboard = append(keyboard, []telegram.InlineKeyboardButton{{Text: backLabel, CallbackData: "dashboard:home"}})
	return strings.Join(lines, "\n"), &telegram.InlineKeyboardMarkup{InlineKeyboard: keyboard}, nil
}

func (r *Renderer) DashboardStats(stats domain.DashboardStats) (string, *telegram.InlineKeyboardMarkup, error) {
	text, err := r.copy.Render("dashboard.stats", map[string]any{
		"active_count":    stats.ActiveCount,
		"soon_count":      stats.SoonCount,
		"expired_count":   stats.ExpiredCount,
		"consumed_count":  stats.ConsumedCount,
		"discarded_count": stats.DiscardedCount,
		"deleted_count":   stats.DeletedCount,
	})
	if err != nil {
		return "", nil, err
	}
	keyboard, err := r.dashboardBackKeyboard()
	return text, keyboard, err
}

func (r *Renderer) dashboardBackKeyboard() (*telegram.InlineKeyboardMarkup, error) {
	backLabel, err := r.copy.Label("dashboard.button.back")
	if err != nil {
		return nil, err
	}
	return &telegram.InlineKeyboardMarkup{
		InlineKeyboard: [][]telegram.InlineKeyboardButton{{{Text: backLabel, CallbackData: "dashboard:home"}}},
	}, nil
}
