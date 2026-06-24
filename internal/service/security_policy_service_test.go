package service

import (
	"errors"
	"testing"

	"orbitterm-server/internal/model"
)

func TestSecurityPolicyServiceReturnsDefaultWhenMissing(t *testing.T) {
	svc := NewSecurityPolicyService(newFakeSystemSettingRepo(), &fakeAdminAuditService{})

	policy, err := svc.GetSecurityPolicy()
	if err != nil {
		t.Fatalf("GetSecurityPolicy failed: %v", err)
	}
	if !policy.RegistrationEnabled || policy.MinPasswordLength != 12 || !policy.InvitationRequired || !policy.StrictPasswordComplexity || len(policy.AllowedEmailDomains) == 0 || policy.DefaultUserRole != model.UserRoleUser || policy.DefaultUserStatus != model.UserStatusNormal {
		t.Fatalf("unexpected default policy: %+v", policy)
	}
}

func TestSecurityPolicyServiceUpdateNormalizesAndAudits(t *testing.T) {
	repo := newFakeSystemSettingRepo()
	audit := &fakeAdminAuditService{}
	svc := NewSecurityPolicyService(repo, audit)

	enabled := false
	reason := "维护窗口"
	minPasswordLength := 4
	status := model.UserStatusRisk
	policy, err := svc.UpdateSecurityPolicy(1, SecurityPolicyUpdate{
		RegistrationEnabled:        &enabled,
		RegistrationDisabledReason: &reason,
		MinPasswordLength:          &minPasswordLength,
		DefaultUserStatus:          &status,
		Reason:                     "安全策略调整",
	}, AdminRequestMeta{IPAddress: "127.0.0.1"})
	if err != nil {
		t.Fatalf("UpdateSecurityPolicy failed: %v", err)
	}
	if policy.RegistrationEnabled || policy.RegistrationDisabledReason != reason || policy.MinPasswordLength != 12 || policy.DefaultUserRole != model.UserRoleUser || policy.DefaultUserStatus != model.UserStatusRisk {
		t.Fatalf("unexpected normalized policy: %+v", policy)
	}
	if len(audit.entries) != 1 || audit.entries[0].Action != model.AuditActionSystemSecurityPolicyUpdate {
		t.Fatalf("expected policy audit, got %+v", audit.entries)
	}

	stored, err := repo.FindByKey(model.SystemSettingKeySecurityPolicy)
	if err != nil || stored == nil || stored.Value == "" {
		t.Fatalf("expected persisted setting, got setting=%+v err=%v", stored, err)
	}
}

func TestSecurityPolicyServiceRejectsMissingAdmin(t *testing.T) {
	svc := NewSecurityPolicyService(newFakeSystemSettingRepo(), &fakeAdminAuditService{})

	_, err := svc.UpdateSecurityPolicy(0, SecurityPolicyUpdate{}, AdminRequestMeta{})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput, got %v", err)
	}
}

type fakeSystemSettingRepo struct {
	settings map[string]*model.SystemSetting
}

func newFakeSystemSettingRepo(settings ...*model.SystemSetting) *fakeSystemSettingRepo {
	repo := &fakeSystemSettingRepo{settings: make(map[string]*model.SystemSetting)}
	for _, setting := range settings {
		copy := *setting
		repo.settings[setting.Key] = &copy
	}
	return repo
}

func (r *fakeSystemSettingRepo) FindByKey(key string) (*model.SystemSetting, error) {
	setting, ok := r.settings[key]
	if !ok {
		return nil, nil
	}
	copy := *setting
	return &copy, nil
}

func (r *fakeSystemSettingRepo) Upsert(setting *model.SystemSetting) error {
	copy := *setting
	if copy.ID == 0 {
		copy.ID = uint(len(r.settings) + 1)
	}
	r.settings[setting.Key] = &copy
	return nil
}
