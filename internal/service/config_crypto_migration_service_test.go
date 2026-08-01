package service

import (
	"crypto/sha256"
	"errors"
	"testing"

	"orbitterm-server/internal/repository"
)

type cipherMigrationRepositoryFixture struct {
	called       bool
	returnedErr  error
	replacements []repository.ConfigCipherMigrationReplacement
}

func (r *cipherMigrationRepositoryFixture) MigrateEncryptedConfigsToV2(
	_ uint,
	replacements []repository.ConfigCipherMigrationReplacement,
) error {
	r.called = true
	r.replacements = replacements
	return r.returnedErr
}

func migrationItem(id uint, expected, next string) ConfigCipherMigrationItem {
	legacy := []byte("OTC1-old")
	digest := sha256.Sum256(legacy)
	return ConfigCipherMigrationItem{
		ID: id, ExpectedVectorClock: expected, ExpectedBlobSHA256: digest,
		EncryptedBlob: []byte("OTC2-new-ciphertext"), NextVectorClock: next,
	}
}

func TestConfigCipherMigrationAcceptsStrictV2Snapshot(t *testing.T) {
	repo := &cipherMigrationRepositoryFixture{}
	svc := NewConfigCipherMigrationService(repo)
	if err := svc.MigrateToV2(7, []ConfigCipherMigrationItem{
		migrationItem(11, `{"ios":1}`, `{"ios":1,"crypto-v2":1}`),
	}); err != nil {
		t.Fatalf("migration failed: %v", err)
	}
	if !repo.called || len(repo.replacements) != 1 {
		t.Fatal("expected exactly one opaque replacement")
	}
}

func TestConfigCipherMigrationRejectsNonV2AndNonAdvancingClock(t *testing.T) {
	repo := &cipherMigrationRepositoryFixture{}
	svc := NewConfigCipherMigrationService(repo)
	item := migrationItem(11, `{"ios":1}`, `{"ios":1}`)
	item.EncryptedBlob = []byte("OTC1-not-v2")
	if err := svc.MigrateToV2(7, []ConfigCipherMigrationItem{item}); !errors.Is(err, ErrConfigCipherMigrationInvalidInput) {
		t.Fatalf("expected invalid input, got %v", err)
	}
	if repo.called {
		t.Fatal("invalid input must not reach persistence")
	}
}

func TestConfigCipherMigrationMapsStaleSnapshotWithoutPartialSuccess(t *testing.T) {
	repo := &cipherMigrationRepositoryFixture{returnedErr: repository.ErrCipherMigrationSnapshotMismatch}
	svc := NewConfigCipherMigrationService(repo)
	err := svc.MigrateToV2(7, []ConfigCipherMigrationItem{
		migrationItem(11, `{"ios":1}`, `{"ios":1,"crypto-v2":1}`),
	})
	if !errors.Is(err, ErrConfigCipherMigrationConflict) {
		t.Fatalf("expected safe conflict, got %v", err)
	}
}
