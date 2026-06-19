package repository

import (
	"errors"
	"time"

	"orbitterm-server/internal/model"

	"gorm.io/gorm"
)

// UserRepository 封装用户数据访问逻辑，避免业务层直接依赖 ORM 细节。
type UserRepository interface {
	Create(user *model.User) error
	Save(user *model.User) error
	FindByUsername(username string) (*model.User, error)
	FindByID(id uint) (*model.User, error)
	CountByRoles(roles []string) (int64, error)
	CountByStatus(status string) (int64, error)
	CountAll() (int64, error)
	ListExpiredBans(now time.Time, limit int) ([]model.User, error)
	List(filter UserListFilter) ([]model.User, int64, error)
}

type UserListFilter struct {
	Query  string
	Role   string
	Status string
	Limit  int
	Offset int
}

type userRepository struct {
	db *gorm.DB
}

func NewUserRepository(db *gorm.DB) UserRepository {
	return &userRepository{db: db}
}

func (r *userRepository) Create(user *model.User) error {
	return r.db.Create(user).Error
}

func (r *userRepository) Save(user *model.User) error {
	return r.db.Save(user).Error
}

func (r *userRepository) FindByUsername(username string) (*model.User, error) {
	var user model.User
	err := r.db.Where("username = ?", username).First(&user).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &user, nil
}

func (r *userRepository) FindByID(id uint) (*model.User, error) {
	var user model.User
	err := r.db.Where("id = ?", id).First(&user).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &user, nil
}

func (r *userRepository) CountByRoles(roles []string) (int64, error) {
	if len(roles) == 0 {
		return 0, nil
	}

	var count int64
	err := r.db.Model(&model.User{}).Where("role IN ?", roles).Count(&count).Error
	return count, err
}

func (r *userRepository) CountByStatus(status string) (int64, error) {
	var count int64
	query := r.db.Model(&model.User{})
	if status != "" {
		query = query.Where("status = ?", status)
	}
	err := query.Count(&count).Error
	return count, err
}

func (r *userRepository) CountAll() (int64, error) {
	return r.CountByStatus("")
}

func (r *userRepository) ListExpiredBans(now time.Time, limit int) ([]model.User, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}

	var users []model.User
	err := r.db.
		Where("is_banned = ? AND ban_until IS NOT NULL AND ban_until <= ?", true, now).
		Order("ban_until ASC").
		Limit(limit).
		Find(&users).Error
	return users, err
}

func (r *userRepository) List(filter UserListFilter) ([]model.User, int64, error) {
	limit := filter.Limit
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}

	query := r.db.Model(&model.User{})
	if filter.Query != "" {
		like := "%" + filter.Query + "%"
		query = query.Where("username ILIKE ?", like)
	}
	if filter.Role != "" {
		query = query.Where("role = ?", filter.Role)
	}
	if filter.Status != "" {
		query = query.Where("status = ?", filter.Status)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var users []model.User
	if err := query.Order("id DESC").Limit(limit).Offset(offset).Find(&users).Error; err != nil {
		return nil, 0, err
	}
	return users, total, nil
}
