package service

import (
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"orbitterm-server/internal/config"
	"orbitterm-server/internal/model"
	"orbitterm-server/internal/repository"
)

func TestPostgresMigrationRestoreRejectsRollbackAndRevokesSessions(t *testing.T) {
	dsn := os.Getenv("ORBITTERM_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("ORBITTERM_TEST_DATABASE_URL is not configured")
	}
	adminDB, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	schema := fmt.Sprintf("maturity_restore_%d", time.Now().UnixNano())
	if err := adminDB.Exec("CREATE SCHEMA " + schema).Error; err != nil {
		t.Fatal(err)
	}
	defer adminDB.Exec("DROP SCHEMA " + schema + " CASCADE")
	db, err := gorm.Open(postgres.Open(dsn+" search_path="+schema), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := config.MigrateDatabase(db); err != nil {
		t.Fatal(err)
	}
	admin := model.User{Username: "restore-admin@example.com", PasswordHash: "integration-hash", Role: model.UserRoleSuperAdmin, Status: model.UserStatusNormal, TokenVersion: 4}
	if err := db.Create(&admin).Error; err != nil {
		t.Fatal(err)
	}
	svc := NewMigrationBundleService(db, config.Config{}, NewAdminAuditService(repository.NewAdminAuditRepository(db)))
	passphrase := "Integration-Restore-123!"
	packageBytes, _, err := svc.Export(admin.ID, passphrase, AdminRequestMeta{})
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * time.Millisecond)
	device := model.SyncDeviceState{UserID: admin.ID, DeviceID: "later-device", LastSeenAt: time.Now()}
	if err := db.Create(&device).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Restore(admin.ID, packageBytes, passphrase, AdminRequestMeta{}, false); !errors.Is(err, ErrMigrationRollback) {
		t.Fatalf("expected rollback guard for device acknowledgement, got %v", err)
	}
	if err := db.Delete(&device).Error; err != nil {
		t.Fatal(err)
	}
	change := model.ConfigSyncChange{UserID: admin.ID, ConfigID: 1, State: "active", ChangedAt: time.Now()}
	if err := db.Create(&change).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Restore(admin.ID, packageBytes, passphrase, AdminRequestMeta{}, false); !errors.Is(err, ErrMigrationRollback) {
		t.Fatalf("expected rollback guard for sync change, got %v", err)
	}
	if err := db.Delete(&change).Error; err != nil {
		t.Fatal(err)
	}
	newUser := model.User{Username: "newer@example.com", PasswordHash: "integration-hash", Role: model.UserRoleUser, Status: model.UserStatusNormal}
	if err := db.Create(&newUser).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&admin).Update("token_version", 9).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.RefreshSession{UserID: admin.ID, FamilyID: "session-to-clear", TokenDigest: "abcdef", TokenVersion: 4, ExpiresAt: time.Now().Add(time.Hour)}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.AdminAuditLog{AdminUserID: admin.ID, Action: "later_sensitive_action"}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Restore(admin.ID, packageBytes, passphrase, AdminRequestMeta{}, false); !errors.Is(err, ErrMigrationRollback) {
		t.Fatalf("expected rollback guard, got %v", err)
	}
	if _, err := svc.Restore(admin.ID, packageBytes, passphrase, AdminRequestMeta{}, true); err != nil {
		t.Fatal(err)
	}
	var count int64
	if err := db.Model(&model.RefreshSession{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("sessions after restore: count=%d err=%v", count, err)
	}
	if err := db.Model(&model.User{}).Where("username = ?", newUser.Username).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("newer user after restore: count=%d err=%v", count, err)
	}
	var restored model.User
	if err := db.First(&restored, admin.ID).Error; err != nil {
		t.Fatal(err)
	}
	if restored.TokenVersion <= 9 {
		t.Fatalf("token version not advanced: %d", restored.TokenVersion)
	}
	if err := db.Model(&model.AdminAuditLog{}).Where("action = ?", model.AuditActionMigrationBundleRestore).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("restore audit: count=%d err=%v", count, err)
	}
	if err := db.Model(&model.AdminAuditLog{}).Where("action = ?", "later_sensitive_action").Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("later audit lost on override restore: count=%d err=%v", count, err)
	}
	if err := db.Model(&model.AdminAuditLog{}).Where("action = ?", model.AuditActionMigrationBundleExport).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("export audit lost on override restore: count=%d err=%v", count, err)
	}
}
