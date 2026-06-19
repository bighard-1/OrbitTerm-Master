package model

import "time"

const (
	AuditActionAdminBootstrap            = "admin_bootstrap"
	AuditActionAdminLogin                = "admin_login"
	AuditActionAdminMe                   = "admin_me"
	AuditActionSystemBackupReadinessView = "system_backup_readiness_view"
	AuditActionUserBan                   = "user_ban"
	AuditActionUserUnban                 = "user_unban"
	AuditActionUserResetPassword         = "user_reset_password"
	AuditActionUserForceLogout           = "user_force_logout"
	AuditActionUserSoftDelete            = "user_soft_delete"
	AuditActionUserRestore               = "user_restore"
)

// AdminAuditLog 记录管理端敏感操作。
// 重要：审计日志不保存密码、Token、私钥、主密码等敏感明文。
type AdminAuditLog struct {
	ID uint `gorm:"primaryKey" json:"id"`

	AdminUserID  uint  `gorm:"not null;index" json:"admin_user_id"`
	TargetUserID *uint `gorm:"index" json:"target_user_id,omitempty"`

	Action       string `gorm:"size:64;not null;index" json:"action"`
	ResourceType string `gorm:"size:64;index" json:"resource_type,omitempty"`
	ResourceID   string `gorm:"size:128;index" json:"resource_id,omitempty"`

	BeforeSnapshot string `gorm:"type:text" json:"before_snapshot,omitempty"`
	AfterSnapshot  string `gorm:"type:text" json:"after_snapshot,omitempty"`

	IPAddress string `gorm:"size:64" json:"ip_address,omitempty"`
	UserAgent string `gorm:"size:512" json:"user_agent,omitempty"`
	Reason    string `gorm:"size:512" json:"reason,omitempty"`

	CreatedAt time.Time `gorm:"not null;autoCreateTime;index" json:"created_at"`
}

func (AdminAuditLog) TableName() string {
	return "admin_audit_logs"
}
