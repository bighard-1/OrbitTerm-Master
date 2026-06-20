package model

import "time"

const (
	ServerConfigStateActive  = "active"
	ServerConfigStateDeleted = "deleted"
	ServerConfigStatePurged  = "purged"
)

// ServerConfig 存储用户的服务端配置快照。
// EncryptedBlob 存的是加密后的二进制数据，服务端不会保存明文配置。
// VectorClock 用字符串形式存储版本向量（建议 JSON 字符串），用于多端冲突检测。
// 重要审计说明：后端不负责解密，仅负责存储加密后的密文。
type ServerConfig struct {
	ID     uint `gorm:"primaryKey" json:"id"`
	UserID uint `gorm:"not null;index;index:idx_server_configs_user_asset,priority:1;index:idx_server_configs_user_fingerprint,priority:1" json:"user_id"`

	// AssetID 是客户端生成的随机 UUID，用于跨设备识别同一逻辑资产。
	// 历史记录在客户端完成解密并回填前允许为空；服务端不能从密文中推导该值。
	AssetID string `gorm:"size:36;not null;default:'';index:idx_server_configs_user_asset,priority:2" json:"asset_id,omitempty"`

	// IdentityFingerprint 是客户端使用账户同步密钥计算的 HMAC-SHA256 十六进制摘要。
	// 服务端只能用它提示“活动/最近删除中存在相同连接身份”，无法反推出主机或用户名。
	IdentityFingerprint string `gorm:"size:64;index:idx_server_configs_user_fingerprint,priority:2" json:"identity_fingerprint,omitempty"`

	EncryptedBlob []byte `gorm:"type:bytea;not null" json:"encrypted_blob"`
	VectorClock   string `gorm:"type:text;not null;default:'{}'" json:"vector_clock"`

	// State 与删除元数据构成云端权威墓碑。deleted 阶段保留密文以支持恢复；
	// purged 阶段仅保留不可逆的最小墓碑，防止长期离线设备让旧资产复活。
	State             string     `gorm:"size:16;not null;default:'active';index" json:"state"`
	DeletedAt         *time.Time `gorm:"index" json:"deleted_at,omitempty"`
	PurgeAfter        *time.Time `gorm:"index" json:"purge_after,omitempty"`
	DeletedByDeviceID string     `gorm:"size:128" json:"deleted_by_device_id,omitempty"`
	LastOperationID   string     `gorm:"size:64;index" json:"last_operation_id,omitempty"`
	ServerRevision    uint64     `gorm:"not null;default:0;index" json:"server_revision"`

	UpdatedAt time.Time `gorm:"not null;autoUpdateTime;index" json:"updated_at"`

	// User 是外键关联，用于确保 ServerConfig 与 User 通过 UserID 正确绑定。
	User User `gorm:"foreignKey:UserID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;" json:"-"`
}

func (c ServerConfig) IsDeleted() bool {
	return c.State == ServerConfigStateDeleted || c.State == ServerConfigStatePurged
}

func IsValidServerConfigState(state string) bool {
	switch state {
	case ServerConfigStateActive, ServerConfigStateDeleted, ServerConfigStatePurged:
		return true
	default:
		return false
	}
}

func (ServerConfig) TableName() string {
	return "server_configs"
}
