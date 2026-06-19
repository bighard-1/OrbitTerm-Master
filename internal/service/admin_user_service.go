package service

import (
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"orbitterm-server/internal/model"
	"orbitterm-server/internal/repository"
	"orbitterm-server/internal/utils"
)

var (
	ErrAdminTargetNotFound = errors.New("目标用户不存在")
	ErrAdminInvalidAction  = errors.New("管理操作不合法")
	ErrAdminReasonRequired = errors.New("管理操作原因必填")
)

type AdminUserService interface {
	ListUsers(filter AdminUserListFilter) ([]model.User, int64, error)
	GetUser(id uint) (*model.User, error)
	BanUser(adminID, targetUserID uint, durationMinutes *int, reason string, meta AdminRequestMeta) (*model.User, error)
	UnbanUser(adminID, targetUserID uint, reason string, meta AdminRequestMeta) (*model.User, error)
	ResetPassword(adminID, targetUserID uint, newPassword, reason string, meta AdminRequestMeta) (*model.User, error)
	ForceLogout(adminID, targetUserID uint, reason string, meta AdminRequestMeta) (*model.User, error)
	SoftDeleteUser(adminID, targetUserID uint, reason string, meta AdminRequestMeta) (*model.User, error)
	RestoreUser(adminID, targetUserID uint, reason string, meta AdminRequestMeta) (*model.User, error)
	ScanExpiredBans(adminID uint, limit int, reason string, meta AdminRequestMeta) (*AdminExpiredBanScanResult, error)
	ScanExpiredBansBySystem(limit int, reason string) (*AdminExpiredBanScanResult, error)
}

type AdminUserListFilter struct {
	Query  string
	Role   string
	Status string
	Limit  int
	Offset int
}

type AdminRequestMeta struct {
	IPAddress string
	UserAgent string
}

type AdminExpiredBanScanResult struct {
	ScannedCount  int                   `json:"scanned_count"`
	UnbannedCount int                   `json:"unbanned_count"`
	Items         []AdminExpiredBanItem `json:"items"`
}

type AdminExpiredBanItem struct {
	UserID   uint   `json:"user_id"`
	Username string `json:"username"`
	Status   string `json:"status"`
}

type adminUserService struct {
	userRepo     repository.UserRepository
	auditService AdminAuditService
	now          func() time.Time
}

func NewAdminUserService(userRepo repository.UserRepository, auditService AdminAuditService) AdminUserService {
	return &adminUserService{
		userRepo:     userRepo,
		auditService: auditService,
		now: func() time.Time {
			return time.Now().UTC()
		},
	}
}

func (s *adminUserService) ListUsers(filter AdminUserListFilter) ([]model.User, int64, error) {
	return s.userRepo.List(repository.UserListFilter{
		Query:  strings.TrimSpace(filter.Query),
		Role:   strings.TrimSpace(filter.Role),
		Status: strings.TrimSpace(filter.Status),
		Limit:  filter.Limit,
		Offset: filter.Offset,
	})
}

func (s *adminUserService) GetUser(id uint) (*model.User, error) {
	if id == 0 {
		return nil, ErrInvalidInput
	}
	user, err := s.userRepo.FindByID(id)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, ErrAdminTargetNotFound
	}
	return user, nil
}

func (s *adminUserService) BanUser(adminID, targetUserID uint, durationMinutes *int, reason string, meta AdminRequestMeta) (*model.User, error) {
	if adminID == 0 || targetUserID == 0 {
		return nil, ErrInvalidInput
	}
	reason = strings.TrimSpace(reason)
	if !validAdminReason(reason) {
		return nil, ErrAdminReasonRequired
	}
	if adminID == targetUserID {
		return nil, ErrAdminInvalidAction
	}

	user, err := s.GetUser(targetUserID)
	if err != nil {
		return nil, err
	}
	before := auditSnapshot(user)

	now := s.now()
	user.IsBanned = true
	user.Status = model.UserStatusBanned
	user.BanReason = reason
	user.BannedAt = &now
	user.BannedBy = &adminID
	user.TokenVersion++

	if durationMinutes != nil && *durationMinutes > 0 {
		until := now.Add(time.Duration(*durationMinutes) * time.Minute)
		user.BanUntil = &until
	} else {
		user.BanUntil = nil
	}

	if err := s.userRepo.Save(user); err != nil {
		return nil, err
	}
	_ = s.auditService.Record(AdminAuditEntry{
		AdminUserID:    adminID,
		TargetUserID:   &targetUserID,
		Action:         model.AuditActionUserBan,
		ResourceType:   "user",
		ResourceID:     strconv.FormatUint(uint64(targetUserID), 10),
		BeforeSnapshot: before,
		AfterSnapshot:  auditSnapshot(user),
		IPAddress:      meta.IPAddress,
		UserAgent:      meta.UserAgent,
		Reason:         user.BanReason,
	})
	return user, nil
}

func (s *adminUserService) UnbanUser(adminID, targetUserID uint, reason string, meta AdminRequestMeta) (*model.User, error) {
	if adminID == 0 || targetUserID == 0 {
		return nil, ErrInvalidInput
	}
	reason = strings.TrimSpace(reason)
	if !validAdminReason(reason) {
		return nil, ErrAdminReasonRequired
	}

	user, err := s.GetUser(targetUserID)
	if err != nil {
		return nil, err
	}
	before := auditSnapshot(user)

	user.IsBanned = false
	user.BanUntil = nil
	user.BanReason = ""
	user.BannedAt = nil
	user.BannedBy = nil
	if user.IsDeleted {
		user.Status = model.UserStatusDeleted
	} else {
		user.Status = model.UserStatusNormal
	}
	user.TokenVersion++

	if err := s.userRepo.Save(user); err != nil {
		return nil, err
	}
	_ = s.auditService.Record(AdminAuditEntry{
		AdminUserID:    adminID,
		TargetUserID:   &targetUserID,
		Action:         model.AuditActionUserUnban,
		ResourceType:   "user",
		ResourceID:     strconv.FormatUint(uint64(targetUserID), 10),
		BeforeSnapshot: before,
		AfterSnapshot:  auditSnapshot(user),
		IPAddress:      meta.IPAddress,
		UserAgent:      meta.UserAgent,
		Reason:         reason,
	})
	return user, nil
}

func (s *adminUserService) ResetPassword(adminID, targetUserID uint, newPassword, reason string, meta AdminRequestMeta) (*model.User, error) {
	if adminID == 0 || targetUserID == 0 || len(newPassword) < 8 {
		return nil, ErrInvalidInput
	}
	reason = strings.TrimSpace(reason)
	if !validAdminReason(reason) {
		return nil, ErrAdminReasonRequired
	}

	user, err := s.GetUser(targetUserID)
	if err != nil {
		return nil, err
	}
	before := auditSnapshot(user)

	hashed, err := utils.HashPasswordArgon2ID(newPassword)
	if err != nil {
		return nil, err
	}

	user.PasswordHash = hashed
	user.MustChangePassword = true
	user.TokenVersion++

	if err := s.userRepo.Save(user); err != nil {
		return nil, err
	}
	_ = s.auditService.Record(AdminAuditEntry{
		AdminUserID:    adminID,
		TargetUserID:   &targetUserID,
		Action:         model.AuditActionUserResetPassword,
		ResourceType:   "user",
		ResourceID:     strconv.FormatUint(uint64(targetUserID), 10),
		BeforeSnapshot: before,
		AfterSnapshot:  auditSnapshot(user),
		IPAddress:      meta.IPAddress,
		UserAgent:      meta.UserAgent,
		Reason:         reason,
	})
	return user, nil
}

func (s *adminUserService) ForceLogout(adminID, targetUserID uint, reason string, meta AdminRequestMeta) (*model.User, error) {
	if adminID == 0 || targetUserID == 0 {
		return nil, ErrInvalidInput
	}
	reason = strings.TrimSpace(reason)
	if !validAdminReason(reason) {
		return nil, ErrAdminReasonRequired
	}

	user, err := s.GetUser(targetUserID)
	if err != nil {
		return nil, err
	}
	before := auditSnapshot(user)

	user.TokenVersion++

	if err := s.userRepo.Save(user); err != nil {
		return nil, err
	}
	_ = s.auditService.Record(AdminAuditEntry{
		AdminUserID:    adminID,
		TargetUserID:   &targetUserID,
		Action:         model.AuditActionUserForceLogout,
		ResourceType:   "user",
		ResourceID:     strconv.FormatUint(uint64(targetUserID), 10),
		BeforeSnapshot: before,
		AfterSnapshot:  auditSnapshot(user),
		IPAddress:      meta.IPAddress,
		UserAgent:      meta.UserAgent,
		Reason:         reason,
	})
	return user, nil
}

func (s *adminUserService) SoftDeleteUser(adminID, targetUserID uint, reason string, meta AdminRequestMeta) (*model.User, error) {
	if adminID == 0 || targetUserID == 0 {
		return nil, ErrInvalidInput
	}
	reason = strings.TrimSpace(reason)
	if !validAdminReason(reason) {
		return nil, ErrAdminReasonRequired
	}
	if adminID == targetUserID {
		return nil, ErrAdminInvalidAction
	}

	user, err := s.GetUser(targetUserID)
	if err != nil {
		return nil, err
	}
	before := auditSnapshot(user)

	now := s.now()
	user.IsDeleted = true
	user.DeletedAt = &now
	user.Status = model.UserStatusDeleted
	user.TokenVersion++

	if err := s.userRepo.Save(user); err != nil {
		return nil, err
	}
	_ = s.auditService.Record(AdminAuditEntry{
		AdminUserID:    adminID,
		TargetUserID:   &targetUserID,
		Action:         model.AuditActionUserSoftDelete,
		ResourceType:   "user",
		ResourceID:     strconv.FormatUint(uint64(targetUserID), 10),
		BeforeSnapshot: before,
		AfterSnapshot:  auditSnapshot(user),
		IPAddress:      meta.IPAddress,
		UserAgent:      meta.UserAgent,
		Reason:         reason,
	})
	return user, nil
}

func (s *adminUserService) RestoreUser(adminID, targetUserID uint, reason string, meta AdminRequestMeta) (*model.User, error) {
	if adminID == 0 || targetUserID == 0 {
		return nil, ErrInvalidInput
	}
	reason = strings.TrimSpace(reason)
	if !validAdminReason(reason) {
		return nil, ErrAdminReasonRequired
	}

	user, err := s.GetUser(targetUserID)
	if err != nil {
		return nil, err
	}
	before := auditSnapshot(user)

	user.IsDeleted = false
	user.DeletedAt = nil
	if user.IsBanned {
		user.Status = model.UserStatusBanned
	} else {
		user.Status = model.UserStatusNormal
	}
	user.TokenVersion++

	if err := s.userRepo.Save(user); err != nil {
		return nil, err
	}
	_ = s.auditService.Record(AdminAuditEntry{
		AdminUserID:    adminID,
		TargetUserID:   &targetUserID,
		Action:         model.AuditActionUserRestore,
		ResourceType:   "user",
		ResourceID:     strconv.FormatUint(uint64(targetUserID), 10),
		BeforeSnapshot: before,
		AfterSnapshot:  auditSnapshot(user),
		IPAddress:      meta.IPAddress,
		UserAgent:      meta.UserAgent,
		Reason:         reason,
	})
	return user, nil
}

func (s *adminUserService) ScanExpiredBans(adminID uint, limit int, reason string, meta AdminRequestMeta) (*AdminExpiredBanScanResult, error) {
	if adminID == 0 {
		return nil, ErrInvalidInput
	}
	return s.scanExpiredBans(adminID, limit, reason, meta)
}

func (s *adminUserService) ScanExpiredBansBySystem(limit int, reason string) (*AdminExpiredBanScanResult, error) {
	return s.scanExpiredBans(0, limit, reason, AdminRequestMeta{IPAddress: "system", UserAgent: "orbitterm-auto-unban-worker"})
}

func (s *adminUserService) scanExpiredBans(adminID uint, limit int, reason string, meta AdminRequestMeta) (*AdminExpiredBanScanResult, error) {
	reason = strings.TrimSpace(reason)
	if !validAdminReason(reason) {
		return nil, ErrAdminReasonRequired
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}

	now := s.now()
	users, err := s.userRepo.ListExpiredBans(now, limit)
	if err != nil {
		return nil, err
	}

	result := &AdminExpiredBanScanResult{
		ScannedCount: len(users),
		Items:        make([]AdminExpiredBanItem, 0, len(users)),
	}
	for i := range users {
		user := users[i]
		before := auditSnapshot(&user)
		if !user.ClearExpiredBan(now) {
			continue
		}
		user.TokenVersion++
		if err := s.userRepo.Save(&user); err != nil {
			return nil, err
		}

		result.UnbannedCount++
		result.Items = append(result.Items, AdminExpiredBanItem{
			UserID:   user.ID,
			Username: user.Username,
			Status:   user.Status,
		})
		_ = s.auditService.Record(AdminAuditEntry{
			AdminUserID:    adminID,
			TargetUserID:   &user.ID,
			Action:         model.AuditActionUserAutoUnban,
			ResourceType:   "user",
			ResourceID:     strconv.FormatUint(uint64(user.ID), 10),
			BeforeSnapshot: before,
			AfterSnapshot:  auditSnapshot(&user),
			IPAddress:      meta.IPAddress,
			UserAgent:      meta.UserAgent,
			Reason:         reason,
		})
	}
	return result, nil
}

func validAdminReason(reason string) bool {
	return len(strings.TrimSpace(reason)) >= 2
}

func auditSnapshot(user *model.User) string {
	if user == nil {
		return "{}"
	}
	payload := map[string]any{
		"id":                   user.ID,
		"username":             user.Username,
		"role":                 user.Role,
		"status":               user.Status,
		"is_banned":            user.IsBanned,
		"ban_until":            user.BanUntil,
		"is_deleted":           user.IsDeleted,
		"must_change_password": user.MustChangePassword,
		"token_version":        user.TokenVersion,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return "{}"
	}
	return string(data)
}
