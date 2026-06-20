package model

import "time"

// ConfigSyncChange 为每次配置状态变化分配全局单调递增游标。
// 它不保存密文或资产明文，只用于增量拉取、设备确认和墓碑清理判断。
type ConfigSyncChange struct {
	ID uint64 `gorm:"primaryKey;autoIncrement" json:"revision"`

	UserID   uint   `gorm:"not null;index:idx_config_changes_user_revision,priority:1" json:"user_id"`
	ConfigID uint   `gorm:"not null;index" json:"config_id"`
	AssetID  string `gorm:"size:36;not null;default:'';index" json:"asset_id,omitempty"`
	State    string `gorm:"size:16;not null" json:"state"`

	OperationID string    `gorm:"size:64;index" json:"operation_id,omitempty"`
	ChangedAt   time.Time `gorm:"not null;autoCreateTime;index:idx_config_changes_user_revision,priority:2" json:"changed_at"`
}

func (ConfigSyncChange) TableName() string {
	return "config_sync_changes"
}

// SyncDeviceState 保存设备已经应用的服务端修订号，不包含设备名称或硬件标识。
type SyncDeviceState struct {
	ID uint `gorm:"primaryKey" json:"id"`

	UserID   uint   `gorm:"not null;uniqueIndex:uq_sync_device_user_device,priority:1" json:"user_id"`
	DeviceID string `gorm:"size:36;not null;uniqueIndex:uq_sync_device_user_device,priority:2" json:"device_id"`

	LastAckRevision uint64    `gorm:"not null;default:0;index" json:"last_ack_revision"`
	Platform        string    `gorm:"size:32" json:"platform,omitempty"`
	ClientVersion   string    `gorm:"size:32" json:"client_version,omitempty"`
	LastSeenAt      time.Time `gorm:"not null;index" json:"last_seen_at"`
	CreatedAt       time.Time `gorm:"not null;autoCreateTime" json:"created_at"`
	UpdatedAt       time.Time `gorm:"not null;autoUpdateTime" json:"updated_at"`
}

func (SyncDeviceState) TableName() string {
	return "sync_device_states"
}
