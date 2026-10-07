package model

import "time"

// RefreshSession stores only a digest of the refresh JWT ID. A family maps to
// one device login and survives one-time refresh rotation.
type RefreshSession struct {
	ID           uint      `gorm:"primaryKey"`
	UserID       uint      `gorm:"not null;index:idx_refresh_family"`
	FamilyID     string    `gorm:"size:64;not null;index:idx_refresh_family"`
	TokenDigest  string    `gorm:"size:64;not null;uniqueIndex"`
	TokenVersion int64     `gorm:"not null"`
	ExpiresAt    time.Time `gorm:"not null;index"`
	UsedAt       *time.Time
	RevokedAt    *time.Time
	CreatedAt    time.Time `gorm:"not null;autoCreateTime"`
}

func (RefreshSession) TableName() string { return "refresh_sessions" }
