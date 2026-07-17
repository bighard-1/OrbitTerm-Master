package service

import (
	"errors"
	"testing"

	"orbitterm-server/internal/model"
	"orbitterm-server/internal/repository"
	"orbitterm-server/internal/utils"
)

type rotationRepositoryFixture struct {
	user          model.User
	expected      map[uint]string
	stored        map[uint][]byte
	forceConflict bool
}

func (r *rotationRepositoryFixture) RotateEncryptedConfigsAndToken(
	userID uint,
	replacements []repository.ConfigCipherReplacement,
	authorize func(*model.User) error,
) (*model.User, error) {
	if userID != r.user.ID {
		return nil, errors.New("unexpected user")
	}
	if err := authorize(&r.user); err != nil {
		return nil, err
	}
	if r.forceConflict || len(replacements) != len(r.expected) {
		return nil, repository.ErrRotationSnapshotMismatch
	}
	seen := make(map[uint]struct{}, len(replacements))
	for _, replacement := range replacements {
		if _, duplicate := seen[replacement.ID]; duplicate || r.expected[replacement.ID] != replacement.ExpectedVectorClock {
			return nil, repository.ErrRotationSnapshotMismatch
		}
		seen[replacement.ID] = struct{}{}
	}
	// Simulate the repository transaction: no replacement is committed until
	// every ID/vector-clock check has passed.
	updated := make(map[uint][]byte, len(replacements))
	for _, replacement := range replacements {
		updated[replacement.ID] = append([]byte(nil), replacement.EncryptedBlob...)
	}
	r.stored = updated
	r.user.TokenVersion++
	return &r.user, nil
}

func newRotationFixture(t *testing.T) (*rotationRepositoryFixture, string) {
	t.Helper()
	hash, err := hashTestPassword("StrongPass123!")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	return &rotationRepositoryFixture{
		user:     model.User{ID: 7, Username: "alice@example.com", PasswordHash: hash, Status: model.UserStatusNormal},
		expected: map[uint]string{11: `{"device-a":3}`, 12: `{"device-b":2}`},
		stored:   map[uint][]byte{11: []byte("old-a"), 12: []byte("old-b")},
	}, "StrongPass123!"
}

func hashTestPassword(password string) (string, error) {
	return utils.HashPasswordArgon2ID(password)
}

func TestMasterKeyRotationReplacesCompleteSnapshotAndInvalidatesOldTokens(t *testing.T) {
	repo, currentPassword := newRotationFixture(t)
	jwt := newTestJWTManager()
	service := NewMasterKeyRotationService(repo, jwt)

	pair, err := service.Rotate(repo.user.ID, currentPassword, []MasterKeyRotationItem{
		{ID: 11, ExpectedVectorClock: `{"device-a":3}`, EncryptedBlob: []byte("new-a")},
		{ID: 12, ExpectedVectorClock: `{"device-b":2}`, EncryptedBlob: []byte("new-b")},
	})
	if err != nil {
		t.Fatalf("Rotate failed: %v", err)
	}
	if string(repo.stored[11]) != "new-a" || string(repo.stored[12]) != "new-b" {
		t.Fatalf("ciphertexts were not replaced atomically: %#v", repo.stored)
	}
	claims, err := jwt.ParseAndVerifyAccessToken(pair.AccessToken)
	if err != nil || claims.TokenVersion != 1 {
		t.Fatalf("expected replacement token version 1, claims=%+v err=%v", claims, err)
	}
}

func TestMasterKeyRotationRejectsBadCredentialAndStaleSnapshotWithoutMutation(t *testing.T) {
	repo, currentPassword := newRotationFixture(t)
	service := NewMasterKeyRotationService(repo, newTestJWTManager())

	_, err := service.Rotate(repo.user.ID, "wrong", []MasterKeyRotationItem{
		{ID: 11, ExpectedVectorClock: `{"device-a":3}`, EncryptedBlob: []byte("new-a")},
		{ID: 12, ExpectedVectorClock: `{"device-b":2}`, EncryptedBlob: []byte("new-b")},
	})
	if !errors.Is(err, ErrInvalidCredential) {
		t.Fatalf("expected invalid credential, got %v", err)
	}
	if string(repo.stored[11]) != "old-a" || repo.user.TokenVersion != 0 {
		t.Fatal("invalid credential must not mutate stored ciphertext or tokens")
	}

	_, err = service.Rotate(repo.user.ID, currentPassword, []MasterKeyRotationItem{
		{ID: 11, ExpectedVectorClock: `{"device-a":999}`, EncryptedBlob: []byte("new-a")},
		{ID: 12, ExpectedVectorClock: `{"device-b":2}`, EncryptedBlob: []byte("new-b")},
	})
	if !errors.Is(err, ErrMasterKeyRotationConflict) {
		t.Fatalf("expected stale snapshot conflict, got %v", err)
	}
	if string(repo.stored[11]) != "old-a" || string(repo.stored[12]) != "old-b" || repo.user.TokenVersion != 0 {
		t.Fatal("stale snapshot must not partially mutate stored ciphertext or tokens")
	}
}
