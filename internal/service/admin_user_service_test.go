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
