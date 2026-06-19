package repository

import (
	"errors"

	"orbitterm-server/internal/model"

	"gorm.io/gorm"
)

// ServerConfigRepository 封装配置同步相关数据访问逻辑。
type ServerConfigRepository interface {
	Create(config *model.ServerConfig) error
	Update(config *model.ServerConfig) error
	FindByIDAndUserID(id, userID uint) (*model.ServerConfig, error)
	ListByUserID(userID uint) ([]model.ServerConfig, error)
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
	err := r.db.Where("user_id = ?", userID).Order("id ASC").Find(&configs).Error
	if err != nil {
		return nil, err
	}
	return configs, nil
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
