package model

import "time"

// User 表示系统用户。
// 密码字段必须存储 Argon2id 生成的哈希字符串，严禁保存明文密码。
type User struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	Username     string    `gorm:"size:64;uniqueIndex;not null" json:"username"`
	PasswordHash string    `gorm:"size:512;not null" json:"-"`
	CreatedAt    time.Time `gorm:"not null;autoCreateTime" json:"created_at"`
}

// TableName 显式指定表名，避免未来结构重构时出现不必要的迁移歧义。
func (User) TableName() string {
	return "users"
}
