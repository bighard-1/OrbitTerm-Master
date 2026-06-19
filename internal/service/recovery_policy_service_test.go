package service

import (
	"errors"
	"testing"

	"orbitterm-server/internal/model"
)

func TestRecoveryPolicyServiceReturnsDefaultZeroKnowledgePolicy(t *testing.T) {
	svc := NewRecoveryPolicyService(newFakeSystemSettingRepo(), &fakeAdminAuditService{})

	policy, err := svc.GetRecoveryPolicy()
	if err != nil {
		t.Fatalf("GetRecoveryPolicy failed: %v", err)
	}
	assertZeroKnowledgeRecoveryPolicy(t, policy)
	if !policy.LoginPasswordResetEnabled || !policy.RequireUserAcknowledgement {
		t.Fatalf("unexpected default recovery flags: %+v", policy)
	}
}

func TestRecoveryPolicyServiceUpdatePreservesZeroKnowledgeBoundary(t *testing.T) {
	repo := newFakeSystemSettingRepo()
	audit := &fakeAdminAuditService{}
	svc := NewRecoveryPolicyService(repo, audit)

	loginReset := false
	ack := false
	contact := "support@orbitterm.com"
	message := "请联系管理员重置登录密码；主密码无法找回。"
	policy, err := svc.UpdateRecoveryPolicy(1, RecoveryPolicyUpdate{
		LoginPasswordResetEnabled:  &loginReset,
		RequireUserAcknowledgement: &ack,
		SupportContact:             &contact,
		UserFacingMessage:          &message,
		Reason:                     "明确零知识恢复边界",
	}, AdminRequestMeta{IPAddress: "127.0.0.1"})
	if err != nil {
		t.Fatalf("UpdateRecoveryPolicy failed: %v", err)
	}
	assertZeroKnowledgeRecoveryPolicy(t, policy)
	if policy.LoginPasswordResetEnabled || policy.RequireUserAcknowledgement || policy.SupportContact != contact || policy.UserFacingMessage != message {
		t.Fatalf("unexpected updated recovery policy: %+v", policy)
	}
	if len(audit.entries) != 1 || audit.entries[0].Action != model.AuditActionSystemRecoveryPolicyUpdate {
		t.Fatalf("expected recovery policy audit, got %+v", audit.entries)
	}

	stored, err := repo.FindByKey(model.SystemSettingKeyRecoveryPolicy)
	if err != nil || stored == nil || stored.Value == "" {
		t.Fatalf("expected persisted recovery policy, got setting=%+v err=%v", stored, err)
	}
}

func TestRecoveryPolicyServiceRejectsMissingAdmin(t *testing.T) {
	svc := NewRecoveryPolicyService(newFakeSystemSettingRepo(), &fakeAdminAuditService{})

	_, err := svc.UpdateRecoveryPolicy(0, RecoveryPolicyUpdate{}, AdminRequestMeta{})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput, got %v", err)
	}
}

func TestPublicRecoveryInfoReflectsZeroKnowledgeBoundary(t *testing.T) {
	policy := model.DefaultRecoveryPolicy()
	policy.LoginPasswordResetEnabled = false
	policy.SupportContact = "support@orbitterm.com"

	info := ToPublicRecoveryInfo(policy)
	if info.MasterPasswordRecoverable || info.ServerCanDecryptUserAssets || info.MasterPasswordRecoveryMode != model.MasterPasswordRecoveryModeZeroKnowledge {
		t.Fatalf("public info must preserve zero-knowledge boundary: %+v", info)
	}
	if info.LoginPasswordResetEnabled || info.SupportContact != "support@orbitterm.com" {
		t.Fatalf("unexpected public recovery info: %+v", info)
	}
}

func assertZeroKnowledgeRecoveryPolicy(t *testing.T, policy model.RecoveryPolicy) {
	t.Helper()
	if policy.MasterPasswordRecoverable {
		t.Fatalf("master password must not be recoverable: %+v", policy)
	}
	if policy.ServerCanDecryptUserAssets {
		t.Fatalf("server must not be able to decrypt user assets: %+v", policy)
	}
	if policy.MasterPasswordRecoveryMode != model.MasterPasswordRecoveryModeZeroKnowledge {
		t.Fatalf("unexpected recovery mode: %+v", policy)
	}
	if policy.MasterPasswordResetBehavior != model.MasterPasswordResetBehaviorClientSide {
		t.Fatalf("unexpected reset behavior: %+v", policy)
	}
	if !policy.EncryptedAssetsPreservedOnReset {
		t.Fatalf("encrypted assets should be preserved on reset: %+v", policy)
	}
}
