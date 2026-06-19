package service

import (
	"time"

	"orbitterm-server/internal/model"
	"orbitterm-server/internal/repository"
)

type AdminDashboardService interface {
	Overview(adminID uint, meta AdminRequestMeta) (*AdminDashboardOverview, error)
}

type AdminDashboardOverview struct {
	GeneratedAt  time.Time                 `json:"generated_at"`
	Users        AdminDashboardUserStats   `json:"users"`
	Configs      AdminDashboardConfigStats `json:"configs"`
	Backup       AdminDashboardBackupStats `json:"backup"`
	RecentAudits []model.AdminAuditLog     `json:"recent_audits"`
}

type AdminDashboardUserStats struct {
	Total   int64 `json:"total"`
	Normal  int64 `json:"normal"`
	Risk    int64 `json:"risk"`
	Banned  int64 `json:"banned"`
	Deleted int64 `json:"deleted"`
	Admins  int64 `json:"admins"`
}

type AdminDashboardConfigStats struct {
	Total int64 `json:"total"`
}

type AdminDashboardBackupStats struct {
	Ready        bool     `json:"ready"`
	WarningCount int      `json:"warning_count"`
	Warnings     []string `json:"warnings,omitempty"`
}

type adminDashboardService struct {
	userRepo        repository.UserRepository
	configRepo      repository.ServerConfigRepository
	auditService    AdminAuditService
	backupReadiness BackupReadinessService
	now             func() time.Time
}

func NewAdminDashboardService(
	userRepo repository.UserRepository,
	configRepo repository.ServerConfigRepository,
	auditService AdminAuditService,
	backupReadiness BackupReadinessService,
) AdminDashboardService {
	return &adminDashboardService{
		userRepo:        userRepo,
		configRepo:      configRepo,
		auditService:    auditService,
		backupReadiness: backupReadiness,
		now:             func() time.Time { return time.Now().UTC() },
	}
}

func (s *adminDashboardService) Overview(adminID uint, meta AdminRequestMeta) (*AdminDashboardOverview, error) {
	if adminID == 0 {
		return nil, ErrInvalidInput
	}

	users, err := s.userStats()
	if err != nil {
		return nil, err
	}
	configs, err := s.configStats()
	if err != nil {
		return nil, err
	}
	recentAudits, err := s.auditService.ListRecent(8)
	if err != nil {
		return nil, err
	}
	backup, err := s.backupStats(adminID, meta)
	if err != nil {
		return nil, err
	}

	return &AdminDashboardOverview{
		GeneratedAt:  s.now(),
		Users:        users,
		Configs:      configs,
		Backup:       backup,
		RecentAudits: recentAudits,
	}, nil
}

func (s *adminDashboardService) userStats() (AdminDashboardUserStats, error) {
	total, err := s.userRepo.CountAll()
	if err != nil {
		return AdminDashboardUserStats{}, err
	}
	normal, err := s.userRepo.CountByStatus(model.UserStatusNormal)
	if err != nil {
		return AdminDashboardUserStats{}, err
	}
	risk, err := s.userRepo.CountByStatus(model.UserStatusRisk)
	if err != nil {
		return AdminDashboardUserStats{}, err
	}
	banned, err := s.userRepo.CountByStatus(model.UserStatusBanned)
	if err != nil {
		return AdminDashboardUserStats{}, err
	}
	deleted, err := s.userRepo.CountByStatus(model.UserStatusDeleted)
	if err != nil {
		return AdminDashboardUserStats{}, err
	}
	admins, err := s.userRepo.CountByRoles(adminRoles)
	if err != nil {
		return AdminDashboardUserStats{}, err
	}

	return AdminDashboardUserStats{
		Total:   total,
		Normal:  normal,
		Risk:    risk,
		Banned:  banned,
		Deleted: deleted,
		Admins:  admins,
	}, nil
}

func (s *adminDashboardService) configStats() (AdminDashboardConfigStats, error) {
	total, err := s.configRepo.CountAll()
	if err != nil {
		return AdminDashboardConfigStats{}, err
	}
	return AdminDashboardConfigStats{Total: total}, nil
}

func (s *adminDashboardService) backupStats(adminID uint, meta AdminRequestMeta) (AdminDashboardBackupStats, error) {
	report, err := s.backupReadiness.GetReadiness(adminID, meta)
	if err != nil {
		return AdminDashboardBackupStats{}, err
	}
	return AdminDashboardBackupStats{
		Ready:        report.Ready,
		WarningCount: len(report.Warnings),
		Warnings:     report.Warnings,
	}, nil
}
