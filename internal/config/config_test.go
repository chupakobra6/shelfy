package config

import "testing"

func TestLoadParsesE2ETestUserID(t *testing.T) {
	t.Setenv("SHELFY_BOT_TOKEN", "token")
	t.Setenv("SHELFY_DATABASE_URL", "postgres://example")
	t.Setenv("SHELFY_E2E_TEST_USER_ID", "8031865593")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.E2ETestUserID != 8031865593 {
		t.Fatalf("E2ETestUserID = %d, want 8031865593", cfg.E2ETestUserID)
	}
}
