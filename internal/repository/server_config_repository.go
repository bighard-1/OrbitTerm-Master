package repository

import (
	"errors"

	"orbitterm-server/internal/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ServerConfigRepository 封装配置同步相关数据访问逻辑。
type ServerConfigRepository interface {
	Create(config *model.ServerConfig) error
	Update(config *model.ServerConfig) error
	FindByIDAndUserID(id, userID uint) (*model.ServerConfig, error)
	FindByAssetIDAndUserID(assetID string, userID uint) (*model.ServerConfig, error)
	MutateByAssetID(userID uint, assetID string, mutate func(*model.ServerConfig) (bool, error)) (*model.ServerConfig, error)
	ListByUserID(userID uint) ([]model.ServerConfig, error)
	ListTrashByUserID(userID uint, limit, offset int) ([]model.ServerConfig, int64, error)
	CountAll() (int64, error)
	DeleteByIDAndUserID(id, userID uint) (bool, error)
}

func (r *serverConfigRepository) FindByAssetIDAndUserID(assetID string, userID uint) (*model.ServerConfig, error) {
	var cfg model.ServerConfig
	err := r.db.Where("asset_id = ? AND user_id = ?", assetID, userID).First(&cfg).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &cfg, nil
}

// MutateByAssetID 在事务内锁定单个资产并执行状态转换，避免删除、恢复和清理并发覆盖。
func (r *serverConfigRepository) MutateByAssetID(
	userID uint,
	assetID string,
	mutate func(*model.ServerConfig) (bool, error),
) (*model.ServerConfig, error) {
	var result *model.ServerConfig
	err := r.db.Transaction(func(tx *gorm.DB) error {
		var cfg model.ServerConfig
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("user_id = ? AND asset_id = ?", userID, assetID).
			First(&cfg).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}

		changed, err := mutate(&cfg)
		if err != nil {
			return err
		}
		if changed {
			if err := tx.Save(&cfg).Error; err != nil {
				return err
			}
		}
		result = &cfg
		return nil
	})
	return result, err
}

type serverConfigRepository struct {
	db *gorm.DB
}

func NewServerConfigRepository(db *gorm.DB) ServerConfigRepository {
	return &serverConfigRepository{db: db}
}

func (r *serverConfigRepository) Create(config *model.ServerConfig) error {
	return r.db.Create(config).Error
}

func (r *serverConfigRepository) Update(config *model.ServerConfig) error {
	return r.db.Save(config).Error
}

func (r *serverConfigRepository) FindByIDAndUserID(id, userID uint) (*model.ServerConfig, error) {
	var cfg model.ServerConfig
	err := r.db.Where("id = ? AND user_id = ?", id, userID).First(&cfg).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &cfg, nil
}

func (r *serverConfigRepository) ListByUserID(userID uint) ([]model.ServerConfig, error) {
	var configs []model.ServerConfig
	err := r.db.Where("user_id = ? AND state = ?", userID, model.ServerConfigStateActive).
		Order("id ASC").Find(&configs).Error
	if err != nil {
		return nil, err
	}
	return configs, nil
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

func (r *serverConfigRepository) CountAll() (int64, error) {
	var count int64
	err := r.db.Model(&model.ServerConfig{}).Count(&count).Error
	return count, err
}

func (r *serverConfigRepository) DeleteByIDAndUserID(id, userID uint) (bool, error) {
	result := r.db.Where("id = ? AND user_id = ?", id, userID).Delete(&model.ServerConfig{})
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}
