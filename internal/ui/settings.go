package ui

import (
	"fmt"

	"github.com/igor/shelfy/internal/telegram"
)

func (r *Renderer) SettingsCard(timezone, digestTime string) (string, *telegram.InlineKeyboardMarkup, error) {
	text, err := r.copy.Render("dashboard.settings", map[string]any{
		"timezone":    escapeHTML(timezone),
		"digest_time": escapeHTML(digestTime),
	})
	if err != nil {
		return "", nil, err
	}
	digestEarlierLabel, err := r.copy.Label("settings.button.digest_earlier")
	if err != nil {
		return "", nil, err
	}
	digestLaterLabel, err := r.copy.Label("settings.button.digest_later")
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
				{Text: digestEarlierLabel, CallbackData: "settings:digest:-60"},
				{Text: digestLaterLabel, CallbackData: "settings:digest:60"},
			},
			r.settingsTimezoneRow(timezone, []timezoneButton{
				{LabelID: "settings.button.timezone.moscow", Value: "Europe/Moscow"},
				{LabelID: "settings.button.timezone.utc", Value: "UTC"},
			}),
			r.settingsTimezoneRow(timezone, []timezoneButton{
				{LabelID: "settings.button.timezone.berlin", Value: "Europe/Berlin"},
				{LabelID: "settings.button.timezone.new_york", Value: "America/New_York"},
			}),
			{{Text: backLabel, CallbackData: "dashboard:home"}},
		},
	}, nil
}

type timezoneButton struct {
	LabelID string
	Value   string
}

func (r *Renderer) settingsTimezoneRow(current string, buttons []timezoneButton) []telegram.InlineKeyboardButton {
	row := make([]telegram.InlineKeyboardButton, 0, len(buttons))
	for _, item := range buttons {
		label, err := r.copy.Label(item.LabelID)
		if err != nil {
			label = item.Value
		}
		if current == item.Value {
			label = "✅ " + label
		}
		row = append(row, telegram.InlineKeyboardButton{
			Text:         label,
			CallbackData: fmt.Sprintf("settings:timezone:%s", item.Value),
		})
	}
	return row
}
