package worker

import (
	"errors"
	"testing"

	"orbitterm-server/internal/service"
)

type fakeAssetTrashCleaner struct {
	result service.AssetTrashCleanupResult
	err    error
	calls  int
}

func (f *fakeAssetTrashCleaner) CleanupExpiredAssetsBySystem() (service.AssetTrashCleanupResult, error) {
	f.calls++
	return f.result, f.err
}

func TestRunAssetTrashCleanupInvokesCleaner(t *testing.T) {
	cleaner := &fakeAssetTrashCleaner{result: service.AssetTrashCleanupResult{PurgedCount: 2}}
	runAssetTrashCleanup(cleaner)
	if cleaner.calls != 1 {
		t.Fatalf("expected one cleanup call, got %d", cleaner.calls)
	}
}

func TestRunAssetTrashCleanupHandlesError(t *testing.T) {
	cleaner := &fakeAssetTrashCleaner{err: errors.New("database unavailable")}
	runAssetTrashCleanup(cleaner)
	if cleaner.calls != 1 {
		t.Fatalf("expected one cleanup call, got %d", cleaner.calls)
	}
}
