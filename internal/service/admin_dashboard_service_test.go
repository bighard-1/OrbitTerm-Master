package service

import (
	"testing"
	"time"

	"orbitterm-server/internal/model"
)

func TestAdminDashboardServiceOverview(t *testing.T) {
	userRepo := newFakeUserRepo(
		&model.User{ID: 1, Username: "root", Role: model.UserRoleSuperAdmin, Status: model.UserStatusNormal},
		&model.User{ID: 2, Username: "alice", Role: model.UserRoleUser, Status: model.UserStatusNormal},
		&model.User{ID: 3, Username: "bob", Role: model.UserRoleUser, Status: model.UserStatusBanned},
		&model.User{ID: 4, Username: "carol", Role: model.UserRoleUser, Status: model.UserStatusDeleted},
	)
	audit := &fakeAdminAuditService{logs: []model.AdminAuditLog{
		{ID: 1, AdminUserID: 1, Action: model.AuditActionAdminLogin},
	}}
	backup := fakeBackupReadinessService{report: BackupReadinessReport{Ready: false, Warnings: []string{"JWT_SECRET: warning"}}}
	svc := &adminDashboardService{
		userRepo:        userRepo,
		configRepo:      fakeServerConfigCounter{count: 7},
		auditService:    audit,
		backupReadiness: backup,
		now:             func() time.Time { return time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC) },
	}

	overview, err := svc.Overview(1, AdminRequestMeta{IPAddress: "127.0.0.1"})
	if err != nil {
		t.Fatalf("Overview failed: %v", err)
	}
	if overview.Users.Total != 4 || overview.Users.Normal != 2 || overview.Users.Banned != 1 || overview.Users.Deleted != 1 || overview.Users.Admins != 1 {
		t.Fatalf("unexpected user stats: %+v", overview.Users)
	}
	if overview.Configs.Total != 7 {
		t.Fatalf("unexpected config stats: %+v", overview.Configs)
	}
	if overview.Backup.Ready || overview.Backup.WarningCount != 1 {
		t.Fatalf("unexpected backup stats: %+v", overview.Backup)
	}
	if len(overview.RecentAudits) != 1 || overview.RecentAudits[0].Action != model.AuditActionAdminLogin {
		t.Fatalf("unexpected recent audits: %+v", overview.RecentAudits)
	}
}

func TestAdminDashboardServiceRejectsMissingAdmin(t *testing.T) {
	svc := NewAdminDashboardService(newFakeUserRepo(), fakeServerConfigCounter{}, &fakeAdminAuditService{}, fakeBackupReadinessService{})

	_, err := svc.Overview(0, AdminRequestMeta{})
	if err == nil {
		t.Fatal("expected error for missing admin id")
	}
}

type fakeServerConfigCounter struct {
	count int64
}

func (f fakeServerConfigCounter) Create(_ *model.ServerConfig) error { return nil }
func (f fakeServerConfigCounter) Update(_ *model.ServerConfig) error { return nil }
func (f fakeServerConfigCounter) FindByIDAndUserID(_, _ uint) (*model.ServerConfig, error) {
	return nil, nil
}
func (f fakeServerConfigCounter) FindByAssetIDAndUserID(_ string, _ uint) (*model.ServerConfig, error) {
	return nil, nil
}
func (f fakeServerConfigCounter) ListByIdentityFingerprint(_ uint, _ string) ([]model.ServerConfig, error) {
	return nil, nil
}
func (f fakeServerConfigCounter) MutateByAssetID(_ uint, _ string, _ func(*model.ServerConfig) (bool, error)) (*model.ServerConfig, error) {
	return nil, nil
}
func (f fakeServerConfigCounter) ListByUserID(_ uint) ([]model.ServerConfig, error) { return nil, nil }
func (f fakeServerConfigCounter) ListTrashByUserID(_ uint, _, _ int) ([]model.ServerConfig, int64, error) {
	return nil, 0, nil
}
func (f fakeServerConfigCounter) ListChangedByUserID(_ uint, _ uint64, _ int) ([]model.ServerConfig, bool, error) {
	return nil, false, nil
}
func (f fakeServerConfigCounter) MaxRevisionByUserID(_ uint) (uint64, error) { return 0, nil }
func (f fakeServerConfigCounter) AcknowledgeDevice(_ uint, _ string, _ uint64, _, _ string, _ time.Time) error {
	return nil
}
func (f fakeServerConfigCounter) CountAll() (int64, error)                    { return f.count, nil }
func (f fakeServerConfigCounter) DeleteByIDAndUserID(_, _ uint) (bool, error) { return false, nil }

type fakeBackupReadinessService struct {
	report BackupReadinessReport
}

func (f fakeBackupReadinessService) GetReadiness(_ uint, _ AdminRequestMeta) (BackupReadinessReport, error) {
	return f.report, nil
}
