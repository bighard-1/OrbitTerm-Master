package service

import (
	"crypto/sha256"
	"errors"
	"os"
	"testing"

	"orbitterm-server/internal/config"
	"orbitterm-server/internal/model"
	"orbitterm-server/internal/repository"

	"gorm.io/gorm"
)

// These tests are intentionally opt-in: they exercise PostgreSQL row locking
// and transaction rollback rather than a repository fake. CI supplies an
// isolated ORBITTERM_TEST_DATABASE_URL for this lane.
func TestPostgresConfigCipherMigrationUpdatesExactFullSnapshot(t *testing.T) {
	db := openConfigCipherMigrationIntegrationDB(t)
	user := createConfigCipherMigrationUser(t, db, "crypto-migration-success")
	repo := repository.NewServerConfigRepository(db)

	active := createConfigCipherMigrationConfig(t, repo, user.ID, "crypto-v2-active", "active-v1", `{"ios":1}`, model.ServerConfigStateActive)
	deleted := createConfigCipherMigrationConfig(t, repo, user.ID, "crypto-v2-deleted", "deleted-v1", `{"macos":2}`, model.ServerConfigStateDeleted)
	beforeChanges := configCipherMigrationChangeCount(t, db, user.ID)

	replacements := []repository.ConfigCipherMigrationReplacement{
		configCipherMigrationReplacement(active, []byte("OTC2-active-reencrypted"), `{"ios":2,"crypto_v2":1}`),
		configCipherMigrationReplacement(deleted, []byte("OTC2-deleted-reencrypted"), `{"macos":3,"crypto_v2":1}`),
	}
	if err := repo.MigrateEncryptedConfigsToV2(user.ID, replacements); err != nil {
		t.Fatalf("migrate exact full snapshot: %v", err)
	}

	assertConfigCipherMigrationConfig(t, repo, active.ID, user.ID, []byte("OTC2-active-reencrypted"), `{"ios":2,"crypto_v2":1}`)
	assertConfigCipherMigrationConfig(t, repo, deleted.ID, user.ID, []byte("OTC2-deleted-reencrypted"), `{"macos":3,"crypto_v2":1}`)
	if got := configCipherMigrationChangeCount(t, db, user.ID); got != beforeChanges+2 {
		t.Fatalf("migration must append one revision per replacement: before=%d after=%d", beforeChanges, got)
	}
}

func TestPostgresConfigCipherMigrationStaleSnapshotRollsBackEveryWrite(t *testing.T) {
	db := openConfigCipherMigrationIntegrationDB(t)
	user := createConfigCipherMigrationUser(t, db, "crypto-migration-conflict")
	repo := repository.NewServerConfigRepository(db)

	first := createConfigCipherMigrationConfig(t, repo, user.ID, "crypto-v2-first", "first-v1", `{"ios":1}`, model.ServerConfigStateActive)
	second := createConfigCipherMigrationConfig(t, repo, user.ID, "crypto-v2-second", "second-v1", `{"macos":1}`, model.ServerConfigStateActive)
	replacements := []repository.ConfigCipherMigrationReplacement{
		configCipherMigrationReplacement(first, []byte("OTC2-first-reencrypted"), `{"ios":2,"crypto_v2":1}`),
		configCipherMigrationReplacement(second, []byte("OTC2-second-reencrypted"), `{"macos":2,"crypto_v2":1}`),
	}
	beforeChanges := configCipherMigrationChangeCount(t, db, user.ID)

	// Simulate a different client committing after this migration snapshot was
	// read. The first replacement is still valid; the second is stale. Atomicity
	// requires the first ciphertext and its revision to remain untouched.
	if err := db.Model(&model.ServerConfig{}).Where("id = ?", second.ID).Updates(map[string]any{
		"encrypted_blob": []byte("second-changed-by-other-client"),
		"vector_clock":   `{"macos":2}`,
	}).Error; err != nil {
		t.Fatalf("make second snapshot record stale: %v", err)
	}
	if err := repo.MigrateEncryptedConfigsToV2(user.ID, replacements); !errors.Is(err, repository.ErrCipherMigrationSnapshotMismatch) {
		t.Fatalf("stale snapshot error = %v, want %v", err, repository.ErrCipherMigrationSnapshotMismatch)
	}

	assertConfigCipherMigrationConfig(t, repo, first.ID, user.ID, []byte("first-v1"), `{"ios":1}`)
	assertConfigCipherMigrationConfig(t, repo, second.ID, user.ID, []byte("second-changed-by-other-client"), `{"macos":2}`)
	if got := configCipherMigrationChangeCount(t, db, user.ID); got != beforeChanges {
		t.Fatalf("stale migration must not append partial revisions: before=%d after=%d", beforeChanges, got)
	}
}

func openConfigCipherMigrationIntegrationDB(t *testing.T) *gorm.DB {
	t.Helper()
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
	return db
}

func createConfigCipherMigrationUser(t *testing.T, db *gorm.DB, username string) *model.User {
	t.Helper()
	user := &model.User{Username: username, PasswordHash: "integration-only", Role: model.UserRoleUser, Status: model.UserStatusNormal}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create integration user: %v", err)
	}
	return user
}

func createConfigCipherMigrationConfig(t *testing.T, repo repository.ServerConfigRepository, userID uint, assetID, ciphertext, vectorClock, state string) *model.ServerConfig {
	t.Helper()
	item := &model.ServerConfig{UserID: userID, AssetID: assetID, EncryptedBlob: []byte(ciphertext), VectorClock: vectorClock, State: state}
	if err := repo.Create(item); err != nil {
		t.Fatalf("create integration config: %v", err)
	}
	return item
}

func configCipherMigrationReplacement(config *model.ServerConfig, encryptedBlob []byte, nextVectorClock string) repository.ConfigCipherMigrationReplacement {
	return repository.ConfigCipherMigrationReplacement{
		ID:                  config.ID,
		ExpectedVectorClock: config.VectorClock,
		ExpectedBlobSHA256:  sha256.Sum256(config.EncryptedBlob),
		EncryptedBlob:       encryptedBlob,
		NextVectorClock:     nextVectorClock,
	}
}

func assertConfigCipherMigrationConfig(t *testing.T, repo repository.ServerConfigRepository, id, userID uint, encryptedBlob []byte, vectorClock string) {
	t.Helper()
	config, err := repo.FindByIDAndUserID(id, userID)
	if err != nil || config == nil {
		t.Fatalf("load config %d: config=%+v err=%v", id, config, err)
	}
	if string(config.EncryptedBlob) != string(encryptedBlob) || config.VectorClock != vectorClock {
		t.Fatalf("config %d mismatch: blob=%q clock=%s", id, config.EncryptedBlob, config.VectorClock)
	}
}

func configCipherMigrationChangeCount(t *testing.T, db *gorm.DB, userID uint) int64 {
	t.Helper()
	var count int64
	if err := db.Model(&model.ConfigSyncChange{}).Where("user_id = ?", userID).Count(&count).Error; err != nil {
		t.Fatalf("count config changes: %v", err)
	}
	return count
}
