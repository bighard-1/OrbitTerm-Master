package repository

import (
	"errors"
	"time"

	"orbitterm-server/internal/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrInviteNotFound    = errors.New("registration invite not found")
	ErrInviteUnavailable = errors.New("registration invite unavailable")
)

type RegistrationInviteRepository interface {
	Create(invite *model.RegistrationInvite) error
	List(limit, offset int) ([]model.RegistrationInvite, int64, error)
	Disable(id uint, now time.Time) (*model.RegistrationInvite, error)
	Consume(codeHash string, now time.Time) (*model.RegistrationInvite, error)
	Release(codeHash string) error
}

type registrationInviteRepository struct{ db *gorm.DB }

func NewRegistrationInviteRepository(db *gorm.DB) RegistrationInviteRepository {
	return &registrationInviteRepository{db: db}
}

func (r *registrationInviteRepository) Create(invite *model.RegistrationInvite) error {
	return r.db.Create(invite).Error
}

func (r *registrationInviteRepository) List(limit, offset int) ([]model.RegistrationInvite, int64, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	var total int64
	if err := r.db.Model(&model.RegistrationInvite{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var invites []model.RegistrationInvite
	err := r.db.Order("id DESC").Limit(limit).Offset(offset).Find(&invites).Error
	return invites, total, err
}

func (r *registrationInviteRepository) Disable(id uint, now time.Time) (*model.RegistrationInvite, error) {
	var invite model.RegistrationInvite
	err := r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&invite, id).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrInviteNotFound
			}
			return err
		}
		if invite.DisabledAt == nil {
			invite.DisabledAt = &now
			return tx.Save(&invite).Error
		}
		return nil
	})
	return &invite, err
}

func (r *registrationInviteRepository) Consume(codeHash string, now time.Time) (*model.RegistrationInvite, error) {
	var invite model.RegistrationInvite
	err := r.db.Transaction(func(tx *gorm.DB) error {
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("code_hash = ?", codeHash).
			First(&invite).Error
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrInviteNotFound
			}
			return err
		}
		if !invite.AvailableAt(now) {
			return ErrInviteUnavailable
		}
		invite.UseCount++
		invite.LastUsedAt = &now
		return tx.Save(&invite).Error
	})
	return &invite, err
}

func (r *registrationInviteRepository) Release(codeHash string) error {
	result := r.db.Model(&model.RegistrationInvite{}).
		Where("code_hash = ? AND use_count > 0", codeHash).
		Updates(map[string]any{"use_count": gorm.Expr("use_count - 1"), "last_used_at": nil})
	return result.Error
}
