package service

import (
	"testing"

	"orbitterm-server/internal/model"
)

func TestAssetDeletionPolicyServiceDefaults(t *testing.T) {
	service := NewAssetDeletionPolicyService(newFakeSystemSettingRepo())
	policy, err := service.GetAssetDeletionPolicy()
	if err != nil {
		t.Fatalf("GetAssetDeletionPolicy failed: %v", err)
	}
	if policy.RecentDeletedRetentionDays != 90 || policy.TombstoneRetentionDays != 0 || !policy.AutoCleanupEnabled {
		t.Fatalf("unexpected default policy: %+v", policy)
	}
}

func TestAssetDeletionPolicyServiceNormalizesStoredPolicy(t *testing.T) {
	repo := newFakeSystemSettingRepo(&model.SystemSetting{
		Key:   model.SystemSettingKeyAssetDeletionPolicy,
		Value: `{"recent_deleted_retention_days":1,"tombstone_retention_days":30,"cleanup_batch_limit":1,"auto_cleanup_enabled":true}`,
	})
	service := NewAssetDeletionPolicyService(repo)
	policy, err := service.GetAssetDeletionPolicy()
	if err != nil {
		t.Fatalf("GetAssetDeletionPolicy failed: %v", err)
	}
	if policy.RecentDeletedRetentionDays != 7 || policy.TombstoneRetentionDays != 365 || policy.CleanupBatchLimit != 100 {
		t.Fatalf("stored policy was not normalized: %+v", policy)
	}
}
