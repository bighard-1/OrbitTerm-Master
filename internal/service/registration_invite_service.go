package service

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
	"orbitterm-server/internal/model"
	"orbitterm-server/internal/repository"
)

var (
	ErrInviteRequired = errors.New("registration invite required")
	ErrInviteInvalid  = errors.New("registration invite invalid")
)

type RegistrationInviteCreated struct {
	Invite model.RegistrationInvite `json:"invite"`
	Code   string                   `json:"code"`
}

type RegistrationInviteService interface {
	Create(adminID uint, note string, maxUses, validDays int, reason string, meta AdminRequestMeta) (RegistrationInviteCreated, error)
	List(limit, offset int) ([]model.RegistrationInvite, int64, error)
	Revoke(adminID, inviteID uint, reason string, meta AdminRequestMeta) (*model.RegistrationInvite, error)
	Consume(code string) error
	Release(code string) error
}

type RegistrationInviteConsumer interface {
	Consume(code string) error
	Release(code string) error
}

type registrationInviteService struct {
	db           *gorm.DB
	repo         repository.RegistrationInviteRepository
	auditService AdminAuditService
	now          func() time.Time
}

func NewRegistrationInviteService(repo repository.RegistrationInviteRepository, audit AdminAuditService, dbs ...*gorm.DB) RegistrationInviteService {
	s := &registrationInviteService{repo: repo, auditService: audit, now: func() time.Time { return time.Now().UTC() }}
	if len(dbs) > 0 {
		s.db = dbs[0]
	}
	return s
}

func (s *registrationInviteService) Create(adminID uint, note string, maxUses, validDays int, reason string, meta AdminRequestMeta) (RegistrationInviteCreated, error) {
	if s.db != nil {
		var result RegistrationInviteCreated
		err := s.db.Transaction(func(tx *gorm.DB) error {
			copy := *s
			copy.db = nil
			copy.repo = repository.NewRegistrationInviteRepository(tx)
			copy.auditService = NewAdminAuditService(repository.NewAdminAuditRepository(tx))
			var runErr error
			result, runErr = copy.Create(adminID, note, maxUses, validDays, reason, meta)
			return runErr
		})
		return result, err
	}
	if adminID == 0 || maxUses < 1 || maxUses > 1000 || validDays < 1 || validDays > 365 {
		return RegistrationInviteCreated{}, ErrInvalidInput
	}
	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		return RegistrationInviteCreated{}, err
	}
	code := "OTI-" + base64.RawURLEncoding.EncodeToString(raw)
	now := s.now()
	expiresAt := now.Add(time.Duration(validDays) * 24 * time.Hour)
	invite := model.RegistrationInvite{
		CodeHash:   invitationDigest(code),
		CodePrefix: code[:12],
		Note:       strings.TrimSpace(note),
		MaxUses:    maxUses,
		ExpiresAt:  &expiresAt,
		CreatedBy:  adminID,
	}
	if err := s.repo.Create(&invite); err != nil {
		return RegistrationInviteCreated{}, err
	}
	if err := s.auditService.Record(AdminAuditEntry{
		AdminUserID: adminID, Action: model.AuditActionRegistrationInviteCreate,
		ResourceType: "registration_invite", ResourceID: invite.CodePrefix,
		IPAddress: meta.IPAddress, UserAgent: meta.UserAgent,
		Reason: strings.TrimSpace(reason),
	}); err != nil {
		return RegistrationInviteCreated{}, err
	}
	return RegistrationInviteCreated{Invite: invite, Code: code}, nil
}

func (s *registrationInviteService) List(limit, offset int) ([]model.RegistrationInvite, int64, error) {
	return s.repo.List(limit, offset)
}

func (s *registrationInviteService) Revoke(adminID, inviteID uint, reason string, meta AdminRequestMeta) (*model.RegistrationInvite, error) {
	if s.db != nil {
		var result *model.RegistrationInvite
		err := s.db.Transaction(func(tx *gorm.DB) error {
			copy := *s
			copy.db = nil
			copy.repo = repository.NewRegistrationInviteRepository(tx)
			copy.auditService = NewAdminAuditService(repository.NewAdminAuditRepository(tx))
			var runErr error
			result, runErr = copy.Revoke(adminID, inviteID, reason, meta)
			return runErr
		})
		return result, err
	}
	if adminID == 0 || inviteID == 0 || len(strings.TrimSpace(reason)) < 2 {
		return nil, ErrInvalidInput
	}
	invite, err := s.repo.Disable(inviteID, s.now())
	if err != nil {
		return nil, err
	}
	if err := s.auditService.Record(AdminAuditEntry{
		AdminUserID: adminID, Action: model.AuditActionRegistrationInviteRevoke,
		ResourceType: "registration_invite", ResourceID: invite.CodePrefix,
		IPAddress: meta.IPAddress, UserAgent: meta.UserAgent, Reason: strings.TrimSpace(reason),
	}); err != nil {
		return nil, err
	}
	return invite, nil
}

func (s *registrationInviteService) Consume(code string) error {
	code = strings.TrimSpace(code)
	if code == "" {
		return ErrInviteRequired
	}
	_, err := s.repo.Consume(invitationDigest(code), s.now())
	if errors.Is(err, repository.ErrInviteNotFound) || errors.Is(err, repository.ErrInviteUnavailable) {
		return ErrInviteInvalid
	}
	return err
}

func (s *registrationInviteService) Release(code string) error {
	code = strings.TrimSpace(code)
	if code == "" {
		return ErrInviteRequired
	}
	return s.repo.Release(invitationDigest(code))
}

func invitationDigest(code string) string {
	digest := sha256.Sum256([]byte(strings.TrimSpace(code)))
	return hex.EncodeToString(digest[:])
}
