package service

import (
	"errors"
	"testing"
	"time"

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

func TestAssetDeletionPolicyManagerUpdatesAndAudits(t *testing.T) {
	settingRepo := newFakeSystemSettingRepo()
	audit := &fakeAdminAuditService{}
	manager := NewAssetDeletionPolicyManager(settingRepo, newFakeConfigRepo(), audit)
	recentDays := 2
	tombstoneDays := 12
	batchLimit := 12
	disabled := false

	policy, err := manager.UpdateAssetDeletionPolicy(7, AssetDeletionPolicyUpdate{
		RecentDeletedRetentionDays: &recentDays,
		TombstoneRetentionDays:     &tombstoneDays,
		CleanupBatchLimit:          &batchLimit,
		AutoCleanupEnabled:         &disabled,
		Reason:                     "调整最近删除策略",
	}, AdminRequestMeta{IPAddress: "127.0.0.1"})
	if err != nil {
		t.Fatalf("UpdateAssetDeletionPolicy failed: %v", err)
	}
	if policy.RecentDeletedRetentionDays != 7 || policy.TombstoneRetentionDays != 365 || policy.CleanupBatchLimit != 100 || policy.AutoCleanupEnabled {
		t.Fatalf("unexpected normalized policy: %+v", policy)
	}
	if len(audit.entries) != 1 || audit.entries[0].Action != model.AuditActionSystemAssetDeletionPolicyUpdate {
		t.Fatalf("missing policy audit: %+v", audit.entries)
	}
}

func TestAssetDeletionCleanupPurgesOnlyExpiredCiphertext(t *testing.T) {
	now := time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC)
	expiredAt := now.Add(-time.Minute)
	future := now.Add(time.Hour)
	repo := newFakeConfigRepo(
		&model.ServerConfig{ID: 1, UserID: 1, AssetID: testAssetID, State: model.ServerConfigStateDeleted, EncryptedBlob: []byte("secret"), VectorClock: `{"ios":2}`, PurgeAfter: &expiredAt},
		&model.ServerConfig{ID: 2, UserID: 1, AssetID: "77777777-7777-4777-8777-777777777777", State: model.ServerConfigStateDeleted, EncryptedBlob: []byte("keep"), VectorClock: `{"ios":2}`, PurgeAfter: &future},
	)
	manager := NewAssetDeletionPolicyManager(newFakeSystemSettingRepo(), repo, &fakeAdminAuditService{}).(*assetDeletionPolicyService)
	manager.now = func() time.Time { return now }

	result, err := manager.CleanupExpiredAssets(9, "手动清理过期资产", AdminRequestMeta{})
	if err != nil {
		t.Fatalf("CleanupExpiredAssets failed: %v", err)
	}
	if result.ScannedCount != 1 || result.PurgedCount != 1 || result.FailedCount != 0 {
		t.Fatalf("unexpected cleanup result: %+v", result)
	}
	purged := repo.items[testAssetID]
	if purged.State != model.ServerConfigStatePurged || len(purged.EncryptedBlob) != 0 || purged.ServerRevision == 0 {
		t.Fatalf("expired config was not safely purged: %+v", purged)
	}
	if repo.items["77777777-7777-4777-8777-777777777777"].State != model.ServerConfigStateDeleted {
		t.Fatal("future trash item must remain recoverable")
	}
}

func TestAssetDeletionSystemCleanupHonorsDisabledPolicy(t *testing.T) {
	settingRepo := newFakeSystemSettingRepo(&model.SystemSetting{
		Key:   model.SystemSettingKeyAssetDeletionPolicy,
		Value: `{"recent_deleted_retention_days":90,"tombstone_retention_days":0,"cleanup_batch_limit":500,"auto_cleanup_enabled":false}`,
	})
	now := time.Now().UTC()
	repo := newFakeConfigRepo(&model.ServerConfig{
		ID: 1, UserID: 1, AssetID: testAssetID, State: model.ServerConfigStateDeleted,
		EncryptedBlob: []byte("secret"), VectorClock: `{}`, PurgeAfter: &now,
	})
	manager := NewAssetDeletionPolicyManager(settingRepo, repo, &fakeAdminAuditService{})
	result, err := manager.CleanupExpiredAssetsBySystem()
	if err != nil {
		t.Fatalf("CleanupExpiredAssetsBySystem failed: %v", err)
	}
	if result.Enabled || repo.items[testAssetID].State != model.ServerConfigStateDeleted {
		t.Fatalf("disabled automatic cleanup mutated data: %+v", result)
	}
}

func TestAssetDeletionPolicyRejectsMissingReason(t *testing.T) {
	manager := NewAssetDeletionPolicyManager(newFakeSystemSettingRepo(), newFakeConfigRepo(), &fakeAdminAuditService{})
	_, err := manager.UpdateAssetDeletionPolicy(1, AssetDeletionPolicyUpdate{}, AdminRequestMeta{})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput, got %v", err)
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
