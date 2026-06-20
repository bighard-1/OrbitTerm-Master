package model

import (
	"strings"
	"time"
)

const (
	SystemSettingKeySecurityPolicy      = "security_policy"
	SystemSettingKeyRecoveryPolicy      = "recovery_policy"
	SystemSettingKeyAuditPolicy         = "audit_policy"
	SystemSettingKeyAssetDeletionPolicy = "asset_deletion_policy"

	AuditActionSystemSecurityPolicyUpdate      = "system_security_policy_update"
	AuditActionSystemRecoveryPolicyUpdate      = "system_recovery_policy_update"
	AuditActionSystemAuditPolicyUpdate         = "system_audit_policy_update"
	AuditActionSystemAuditCleanup              = "system_audit_cleanup"
	AuditActionSystemAssetDeletionPolicyUpdate = "system_asset_deletion_policy_update"
	AuditActionSystemAssetTrashCleanup         = "system_asset_trash_cleanup"
)

// SystemSetting 存储服务端运行策略等小型配置。
// Value 使用 JSON 文本，便于后续在不频繁变更表结构的情况下扩展管理端策略。
type SystemSetting struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Key       string    `gorm:"size:128;uniqueIndex;not null" json:"key"`
	Value     string    `gorm:"type:text;not null" json:"value"`
	CreatedAt time.Time `gorm:"not null;autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"not null;autoUpdateTime" json:"updated_at"`
}

func (SystemSetting) TableName() string {
	return "system_settings"
}

// SecurityPolicy 是管理端可动态调整的基础安全策略。
// 注意：普通用户注册永远只能落为 user 角色，避免配置错误导致越权。
type SecurityPolicy struct {
	RegistrationEnabled        bool   `json:"registration_enabled"`
	RegistrationDisabledReason string `json:"registration_disabled_reason,omitempty"`
	MinPasswordLength          int    `json:"min_password_length"`
	DefaultUserRole            string `json:"default_user_role"`
	DefaultUserStatus          string `json:"default_user_status"`
}

func DefaultSecurityPolicy() SecurityPolicy {
	return SecurityPolicy{
		RegistrationEnabled: true,
		MinPasswordLength:   8,
		DefaultUserRole:     UserRoleUser,
		DefaultUserStatus:   UserStatusNormal,
	}
}

func (p *SecurityPolicy) Normalize() {
	if p.MinPasswordLength < 8 {
		p.MinPasswordLength = 8
	}
	if p.MinPasswordLength > 128 {
		p.MinPasswordLength = 128
	}

	// 注册入口只允许创建普通用户。该字段保留给管理端展示与未来兼容，不允许配置成管理员角色。
	p.DefaultUserRole = UserRoleUser

	switch strings.TrimSpace(p.DefaultUserStatus) {
	case UserStatusNormal, UserStatusRisk:
		p.DefaultUserStatus = strings.TrimSpace(p.DefaultUserStatus)
	default:
		p.DefaultUserStatus = UserStatusNormal
	}
	p.RegistrationDisabledReason = strings.TrimSpace(p.RegistrationDisabledReason)
}

const (
	MasterPasswordRecoveryModeZeroKnowledge = "zero_knowledge_reset_only"
	MasterPasswordResetBehaviorClientSide   = "client_reencrypt_required"
)

// RecoveryPolicy 描述账号密码、主密码与云端密文之间的恢复边界。
// 安全不变量：后端不保存主密码、不保存派生密钥，也不具备解密用户资产的能力。
type RecoveryPolicy struct {
	LoginPasswordResetEnabled       bool   `json:"login_password_reset_enabled"`
	MasterPasswordRecoverable       bool   `json:"master_password_recoverable"`
	MasterPasswordRecoveryMode      string `json:"master_password_recovery_mode"`
	MasterPasswordResetBehavior     string `json:"master_password_reset_behavior"`
	ServerCanDecryptUserAssets      bool   `json:"server_can_decrypt_user_assets"`
	EncryptedAssetsPreservedOnReset bool   `json:"encrypted_assets_preserved_on_reset"`
	RequireUserAcknowledgement      bool   `json:"require_user_acknowledgement"`
	SupportContact                  string `json:"support_contact,omitempty"`
	UserFacingMessage               string `json:"user_facing_message"`
}

func DefaultRecoveryPolicy() RecoveryPolicy {
	return RecoveryPolicy{
		LoginPasswordResetEnabled:       true,
		MasterPasswordRecoverable:       false,
		MasterPasswordRecoveryMode:      MasterPasswordRecoveryModeZeroKnowledge,
		MasterPasswordResetBehavior:     MasterPasswordResetBehaviorClientSide,
		ServerCanDecryptUserAssets:      false,
		EncryptedAssetsPreservedOnReset: true,
		RequireUserAcknowledgement:      true,
		UserFacingMessage:               "OrbitTerm 采用零知识加密。管理员可以重置登录密码，但无法找回或解密您的主密码与服务器资产；修改主密码需要在客户端用旧主密码解密后重新加密。",
	}
}

func (p *RecoveryPolicy) Normalize() {
	// 以下字段是零知识安全边界，不允许通过管理端配置放宽。
	p.MasterPasswordRecoverable = false
	p.MasterPasswordRecoveryMode = MasterPasswordRecoveryModeZeroKnowledge
	p.MasterPasswordResetBehavior = MasterPasswordResetBehaviorClientSide
	p.ServerCanDecryptUserAssets = false
	p.EncryptedAssetsPreservedOnReset = true

	p.SupportContact = strings.TrimSpace(p.SupportContact)
	p.UserFacingMessage = strings.TrimSpace(p.UserFacingMessage)
	if p.UserFacingMessage == "" {
		p.UserFacingMessage = DefaultRecoveryPolicy().UserFacingMessage
	}
}

// AuditPolicy 控制管理端审计日志生命周期。
// 审计日志用于追责和排障，不能无限期堆积；保留周期过短也会降低安全可追溯性。
type AuditPolicy struct {
	RetentionDays     int `json:"retention_days"`
	CleanupBatchLimit int `json:"cleanup_batch_limit"`
}

func DefaultAuditPolicy() AuditPolicy {
	return AuditPolicy{
		RetentionDays:     180,
		CleanupBatchLimit: 500,
	}
}

func (p *AuditPolicy) Normalize() {
	if p.RetentionDays < 30 {
		p.RetentionDays = 30
	}
	if p.RetentionDays > 3650 {
		p.RetentionDays = 3650
	}
	if p.CleanupBatchLimit < 100 {
		p.CleanupBatchLimit = 100
	}
	if p.CleanupBatchLimit > 5000 {
		p.CleanupBatchLimit = 5000
	}
}

// AssetDeletionPolicy 控制“最近删除”与最小墓碑的生命周期。
// TombstoneRetentionDays=0 表示永久保留最小墓碑，这是最稳妥的防复活策略。
type AssetDeletionPolicy struct {
	RecentDeletedRetentionDays int  `json:"recent_deleted_retention_days"`
	TombstoneRetentionDays     int  `json:"tombstone_retention_days"`
	CleanupBatchLimit          int  `json:"cleanup_batch_limit"`
	AutoCleanupEnabled         bool `json:"auto_cleanup_enabled"`
}

func DefaultAssetDeletionPolicy() AssetDeletionPolicy {
	return AssetDeletionPolicy{
		RecentDeletedRetentionDays: 90,
		TombstoneRetentionDays:     0,
		CleanupBatchLimit:          500,
		AutoCleanupEnabled:         true,
	}
}

func (p *AssetDeletionPolicy) Normalize() {
	if p.RecentDeletedRetentionDays < 7 {
		p.RecentDeletedRetentionDays = 7
	}
	if p.RecentDeletedRetentionDays > 3650 {
		p.RecentDeletedRetentionDays = 3650
	}

	// 非永久墓碑至少保留一年，降低长期离线设备重新上传旧资产的风险。
	if p.TombstoneRetentionDays != 0 && p.TombstoneRetentionDays < 365 {
		p.TombstoneRetentionDays = 365
	}
	if p.TombstoneRetentionDays > 3650 {
		p.TombstoneRetentionDays = 3650
	}

	if p.CleanupBatchLimit < 100 {
		p.CleanupBatchLimit = 100
	}
	if p.CleanupBatchLimit > 5000 {
		p.CleanupBatchLimit = 5000
	}
}
