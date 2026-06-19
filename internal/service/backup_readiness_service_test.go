package service

import (
	"strings"
	"testing"

	"orbitterm-server/internal/config"
)

func TestBackupReadinessRejectsMissingAdmin(t *testing.T) {
	svc := NewBackupReadinessService(nil, config.Config{}, &fakeAdminAuditService{})

	_, err := svc.GetReadiness(0, AdminRequestMeta{})
	if err == nil {
		t.Fatal("expected error for missing admin id")
	}
}

func TestMaskSecretDoesNotLeakFullValue(t *testing.T) {
	masked := maskSecret("abcdefghijklmnopqrstuvwxyz123456")
	if masked == "abcdefghijklmnopqrstuvwxyz123456" {
		t.Fatal("secret was not masked")
	}
	if !strings.HasPrefix(masked, "abcd") || !strings.HasSuffix(masked, "3456") {
		t.Fatalf("unexpected masked secret shape: %q", masked)
	}
	if strings.Contains(masked, "efghijklmnop") {
		t.Fatalf("masked secret leaks middle content: %q", masked)
	}
}

func TestMaskDatabaseURLMasksKeywordPassword(t *testing.T) {
	masked := maskDatabaseURL("host=orbit-db user=orbitterm password=super-secret dbname=orbitterm")
	if strings.Contains(masked, "super-secret") {
		t.Fatalf("database password leaked: %q", masked)
	}
	if !strings.Contains(masked, "password=******") {
		t.Fatalf("expected masked keyword password, got: %q", masked)
	}
}

func TestMaskDatabaseURLMasksURLPassword(t *testing.T) {
	masked := maskDatabaseURL("postgres://orbitterm:super-secret@db:5432/orbitterm?sslmode=disable")
	if strings.Contains(masked, "super-secret") {
		t.Fatalf("database URL password leaked: %q", masked)
	}
	if !strings.Contains(masked, "orbitterm:%2A%2A%2A%2A%2A%2A@") {
		t.Fatalf("expected masked URL password, got: %q", masked)
	}
}

func TestEnvironmentChecksFlagInsecureDefaults(t *testing.T) {
	svc := &backupReadinessService{cfg: config.Config{
		JWTSecret:              "replace-this-with-a-strong-secret",
		JWTIssuer:              "orbitterm-server",
		JWTAccessExpireMinutes: 15,
		JWTRefreshExpireDays:   30,
		DatabaseURL:            "host=orbit-db user=orbitterm password=orbitterm_pass dbname=orbitterm",
	}}

	checks := svc.environmentChecks()
	byKey := make(map[string]EnvironmentCheck, len(checks))
	for _, check := range checks {
		byKey[check.Key] = check
	}
	if byKey["JWT_SECRET"].Secure {
		t.Fatalf("default JWT secret must be marked insecure: %+v", byKey["JWT_SECRET"])
	}
	if byKey["DATABASE_URL"].Secure {
		t.Fatalf("default database password must be marked insecure: %+v", byKey["DATABASE_URL"])
	}
	if byKey["ADMIN_BOOTSTRAP_TOKEN"].Severity != "info" || !byKey["ADMIN_BOOTSTRAP_TOKEN"].Secure {
		t.Fatalf("empty bootstrap token should be informational after initialization: %+v", byKey["ADMIN_BOOTSTRAP_TOKEN"])
	}
}
