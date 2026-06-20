package repository

import (
	"errors"
	"time"

	"orbitterm-server/internal/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrLegacyDeleteProtected = errors.New("asset has migrated to tombstone sync")

// ServerConfigRepository 封装配置同步相关数据访问逻辑。
type ServerConfigRepository interface {
	Create(config *model.ServerConfig) error
	Update(config *model.ServerConfig) error
	FindByIDAndUserID(id, userID uint) (*model.ServerConfig, error)
	FindByAssetIDAndUserID(assetID string, userID uint) (*model.ServerConfig, error)
	ListByIdentityFingerprint(userID uint, fingerprint string) ([]model.ServerConfig, error)
	MutateByAssetID(userID uint, assetID string, mutate func(*model.ServerConfig) (bool, error)) (*model.ServerConfig, error)
	ListByUserID(userID uint) ([]model.ServerConfig, error)
	ListTrashByUserID(userID uint, limit, offset int) ([]model.ServerConfig, int64, error)
	ListChangedByUserID(userID uint, afterRevision uint64, limit int) ([]model.ServerConfig, bool, error)
	MaxRevisionByUserID(userID uint) (uint64, error)
	AcknowledgeDevice(userID uint, deviceID string, revision uint64, platform, clientVersion string, seenAt time.Time) error
	ListExpiredDeleted(now time.Time, limit int) ([]model.ServerConfig, error)
	DeleteAcknowledgedPurgedBefore(cutoff time.Time, limit int) (deleted int64, deferred int64, err error)
	CountAll() (int64, error)
	DeleteByIDAndUserID(id, userID uint) (bool, error)
}

type serverConfigRepository struct {
	db *gorm.DB
}

func NewServerConfigRepository(db *gorm.DB) ServerConfigRepository {
	return &serverConfigRepository{db: db}
}

func (r *serverConfigRepository) Create(config *model.ServerConfig) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(config).Error; err != nil {
			return err
		}
		return appendConfigChange(tx, config)
	})
}

func (r *serverConfigRepository) Update(config *model.ServerConfig) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(config).Error; err != nil {
			return err
		}
		return appendConfigChange(tx, config)
	})
}

func (r *serverConfigRepository) FindByIDAndUserID(id, userID uint) (*model.ServerConfig, error) {
	var config model.ServerConfig
	err := r.db.Where("id = ? AND user_id = ?", id, userID).First(&config).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &config, nil
}

func (r *serverConfigRepository) FindByAssetIDAndUserID(assetID string, userID uint) (*model.ServerConfig, error) {
	var config model.ServerConfig
	err := r.db.Where("asset_id = ? AND user_id = ?", assetID, userID).First(&config).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &config, nil
}

func (r *serverConfigRepository) ListByIdentityFingerprint(userID uint, fingerprint string) ([]model.ServerConfig, error) {
	var configs []model.ServerConfig
	err := r.db.Where("user_id = ? AND identity_fingerprint = ?", userID, fingerprint).
		Order("updated_at DESC, id DESC").Limit(20).Find(&configs).Error
	return configs, err
}

// MutateByAssetID 在事务内锁定单个资产并执行状态转换，避免删除、恢复和清理并发覆盖。
func (r *serverConfigRepository) MutateByAssetID(
	userID uint,
	assetID string,
	mutate func(*model.ServerConfig) (bool, error),
) (*model.ServerConfig, error) {
	var result *model.ServerConfig
	err := r.db.Transaction(func(tx *gorm.DB) error {
		var config model.ServerConfig
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("user_id = ? AND asset_id = ?", userID, assetID).
			First(&config).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}

		changed, err := mutate(&config)
		if err != nil {
			return err
		}
		if changed {
			if err := tx.Save(&config).Error; err != nil {
				return err
			}
			if err := appendConfigChange(tx, &config); err != nil {
				return err
			}
		}
		result = &config
		return nil
	})
	return result, err
}

func appendConfigChange(tx *gorm.DB, config *model.ServerConfig) error {
	change := &model.ConfigSyncChange{
		UserID:      config.UserID,
		ConfigID:    config.ID,
		AssetID:     config.AssetID,
		State:       config.State,
		OperationID: config.LastOperationID,
	}
	if err := tx.Create(change).Error; err != nil {
		return err
	}
	if err := tx.Model(&model.ServerConfig{}).
		Where("id = ? AND user_id = ?", config.ID, config.UserID).
		UpdateColumn("server_revision", change.ID).Error; err != nil {
		return err
	}
	config.ServerRevision = change.ID
	return nil
}

func (r *serverConfigRepository) ListByUserID(userID uint) ([]model.ServerConfig, error) {
	var configs []model.ServerConfig
	err := r.db.Where("user_id = ? AND state = ?", userID, model.ServerConfigStateActive).
		Order("id ASC").Find(&configs).Error
	return configs, err
}

func (r *serverConfigRepository) ListTrashByUserID(userID uint, limit, offset int) ([]model.ServerConfig, int64, error) {
	query := r.db.Model(&model.ServerConfig{}).
		Where("user_id = ? AND state = ?", userID, model.ServerConfigStateDeleted)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var configs []model.ServerConfig
	if err := query.Order("deleted_at DESC, id DESC").Limit(limit).Offset(offset).Find(&configs).Error; err != nil {
		return nil, 0, err
	}
	return configs, total, nil
}

func (r *serverConfigRepository) ListChangedByUserID(userID uint, afterRevision uint64, limit int) ([]model.ServerConfig, bool, error) {
	var configs []model.ServerConfig
	err := r.db.Where("user_id = ? AND server_revision > ?", userID, afterRevision).
		Order("server_revision ASC").Limit(limit + 1).Find(&configs).Error
	if err != nil {
		return nil, false, err
	}
	hasMore := len(configs) > limit
	if hasMore {
		configs = configs[:limit]
	}
	return configs, hasMore, nil
}

func (r *serverConfigRepository) MaxRevisionByUserID(userID uint) (uint64, error) {
	var revision uint64
	err := r.db.Model(&model.ServerConfig{}).Where("user_id = ?", userID).
		Select("COALESCE(MAX(server_revision), 0)").Scan(&revision).Error
	return revision, err
}

func (r *serverConfigRepository) AcknowledgeDevice(
	userID uint,
	deviceID string,
	revision uint64,
	platform string,
	clientVersion string,
	seenAt time.Time,
) error {
	state := &model.SyncDeviceState{
		UserID:          userID,
		DeviceID:        deviceID,
		LastAckRevision: revision,
		Platform:        platform,
		ClientVersion:   clientVersion,
		LastSeenAt:      seenAt,
	}
	return r.db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "user_id"}, {Name: "device_id"}},
		DoUpdates: clause.Assignments(map[string]any{
			"last_ack_revision": gorm.Expr("GREATEST(sync_device_states.last_ack_revision, EXCLUDED.last_ack_revision)"),
			"platform":          platform,
			"client_version":    clientVersion,
			"last_seen_at":      seenAt,
			"updated_at":        seenAt,
		}),
	}).Create(state).Error
}

// ListExpiredDeleted 返回恢复窗口已经结束、等待清除密文的资产。
// 实际状态转换仍由 Service 通过 MutateByAssetID 完成，保证向量钟和修订日志一致。
func (r *serverConfigRepository) ListExpiredDeleted(now time.Time, limit int) ([]model.ServerConfig, error) {
	if limit <= 0 || limit > 5000 {
		limit = 500
	}
	var configs []model.ServerConfig
	err := r.db.Where("state = ? AND purge_after IS NOT NULL AND purge_after <= ?", model.ServerConfigStateDeleted, now).
		Order("purge_after ASC, id ASC").Limit(limit).Find(&configs).Error
	return configs, err
}

// DeleteAcknowledgedPurgedBefore 只物理回收所有已登记设备均确认过的最小墓碑。
// 没有设备确认记录，或任一设备仍落后于墓碑修订时，一律延期，优先保证防复活安全性。
func (r *serverConfigRepository) DeleteAcknowledgedPurgedBefore(cutoff time.Time, limit int) (int64, int64, error) {
	if limit <= 0 || limit > 5000 {
		limit = 500
	}

	var candidates []uint
	if err := r.db.Model(&model.ServerConfig{}).
		Where("state = ? AND updated_at <= ?", model.ServerConfigStatePurged, cutoff).
		Order("updated_at ASC, id ASC").Limit(limit).Pluck("id", &candidates).Error; err != nil {
		return 0, 0, err
	}
	if len(candidates) == 0 {
		return 0, 0, nil
	}

	var eligible []uint
	err := r.db.Raw(`
		SELECT sc.id
		FROM server_configs sc
		WHERE sc.id IN ?
		  AND EXISTS (
		      SELECT 1 FROM sync_device_states s
		      WHERE s.user_id = sc.user_id
		  )
		  AND NOT EXISTS (
		      SELECT 1 FROM sync_device_states s
		      WHERE s.user_id = sc.user_id
		        AND s.last_ack_revision < sc.server_revision
		  )
		ORDER BY sc.updated_at ASC, sc.id ASC
	`, candidates).Scan(&eligible).Error
	if err != nil {
		return 0, 0, err
	}
	deferred := int64(len(candidates) - len(eligible))
	if len(eligible) == 0 {
		return 0, deferred, nil
	}

	result := r.db.Unscoped().Where("id IN ? AND state = ?", eligible, model.ServerConfigStatePurged).
		Delete(&model.ServerConfig{})
	return result.RowsAffected, deferred, result.Error
}

func (r *serverConfigRepository) CountAll() (int64, error) {
	var count int64
	err := r.db.Model(&model.ServerConfig{}).Count(&count).Error
	return count, err
}

// DeleteByIDAndUserID 仅允许物理删除尚未迁移 AssetID 的旧记录。
func (r *serverConfigRepository) DeleteByIDAndUserID(id, userID uint) (bool, error) {
	deleted := false
	err := r.db.Transaction(func(tx *gorm.DB) error {
		var config model.ServerConfig
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND user_id = ?", id, userID).First(&config).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if config.AssetID != "" {
			return ErrLegacyDeleteProtected
		}
		if err := tx.Delete(&config).Error; err != nil {
			return err
		}
		deleted = true
		return nil
	})
	return deleted, err
}
