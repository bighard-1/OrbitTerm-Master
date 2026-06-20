package config

import (
	"fmt"

	"orbitterm-server/internal/model"

	"gorm.io/gorm"
)

// MigrateDatabase 集中管理兼容性数据库迁移，避免启动入口持续堆叠迁移细节。
// 新增的墓碑字段均有安全默认值；历史配置会保持 active，并等待新版客户端回填 AssetID。
func MigrateDatabase(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("database is nil")
	}

	if err := db.AutoMigrate(
		&model.User{},
		&model.ServerConfig{},
		&model.AdminAuditLog{},
		&model.SystemSetting{},
	); err != nil {
		return fmt.Errorf("auto migrate database: %w", err)
	}

	result := db.Model(&model.ServerConfig{}).
		Where("state IS NULL OR state = ''").
		Update("state", model.ServerConfigStateActive)
	if result.Error != nil {
		return fmt.Errorf("backfill server config state: %w", result.Error)
	}

	// 历史记录的 AssetID 为空，因此使用 PostgreSQL 部分唯一索引：
	// 既允许旧记录渐进回填，又阻止新版客户端为同一用户创建重复逻辑资产。
	if err := db.Exec(`
		CREATE UNIQUE INDEX IF NOT EXISTS uq_server_configs_user_asset_nonempty
		ON server_configs (user_id, asset_id)
		WHERE asset_id <> ''
	`).Error; err != nil {
		return fmt.Errorf("create server config asset identity index: %w", err)
	}

	return nil
}
