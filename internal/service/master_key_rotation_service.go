package service

import (
	"errors"
	"strings"

	"orbitterm-server/internal/model"
	"orbitterm-server/internal/repository"
	"orbitterm-server/internal/utils"
)

var (
	ErrMasterKeyRotationInvalidInput = errors.New("master key rotation input is invalid")
	ErrMasterKeyRotationConflict     = errors.New("master key rotation snapshot is stale")
)

// MasterKeyRotationItem contains ciphertext only. The server never receives a
// master password or plaintext configuration during a rotation.
type MasterKeyRotationItem struct {
	ID                  uint
	ExpectedVectorClock string
	EncryptedBlob       []byte
}

type MasterKeyRotationService interface {
	Rotate(userID uint, currentLoginPassword string, items []MasterKeyRotationItem) (*utils.TokenPair, error)
}

type masterKeyRotationService struct {
	repository repository.MasterKeyRotationRepository
	jwtManager *utils.JWTManager
}

func NewMasterKeyRotationService(
	repository repository.MasterKeyRotationRepository,
	jwtManager *utils.JWTManager,
) MasterKeyRotationService {
	return &masterKeyRotationService{repository: repository, jwtManager: jwtManager}
}

// Rotate serializes configuration writes with the account row, verifies the
// current login credential under that lock, replaces the complete ciphertext
// snapshot atomically, and invalidates old device tokens.
func (s *masterKeyRotationService) Rotate(
	userID uint,
	currentLoginPassword string,
	items []MasterKeyRotationItem,
) (*utils.TokenPair, error) {
	if userID == 0 || strings.TrimSpace(currentLoginPassword) == "" || s.repository == nil || s.jwtManager == nil {
		return nil, ErrMasterKeyRotationInvalidInput
	}

	replacements := make([]repository.ConfigCipherReplacement, 0, len(items))
	for _, item := range items {
		if item.ID == 0 || strings.TrimSpace(item.ExpectedVectorClock) == "" || len(item.EncryptedBlob) == 0 {
			return nil, ErrMasterKeyRotationInvalidInput
		}
		replacements = append(replacements, repository.ConfigCipherReplacement{
			ID:                  item.ID,
			ExpectedVectorClock: item.ExpectedVectorClock,
			EncryptedBlob:       item.EncryptedBlob,
		})
	}

	user, err := s.repository.RotateEncryptedConfigsAndToken(userID, replacements, func(user *model.User) error {
		if err := ensureRotationUserCanAuthenticate(user); err != nil {
			return err
		}
		matched, verifyErr := utils.VerifyPasswordArgon2ID(currentLoginPassword, user.PasswordHash)
		if verifyErr != nil {
			return verifyErr
		}
		if !matched {
			return ErrInvalidCredential
		}
		return nil
	})
	if errors.Is(err, repository.ErrRotationSnapshotMismatch) {
		return nil, ErrMasterKeyRotationConflict
	}
	if err != nil {
		return nil, err
	}

	return s.jwtManager.GenerateTokenPair(user.ID, user.Username, user.TokenVersion)
}

func ensureRotationUserCanAuthenticate(user *model.User) error {
	if user == nil {
		return ErrInvalidCredential
	}
	if user.IsDeleted {
		return ErrAccountDeleted
	}
	if user.IsBanned || user.Status == model.UserStatusBanned {
		return ErrAccountBanned
	}
	return nil
}
