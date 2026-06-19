package service

import (
	"encoding/json"
	"strings"

	"orbitterm-server/internal/model"
	"orbitterm-server/internal/repository"
)

type RecoveryPolicyReader interface {
	GetRecoveryPolicy() (model.RecoveryPolicy, error)
}

type RecoveryPolicyUpdate struct {
	LoginPasswordResetEnabled  *bool   `json:"login_password_reset_enabled,omitempty"`
	RequireUserAcknowledgement *bool   `json:"require_user_acknowledgement,omitempty"`
	SupportContact             *string `json:"support_contact,omitempty"`
	UserFacingMessage          *string `json:"user_facing_message,omitempty"`
	Reason                     string  `json:"reason,omitempty"`
}

type RecoveryPolicyService interface {
	RecoveryPolicyReader
	UpdateRecoveryPolicy(adminID uint, update RecoveryPolicyUpdate, meta AdminRequestMeta) (model.RecoveryPolicy, error)
}

type PublicRecoveryInfo struct {
	LoginPasswordResetEnabled       bool   `json:"login_password_reset_enabled"`
	MasterPasswordRecoverable       bool   `json:"master_password_recoverable"`
	MasterPasswordRecoveryMode      string `json:"master_password_recovery_mode"`
	MasterPasswordResetBehavior     string `json:"master_password_reset_behavior"`
	ServerCanDecryptUserAssets      bool   `json:"server_can_decrypt_user_assets"`
	EncryptedAssetsPreservedOnReset bool   `json:"encrypted_assets_preserved_on_reset"`
	RequireUserAcknowledgement      bool   `json:"require_user_acknowledgement"`
	SupportContact                  string `json:"support_contact,omitempty"`
	UserFacingMessage               string `json:"user_facing_message"`
}

func DefaultPublicRecoveryInfo() PublicRecoveryInfo {
	return ToPublicRecoveryInfo(model.DefaultRecoveryPolicy())
}

func ToPublicRecoveryInfo(policy model.RecoveryPolicy) PublicRecoveryInfo {
	policy.Normalize()
	return PublicRecoveryInfo{
		LoginPasswordResetEnabled:       policy.LoginPasswordResetEnabled,
		MasterPasswordRecoverable:       policy.MasterPasswordRecoverable,
		MasterPasswordRecoveryMode:      policy.MasterPasswordRecoveryMode,
		MasterPasswordResetBehavior:     policy.MasterPasswordResetBehavior,
		ServerCanDecryptUserAssets:      policy.ServerCanDecryptUserAssets,
		EncryptedAssetsPreservedOnReset: policy.EncryptedAssetsPreservedOnReset,
		RequireUserAcknowledgement:      policy.RequireUserAcknowledgement,
		SupportContact:                  policy.SupportContact,
		UserFacingMessage:               policy.UserFacingMessage,
	}
}

type recoveryPolicyService struct {
	settingRepo  repository.SystemSettingRepository
	auditService AdminAuditService
}

func NewRecoveryPolicyService(settingRepo repository.SystemSettingRepository, auditService AdminAuditService) RecoveryPolicyService {
	return &recoveryPolicyService{
		settingRepo:  settingRepo,
		auditService: auditService,
	}
}

func (s *recoveryPolicyService) GetRecoveryPolicy() (model.RecoveryPolicy, error) {
	setting, err := s.settingRepo.FindByKey(model.SystemSettingKeyRecoveryPolicy)
	if err != nil {
		return model.RecoveryPolicy{}, err
	}
	if setting == nil || strings.TrimSpace(setting.Value) == "" {
		policy := model.DefaultRecoveryPolicy()
		policy.Normalize()
		return policy, nil
	}

	policy := model.DefaultRecoveryPolicy()
	if err := json.Unmarshal([]byte(setting.Value), &policy); err != nil {
		return model.RecoveryPolicy{}, err
	}
	policy.Normalize()
	return policy, nil
}

func (s *recoveryPolicyService) UpdateRecoveryPolicy(adminID uint, update RecoveryPolicyUpdate, meta AdminRequestMeta) (model.RecoveryPolicy, error) {
	if adminID == 0 {
		return model.RecoveryPolicy{}, ErrInvalidInput
	}

	policy, err := s.GetRecoveryPolicy()
	if err != nil {
		return model.RecoveryPolicy{}, err
	}
	before := recoveryPolicySnapshot(policy)

	if update.LoginPasswordResetEnabled != nil {
		policy.LoginPasswordResetEnabled = *update.LoginPasswordResetEnabled
	}
	if update.RequireUserAcknowledgement != nil {
		policy.RequireUserAcknowledgement = *update.RequireUserAcknowledgement
	}
	if update.SupportContact != nil {
		policy.SupportContact = strings.TrimSpace(*update.SupportContact)
	}
	if update.UserFacingMessage != nil {
		policy.UserFacingMessage = strings.TrimSpace(*update.UserFacingMessage)
	}
	policy.Normalize()

	encoded, err := json.Marshal(policy)
	if err != nil {
		return model.RecoveryPolicy{}, err
	}
	if err := s.settingRepo.Upsert(&model.SystemSetting{
		Key:   model.SystemSettingKeyRecoveryPolicy,
		Value: string(encoded),
	}); err != nil {
		return model.RecoveryPolicy{}, err
	}

	_ = s.auditService.Record(AdminAuditEntry{
		AdminUserID:    adminID,
		Action:         model.AuditActionSystemRecoveryPolicyUpdate,
		ResourceType:   "system_setting",
		ResourceID:     model.SystemSettingKeyRecoveryPolicy,
		BeforeSnapshot: before,
		AfterSnapshot:  recoveryPolicySnapshot(policy),
		IPAddress:      meta.IPAddress,
		UserAgent:      meta.UserAgent,
		Reason:         strings.TrimSpace(update.Reason),
	})
	return policy, nil
}

func recoveryPolicySnapshot(policy model.RecoveryPolicy) string {
	encoded, err := json.Marshal(policy)
	if err != nil {
		return "{}"
	}
	return string(encoded)
}
