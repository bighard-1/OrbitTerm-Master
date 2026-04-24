package model

import "time"

// ServerConfig 存储用户的服务端配置快照。
// EncryptedBlob 存的是加密后的二进制数据，服务端不会保存明文配置。
// VectorClock 用字符串形式存储版本向量（建议 JSON 字符串），用于多端冲突检测。
// 重要审计说明：后端不负责解密，仅负责存储加密后的密文。
type ServerConfig struct {
	ID            uint      `gorm:"primaryKey" json:"id"`
	UserID        uint      `gorm:"not null;index" json:"user_id"`
	EncryptedBlob []byte    `gorm:"type:bytea;not null" json:"encrypted_blob"`
	VectorClock   string    `gorm:"type:text;not null;default:'{}'" json:"vector_clock"`
	UpdatedAt     time.Time `gorm:"not null;autoUpdateTime" json:"updated_at"`

	// User 是外键关联，用于确保 ServerConfig 与 User 通过 UserID 正确绑定。
	User User `gorm:"foreignKey:UserID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;" json:"-"`
}

func (ServerConfig) TableName() string {
	return "server_configs"
}
