package model

import "time"

const (
	UserRoleSuperAdmin = "super_admin"
	UserRoleAdmin      = "admin"
	UserRoleSupport    = "support"
	UserRoleUser       = "user"

	UserStatusNormal  = "normal"
	UserStatusBanned  = "banned"
	UserStatusDeleted = "deleted"
	UserStatusRisk    = "risk"
)

// User 表示系统用户。
// 密码字段必须存储 Argon2id 生成的哈希字符串，严禁保存明文密码。
type User struct {
	ID           uint   `gorm:"primaryKey" json:"id"`
	Username     string `gorm:"size:64;uniqueIndex;not null" json:"username"`
	PasswordHash string `gorm:"size:512;not null" json:"-"`

	// Role 用于区分普通用户与后续管理端角色。默认普通用户，避免迁移后误授予管理权限。
	Role string `gorm:"size:32;not null;default:'user';index" json:"role"`
	// Status 是账号状态的聚合展示字段，便于管理端筛选；具体封禁/删除仍以对应字段为准。
	Status string `gorm:"size:32;not null;default:'normal';index" json:"status"`

	IsBanned  bool       `gorm:"not null;default:false;index" json:"is_banned"`
	BanUntil  *time.Time `gorm:"index" json:"ban_until,omitempty"`
	BanReason string     `gorm:"size:512" json:"ban_reason,omitempty"`
	BannedAt  *time.Time `json:"banned_at,omitempty"`
	BannedBy  *uint      `json:"banned_by,omitempty"`

	IsDeleted bool       `gorm:"not null;default:false;index" json:"is_deleted"`
	DeletedAt *time.Time `json:"deleted_at,omitempty"`

	MustChangePassword bool `gorm:"not null;default:false" json:"must_change_password"`
	// TokenVersion 会写入 JWT。管理员强制下线/重置密码时提升该值即可让旧 Token 失效。
	TokenVersion int64 `gorm:"not null;default:0" json:"token_version"`

	LastLoginAt        *time.Time `json:"last_login_at,omitempty"`
	LastLoginIP        string     `gorm:"size:64" json:"last_login_ip,omitempty"`
	LastLoginUserAgent string     `gorm:"size:512" json:"last_login_user_agent,omitempty"`

	CreatedBy *uint     `json:"created_by,omitempty"`
	UpdatedBy *uint     `json:"updated_by,omitempty"`
	CreatedAt time.Time `gorm:"not null;autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"not null;autoUpdateTime" json:"updated_at"`
}

// TableName 显式指定表名，避免未来结构重构时出现不必要的迁移歧义。
func (User) TableName() string {
	return "users"
}

func (u User) BanExpired(now time.Time) bool {
	return u.IsBanned && u.BanUntil != nil && !u.BanUntil.After(now)
}

func (u User) IsActiveAt(now time.Time) bool {
	if u.IsDeleted {
		return false
	}
	if u.IsBanned && !u.BanExpired(now) {
		return false
	}
	return true
}

func (u *User) ClearExpiredBan(now time.Time) bool {
	if !u.BanExpired(now) {
		return false
	}
	u.IsBanned = false
	u.BanUntil = nil
	u.BanReason = ""
	u.BannedAt = nil
	u.BannedBy = nil
	if u.IsDeleted {
		u.Status = UserStatusDeleted
	} else {
		u.Status = UserStatusNormal
	}
	u.UpdatedAt = now
	return true
}
