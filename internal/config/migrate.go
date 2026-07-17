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
		&model.ConfigSyncChange{},
		&model.SyncDeviceState{},
		&model.AdminAuditLog{},
		&model.SystemSetting{},
		&model.RegistrationInvite{},
	); err != nil {
		return fmt.Errorf("auto migrate database: %w", err)
	}

	if err := enforceCanonicalUsernameUniqueness(db); err != nil {
		return err
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

	// 为升级前的配置建立初始修订记录，使新版客户端第一次 cursor=0 时能够完整拉取。
	if err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`
			INSERT INTO config_sync_changes (user_id, config_id, asset_id, state, operation_id, changed_at)
			SELECT sc.user_id, sc.id, sc.asset_id, sc.state, sc.last_operation_id, sc.updated_at
			FROM server_configs sc
			WHERE sc.server_revision = 0
			  AND NOT EXISTS (
				SELECT 1 FROM config_sync_changes c WHERE c.config_id = sc.id
			  )
		`).Error; err != nil {
			return err
		}
		return tx.Exec(`
			UPDATE server_configs sc
			SET server_revision = latest.revision
			FROM (
				SELECT config_id, MAX(id) AS revision
				FROM config_sync_changes
				GROUP BY config_id
			) latest
			WHERE sc.id = latest.config_id AND sc.server_revision = 0
		`).Error
	}); err != nil {
		return fmt.Errorf("backfill config sync revisions: %w", err)
	}

	return nil
}

// enforceCanonicalUsernameUniqueness makes the database enforce the same
// account identity rule as the service layer. Historical case-only conflicts
// are intentionally not merged automatically: their encrypted assets belong
// to distinct user IDs and require an operator-approved recovery decision.
func enforceCanonicalUsernameUniqueness(db *gorm.DB) error {
	var duplicateGroups int64
	if err := db.Raw(`
		SELECT COUNT(*)
		FROM (
			SELECT LOWER(BTRIM(username)) AS canonical_username
			FROM users
			GROUP BY LOWER(BTRIM(username))
			HAVING COUNT(*) > 1
		) AS duplicate_usernames
	`).Scan(&duplicateGroups).Error; err != nil {
		return fmt.Errorf("audit canonical usernames: %w", err)
	}
	if duplicateGroups > 0 {
		return fmt.Errorf(
			"refusing username canonicalization: %d duplicate canonical username group(s) require explicit operator recovery before deployment",
			duplicateGroups,
		)
	}

	if err := db.Exec(`
		UPDATE users
		SET username = LOWER(BTRIM(username)),
		    token_version = token_version + 1
		WHERE username <> LOWER(BTRIM(username))
	`).Error; err != nil {
		return fmt.Errorf("normalize stored usernames: %w", err)
	}

	if err := db.Exec(`
		CREATE UNIQUE INDEX IF NOT EXISTS uq_users_canonical_username
		ON users (LOWER(BTRIM(username)))
	`).Error; err != nil {
		return fmt.Errorf("create canonical username index: %w", err)
	}
	return nil
}
