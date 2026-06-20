package service

import (
	"errors"
	"testing"

	"orbitterm-server/internal/model"
)

const testIdentityFingerprint = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func TestConfigSyncPullUsesMonotonicCursor(t *testing.T) {
	repo := newFakeConfigRepo(
		&model.ServerConfig{ID: 1, UserID: 1, AssetID: testAssetID, State: model.ServerConfigStateActive, ServerRevision: 2},
		&model.ServerConfig{ID: 2, UserID: 1, AssetID: "55555555-5555-4555-8555-555555555555", State: model.ServerConfigStateDeleted, ServerRevision: 5},
		&model.ServerConfig{ID: 3, UserID: 1, AssetID: "66666666-6666-4666-8666-666666666666", State: model.ServerConfigStatePurged, ServerRevision: 8},
	)
	svc := NewConfigService(repo)

	first, err := svc.PullChanges(1, 0, 2)
	if err != nil {
		t.Fatalf("PullChanges failed: %v", err)
	}
	if len(first.Items) != 2 || !first.HasMore || first.NextCursor != 5 {
		t.Fatalf("unexpected first page: %+v", first)
	}
	second, err := svc.PullChanges(1, first.NextCursor, 2)
	if err != nil {
		t.Fatalf("second PullChanges failed: %v", err)
	}
	if len(second.Items) != 1 || second.HasMore || second.NextCursor != 8 || second.Items[0].State != model.ServerConfigStatePurged {
		t.Fatalf("unexpected second page: %+v", second)
	}
}

func TestConfigSyncRequestsResetWhenCursorIsAhead(t *testing.T) {
	repo := newFakeConfigRepo(&model.ServerConfig{
		ID: 1, UserID: 1, AssetID: testAssetID, State: model.ServerConfigStateActive, ServerRevision: 4,
	})
	svc := NewConfigService(repo)

	page, err := svc.PullChanges(1, 99, 100)
	if err != nil {
		t.Fatalf("PullChanges failed: %v", err)
	}
	if !page.ResetRequired || page.NextCursor != 0 || len(page.Items) != 0 {
		t.Fatalf("expected reset instruction, got %+v", page)
	}
}

func TestConfigSyncAcknowledgementRejectsUnknownRevision(t *testing.T) {
	repo := newFakeConfigRepo(&model.ServerConfig{
		ID: 1, UserID: 1, AssetID: testAssetID, State: model.ServerConfigStateActive, ServerRevision: 4,
	})
	svc := NewConfigService(repo)

	err := svc.AcknowledgeSync(1, SyncAcknowledgementInput{
		DeviceID: testDeviceID, Revision: 5, Platform: "ios", ClientVersion: "1.0.0",
	})
	if !errors.Is(err, ErrConfigInvalidInput) {
		t.Fatalf("expected invalid acknowledgement, got %v", err)
	}
	if err := svc.AcknowledgeSync(1, SyncAcknowledgementInput{
		DeviceID: testDeviceID, Revision: 4, Platform: "ios", ClientVersion: "1.0.0",
	}); err != nil {
		t.Fatalf("valid acknowledgement failed: %v", err)
	}
	if repo.ackRevision != 4 {
		t.Fatalf("unexpected acknowledged revision: %d", repo.ackRevision)
	}
}

func TestConfigUploadEqualVersionIsIdempotentOnlyForSameCiphertext(t *testing.T) {
	repo := newFakeConfigRepo(&model.ServerConfig{
		ID: 1, UserID: 1, AssetID: testAssetID, State: model.ServerConfigStateActive,
		EncryptedBlob: []byte("cipher"), VectorClock: `{"mac":1}`, ServerRevision: 7,
	})
	svc := NewConfigService(repo)

	result, err := svc.Upload(1, nil, testAssetID, "", []byte("cipher"), `{"mac":1}`)
	if err != nil {
		t.Fatalf("idempotent Upload failed: %v", err)
	}
	if result.ServerRevision != 7 || repo.mutationCount != 0 {
		t.Fatalf("idempotent upload created a false revision: %+v", result)
	}
	_, err = svc.Upload(1, nil, testAssetID, "", []byte("different"), `{"mac":1}`)
	if !errors.Is(err, ErrVectorClockConflict) {
		t.Fatalf("same version with different ciphertext must conflict, got %v", err)
	}
}

func TestConfigIdentityMatchUsesOpaqueFingerprint(t *testing.T) {
	repo := newFakeConfigRepo(
		&model.ServerConfig{ID: 1, UserID: 1, AssetID: testAssetID, State: model.ServerConfigStateActive, IdentityFingerprint: testIdentityFingerprint},
		&model.ServerConfig{ID: 2, UserID: 1, AssetID: "55555555-5555-4555-8555-555555555555", State: model.ServerConfigStateDeleted, IdentityFingerprint: testIdentityFingerprint},
		&model.ServerConfig{ID: 3, UserID: 1, AssetID: "66666666-6666-4666-8666-666666666666", State: model.ServerConfigStateActive, IdentityFingerprint: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"},
	)
	svc := NewConfigService(repo)

	matches, err := svc.FindIdentityMatches(1, testIdentityFingerprint)
	if err != nil {
		t.Fatalf("FindIdentityMatches failed: %v", err)
	}
	if len(matches) != 2 {
		t.Fatalf("unexpected identity matches: %+v", matches)
	}
	if _, err := svc.FindIdentityMatches(1, "host.example.com"); !errors.Is(err, ErrConfigInvalidInput) {
		t.Fatalf("raw endpoint data must not be accepted as a fingerprint: %v", err)
	}
}
