package service

import (
	"orbitterm-server/internal/model"
	"orbitterm-server/internal/repository"
)

type AdminAuditService interface {
	Record(entry AdminAuditEntry) error
	ListRecent(limit int) ([]model.AdminAuditLog, error)
}

type AdminAuditEntry struct {
	AdminUserID    uint
	TargetUserID   *uint
	Action         string
	ResourceType   string
	ResourceID     string
	BeforeSnapshot string
	AfterSnapshot  string
	IPAddress      string
	UserAgent      string
	Reason         string
}

type adminAuditService struct {
	repo repository.AdminAuditRepository
}

func NewAdminAuditService(repo repository.AdminAuditRepository) AdminAuditService {
	return &adminAuditService{repo: repo}
}

func (s *adminAuditService) Record(entry AdminAuditEntry) error {
	if entry.AdminUserID == 0 || entry.Action == "" {
		return ErrInvalidInput
	}

	return s.repo.Create(&model.AdminAuditLog{
		AdminUserID:    entry.AdminUserID,
		TargetUserID:   entry.TargetUserID,
		Action:         entry.Action,
		ResourceType:   entry.ResourceType,
		ResourceID:     entry.ResourceID,
		BeforeSnapshot: entry.BeforeSnapshot,
		AfterSnapshot:  entry.AfterSnapshot,
		IPAddress:      entry.IPAddress,
		UserAgent:      entry.UserAgent,
		Reason:         entry.Reason,
	})
}

func (s *adminAuditService) ListRecent(limit int) ([]model.AdminAuditLog, error) {
	return s.repo.List(limit)
}
