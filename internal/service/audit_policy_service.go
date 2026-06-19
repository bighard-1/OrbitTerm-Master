package service

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"orbitterm-server/internal/model"
	"orbitterm-server/internal/repository"
)

type AuditPolicyService interface {
	GetAuditPolicy() (model.AuditPolicy, error)
	UpdateAuditPolicy(adminID uint, update AuditPolicyUpdate, meta AdminRequestMeta) (model.AuditPolicy, error)
	CleanupExpiredAuditLogs(adminID uint, reason string, meta AdminRequestMeta) (AuditCleanupResult, error)
}

type AuditPolicyUpdate struct {
	RetentionDays     *int   `json:"retention_days,omitempty"`
	CleanupBatchLimit *int   `json:"cleanup_batch_limit,omitempty"`
	Reason            string `json:"reason,omitempty"`
}

type AuditCleanupResult struct {
	RetentionDays int       `json:"retention_days"`
	Cutoff        time.Time `json:"cutoff"`
	DeletedCount  int64     `json:"deleted_count"`
	BatchLimit    int       `json:"batch_limit"`
}

type auditPolicyService struct {
	settingRepo  repository.SystemSettingRepository
	auditRepo    repository.AdminAuditRepository
	auditService AdminAuditService
	now          func() time.Time
}

func NewAuditPolicyService(
	settingRepo repository.SystemSettingRepository,
	auditRepo repository.AdminAuditRepository,
	auditService AdminAuditService,
) AuditPolicyService {
	return &auditPolicyService{
		settingRepo:  settingRepo,
		auditRepo:    auditRepo,
		auditService: auditService,
		now:          func() time.Time { return time.Now().UTC() },
	}
}

func (s *auditPolicyService) GetAuditPolicy() (model.AuditPolicy, error) {
	setting, err := s.settingRepo.FindByKey(model.SystemSettingKeyAuditPolicy)
	if err != nil {
		return model.AuditPolicy{}, err
	}
	if setting == nil || strings.TrimSpace(setting.Value) == "" {
		policy := model.DefaultAuditPolicy()
		policy.Normalize()
		return policy, nil
	}

	policy := model.DefaultAuditPolicy()
	if err := json.Unmarshal([]byte(setting.Value), &policy); err != nil {
		return model.AuditPolicy{}, err
	}
	policy.Normalize()
	return policy, nil
}

func (s *auditPolicyService) UpdateAuditPolicy(adminID uint, update AuditPolicyUpdate, meta AdminRequestMeta) (model.AuditPolicy, error) {
	if adminID == 0 {
		return model.AuditPolicy{}, ErrInvalidInput
	}

	policy, err := s.GetAuditPolicy()
	if err != nil {
		return model.AuditPolicy{}, err
	}
	before := auditPolicySnapshot(policy)

	if update.RetentionDays != nil {
		policy.RetentionDays = *update.RetentionDays
	}
	if update.CleanupBatchLimit != nil {
		policy.CleanupBatchLimit = *update.CleanupBatchLimit
	}
	policy.Normalize()

	encoded, err := json.Marshal(policy)
	if err != nil {
		return model.AuditPolicy{}, err
	}
	if err := s.settingRepo.Upsert(&model.SystemSetting{
		Key:   model.SystemSettingKeyAuditPolicy,
		Value: string(encoded),
	}); err != nil {
		return model.AuditPolicy{}, err
	}

	_ = s.auditService.Record(AdminAuditEntry{
		AdminUserID:    adminID,
		Action:         model.AuditActionSystemAuditPolicyUpdate,
		ResourceType:   "system_setting",
		ResourceID:     model.SystemSettingKeyAuditPolicy,
		BeforeSnapshot: before,
		AfterSnapshot:  auditPolicySnapshot(policy),
		IPAddress:      meta.IPAddress,
		UserAgent:      meta.UserAgent,
		Reason:         strings.TrimSpace(update.Reason),
	})
	return policy, nil
}

func (s *auditPolicyService) CleanupExpiredAuditLogs(adminID uint, reason string, meta AdminRequestMeta) (AuditCleanupResult, error) {
	if adminID == 0 {
		return AuditCleanupResult{}, ErrInvalidInput
	}
	reason = strings.TrimSpace(reason)
	if !validAdminReason(reason) {
		return AuditCleanupResult{}, ErrAdminReasonRequired
	}

	policy, err := s.GetAuditPolicy()
	if err != nil {
		return AuditCleanupResult{}, err
	}
	cutoff := s.now().AddDate(0, 0, -policy.RetentionDays)
	deleted, err := s.auditRepo.DeleteOlderThan(cutoff, policy.CleanupBatchLimit)
	if err != nil {
		return AuditCleanupResult{}, err
	}
	result := AuditCleanupResult{
		RetentionDays: policy.RetentionDays,
		Cutoff:        cutoff,
		DeletedCount:  deleted,
		BatchLimit:    policy.CleanupBatchLimit,
	}

	_ = s.auditService.Record(AdminAuditEntry{
		AdminUserID:   adminID,
		Action:        model.AuditActionSystemAuditCleanup,
		ResourceType:  "admin_audit_logs",
		ResourceID:    "cleanup",
		AfterSnapshot: auditCleanupSnapshot(result),
		IPAddress:     meta.IPAddress,
		UserAgent:     meta.UserAgent,
		Reason:        reason,
	})
	return result, nil
}

func auditPolicySnapshot(policy model.AuditPolicy) string {
	encoded, err := json.Marshal(policy)
	if err != nil {
		return strconv.Quote("audit_policy_snapshot_failed")
	}
	return string(encoded)
}

func auditCleanupSnapshot(result AuditCleanupResult) string {
	encoded, err := json.Marshal(result)
	if err != nil {
		return strconv.Quote("audit_cleanup_snapshot_failed")
	}
	return string(encoded)
}
