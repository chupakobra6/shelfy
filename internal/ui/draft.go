package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/igor/shelfy/internal/domain"
	"github.com/igor/shelfy/internal/telegram"
)

func (r *Renderer) DraftCard(draft domain.DraftSession) (string, *telegram.InlineKeyboardMarkup, error) {
	name := draft.DraftName
	if strings.TrimSpace(name) == "" {
		var err error
		name, err = r.copy.Label("draft.value.missing")
		if err != nil {
			return "", nil, err
		}
	}
	missing, err := r.missingFieldsText(draft)
	if err != nil {
		return "", nil, err
	}
	sourceLabel, err := r.copy.Label("source_kind." + string(draft.SourceKind))
	if err != nil {
		return "", nil, err
	}
	text, err := r.copy.Render("draft.card", map[string]any{
		"name":           escapeHTML(name),
		"expires_on":     r.formatOptionalDate(draft.DraftExpiresOn),
		"missing_fields": missing,
		"source_kind":    sourceLabel,
	})
	if err != nil {
		return "", nil, err
	}
	confirmLabel, err := r.copy.Label("draft.button.confirm")
	if err != nil {
		return "", nil, err
	}
	editNameLabel, err := r.copy.Label("draft.button.edit_name")
	if err != nil {
		return "", nil, err
	}
	editDateLabel, err := r.copy.Label("draft.button.edit_date")
	if err != nil {
		return "", nil, err
	}
	deleteLabel, err := r.copy.Label("draft.button.delete")
	if err != nil {
		return "", nil, err
	}
	cancelLabel, err := r.copy.Label("draft.button.cancel")
	if err != nil {
		return "", nil, err
	}
	return text, &telegram.InlineKeyboardMarkup{
		InlineKeyboard: [][]telegram.InlineKeyboardButton{
			{
				{Text: confirmLabel, CallbackData: fmt.Sprintf("draft:confirm:%d", draft.ID)},
				{Text: editNameLabel, CallbackData: fmt.Sprintf("draft:edit_name:%d", draft.ID)},
			},
			{
				{Text: editDateLabel, CallbackData: fmt.Sprintf("draft:edit_date:%d", draft.ID)},
				{Text: deleteLabel, CallbackData: fmt.Sprintf("draft:delete:%d", draft.ID)},
			},
			{{Text: cancelLabel, CallbackData: fmt.Sprintf("draft:cancel:%d", draft.ID)}},
		},
	}, nil
}

func (r *Renderer) ProcessingMessage() (string, error) {
	return r.copy.Render("ingest.processing", nil)
}

func (r *Renderer) UnsupportedMessage() (string, error) {
	return r.copy.Render("ingest.unsupported", nil)
}

func (r *Renderer) DraftConfirmed(name string, expiresOn time.Time) (string, error) {
	return r.copy.Render("draft.confirmed", map[string]any{
		"name":       escapeHTML(name),
		"expires_on": expiresOn.Format("2006-01-02"),
	})
}

func (r *Renderer) DraftCanceled() (string, error) {
	return r.copy.Render("draft.canceled", nil)
}

func (r *Renderer) DraftIncomplete() (string, error) {
	return r.copy.Render("draft.incomplete", nil)
}

func (r *Renderer) DashboardStale() (string, error) {
	return r.copy.Render("callback.dashboard_stale", nil)
}

func (r *Renderer) DashboardRecoverHint() (string, error) {
	return r.copy.Render("command.dashboard_recover_hint", nil)
}

func (r *Renderer) DraftAlreadyProcessed() (string, error) {
	return r.copy.Render("callback.draft_already_processed", nil)
}

func (r *Renderer) IngestFailed() (string, error) {
	return r.copy.Render("ingest.failed", nil)
}

func (r *Renderer) DraftEditNamePrompt() (string, error) {
	return r.copy.Render("draft.edit_name_prompt", nil)
}

func (r *Renderer) DraftEditDatePrompt() (string, error) {
	return r.copy.Render("draft.edit_date_prompt", nil)
}

func (r *Renderer) DraftEditDateInvalid() (string, error) {
	return r.copy.Render("draft.edit_date_invalid", nil)
}

func (r *Renderer) missingFieldsText(draft domain.DraftSession) (string, error) {
	switch {
	case strings.TrimSpace(draft.DraftName) == "" && draft.DraftExpiresOn == nil:
		return r.copy.Render("draft.missing.both", nil)
	case strings.TrimSpace(draft.DraftName) == "":
		return r.copy.Render("draft.missing.name", nil)
	case draft.DraftExpiresOn == nil:
		return r.copy.Render("draft.missing.date", nil)
	default:
		return r.copy.Render("draft.missing.none", nil)
	}
}

func (r *Renderer) formatOptionalDate(value *time.Time) string {
	if value == nil {
		label, err := r.copy.Label("draft.value.missing")
		if err == nil {
			return label
		}
		return ""
	}
	return value.Format("2006-01-02")
}
