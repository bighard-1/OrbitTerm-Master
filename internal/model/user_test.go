package model

import (
	"testing"
	"time"
)

func TestClearExpiredBan(t *testing.T) {
	now := time.Date(2026, 6, 18, 12, 0, 0, 0, time.UTC)
	expiredAt := now.Add(-time.Minute)
	bannedAt := now.Add(-time.Hour)
	adminID := uint(1)

	user := &User{
		Status:    UserStatusBanned,
		IsBanned:  true,
		BanUntil:  &expiredAt,
		BanReason: "测试封禁",
		BannedAt:  &bannedAt,
		BannedBy:  &adminID,
	}

	if !user.ClearExpiredBan(now) {
		t.Fatal("expected expired ban to be cleared")
	}
	if user.IsBanned || user.BanUntil != nil || user.BanReason != "" || user.BannedAt != nil || user.BannedBy != nil {
		t.Fatalf("ban fields were not fully cleared: %+v", user)
	}
	if user.Status != UserStatusNormal {
		t.Fatalf("expected normal status, got %q", user.Status)
	}
}

func TestClearExpiredBanKeepsDeletedStatus(t *testing.T) {
	now := time.Date(2026, 6, 18, 12, 0, 0, 0, time.UTC)
	expiredAt := now.Add(-time.Minute)

	user := &User{
		Status:    UserStatusBanned,
		IsBanned:  true,
		BanUntil:  &expiredAt,
		IsDeleted: true,
	}

	if !user.ClearExpiredBan(now) {
		t.Fatal("expected expired ban to be cleared")
	}
	if user.Status != UserStatusDeleted {
		t.Fatalf("expected deleted status, got %q", user.Status)
	}
}

func TestPermanentBanDoesNotExpire(t *testing.T) {
	now := time.Date(2026, 6, 18, 12, 0, 0, 0, time.UTC)
	user := User{IsBanned: true}

	if user.BanExpired(now) {
		t.Fatal("permanent ban must not expire automatically")
	}
	if user.IsActiveAt(now) {
		t.Fatal("permanently banned user must not be active")
	}
}
