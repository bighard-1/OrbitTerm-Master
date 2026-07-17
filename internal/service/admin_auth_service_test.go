package service

import (
	"errors"
	"testing"
	"time"

	"orbitterm-server/internal/model"
	"orbitterm-server/internal/utils"
)

func TestAdminAuthServiceBootstrapStatusNeedsSetup(t *testing.T) {
	svc := NewAdminAuthService(newFakeUserRepo(), newTestJWTManager(), &fakeAdminAuditService{})

	status, err := svc.BootstrapStatus()
	if err != nil {
		t.Fatalf("BootstrapStatus failed: %v", err)
	}
	if !status.NeedsSetup || status.AdminCount != 0 {
		t.Fatalf("unexpected bootstrap status: %+v", status)
	}
}

func TestAdminAuthServiceBootstrapSuperAdmin(t *testing.T) {
	audit := &fakeAdminAuditService{}
	svc := NewAdminAuthService(newFakeUserRepo(), newTestJWTManager(), audit)

	user, err := svc.BootstrapSuperAdmin("root-admin", "VeryStrongPass123", AdminRequestMeta{IPAddress: "127.0.0.1"})
	if err != nil {
		t.Fatalf("BootstrapSuperAdmin failed: %v", err)
	}
	if user.ID == 0 || user.Role != model.UserRoleSuperAdmin || user.Status != model.UserStatusNormal {
		t.Fatalf("unexpected super admin: %+v", user)
	}
	if user.PasswordHash == "VeryStrongPass123" || user.PasswordHash == "" {
		t.Fatalf("password was not hashed safely: %q", user.PasswordHash)
	}
	if len(audit.entries) != 1 || audit.entries[0].Action != model.AuditActionAdminBootstrap {
		t.Fatalf("expected bootstrap audit, got %+v", audit.entries)
	}
}

func TestAdminAuthServiceBootstrapRejectsExistingAdmin(t *testing.T) {
	svc := NewAdminAuthService(newFakeUserRepo(&model.User{
		ID:       1,
		Username: "admin",
		Role:     model.UserRoleAdmin,
	}), newTestJWTManager(), &fakeAdminAuditService{})

	_, err := svc.BootstrapSuperAdmin("root-admin", "VeryStrongPass123", AdminRequestMeta{})
	if !errors.Is(err, ErrAdminAlreadyInitialized) {
		t.Fatalf("expected ErrAdminAlreadyInitialized, got %v", err)
	}
}

func TestAdminAuthServiceLoginRejectsNormalUser(t *testing.T) {
	hash, err := utils.HashPasswordArgon2ID("UserPass123")
	if err != nil {
		t.Fatalf("HashPasswordArgon2ID failed: %v", err)
	}
	svc := NewAdminAuthService(newFakeUserRepo(&model.User{
		ID:           1,
		Username:     "alice",
		PasswordHash: hash,
		Role:         model.UserRoleUser,
		Status:       model.UserStatusNormal,
	}), newTestJWTManager(), &fakeAdminAuditService{})

	_, err = svc.Login("alice", "UserPass123", AdminRequestMeta{})
	if !errors.Is(err, ErrAdminPermissionDenied) {
		t.Fatalf("expected ErrAdminPermissionDenied, got %v", err)
	}
}

func TestAdminAuthServiceLoginReturnsTokenAndWritesAudit(t *testing.T) {
	hash, err := utils.HashPasswordArgon2ID("AdminPass123")
	if err != nil {
		t.Fatalf("HashPasswordArgon2ID failed: %v", err)
	}
	userRepo := newFakeUserRepo(&model.User{
		ID:           1,
		Username:     "admin",
		PasswordHash: hash,
		Role:         model.UserRoleAdmin,
		Status:       model.UserStatusNormal,
		TokenVersion: 7,
	})
	audit := &fakeAdminAuditService{}
	svc := &adminAuthService{
		userRepo:     userRepo,
		jwtManager:   newTestJWTManager(),
		auditService: audit,
		now:          func() time.Time { return time.Date(2026, 6, 18, 12, 0, 0, 0, time.UTC) },
	}

	pair, err := svc.Login("admin", "AdminPass123", AdminRequestMeta{IPAddress: "127.0.0.1", UserAgent: "test-agent"})
	if err != nil {
		t.Fatalf("Login failed: %v", err)
	}
	if pair.AccessToken == "" || pair.RefreshToken == "" {
		t.Fatalf("expected token pair, got %+v", pair)
	}
	updated, _ := userRepo.FindByID(1)
	if updated.LastLoginAt == nil || updated.LastLoginIP != "127.0.0.1" || updated.LastLoginUserAgent != "test-agent" {
		t.Fatalf("expected login metadata saved, got %+v", updated)
	}
	if len(audit.entries) != 1 || audit.entries[0].Action != model.AuditActionAdminLogin {
		t.Fatalf("expected login audit, got %+v", audit.entries)
	}
}

func TestAdminAuthServiceLoginCanonicalizesUsername(t *testing.T) {
	hash, err := utils.HashPasswordArgon2ID("AdminPass123")
	if err != nil {
		t.Fatalf("HashPasswordArgon2ID failed: %v", err)
	}
	svc := NewAdminAuthService(newFakeUserRepo(&model.User{
		ID:           1,
		Username:     "admin@example.com",
		PasswordHash: hash,
		Role:         model.UserRoleAdmin,
		Status:       model.UserStatusNormal,
	}), newTestJWTManager(), &fakeAdminAuditService{})

	if _, err := svc.Login("  ADMIN@EXAMPLE.COM ", "AdminPass123", AdminRequestMeta{}); err != nil {
		t.Fatalf("canonical admin login failed: %v", err)
	}
}

func newTestJWTManager() *utils.JWTManager {
	return utils.NewJWTManager("test-secret", "orbitterm-test", 15, 30, 24)
}
