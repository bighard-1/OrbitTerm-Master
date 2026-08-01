package service

import (
	"bytes"
	"errors"
	"strings"

	"orbitterm-server/internal/repository"
)

var (
	ErrConfigCipherMigrationInvalidInput = errors.New("config cipher migration input is invalid")
	ErrConfigCipherMigrationConflict     = errors.New("config cipher migration snapshot is stale")
)

// ConfigCipherMigrationItem is opaque ciphertext supplied after the client has
// locally decrypted its complete V1 snapshot and re-encrypted it as OTC2.
type ConfigCipherMigrationItem struct {
	ID                  uint
	ExpectedVectorClock string
	ExpectedBlobSHA256  [32]byte
	EncryptedBlob       []byte
	NextVectorClock     string
}

type ConfigCipherMigrationService interface {
	MigrateToV2(userID uint, items []ConfigCipherMigrationItem) error
}

type configCipherMigrationService struct {
	repository repository.ConfigCipherMigrationRepository
}

func NewConfigCipherMigrationService(repository repository.ConfigCipherMigrationRepository) ConfigCipherMigrationService {
	return &configCipherMigrationService{repository: repository}
}

// MigrateToV2 validates a strict vector-clock advance before the repository
// obtains the account lock. The repository then verifies that exact snapshot
// under the lock and writes every replacement in one transaction.
func (s *configCipherMigrationService) MigrateToV2(userID uint, items []ConfigCipherMigrationItem) error {
	if userID == 0 || s.repository == nil || len(items) == 0 {
		return ErrConfigCipherMigrationInvalidInput
	}

	replacements := make([]repository.ConfigCipherMigrationReplacement, 0, len(items))
	seen := make(map[uint]struct{}, len(items))
	for _, item := range items {
		if item.ID == 0 || strings.TrimSpace(item.ExpectedVectorClock) == "" ||
			len(item.EncryptedBlob) == 0 || strings.TrimSpace(item.NextVectorClock) == "" ||
			!bytes.HasPrefix(item.EncryptedBlob, []byte("OTC2")) {
			return ErrConfigCipherMigrationInvalidInput
		}
		if _, duplicate := seen[item.ID]; duplicate {
			return ErrConfigCipherMigrationInvalidInput
		}
		seen[item.ID] = struct{}{}
		relation, err := compareVectorClock(item.NextVectorClock, item.ExpectedVectorClock)
		if err != nil || relation != vectorClockNewer {
			return ErrConfigCipherMigrationInvalidInput
		}
		replacements = append(replacements, repository.ConfigCipherMigrationReplacement{
			ID:                  item.ID,
			ExpectedVectorClock: item.ExpectedVectorClock,
			ExpectedBlobSHA256:  item.ExpectedBlobSHA256,
			EncryptedBlob:       item.EncryptedBlob,
			NextVectorClock:     item.NextVectorClock,
		})
	}
	if err := s.repository.MigrateEncryptedConfigsToV2(userID, replacements); err != nil {
		if errors.Is(err, repository.ErrCipherMigrationSnapshotMismatch) {
			return ErrConfigCipherMigrationConflict
		}
		return err
	}
	return nil
}
