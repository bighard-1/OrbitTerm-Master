package config

import "testing"

func TestLoadAdminAutoUnbanConfig(t *testing.T) {
	t.Setenv("ADMIN_AUTO_UNBAN_ENABLED", "false")
	t.Setenv("ADMIN_AUTO_UNBAN_INTERVAL_MINUTES", "7")
	t.Setenv("ADMIN_AUTO_UNBAN_BATCH_LIMIT", "222")

	cfg := Load()
	if cfg.AdminAutoUnbanEnabled {
		t.Fatal("expected auto unban disabled")
	}
	if cfg.AdminAutoUnbanIntervalMinutes != 7 || cfg.AdminAutoUnbanBatchLimit != 222 {
		t.Fatalf("unexpected auto unban config: %+v", cfg)
	}
}
