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

	_, err := svc.Register("alice", "StrongPass123")
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

	_, err := svc.Register("alice", "shortpass")
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput for short password, got %v", err)
	}

	user, err := svc.Register("alice", "StrongPass123")
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}
	if user.Role != model.UserRoleUser || user.Status != model.UserStatusRisk {
		t.Fatalf("expected policy-backed user role/status, got %+v", user)
	}
}

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
