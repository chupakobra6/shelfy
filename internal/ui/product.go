package ui

import (
	"github.com/igor/shelfy/internal/domain"
	"github.com/igor/shelfy/internal/telegram"
)

func (r *Renderer) ProductCard(product domain.Product, originMode string, originPage int) (string, *telegram.InlineKeyboardMarkup, error) {
	statusLabel, err := r.copy.Label("product.status." + string(product.Status))
	if err != nil {
		return "", nil, err
	}
	text, err := r.copy.Render("product.card", map[string]any{
		"name":       escapeHTML(product.Name),
		"expires_on": product.ExpiresOn.Format("2006-01-02"),
		"status":     statusLabel,
	})
	if err != nil {
		return "", nil, err
	}
	consumedLabel, err := r.copy.Label("product.button.consumed")
	if err != nil {
		return "", nil, err
	}
	discardedLabel, err := r.copy.Label("product.button.discarded")
	if err != nil {
		return "", nil, err
	}
	deletedLabel, err := r.copy.Label("product.button.deleted")
	if err != nil {
		return "", nil, err
	}
	backLabel, err := r.copy.Label("dashboard.button.back")
	if err != nil {
		return "", nil, err
	}
	return text, &telegram.InlineKeyboardMarkup{
		InlineKeyboard: [][]telegram.InlineKeyboardButton{
			{
				{Text: consumedLabel, CallbackData: productSetCallback(product.ID, "consumed", originMode, originPage)},
				{Text: discardedLabel, CallbackData: productSetCallback(product.ID, "discarded", originMode, originPage)},
			},
			{
				{Text: deletedLabel, CallbackData: productSetCallback(product.ID, "deleted", originMode, originPage)},
			},
			{{Text: backLabel, CallbackData: dashboardPageCallback(originMode, originPage)}},
		},
	}, nil
}
