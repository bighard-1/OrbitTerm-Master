package repository

import (
	"orbitterm-server/internal/model"

	"gorm.io/gorm"
)

type AdminAuditRepository interface {
	Create(log *model.AdminAuditLog) error
	List(limit int) ([]model.AdminAuditLog, error)
}

type adminAuditRepository struct {
	db *gorm.DB
}

func NewAdminAuditRepository(db *gorm.DB) AdminAuditRepository {
	return &adminAuditRepository{db: db}
}

func (r *adminAuditRepository) Create(log *model.AdminAuditLog) error {
	return r.db.Create(log).Error
}

func (r *adminAuditRepository) List(limit int) ([]model.AdminAuditLog, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}

	var logs []model.AdminAuditLog
	err := r.db.Order("id DESC").Limit(limit).Find(&logs).Error
	if err != nil {
		return nil, err
	}
	return logs, nil
}
