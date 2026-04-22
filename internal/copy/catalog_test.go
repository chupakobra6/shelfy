package copy

import (
	"path/filepath"
	"testing"
)

func TestLoadAndRenderCatalog(t *testing.T) {
	loader, err := Load(filepath.Join("..", "..", "assets", "copy", "runtime.ru.yaml"))
	if err != nil {
		t.Fatalf("load catalog: %v", err)
	}
	text, err := loader.Render("dashboard.home", map[string]any{
		"active_count":  3,
		"soon_count":    1,
		"expired_count": 2,
	})
	if err != nil {
		t.Fatalf("render catalog: %v", err)
	}
	if text == "" {
		t.Fatalf("expected rendered text")
	}
	label, err := loader.Label("dashboard.button.list")
	if err != nil {
		t.Fatalf("load label: %v", err)
	}
	if label == "" {
		t.Fatalf("expected non-empty label")
	}
}
