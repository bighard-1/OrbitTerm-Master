package model

import (
	"strings"
	"time"
)

const (
	SystemSettingKeySecurityPolicy = "security_policy"

	AuditActionSystemSecurityPolicyUpdate = "system_security_policy_update"
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
