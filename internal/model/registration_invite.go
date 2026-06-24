package model

import "time"

const (
	AuditActionRegistrationInviteCreate = "registration_invite_create"
	AuditActionRegistrationInviteRevoke = "registration_invite_revoke"
)

// RegistrationInvite stores only a SHA-256 digest. The clear-text code is
// returned once at creation and can never be recovered from the database.
type RegistrationInvite struct {
	ID         uint       `gorm:"primaryKey" json:"id"`
	CodeHash   string     `gorm:"size:64;uniqueIndex;not null" json:"-"`
	CodePrefix string     `gorm:"size:16;not null;index" json:"code_prefix"`
	Note       string     `gorm:"size:256" json:"note,omitempty"`
	MaxUses    int        `gorm:"not null;default:1" json:"max_uses"`
	UseCount   int        `gorm:"not null;default:0" json:"use_count"`
	ExpiresAt  *time.Time `gorm:"index" json:"expires_at,omitempty"`
	DisabledAt *time.Time `gorm:"index" json:"disabled_at,omitempty"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	CreatedBy  uint       `gorm:"not null;index" json:"created_by"`
	CreatedAt  time.Time  `gorm:"not null;autoCreateTime" json:"created_at"`
	UpdatedAt  time.Time  `gorm:"not null;autoUpdateTime" json:"updated_at"`
}

func (RegistrationInvite) TableName() string { return "registration_invites" }

func (i RegistrationInvite) AvailableAt(now time.Time) bool {
	return i.DisabledAt == nil && (i.ExpiresAt == nil || i.ExpiresAt.After(now)) && i.UseCount < i.MaxUses
}
