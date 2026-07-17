package service

import (
	"errors"
	"testing"

	"orbitterm-server/internal/model"
	"orbitterm-server/internal/utils"
)

func TestAuthServiceRegisterRejectsClosedRegistration(t *testing.T) {
	policy := model.DefaultSecurityPolicy()
	policy.RegistrationEnabled = false
	svc := NewAuthService(newFakeUserRepo(), newTestJWTManager(), fakeSecurityPolicyProvider{policy: policy})

	_, err := svc.Register("alice@example.com", "StrongPass123!", "")
	if !errors.Is(err, ErrRegistrationClosed) {
		t.Fatalf("expected ErrRegistrationClosed, got %v", err)
	}
}

func TestAuthServiceChangePasswordRotatesTokensAndRejectsOldCredential(t *testing.T) {
	policy := model.DefaultSecurityPolicy()
	policy.InvitationRequired = false
	policy.RegistrationEnabled = true
	policy.AllowedEmailDomains = []string{"example.com"}
	repo := newFakeUserRepo()
	jwt := newTestJWTManager()
	svc := NewAuthService(repo, jwt, fakeSecurityPolicyProvider{policy: policy}, fakeInviteConsumer{})
	user, err := svc.Register("alice@example.com", "StrongPass123!", "invite")
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	pair, err := svc.ChangePassword(user.ID, "StrongPass123!", "NewStrongPass456!")
	if err != nil {
		t.Fatalf("ChangePassword failed: %v", err)
	}
	if pair.AccessToken == "" || pair.RefreshToken == "" {
		t.Fatal("expected a replacement token pair")
	}
	if _, err := svc.Login("alice@example.com", "StrongPass123!"); !errors.Is(err, ErrInvalidCredential) {
		t.Fatalf("old password must fail after change, got %v", err)
	}
	if _, err := svc.Login("alice@example.com", "NewStrongPass456!"); err != nil {
		t.Fatalf("new password must login: %v", err)
	}
	if claims, err := jwt.ParseAndVerifyAccessToken(pair.AccessToken); err != nil || claims.TokenVersion != 1 {
		t.Fatalf("replacement token must carry incremented version: claims=%+v err=%v", claims, err)
	}
	stored, _ := repo.FindByID(user.ID)
	if stored == nil || stored.TokenVersion != 1 {
		t.Fatalf("expected stored token version 1, got %+v", stored)
	}
	matched, err := utils.VerifyPasswordArgon2ID("NewStrongPass456!", stored.PasswordHash)
	if err != nil || !matched {
		t.Fatalf("new password hash was not persisted safely: matched=%v err=%v", matched, err)
	}
}

func TestAuthServiceChangePasswordRequiresCurrentPasswordAndPolicy(t *testing.T) {
	policy := model.DefaultSecurityPolicy()
	policy.InvitationRequired = false
	policy.RegistrationEnabled = true
	policy.AllowedEmailDomains = []string{"example.com"}
	repo := newFakeUserRepo()
	svc := NewAuthService(repo, newTestJWTManager(), fakeSecurityPolicyProvider{policy: policy}, fakeInviteConsumer{})
	user, err := svc.Register("alice@example.com", "StrongPass123!", "invite")
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}
	if _, err := svc.ChangePassword(user.ID, "wrong", "NewStrongPass456!"); !errors.Is(err, ErrInvalidCredential) {
		t.Fatalf("expected invalid credential, got %v", err)
	}
	if _, err := svc.ChangePassword(user.ID, "StrongPass123!", "short"); !errors.Is(err, ErrWeakPassword) {
		t.Fatalf("expected weak password, got %v", err)
	}
	if _, err := svc.ChangePassword(user.ID, "StrongPass123!", "StrongPass123!"); !errors.Is(err, ErrPasswordUnchanged) {
		t.Fatalf("expected unchanged password rejection, got %v", err)
	}
}

func TestAuthServiceRegisterUsesSecurityPolicy(t *testing.T) {
	policy := model.DefaultSecurityPolicy()
	policy.MinPasswordLength = 12
	policy.DefaultUserStatus = model.UserStatusRisk
	userRepo := newFakeUserRepo()
	svc := NewAuthService(userRepo, newTestJWTManager(), fakeSecurityPolicyProvider{policy: policy})

	_, err := svc.Register("alice@gmail.com", "shortpass", "")
	if !errors.Is(err, ErrWeakPassword) {
		t.Fatalf("expected ErrWeakPassword for short password, got %v", err)
	}

	userRepo = newFakeUserRepo()
	svc = NewAuthService(userRepo, newTestJWTManager(), fakeSecurityPolicyProvider{policy: policy}, fakeInviteConsumer{})
	user, err := svc.Register("alice@gmail.com", "StrongPass123!", "valid-invite")
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}
	if user.Role != model.UserRoleUser || user.Status != model.UserStatusRisk {
		t.Fatalf("expected policy-backed user role/status, got %+v", user)
	}
}

func TestAuthServiceRegisterRequiresAllowedEmailAndInvite(t *testing.T) {
	policy := model.DefaultSecurityPolicy()
	svc := NewAuthService(newFakeUserRepo(), newTestJWTManager(), fakeSecurityPolicyProvider{policy: policy}, fakeInviteConsumer{err: ErrInviteInvalid})

	if _, err := svc.Register("alice@unknown.test", "StrongPass123!", "invite"); !errors.Is(err, ErrEmailDomainDenied) {
		t.Fatalf("expected ErrEmailDomainDenied, got %v", err)
	}
	if _, err := svc.Register("alice@gmail.com", "StrongPass123!", "invalid"); !errors.Is(err, ErrInviteInvalid) {
		t.Fatalf("expected ErrInviteInvalid, got %v", err)
	}
}

func TestAuthServiceUsesOneCanonicalUsernameForRegistrationAndLogin(t *testing.T) {
	policy := model.DefaultSecurityPolicy()
	policy.InvitationRequired = false
	policy.RegistrationEnabled = true
	policy.AllowedEmailDomains = []string{"example.com"}

	repo := newFakeUserRepo()
	svc := NewAuthService(repo, newTestJWTManager(), fakeSecurityPolicyProvider{policy: policy}, fakeInviteConsumer{})
	user, err := svc.Register("  Orbit.User@Example.COM  ", "StrongPass123!", "valid-invite")
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}
	if user.Username != "orbit.user@example.com" {
		t.Fatalf("registration stored non-canonical username: %q", user.Username)
	}

	pair, err := svc.Login("ORBIT.USER@example.com", "StrongPass123!")
	if err != nil {
		t.Fatalf("canonical login failed: %v", err)
	}
	if pair.AccessToken == "" || pair.RefreshToken == "" {
		t.Fatalf("expected token pair after canonical login, got %+v", pair)
	}
}

type fakeInviteConsumer struct{ err error }

func (f fakeInviteConsumer) Consume(string) error { return f.err }
func (f fakeInviteConsumer) Release(string) error { return nil }

type fakeSecurityPolicyProvider struct {
	policy model.SecurityPolicy
	err    error
}

func (p fakeSecurityPolicyProvider) GetSecurityPolicy() (model.SecurityPolicy, error) {
	if p.err != nil {
		return model.SecurityPolicy{}, p.err
	}
	policy := p.policy
	policy.Normalize()
	return policy, nil
}
