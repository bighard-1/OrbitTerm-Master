package repository

import (
	"time"

	"orbitterm-server/internal/model"

	"gorm.io/gorm"
)

type AdminAuditRepository interface {
	Create(log *model.AdminAuditLog) error
	List(limit int) ([]model.AdminAuditLog, error)
	ListWithFilter(filter AdminAuditListFilter) ([]model.AdminAuditLog, int64, error)
	DeleteOlderThan(cutoff time.Time, limit int) (int64, error)
}

type AdminAuditListFilter struct {
	Action       string
	ResourceType string
	AdminUserID  uint
	TargetUserID uint
	Limit        int
	Offset       int
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
	logs, _, err := r.ListWithFilter(AdminAuditListFilter{Limit: limit})
	return logs, err
}

func (r *adminAuditRepository) ListWithFilter(filter AdminAuditListFilter) ([]model.AdminAuditLog, int64, error) {
	limit := filter.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}

	query := r.db.Model(&model.AdminAuditLog{})
	if filter.Action != "" {
		query = query.Where("action = ?", filter.Action)
	}
	if filter.ResourceType != "" {
		query = query.Where("resource_type = ?", filter.ResourceType)
	}
	if filter.AdminUserID != 0 {
		query = query.Where("admin_user_id = ?", filter.AdminUserID)
	}
	if filter.TargetUserID != 0 {
		query = query.Where("target_user_id = ?", filter.TargetUserID)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var logs []model.AdminAuditLog
	err := query.Order("id DESC").Limit(limit).Offset(offset).Find(&logs).Error
	if err != nil {
		return nil, 0, err
	}
	return logs, total, nil
}

func (r *adminAuditRepository) DeleteOlderThan(cutoff time.Time, limit int) (int64, error) {
	if limit <= 0 || limit > 5000 {
		limit = 500
	}

	var ids []uint
	if err := r.db.Model(&model.AdminAuditLog{}).
		Where("created_at < ?", cutoff).
		Order("id ASC").
		Limit(limit).
		Pluck("id", &ids).Error; err != nil {
		return 0, err
	}
	if len(ids) == 0 {
		return 0, nil
	}

	result := r.db.Where("id IN ?", ids).Delete(&model.AdminAuditLog{})
	return result.RowsAffected, result.Error
}
