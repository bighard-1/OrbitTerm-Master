package service

import (
	"testing"
	"time"

	"orbitterm-server/internal/config"
)

func TestSystemHealthReportsDegradedWhenDatabaseMissing(t *testing.T) {
	startedAt := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	svc := NewSystemHealthService(nil, config.Config{}, startedAt).(*systemHealthService)
	svc.now = func() time.Time { return startedAt.Add(2 * time.Minute) }

	report := svc.PublicHealth()
	if report.Status != "degraded" {
		t.Fatalf("expected degraded status, got %q", report.Status)
	}
	if report.Database.Reachable {
		t.Fatal("expected database to be unreachable")
	}
	if report.UptimeSeconds != 120 {
		t.Fatalf("unexpected uptime: %d", report.UptimeSeconds)
	}
}

func TestRuntimeStatusNormalizesAutoUnbanConfig(t *testing.T) {
	cfg := config.Config{
		JWTSecret:                     "replace-this-with-a-strong-secret",
		JWTIssuer:                     "orbitterm-test",
		JWTAccessExpireMinutes:        15,
		JWTRefreshExpireDays:          30,
		AdminAutoUnbanEnabled:         true,
		AdminAutoUnbanIntervalMinutes: 0,
		AdminAutoUnbanBatchLimit:      999,
	}
	svc := NewSystemHealthService(nil, cfg, time.Now().UTC())

	runtime := svc.RuntimeStatus()
	if runtime.AutoUnban.EffectiveIntervalMinutes != 10 {
		t.Fatalf("unexpected interval: %d", runtime.AutoUnban.EffectiveIntervalMinutes)
	}
	if runtime.AutoUnban.EffectiveBatchLimit != 100 {
		t.Fatalf("unexpected batch limit: %d", runtime.AutoUnban.EffectiveBatchLimit)
	}
	if runtime.JWT.SecretStrengthStatus != "weak_or_default" {
		t.Fatalf("unexpected jwt strength: %s", runtime.JWT.SecretStrengthStatus)
	}
}
