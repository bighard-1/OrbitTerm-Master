package service

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"orbitterm-server/internal/model"
	"orbitterm-server/internal/repository"
)

type AssetDeletionPolicyReader interface {
	GetAssetDeletionPolicy() (model.AssetDeletionPolicy, error)
}

type AssetDeletionPolicyManager interface {
	AssetDeletionPolicyReader
	UpdateAssetDeletionPolicy(adminID uint, update AssetDeletionPolicyUpdate, meta AdminRequestMeta) (model.AssetDeletionPolicy, error)
	CleanupExpiredAssets(adminID uint, reason string, meta AdminRequestMeta) (AssetTrashCleanupResult, error)
	CleanupExpiredAssetsBySystem() (AssetTrashCleanupResult, error)
}

type AssetDeletionPolicyUpdate struct {
	RecentDeletedRetentionDays *int   `json:"recent_deleted_retention_days,omitempty"`
	TombstoneRetentionDays     *int   `json:"tombstone_retention_days,omitempty"`
	CleanupBatchLimit          *int   `json:"cleanup_batch_limit,omitempty"`
	AutoCleanupEnabled         *bool  `json:"auto_cleanup_enabled,omitempty"`
	Reason                     string `json:"reason,omitempty"`
}

type AssetTrashCleanupResult struct {
	Enabled            bool       `json:"enabled"`
	ScannedCount       int        `json:"scanned_count"`
	PurgedCount        int        `json:"purged_count"`
	FailedCount        int        `json:"failed_count"`
	TombstonesDeleted  int64      `json:"tombstones_deleted"`
	TombstonesDeferred int64      `json:"tombstones_deferred"`
	RecoveryCutoff     time.Time  `json:"recovery_cutoff"`
	TombstoneCutoff    *time.Time `json:"tombstone_cutoff,omitempty"`
	CleanupBatchLimit  int        `json:"cleanup_batch_limit"`
	CompletedAt        time.Time  `json:"completed_at"`
}

type assetDeletionPolicyService struct {
	settingRepo  repository.SystemSettingRepository
	configRepo   repository.ServerConfigRepository
	auditService AdminAuditService
	now          func() time.Time
}

// NewAssetDeletionPolicyService 保留只读构造器，供配置同步领域读取删除策略。
func NewAssetDeletionPolicyService(settingRepo repository.SystemSettingRepository) AssetDeletionPolicyReader {
	return &assetDeletionPolicyService{settingRepo: settingRepo, now: func() time.Time { return time.Now().UTC() }}
}

func NewAssetDeletionPolicyManager(
	settingRepo repository.SystemSettingRepository,
	configRepo repository.ServerConfigRepository,
	auditService AdminAuditService,
) AssetDeletionPolicyManager {
	return &assetDeletionPolicyService{
		settingRepo:  settingRepo,
		configRepo:   configRepo,
		auditService: auditService,
		now:          func() time.Time { return time.Now().UTC() },
	}
}

func (s *assetDeletionPolicyService) GetAssetDeletionPolicy() (model.AssetDeletionPolicy, error) {
	setting, err := s.settingRepo.FindByKey(model.SystemSettingKeyAssetDeletionPolicy)
	if err != nil {
		return model.AssetDeletionPolicy{}, err
	}

	policy := model.DefaultAssetDeletionPolicy()
	if setting != nil && strings.TrimSpace(setting.Value) != "" {
		if err := json.Unmarshal([]byte(setting.Value), &policy); err != nil {
			return model.AssetDeletionPolicy{}, err
		}
	}
	policy.Normalize()
	return policy, nil
}

func (s *assetDeletionPolicyService) UpdateAssetDeletionPolicy(
	adminID uint,
	update AssetDeletionPolicyUpdate,
	meta AdminRequestMeta,
) (model.AssetDeletionPolicy, error) {
	if adminID == 0 || !validAdminReason(update.Reason) {
		return model.AssetDeletionPolicy{}, ErrInvalidInput
	}
	policy, err := s.GetAssetDeletionPolicy()
	if err != nil {
		return model.AssetDeletionPolicy{}, err
	}
	before := assetDeletionPolicySnapshot(policy)
	if update.RecentDeletedRetentionDays != nil {
		policy.RecentDeletedRetentionDays = *update.RecentDeletedRetentionDays
	}
	if update.TombstoneRetentionDays != nil {
		policy.TombstoneRetentionDays = *update.TombstoneRetentionDays
	}
	if update.CleanupBatchLimit != nil {
		policy.CleanupBatchLimit = *update.CleanupBatchLimit
	}
	if update.AutoCleanupEnabled != nil {
		policy.AutoCleanupEnabled = *update.AutoCleanupEnabled
	}
	policy.Normalize()

	encoded, err := json.Marshal(policy)
	if err != nil {
		return model.AssetDeletionPolicy{}, err
	}
	if err := s.settingRepo.Upsert(&model.SystemSetting{
		Key: model.SystemSettingKeyAssetDeletionPolicy, Value: string(encoded),
	}); err != nil {
		return model.AssetDeletionPolicy{}, err
	}
	if s.auditService != nil {
		_ = s.auditService.Record(AdminAuditEntry{
			AdminUserID: adminID, Action: model.AuditActionSystemAssetDeletionPolicyUpdate,
			ResourceType: "system_setting", ResourceID: model.SystemSettingKeyAssetDeletionPolicy,
			BeforeSnapshot: before, AfterSnapshot: assetDeletionPolicySnapshot(policy),
			IPAddress: meta.IPAddress, UserAgent: meta.UserAgent, Reason: strings.TrimSpace(update.Reason),
		})
	}
	return policy, nil
}

func (s *assetDeletionPolicyService) CleanupExpiredAssets(
	adminID uint,
	reason string,
	meta AdminRequestMeta,
) (AssetTrashCleanupResult, error) {
	if adminID == 0 || !validAdminReason(reason) {
		return AssetTrashCleanupResult{}, ErrInvalidInput
	}
	return s.cleanup(adminID, strings.TrimSpace(reason), meta, false)
}

func (s *assetDeletionPolicyService) CleanupExpiredAssetsBySystem() (AssetTrashCleanupResult, error) {
	return s.cleanup(0, "系统周期性清理过期资产密文与安全可回收墓碑", AdminRequestMeta{}, true)
}

func (s *assetDeletionPolicyService) cleanup(
	adminID uint,
	reason string,
	meta AdminRequestMeta,
	systemRun bool,
) (AssetTrashCleanupResult, error) {
	if s.configRepo == nil {
		return AssetTrashCleanupResult{}, errors.New("asset deletion repository is not configured")
	}
	policy, err := s.GetAssetDeletionPolicy()
	if err != nil {
		return AssetTrashCleanupResult{}, err
	}
	now := s.now().UTC().Truncate(time.Millisecond)
	result := AssetTrashCleanupResult{
		Enabled: policy.AutoCleanupEnabled, RecoveryCutoff: now,
		CleanupBatchLimit: policy.CleanupBatchLimit, CompletedAt: now,
	}
	if systemRun && !policy.AutoCleanupEnabled {
		return result, nil
	}

	expired, err := s.configRepo.ListExpiredDeleted(now, policy.CleanupBatchLimit)
	if err != nil {
		return AssetTrashCleanupResult{}, err
	}
	result.ScannedCount = len(expired)
	for i := range expired {
		item := expired[i]
		didPurge := false
		_, mutateErr := s.configRepo.MutateByAssetID(item.UserID, item.AssetID, func(current *model.ServerConfig) (bool, error) {
			if current.State != model.ServerConfigStateDeleted || current.PurgeAfter == nil || current.PurgeAfter.After(now) {
				return false, nil
			}
			clock, clockErr := bumpVectorClock(current.VectorClock, "server-cleanup", now.UnixMilli())
			if clockErr != nil {
				return false, clockErr
			}
			operationID, operationErr := randomUUID()
			if operationErr != nil {
				return false, operationErr
			}
			current.State = model.ServerConfigStatePurged
			current.VectorClock = clock
			current.EncryptedBlob = []byte{}
			current.PurgeAfter = nil
			current.LastOperationID = operationID
			didPurge = true
			return true, nil
		})
		if mutateErr != nil {
			result.FailedCount++
			continue
		}
		if didPurge {
			result.PurgedCount++
		}
	}

	if policy.TombstoneRetentionDays > 0 {
		cutoff := now.AddDate(0, 0, -policy.TombstoneRetentionDays)
		result.TombstoneCutoff = &cutoff
		deleted, deferred, deleteErr := s.configRepo.DeleteAcknowledgedPurgedBefore(cutoff, policy.CleanupBatchLimit)
		if deleteErr != nil {
			return result, deleteErr
		}
		result.TombstonesDeleted = deleted
		result.TombstonesDeferred = deferred
	}
	result.CompletedAt = s.now().UTC().Truncate(time.Millisecond)

	if s.auditService != nil && (!systemRun || result.PurgedCount > 0 || result.TombstonesDeleted > 0) {
		_ = s.auditService.Record(AdminAuditEntry{
			AdminUserID: adminID, Action: model.AuditActionSystemAssetTrashCleanup,
			ResourceType: "server_config", ResourceID: "expired_asset_trash",
			AfterSnapshot: assetTrashCleanupSnapshot(result), IPAddress: meta.IPAddress,
			UserAgent: meta.UserAgent, Reason: reason,
		})
	}
	return result, nil
}

func randomUUID() (string, error) {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", err
	}
	bytes[6] = (bytes[6] & 0x0f) | 0x40
	bytes[8] = (bytes[8] & 0x3f) | 0x80
	hexValue := hex.EncodeToString(bytes[:])
	return hexValue[0:8] + "-" + hexValue[8:12] + "-" + hexValue[12:16] + "-" + hexValue[16:20] + "-" + hexValue[20:32], nil
}

func assetDeletionPolicySnapshot(policy model.AssetDeletionPolicy) string {
	encoded, err := json.Marshal(policy)
	if err != nil {
		return strconv.Quote("asset_deletion_policy_snapshot_failed")
	}
	return string(encoded)
}

func assetTrashCleanupSnapshot(result AssetTrashCleanupResult) string {
	encoded, err := json.Marshal(result)
	if err != nil {
		return strconv.Quote("asset_trash_cleanup_snapshot_failed")
	}
	return string(encoded)
}
