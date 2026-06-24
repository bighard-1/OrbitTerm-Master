package service

import (
	"errors"
	"testing"
	"time"

	"orbitterm-server/internal/model"
	"orbitterm-server/internal/repository"
)

func TestRegistrationInviteCreateReturnsClearTextOnce(t *testing.T) {
	repo := &fakeInviteRepo{}
	svc := NewRegistrationInviteService(repo, &fakeAdminAuditService{})
	result, err := svc.Create(7, "测试团队", 3, 7, "测试创建", AdminRequestMeta{})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if result.Code == "" || result.Invite.CodeHash == "" || result.Invite.CodeHash == result.Code {
		t.Fatalf("expected one-time code and digest-only storage: %+v", result)
	}
	if repo.created == nil || repo.created.CodeHash != result.Invite.CodeHash {
		t.Fatal("expected digest to be persisted")
	}
}

func TestRegistrationInviteConsumeMapsUnavailable(t *testing.T) {
	repo := &fakeInviteRepo{consumeErr: repository.ErrInviteUnavailable}
	svc := NewRegistrationInviteService(repo, &fakeAdminAuditService{})
	if err := svc.Consume("OTI-example"); !errors.Is(err, ErrInviteInvalid) {
		t.Fatalf("expected ErrInviteInvalid, got %v", err)
	}
	if err := svc.Consume(""); !errors.Is(err, ErrInviteRequired) {
		t.Fatalf("expected ErrInviteRequired, got %v", err)
	}
}

type fakeInviteRepo struct {
	created    *model.RegistrationInvite
	consumeErr error
}

func (r *fakeInviteRepo) Create(invite *model.RegistrationInvite) error {
	copy := *invite
	copy.ID = 1
	r.created = &copy
	invite.ID = copy.ID
	return nil
}
func (r *fakeInviteRepo) List(int, int) ([]model.RegistrationInvite, int64, error) {
	return nil, 0, nil
}
func (r *fakeInviteRepo) Disable(uint, time.Time) (*model.RegistrationInvite, error) {
	return &model.RegistrationInvite{}, nil
}
func (r *fakeInviteRepo) Consume(string, time.Time) (*model.RegistrationInvite, error) {
	return &model.RegistrationInvite{}, r.consumeErr
}
func (r *fakeInviteRepo) Release(string) error { return nil }
