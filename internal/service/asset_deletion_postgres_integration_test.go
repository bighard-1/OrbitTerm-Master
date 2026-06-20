package service

import (
	"os"
	"testing"
	"time"

	"orbitterm-server/internal/config"
	"orbitterm-server/internal/model"
	"orbitterm-server/internal/repository"

	"gorm.io/gorm"
)

const integrationDeviceID = "99999999-9999-4999-8999-999999999999"

func TestPostgresAssetDeletionLifecycleAndSafeGC(t *testing.T) {
	dsn := os.Getenv("ORBITTERM_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("ORBITTERM_TEST_DATABASE_URL is not configured")
	}
	db, err := config.NewDatabase(config.Config{DatabaseURL: dsn})
	if err != nil {
		t.Fatalf("connect integration database: %v", err)
	}
	if err := config.MigrateDatabase(db); err != nil {
		t.Fatalf("migrate integration database: %v", err)
	}
	cleanupIntegrationTables(t, db)
	t.Cleanup(func() { cleanupIntegrationTables(t, db) })

	user := &model.User{
		Username: "asset-gc-integration", PasswordHash: "integration-only",
		Role: model.UserRoleUser, Status: model.UserStatusNormal,
	}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create integration user: %v", err)
	}

	configRepo := repository.NewServerConfigRepository(db)
	settingRepo := repository.NewSystemSettingRepository(db)
	auditRepo := repository.NewAdminAuditRepository(db)
	auditService := NewAdminAuditService(auditRepo)
	manager := NewAssetDeletionPolicyManager(settingRepo, configRepo, auditService)
	now := time.Now().UTC().Truncate(time.Millisecond)
	expiredAt := now.Add(-time.Hour)

	recoverable := &model.ServerConfig{
		UserID: user.ID, AssetID: testAssetID,
		EncryptedBlob: []byte("integration-ciphertext"), VectorClock: `{"ios":2}`,
		State: model.ServerConfigStateDeleted, DeletedAt: &expiredAt, PurgeAfter: &expiredAt,
	}
	if err := configRepo.Create(recoverable); err != nil {
		t.Fatalf("create recoverable config: %v", err)
	}

	cleanupResult, err := manager.CleanupExpiredAssets(1, "PostgreSQL 集成清理", AdminRequestMeta{IPAddress: "127.0.0.1"})
	if err != nil {
		t.Fatalf("purge expired ciphertext: %v", err)
	}
	if cleanupResult.PurgedCount != 1 || cleanupResult.TombstonesDeleted != 0 {
		t.Fatalf("unexpected first cleanup result: %+v", cleanupResult)
	}
	purged, err := configRepo.FindByAssetIDAndUserID(testAssetID, user.ID)
	if err != nil || purged == nil {
		t.Fatalf("read purged config: config=%+v err=%v", purged, err)
	}
	if purged.State != model.ServerConfigStatePurged || len(purged.EncryptedBlob) != 0 || purged.ServerRevision <= recoverable.ServerRevision {
		t.Fatalf("purge did not create a minimal revised tombstone: %+v", purged)
	}

	configService := NewConfigService(configRepo, manager)
	page, err := configService.PullChanges(user.ID, 0, 100)
	if err != nil {
		t.Fatalf("pull incremental tombstone: %v", err)
	}
	if len(page.Items) != 1 || page.Items[0].State != model.ServerConfigStatePurged {
		t.Fatalf("incremental pull did not expose purged tombstone: %+v", page)
	}
	if err := configService.AcknowledgeSync(user.ID, SyncAcknowledgementInput{
		DeviceID: integrationDeviceID, Revision: purged.ServerRevision,
		Platform: "integration", ClientVersion: "test",
	}); err != nil {
		t.Fatalf("acknowledge tombstone revision: %v", err)
	}

	unacknowledgedID := "88888888-8888-4888-8888-888888888888"
	unacknowledged := &model.ServerConfig{
		UserID: user.ID, AssetID: unacknowledgedID,
		EncryptedBlob: []byte{}, VectorClock: `{"server-cleanup":1}`,
		State: model.ServerConfigStatePurged, DeletedAt: &expiredAt,
	}
	if err := configRepo.Create(unacknowledged); err != nil {
		t.Fatalf("create unacknowledged tombstone: %v", err)
	}
	oldUpdatedAt := now.AddDate(-2, 0, 0)
	if err := db.Model(&model.ServerConfig{}).
		Where("id IN ?", []uint{purged.ID, unacknowledged.ID}).
		UpdateColumn("updated_at", oldUpdatedAt).Error; err != nil {
		t.Fatalf("age tombstones: %v", err)
	}

	tombstoneDays := 365
	if _, err := manager.UpdateAssetDeletionPolicy(1, AssetDeletionPolicyUpdate{
		TombstoneRetentionDays: &tombstoneDays,
		Reason:                 "启用安全墓碑回收集成测试",
	}, AdminRequestMeta{}); err != nil {
		t.Fatalf("enable finite tombstone retention: %v", err)
	}
	gcResult, err := manager.CleanupExpiredAssets(1, "执行安全墓碑回收", AdminRequestMeta{})
	if err != nil {
		t.Fatalf("garbage collect acknowledged tombstone: %v", err)
	}
	if gcResult.TombstonesDeleted != 1 || gcResult.TombstonesDeferred != 1 {
		t.Fatalf("unexpected safe GC result: %+v", gcResult)
	}
	if found, err := configRepo.FindByAssetIDAndUserID(testAssetID, user.ID); err != nil || found != nil {
		t.Fatalf("acknowledged tombstone was not deleted: config=%+v err=%v", found, err)
	}
	if found, err := configRepo.FindByAssetIDAndUserID(unacknowledgedID, user.ID); err != nil || found == nil {
		t.Fatalf("unacknowledged tombstone must be deferred: config=%+v err=%v", found, err)
	}

	logs, _, err := auditService.List(AdminAuditListFilter{Action: model.AuditActionSystemAssetTrashCleanup, Limit: 20})
	if err != nil || len(logs) < 2 {
		t.Fatalf("cleanup audit records missing: count=%d err=%v", len(logs), err)
	}
}

func cleanupIntegrationTables(t *testing.T, db *gorm.DB) {
	t.Helper()
	if err := db.Exec(`
		TRUNCATE TABLE
			config_sync_changes,
			sync_device_states,
			server_configs,
			admin_audit_logs,
			system_settings,
			users
		RESTART IDENTITY CASCADE
	`).Error; err != nil {
		t.Fatalf("clean integration tables: %v", err)
	}
}
