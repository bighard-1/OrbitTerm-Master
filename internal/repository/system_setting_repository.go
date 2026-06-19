package repository

import (
	"errors"

	"orbitterm-server/internal/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type SystemSettingRepository interface {
	FindByKey(key string) (*model.SystemSetting, error)
	Upsert(setting *model.SystemSetting) error
}

type systemSettingRepository struct {
	db *gorm.DB
}

func NewSystemSettingRepository(db *gorm.DB) SystemSettingRepository {
	return &systemSettingRepository{db: db}
}

func (r *systemSettingRepository) FindByKey(key string) (*model.SystemSetting, error) {
	var setting model.SystemSetting
	err := r.db.Where("key = ?", key).First(&setting).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &setting, nil
}

func (r *systemSettingRepository) Upsert(setting *model.SystemSetting) error {
	return r.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "key"}},
		DoUpdates: clause.AssignmentColumns([]string{"value", "updated_at"}),
	}).Create(setting).Error
}
