package service

import (
	"errors"
	"testing"
	"time"

	"orbitterm-server/internal/model"
)

const (
	testAssetID   = "11111111-1111-4111-8111-111111111111"
	testDeviceID  = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	testDeviceID2 = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
	testDeleteOp  = "22222222-2222-4222-8222-222222222222"
	testRestoreOp = "33333333-3333-4333-8333-333333333333"
	testPurgeOp   = "44444444-4444-4444-8444-444444444444"
)

func TestConfigServiceDeleteIsIdempotentAndRecoverable(t *testing.T) {
	repo := newFakeConfigRepo(&model.ServerConfig{
		ID:            7,
		UserID:        9,
		AssetID:       testAssetID,
		EncryptedBlob: []byte("ciphertext"),
		VectorClock:   `{"mac":1}`,
		State:         model.ServerConfigStateActive,
	})
	now := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	svc := &configService{
		configRepo: repo,
		policy: fakeAssetDeletionPolicyReader{policy: model.AssetDeletionPolicy{
			RecentDeletedRetentionDays: 30,
			TombstoneRetentionDays:     0,
			CleanupBatchLimit:          500,
			AutoCleanupEnabled:         true,
		}},
		now: func() time.Time { return now },
	}
	input := AssetMutationInput{
		AssetID: testAssetID, DeviceID: testDeviceID, OperationID: testDeleteOp, VectorClock: `{"mac":2}`,
	}

	deleted, err := svc.DeleteAsset(9, input)
	if err != nil {
		t.Fatalf("DeleteAsset failed: %v", err)
	}
	if deleted.State != model.ServerConfigStateDeleted || deleted.DeletedAt == nil || deleted.PurgeAfter == nil {
		t.Fatalf("unexpected deleted state: %+v", deleted)
	}
	if got := deleted.PurgeAfter.Sub(*deleted.DeletedAt); got != 30*24*time.Hour {
		t.Fatalf("unexpected recovery window: %v", got)
	}
	if string(deleted.EncryptedBlob) != "ciphertext" {
		t.Fatal("recoverable delete must retain encrypted blob")
	}
	firstPurgeAfter := *deleted.PurgeAfter
	firstMutations := repo.mutationCount

	now = now.Add(24 * time.Hour)
	repeated, err := svc.DeleteAsset(9, input)
	if err != nil {
		t.Fatalf("idempotent DeleteAsset failed: %v", err)
	}
	if !repeated.PurgeAfter.Equal(firstPurgeAfter) {
		t.Fatal("idempotent retry must not extend recovery window")
	}
	if repo.mutationCount != firstMutations {
		t.Fatal("idempotent retry must not write the record again")
	}
}

func TestConfigServiceDeleteWinsConcurrentUpdate(t *testing.T) {
	repo := newFakeConfigRepo(&model.ServerConfig{
		ID: 1, UserID: 1, AssetID: testAssetID, EncryptedBlob: []byte("cipher"),
		VectorClock: `{"mac":2}`, State: model.ServerConfigStateActive,
	})
	svc := &configService{configRepo: repo, policy: fakeAssetDeletionPolicyReader{policy: model.DefaultAssetDeletionPolicy()}, now: time.Now}

	result, err := svc.DeleteAsset(1, AssetMutationInput{
		AssetID: testAssetID, DeviceID: testDeviceID2, OperationID: testDeleteOp, VectorClock: `{"iphone":1}`,
	})
	if err != nil {
		t.Fatalf("concurrent delete failed: %v", err)
	}
	if result.VectorClock != `{"iphone":1,"mac":2}` {
		t.Fatalf("unexpected merged vector clock: %s", result.VectorClock)
	}
	if result.State != model.ServerConfigStateDeleted {
		t.Fatalf("delete must win concurrent update, got %s", result.State)
	}
}

func TestConfigServiceRestoreThenPurgeLifecycle(t *testing.T) {
	now := time.Now().UTC()
	repo := newFakeConfigRepo(&model.ServerConfig{
		ID: 1, UserID: 1, AssetID: testAssetID, EncryptedBlob: []byte("cipher"),
		VectorClock: `{"mac":2}`, State: model.ServerConfigStateDeleted,
		DeletedAt: &now, PurgeAfter: &now,
	})
	svc := &configService{configRepo: repo, policy: fakeAssetDeletionPolicyReader{policy: model.DefaultAssetDeletionPolicy()}, now: time.Now}

	restored, err := svc.RestoreAsset(1, AssetMutationInput{
		AssetID: testAssetID, DeviceID: testDeviceID, OperationID: testRestoreOp, VectorClock: `{"mac":3}`,
	})
	if err != nil {
		t.Fatalf("RestoreAsset failed: %v", err)
	}
	if restored.State != model.ServerConfigStateActive || restored.DeletedAt != nil || restored.PurgeAfter != nil {
		t.Fatalf("unexpected restored state: %+v", restored)
	}

	_, err = svc.DeleteAsset(1, AssetMutationInput{
		AssetID: testAssetID, DeviceID: testDeviceID, OperationID: testDeleteOp, VectorClock: `{"mac":4}`,
	})
	if err != nil {
		t.Fatalf("second DeleteAsset failed: %v", err)
	}
	purged, err := svc.PurgeAsset(1, AssetMutationInput{
		AssetID: testAssetID, DeviceID: testDeviceID, OperationID: testPurgeOp, VectorClock: `{"mac":5}`,
	})
	if err != nil {
		t.Fatalf("PurgeAsset failed: %v", err)
	}
	if purged.State != model.ServerConfigStatePurged || len(purged.EncryptedBlob) != 0 {
		t.Fatalf("purge must retain only minimal tombstone: %+v", purged)
	}
}

func TestConfigServiceRejectsStaleRestore(t *testing.T) {
	now := time.Now().UTC()
	repo := newFakeConfigRepo(&model.ServerConfig{
		ID: 1, UserID: 1, AssetID: testAssetID, EncryptedBlob: []byte("cipher"),
		VectorClock: `{"mac":4}`, State: model.ServerConfigStateDeleted, DeletedAt: &now,
	})
	svc := &configService{configRepo: repo, policy: fakeAssetDeletionPolicyReader{policy: model.DefaultAssetDeletionPolicy()}, now: time.Now}

	_, err := svc.RestoreAsset(1, AssetMutationInput{
		AssetID: testAssetID, DeviceID: testDeviceID2, OperationID: testRestoreOp, VectorClock: `{"iphone":1}`,
	})
	if !errors.Is(err, ErrVectorClockConflict) {
		t.Fatalf("expected stale/concurrent restore conflict, got %v", err)
	}
}

func TestConfigServiceBackfillsAssetIDWithoutRecreatingLegacyRecord(t *testing.T) {
	legacy := &model.ServerConfig{
		ID: 17, UserID: 3, EncryptedBlob: []byte("old-cipher"),
		VectorClock: `{"mac":1}`, State: model.ServerConfigStateActive,
	}
	repo := newFakeConfigRepo(legacy)
	svc := NewConfigService(repo)
	configID := uint(17)

	updated, err := svc.Upload(3, &configID, testAssetID, []byte("new-cipher"), `{"mac":2}`)
	if err != nil {
		t.Fatalf("Upload failed: %v", err)
	}
	if updated.ID != 17 || updated.AssetID != testAssetID || string(updated.EncryptedBlob) != "new-cipher" {
		t.Fatalf("legacy record was not safely backfilled: %+v", updated)
	}
	if len(repo.items) != 1 {
		t.Fatalf("backfill must not create a duplicate record, got %d", len(repo.items))
	}
}

type fakeAssetDeletionPolicyReader struct {
	policy model.AssetDeletionPolicy
	err    error
}

func (f fakeAssetDeletionPolicyReader) GetAssetDeletionPolicy() (model.AssetDeletionPolicy, error) {
	return f.policy, f.err
}

type fakeConfigRepo struct {
	items         map[string]*model.ServerConfig
	mutationCount int
}

func newFakeConfigRepo(items ...*model.ServerConfig) *fakeConfigRepo {
	repo := &fakeConfigRepo{items: make(map[string]*model.ServerConfig)}
	for _, item := range items {
		copy := *item
		copy.EncryptedBlob = append([]byte(nil), item.EncryptedBlob...)
		repo.items[item.AssetID] = &copy
	}
	return repo
}

func (f *fakeConfigRepo) Create(config *model.ServerConfig) error {
	copy := *config
	f.items[config.AssetID] = &copy
	return nil
}
func (f *fakeConfigRepo) Update(config *model.ServerConfig) error {
	for assetID, item := range f.items {
		if item.ID == config.ID && assetID != config.AssetID {
			delete(f.items, assetID)
		}
	}
	copy := *config
	f.items[config.AssetID] = &copy
	return nil
}
func (f *fakeConfigRepo) FindByIDAndUserID(id, userID uint) (*model.ServerConfig, error) {
	for _, item := range f.items {
		if item.ID == id && item.UserID == userID {
			copy := *item
			return &copy, nil
		}
	}
	return nil, nil
}
func (f *fakeConfigRepo) FindByAssetIDAndUserID(assetID string, userID uint) (*model.ServerConfig, error) {
	item := f.items[assetID]
	if item == nil || item.UserID != userID {
		return nil, nil
	}
	copy := *item
	return &copy, nil
}
func (f *fakeConfigRepo) MutateByAssetID(userID uint, assetID string, mutate func(*model.ServerConfig) (bool, error)) (*model.ServerConfig, error) {
	item := f.items[assetID]
	if item == nil || item.UserID != userID {
		return nil, nil
	}
	copy := *item
	copy.EncryptedBlob = append([]byte(nil), item.EncryptedBlob...)
	changed, err := mutate(&copy)
	if err != nil {
		return nil, err
	}
	if changed {
		f.mutationCount++
		stored := copy
		f.items[assetID] = &stored
	}
	return &copy, nil
}
func (f *fakeConfigRepo) ListByUserID(userID uint) ([]model.ServerConfig, error) {
	items := make([]model.ServerConfig, 0)
	for _, item := range f.items {
		if item.UserID == userID && item.State == model.ServerConfigStateActive {
			items = append(items, *item)
		}
	}
	return items, nil
}
func (f *fakeConfigRepo) ListTrashByUserID(userID uint, limit, offset int) ([]model.ServerConfig, int64, error) {
	items := make([]model.ServerConfig, 0)
	for _, item := range f.items {
		if item.UserID == userID && item.State == model.ServerConfigStateDeleted {
			items = append(items, *item)
		}
	}
	total := int64(len(items))
	if offset >= len(items) {
		return nil, total, nil
	}
	end := offset + limit
	if end > len(items) {
		end = len(items)
	}
	return items[offset:end], total, nil
}
func (f *fakeConfigRepo) CountAll() (int64, error) { return int64(len(f.items)), nil }
func (f *fakeConfigRepo) DeleteByIDAndUserID(id, userID uint) (bool, error) {
	for assetID, item := range f.items {
		if item.ID == id && item.UserID == userID {
			delete(f.items, assetID)
			return true, nil
		}
	}
	return false, nil
}
