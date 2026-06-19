package service

import (
	"errors"
	"testing"
	"time"

	"orbitterm-server/internal/model"
	"orbitterm-server/internal/repository"
)

func TestAdminUserServiceBanAndUnban(t *testing.T) {
	now := time.Date(2026, 6, 18, 9, 0, 0, 0, time.UTC)
	userRepo := newFakeUserRepo(&model.User{
		ID:           2,
		Username:     "alice",
		Role:         model.UserRoleUser,
		Status:       model.UserStatusNormal,
		TokenVersion: 3,
	})
	audit := &fakeAdminAuditService{}
	svc := &adminUserService{
		userRepo:     userRepo,
		auditService: audit,
		now:          func() time.Time { return now },
	}

	duration := 60
	banned, err := svc.BanUser(1, 2, &duration, "异常登录", AdminRequestMeta{IPAddress: "127.0.0.1"})
	if err != nil {
		t.Fatalf("BanUser failed: %v", err)
	}
	if !banned.IsBanned || banned.Status != model.UserStatusBanned {
		t.Fatalf("expected banned user, got %+v", banned)
	}
	if banned.BanUntil == nil || !banned.BanUntil.Equal(now.Add(time.Hour)) {
		t.Fatalf("unexpected ban_until: %v", banned.BanUntil)
	}
	if banned.TokenVersion != 4 {
		t.Fatalf("expected token version 4, got %d", banned.TokenVersion)
	}
	if len(audit.entries) != 1 || audit.entries[0].Action != model.AuditActionUserBan {
		t.Fatalf("expected ban audit entry, got %+v", audit.entries)
	}

	unbanned, err := svc.UnbanUser(1, 2, "误封", AdminRequestMeta{IPAddress: "127.0.0.1"})
	if err != nil {
		t.Fatalf("UnbanUser failed: %v", err)
	}
	if unbanned.IsBanned || unbanned.Status != model.UserStatusNormal {
		t.Fatalf("expected normal user, got %+v", unbanned)
	}
	if unbanned.TokenVersion != 5 {
		t.Fatalf("expected token version 5, got %d", unbanned.TokenVersion)
	}
	if len(audit.entries) != 2 || audit.entries[1].Action != model.AuditActionUserUnban {
		t.Fatalf("expected unban audit entry, got %+v", audit.entries)
	}
}

func TestAdminUserServiceRejectsSelfBan(t *testing.T) {
	svc := &adminUserService{
		userRepo:     newFakeUserRepo(&model.User{ID: 1, Username: "admin"}),
		auditService: &fakeAdminAuditService{},
		now:          func() time.Time { return time.Now().UTC() },
	}

	_, err := svc.BanUser(1, 1, nil, "self", AdminRequestMeta{})
	if !errors.Is(err, ErrAdminInvalidAction) {
		t.Fatalf("expected ErrAdminInvalidAction, got %v", err)
	}
}

func TestAdminUserServiceResetPasswordAndForceLogout(t *testing.T) {
	userRepo := newFakeUserRepo(&model.User{
		ID:           2,
		Username:     "alice",
		PasswordHash: "old-hash",
		Role:         model.UserRoleUser,
		Status:       model.UserStatusNormal,
		TokenVersion: 10,
	})
	audit := &fakeAdminAuditService{}
	svc := &adminUserService{
		userRepo:     userRepo,
		auditService: audit,
		now:          func() time.Time { return time.Now().UTC() },
	}

	user, err := svc.ResetPassword(1, 2, "NewStrongPass123", "用户申请", AdminRequestMeta{})
	if err != nil {
		t.Fatalf("ResetPassword failed: %v", err)
	}
	if user.PasswordHash == "old-hash" || user.PasswordHash == "NewStrongPass123" {
		t.Fatalf("password hash was not securely replaced: %q", user.PasswordHash)
	}
	if !user.MustChangePassword {
		t.Fatal("expected must_change_password=true")
	}
	if user.TokenVersion != 11 {
		t.Fatalf("expected token version 11, got %d", user.TokenVersion)
	}

	user, err = svc.ForceLogout(1, 2, "安全检查", AdminRequestMeta{})
	if err != nil {
		t.Fatalf("ForceLogout failed: %v", err)
	}
	if user.TokenVersion != 12 {
		t.Fatalf("expected token version 12, got %d", user.TokenVersion)
	}
	if len(audit.entries) != 2 || audit.entries[0].Action != model.AuditActionUserResetPassword || audit.entries[1].Action != model.AuditActionUserForceLogout {
		t.Fatalf("unexpected audit entries: %+v", audit.entries)
	}
}

func TestAdminUserServiceSoftDeleteAndRestore(t *testing.T) {
	now := time.Date(2026, 6, 18, 9, 0, 0, 0, time.UTC)
	userRepo := newFakeUserRepo(&model.User{
		ID:           2,
		Username:     "alice",
		Role:         model.UserRoleUser,
		Status:       model.UserStatusNormal,
		TokenVersion: 1,
	})
	audit := &fakeAdminAuditService{}
	svc := &adminUserService{
		userRepo:     userRepo,
		auditService: audit,
		now:          func() time.Time { return now },
	}

	user, err := svc.SoftDeleteUser(1, 2, "注销申请", AdminRequestMeta{})
	if err != nil {
		t.Fatalf("SoftDeleteUser failed: %v", err)
	}
	if !user.IsDeleted || user.Status != model.UserStatusDeleted || user.DeletedAt == nil {
		t.Fatalf("expected deleted user, got %+v", user)
	}
	if user.TokenVersion != 2 {
		t.Fatalf("expected token version 2, got %d", user.TokenVersion)
	}

	user, err = svc.RestoreUser(1, 2, "恢复账号", AdminRequestMeta{})
	if err != nil {
		t.Fatalf("RestoreUser failed: %v", err)
	}
	if user.IsDeleted || user.Status != model.UserStatusNormal || user.DeletedAt != nil {
		t.Fatalf("expected restored normal user, got %+v", user)
	}
	if user.TokenVersion != 3 {
		t.Fatalf("expected token version 3, got %d", user.TokenVersion)
	}
	if len(audit.entries) != 2 || audit.entries[0].Action != model.AuditActionUserSoftDelete || audit.entries[1].Action != model.AuditActionUserRestore {
		t.Fatalf("unexpected audit entries: %+v", audit.entries)
	}
}

type fakeUserRepo struct {
	users map[uint]*model.User
}

func newFakeUserRepo(users ...*model.User) *fakeUserRepo {
	repo := &fakeUserRepo{users: make(map[uint]*model.User)}
	for _, user := range users {
		copy := *user
		repo.users[user.ID] = &copy
	}
	return repo
}

func (r *fakeUserRepo) Create(user *model.User) error {
	copy := *user
	r.users[user.ID] = &copy
	return nil
}

func (r *fakeUserRepo) Save(user *model.User) error {
	copy := *user
	r.users[user.ID] = &copy
	return nil
}

func (r *fakeUserRepo) FindByUsername(username string) (*model.User, error) {
	for _, user := range r.users {
		if user.Username == username {
			copy := *user
			return &copy, nil
		}
	}
	return nil, nil
}

func (r *fakeUserRepo) FindByID(id uint) (*model.User, error) {
	user, ok := r.users[id]
	if !ok {
		return nil, nil
	}
	copy := *user
	return &copy, nil
}

func (r *fakeUserRepo) List(filter repository.UserListFilter) ([]model.User, int64, error) {
	users := make([]model.User, 0, len(r.users))
	for _, user := range r.users {
		users = append(users, *user)
	}
	return users, int64(len(users)), nil
}

type fakeAdminAuditService struct {
	entries []AdminAuditEntry
}

func (s *fakeAdminAuditService) Record(entry AdminAuditEntry) error {
	s.entries = append(s.entries, entry)
	return nil
}

func (s *fakeAdminAuditService) ListRecent(_ int) ([]model.AdminAuditLog, error) {
	return nil, nil
}
