package model

import "testing"

func TestAssetDeletionPolicyNormalize(t *testing.T) {
	policy := AssetDeletionPolicy{
		RecentDeletedRetentionDays: 1,
		TombstoneRetentionDays:     30,
		CleanupBatchLimit:          1,
	}
	policy.Normalize()

	if policy.RecentDeletedRetentionDays != 7 {
		t.Fatalf("unexpected recent-deleted retention: %d", policy.RecentDeletedRetentionDays)
	}
	if policy.TombstoneRetentionDays != 365 {
		t.Fatalf("unexpected tombstone retention: %d", policy.TombstoneRetentionDays)
	}
	if policy.CleanupBatchLimit != 100 {
		t.Fatalf("unexpected cleanup batch limit: %d", policy.CleanupBatchLimit)
	}
}

func TestAssetDeletionPolicyPermanentTombstonesRemainPermanent(t *testing.T) {
	policy := DefaultAssetDeletionPolicy()
	policy.Normalize()
	if policy.TombstoneRetentionDays != 0 {
		t.Fatalf("default tombstones must be permanent, got %d days", policy.TombstoneRetentionDays)
	}
}
