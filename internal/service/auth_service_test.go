package service

import (
	"errors"
	"testing"

	"orbitterm-server/internal/model"
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
