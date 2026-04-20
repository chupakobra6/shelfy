package ui

import (
	"fmt"

	"github.com/igor/shelfy/internal/domain"
	"github.com/igor/shelfy/internal/telegram"
)

func (r *Renderer) ProductCard(product domain.Product) (string, *telegram.InlineKeyboardMarkup, error) {
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
				{Text: consumedLabel, CallbackData: fmt.Sprintf("product:set:%d:consumed", product.ID)},
				{Text: discardedLabel, CallbackData: fmt.Sprintf("product:set:%d:discarded", product.ID)},
			},
			{
				{Text: deletedLabel, CallbackData: fmt.Sprintf("product:set:%d:deleted", product.ID)},
			},
			{{Text: backLabel, CallbackData: "dashboard:list"}},
		},
	}, nil
}
