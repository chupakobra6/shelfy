package ui

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	copycat "github.com/igor/shelfy/internal/copy"
	"github.com/igor/shelfy/internal/domain"
)

func newPaginationTestRenderer(t *testing.T) *Renderer {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	loader, err := copycat.Load(filepath.Join(filepath.Dir(filename), "..", "..", "copy", "runtime.ru.yaml"))
	if err != nil {
		t.Fatalf("load runtime copy: %v", err)
	}
	return New(loader)
}

func TestDashboardListFirstPageShowsOnlyNext(t *testing.T) {
	r := newPaginationTestRenderer(t)
	products := []domain.Product{
		{ID: 1, Name: "a", ExpiresOn: time.Date(2026, 4, 22, 0, 0, 0, 0, time.UTC)},
		{ID: 2, Name: "b", ExpiresOn: time.Date(2026, 4, 22, 0, 0, 0, 0, time.UTC)},
		{ID: 3, Name: "c", ExpiresOn: time.Date(2026, 4, 22, 0, 0, 0, 0, time.UTC)},
		{ID: 4, Name: "d", ExpiresOn: time.Date(2026, 4, 22, 0, 0, 0, 0, time.UTC)},
		{ID: 5, Name: "e", ExpiresOn: time.Date(2026, 4, 22, 0, 0, 0, 0, time.UTC)},
		{ID: 6, Name: "f", ExpiresOn: time.Date(2026, 4, 22, 0, 0, 0, 0, time.UTC)},
		{ID: 7, Name: "g", ExpiresOn: time.Date(2026, 4, 22, 0, 0, 0, 0, time.UTC)},
		{ID: 8, Name: "h", ExpiresOn: time.Date(2026, 4, 22, 0, 0, 0, 0, time.UTC)},
	}
	text, markup, err := r.DashboardList(products, "list", 0, 9, 8)
	if err != nil {
		t.Fatalf("DashboardList() error = %v", err)
	}
	if got := len(markup.InlineKeyboard); got != 10 {
		t.Fatalf("keyboard rows = %d, want 10", got)
	}
	navRow := markup.InlineKeyboard[8]
	if len(navRow) != 1 || navRow[0].CallbackData != "dashboard:list:page:1" {
		t.Fatalf("unexpected nav row: %+v", navRow)
	}
	if got := markup.InlineKeyboard[9][0].CallbackData; got != "dashboard:home" {
		t.Fatalf("back callback = %q, want dashboard:home", got)
	}
	if text == "" || !containsAll(text, "Страница <b>1</b> / <b>2</b>", "• <b>a</b> — 2026-04-22") {
		t.Fatalf("unexpected text: %q", text)
	}
}

func TestDashboardListMiddlePageShowsPrevAndNext(t *testing.T) {
	r := newPaginationTestRenderer(t)
	products := []domain.Product{
		{ID: 9, Name: "i", ExpiresOn: time.Date(2026, 4, 22, 0, 0, 0, 0, time.UTC)},
		{ID: 10, Name: "j", ExpiresOn: time.Date(2026, 4, 22, 0, 0, 0, 0, time.UTC)},
		{ID: 11, Name: "k", ExpiresOn: time.Date(2026, 4, 22, 0, 0, 0, 0, time.UTC)},
		{ID: 12, Name: "l", ExpiresOn: time.Date(2026, 4, 22, 0, 0, 0, 0, time.UTC)},
		{ID: 13, Name: "m", ExpiresOn: time.Date(2026, 4, 22, 0, 0, 0, 0, time.UTC)},
		{ID: 14, Name: "n", ExpiresOn: time.Date(2026, 4, 22, 0, 0, 0, 0, time.UTC)},
		{ID: 15, Name: "o", ExpiresOn: time.Date(2026, 4, 22, 0, 0, 0, 0, time.UTC)},
		{ID: 16, Name: "p", ExpiresOn: time.Date(2026, 4, 22, 0, 0, 0, 0, time.UTC)},
	}
	_, markup, err := r.DashboardList(products, "soon", 1, 17, 8)
	if err != nil {
		t.Fatalf("DashboardList() error = %v", err)
	}
	navRow := markup.InlineKeyboard[8]
	if len(navRow) != 2 {
		t.Fatalf("nav row len = %d, want 2", len(navRow))
	}
	if navRow[0].CallbackData != "dashboard:soon" {
		t.Fatalf("prev callback = %q, want dashboard:soon", navRow[0].CallbackData)
	}
	if navRow[1].CallbackData != "dashboard:soon:page:2" {
		t.Fatalf("next callback = %q, want dashboard:soon:page:2", navRow[1].CallbackData)
	}
}

func TestProductCardPreservesOriginCallbacks(t *testing.T) {
	r := newPaginationTestRenderer(t)
	text, markup, err := r.ProductCard(domain.Product{
		ID:        77,
		Name:      "сметана",
		ExpiresOn: time.Date(2026, 4, 22, 0, 0, 0, 0, time.UTC),
		Status:    domain.ProductStatusActive,
	}, "soon", 2)
	if err != nil {
		t.Fatalf("ProductCard() error = %v", err)
	}
	if text == "" {
		t.Fatal("expected product card text")
	}
	if got := markup.InlineKeyboard[0][0].CallbackData; got != "product:set:77:consumed:soon:2" {
		t.Fatalf("consumed callback = %q", got)
	}
	if got := markup.InlineKeyboard[2][0].CallbackData; got != "dashboard:soon:page:2" {
		t.Fatalf("back callback = %q", got)
	}
}

func containsAll(text string, parts ...string) bool {
	for _, part := range parts {
		if !strings.Contains(text, part) {
			return false
		}
	}
	return true
}
