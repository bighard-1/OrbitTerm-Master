package service

import (
	"os"
	"strings"
	"testing"

	"orbitterm-server/internal/config"
	"orbitterm-server/internal/model"
	"orbitterm-server/internal/repository"
)

// Exercises the SQL lookup and migration boundary, not only the in-memory
// service fake. The encrypted payload remains opaque throughout the migration.
func TestPostgresCanonicalAssetIDMigrationAndDeletion(t *testing.T) {
	dsn := os.Getenv("ORBITTERM_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("ORBITTERM_TEST_DATABASE_URL is not configured")
	}
	db, err := config.NewDatabase(config.Config{DatabaseURL: dsn})
	if err != nil {
		t.Fatalf("connect integration database: %v", err)
	}
	if err := config.MigrateDatabase(db); err != nil {
		t.Fatalf("initial migration: %v", err)
	}
	cleanupIntegrationTables(t, db)
	t.Cleanup(func() { cleanupIntegrationTables(t, db) })

	user := &model.User{Username: "canonical-asset-integration", PasswordHash: "integration-only", Role: model.UserRoleUser, Status: model.UserStatusNormal}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	const legacyUpperID = "ABCDEF00-1234-4678-9ABC-DEF012345678"
	stored := &model.ServerConfig{UserID: user.ID, AssetID: legacyUpperID, EncryptedBlob: []byte("opaque-ciphertext"), VectorClock: `{"mac":1}`, State: model.ServerConfigStateActive}
	if err := db.Create(stored).Error; err != nil {
		t.Fatalf("create legacy upper-case record: %v", err)
	}
	if err := config.MigrateDatabase(db); err != nil {
		t.Fatalf("canonical migration: %v", err)
	}
	repo := repository.NewServerConfigRepository(db)
	found, err := repo.FindByAssetIDAndUserID(legacyUpperID, user.ID)
	if err != nil || found == nil || found.AssetID != strings.ToLower(legacyUpperID) || string(found.EncryptedBlob) != "opaque-ciphertext" {
		t.Fatalf("migration changed identity or ciphertext: record=%+v err=%v", found, err)
	}
	if err := db.Create(&model.ServerConfig{UserID: user.ID, AssetID: legacyUpperID, EncryptedBlob: []byte("duplicate"), VectorClock: `{"ios":1}`, State: model.ServerConfigStateActive}).Error; err == nil {
		t.Fatal("canonical unique index allowed a case-only duplicate")
	}
	svc := NewConfigService(repo)
	deleted, err := svc.DeleteAsset(user.ID, AssetMutationInput{AssetID: legacyUpperID, DeviceID: testDeviceID, OperationID: testDeleteOp, VectorClock: `{"mac":2}`})
	if err != nil || deleted.State != model.ServerConfigStateDeleted {
		t.Fatalf("mixed-case deletion failed: record=%+v err=%v", deleted, err)
	}
}
