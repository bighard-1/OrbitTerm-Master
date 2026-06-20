package config

import (
	"reflect"
	"testing"

	"gorm.io/gorm/logger"
)

func TestLoadAdminAutoUnbanConfig(t *testing.T) {
	t.Setenv("ADMIN_AUTO_UNBAN_ENABLED", "false")
	t.Setenv("ADMIN_AUTO_UNBAN_INTERVAL_MINUTES", "7")
	t.Setenv("ADMIN_AUTO_UNBAN_BATCH_LIMIT", "222")
	t.Setenv("ASSET_TRASH_CLEANUP_INTERVAL_MINUTES", "45")
	t.Setenv("DB_LOG_LEVEL", "error")
	t.Setenv("TRUSTED_PROXIES", "127.0.0.1, 10.0.0.0/8")

	cfg := Load()
	if cfg.AdminAutoUnbanEnabled {
		t.Fatal("expected auto unban disabled")
	}
	if cfg.AdminAutoUnbanIntervalMinutes != 7 || cfg.AdminAutoUnbanBatchLimit != 222 {
		t.Fatalf("unexpected auto unban config: %+v", cfg)
	}
	if cfg.AssetTrashCleanupIntervalMinutes != 45 {
		t.Fatalf("unexpected asset trash cleanup interval: %d", cfg.AssetTrashCleanupIntervalMinutes)
	}
	if cfg.DatabaseLogLevel != "error" {
		t.Fatalf("unexpected database log level: %s", cfg.DatabaseLogLevel)
	}
	wantProxies := []string{"127.0.0.1", "10.0.0.0/8"}
	if !reflect.DeepEqual(cfg.TrustedProxies, wantProxies) {
		t.Fatalf("unexpected trusted proxies: %#v", cfg.TrustedProxies)
	}
}

func TestDatabaseLogLevelUsesSafeDefault(t *testing.T) {
	for _, test := range []struct {
		input string
		want  logger.LogLevel
	}{
		{input: "silent", want: logger.Silent},
		{input: "error", want: logger.Error},
		{input: "info", want: logger.Info},
		{input: "warn", want: logger.Warn},
		{input: "unknown", want: logger.Warn},
	} {
		if got := databaseLogLevel(test.input); got != test.want {
			t.Fatalf("databaseLogLevel(%q) = %v, want %v", test.input, got, test.want)
		}
	}
}

func TestEmptyTrustedProxiesDisablesProxyTrust(t *testing.T) {
	t.Setenv("TRUSTED_PROXIES", "")
	if got := Load().TrustedProxies; len(got) != 0 {
		t.Fatalf("expected no trusted proxies, got %#v", got)
	}
}
