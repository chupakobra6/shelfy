package ui

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	copycat "github.com/igor/shelfy/internal/copy"
	"github.com/igor/shelfy/internal/domain"
)

func loadDraftTestRenderer(t *testing.T) *Renderer {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	loader, err := copycat.Load(filepath.Join(filepath.Dir(filename), "..", "..", "assets", "copy", "runtime.ru.yaml"))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	return New(loader)
}

func TestDraftCardRendersPendingAIReviewLine(t *testing.T) {
	renderer := loadDraftTestRenderer(t)
	text, _, err := renderer.DraftCard(domain.DraftSession{
		SourceKind: domain.MessageKindText,
		DraftName:  "молоко",
		DraftPayload: map[string]any{
			domain.DraftPayloadKeyAIReviewStatus: domain.AIReviewStatusPending,
		},
	})
	if err != nil {
		t.Fatalf("DraftCard() error = %v", err)
	}
	if !strings.Contains(text, "Умное распознавание: уточняю карточку.") {
		t.Fatalf("draft card is missing pending ai review line: %q", text)
	}
}

func TestDraftCardOmitsAIReviewLineWhenStatusEmpty(t *testing.T) {
	renderer := loadDraftTestRenderer(t)
	text, _, err := renderer.DraftCard(domain.DraftSession{
		SourceKind: domain.MessageKindText,
		DraftName:  "молоко",
	})
	if err != nil {
		t.Fatalf("DraftCard() error = %v", err)
	}
	if strings.Contains(text, "Умное распознавание") {
		t.Fatalf("draft card unexpectedly contains ai review line: %q", text)
	}
}

func TestDraftCardRendersSingleCloseAction(t *testing.T) {
	renderer := loadDraftTestRenderer(t)
	_, markup, err := renderer.DraftCard(domain.DraftSession{
		ID:         42,
		SourceKind: domain.MessageKindText,
		DraftName:  "молоко",
	})
	if err != nil {
		t.Fatalf("DraftCard() error = %v", err)
	}
	if markup == nil {
		t.Fatal("DraftCard() returned nil markup")
	}
	if len(markup.InlineKeyboard) != 2 {
		t.Fatalf("keyboard rows = %d, want 2", len(markup.InlineKeyboard))
	}
	lastRow := markup.InlineKeyboard[len(markup.InlineKeyboard)-1]
	if len(lastRow) != 2 {
		t.Fatalf("last row buttons = %d, want 2", len(lastRow))
	}
	if lastRow[0].CallbackData != "draft:edit_date:42" {
		t.Fatalf("last row first callback = %q, want draft:edit_date:42", lastRow[0].CallbackData)
	}
	if lastRow[1].CallbackData != "draft:cancel:42" {
		t.Fatalf("last row second callback = %q, want draft:cancel:42", lastRow[1].CallbackData)
	}
	for _, row := range markup.InlineKeyboard {
		for _, button := range row {
			if button.CallbackData == "draft:delete:42" {
				t.Fatal("draft card still contains delete action")
			}
		}
	}
}
