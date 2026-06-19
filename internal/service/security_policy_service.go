package service

import (
	"encoding/json"
	"strconv"
	"strings"

	"orbitterm-server/internal/model"
	"orbitterm-server/internal/repository"
)

type SecurityPolicyProvider interface {
	GetSecurityPolicy() (model.SecurityPolicy, error)
}

type SecurityPolicyUpdate struct {
	RegistrationEnabled        *bool   `json:"registration_enabled,omitempty"`
	RegistrationDisabledReason *string `json:"registration_disabled_reason,omitempty"`
	MinPasswordLength          *int    `json:"min_password_length,omitempty"`
	DefaultUserStatus          *string `json:"default_user_status,omitempty"`
	Reason                     string  `json:"reason,omitempty"`
}

type SecurityPolicyService interface {
	SecurityPolicyProvider
	UpdateSecurityPolicy(adminID uint, update SecurityPolicyUpdate, meta AdminRequestMeta) (model.SecurityPolicy, error)
}

type securityPolicyService struct {
	settingRepo  repository.SystemSettingRepository
	auditService AdminAuditService
}

func NewSecurityPolicyService(settingRepo repository.SystemSettingRepository, auditService AdminAuditService) SecurityPolicyService {
	return &securityPolicyService{
		settingRepo:  settingRepo,
		auditService: auditService,
	}
}

func (s *securityPolicyService) GetSecurityPolicy() (model.SecurityPolicy, error) {
	setting, err := s.settingRepo.FindByKey(model.SystemSettingKeySecurityPolicy)
	if err != nil {
		return model.SecurityPolicy{}, err
	}
	if setting == nil || strings.TrimSpace(setting.Value) == "" {
		policy := model.DefaultSecurityPolicy()
		policy.Normalize()
		return policy, nil
	}

	policy := model.DefaultSecurityPolicy()
	if err := json.Unmarshal([]byte(setting.Value), &policy); err != nil {
		return model.SecurityPolicy{}, err
	}
	policy.Normalize()
	return policy, nil
}

func (s *securityPolicyService) UpdateSecurityPolicy(adminID uint, update SecurityPolicyUpdate, meta AdminRequestMeta) (model.SecurityPolicy, error) {
	if adminID == 0 {
		return model.SecurityPolicy{}, ErrInvalidInput
	}

	policy, err := s.GetSecurityPolicy()
	if err != nil {
		return model.SecurityPolicy{}, err
	}
	before := policySnapshot(policy)

	if update.RegistrationEnabled != nil {
		policy.RegistrationEnabled = *update.RegistrationEnabled
	}
	if update.RegistrationDisabledReason != nil {
		policy.RegistrationDisabledReason = strings.TrimSpace(*update.RegistrationDisabledReason)
	}
	if update.MinPasswordLength != nil {
		policy.MinPasswordLength = *update.MinPasswordLength
	}
	if update.DefaultUserStatus != nil {
		policy.DefaultUserStatus = strings.TrimSpace(*update.DefaultUserStatus)
	}
	policy.Normalize()

	encoded, err := json.Marshal(policy)
	if err != nil {
		return model.SecurityPolicy{}, err
	}
	if err := s.settingRepo.Upsert(&model.SystemSetting{
		Key:   model.SystemSettingKeySecurityPolicy,
		Value: string(encoded),
	}); err != nil {
		return model.SecurityPolicy{}, err
	}

	_ = s.auditService.Record(AdminAuditEntry{
		AdminUserID:    adminID,
		Action:         model.AuditActionSystemSecurityPolicyUpdate,
		ResourceType:   "system_setting",
		ResourceID:     model.SystemSettingKeySecurityPolicy,
		BeforeSnapshot: before,
		AfterSnapshot:  policySnapshot(policy),
		IPAddress:      meta.IPAddress,
		UserAgent:      meta.UserAgent,
		Reason:         strings.TrimSpace(update.Reason),
	})
	return policy, nil
}

func policySnapshot(policy model.SecurityPolicy) string {
	encoded, err := json.Marshal(policy)
	if err != nil {
		return strconv.Quote("policy_snapshot_failed")
	}
	return string(encoded)
}
