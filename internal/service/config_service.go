package service

import (
	"bytes"
	"errors"
	"strings"
	"time"

	"orbitterm-server/internal/model"
	"orbitterm-server/internal/repository"
)

var (
	ErrConfigInvalidInput  = errors.New("配置参数不合法")
	ErrConfigNotFound      = errors.New("配置不存在")
	ErrConfigInvalidState  = errors.New("配置当前状态不允许此操作")
	ErrConfigPurged        = errors.New("配置已经永久删除")
	ErrVectorClockConflict = errors.New("版本冲突，请先拉取最新配置后再上传")
)

// ConfigService 提供密文配置同步与墓碑生命周期能力。
type ConfigService interface {
	Upload(userID uint, configID *uint, assetID, identityFingerprint string, encryptedBlob []byte, vectorClock string) (*model.ServerConfig, error)
	Pull(userID uint) ([]model.ServerConfig, error)
	Delete(userID, configID uint) error
	DeleteAsset(userID uint, input AssetMutationInput) (*model.ServerConfig, error)
	ListTrash(userID uint, limit, offset int) (*TrashPage, error)
	RestoreAsset(userID uint, input AssetMutationInput) (*model.ServerConfig, error)
	PurgeAsset(userID uint, input AssetMutationInput) (*model.ServerConfig, error)
	PullChanges(userID uint, afterRevision uint64, limit int) (*SyncPullPage, error)
	AcknowledgeSync(userID uint, input SyncAcknowledgementInput) error
	FindIdentityMatches(userID uint, fingerprint string) ([]model.ServerConfig, error)
}

type configService struct {
	configRepo repository.ServerConfigRepository
	policy     AssetDeletionPolicyReader
	now        func() time.Time
}

func NewConfigService(configRepo repository.ServerConfigRepository, policy ...AssetDeletionPolicyReader) ConfigService {
	var reader AssetDeletionPolicyReader = defaultAssetDeletionPolicyReader{}
	if len(policy) > 0 && policy[0] != nil {
		reader = policy[0]
	}
	return &configService{
		configRepo: configRepo,
		policy:     reader,
		now:        func() time.Time { return time.Now().UTC() },
	}
}

type defaultAssetDeletionPolicyReader struct{}

func (defaultAssetDeletionPolicyReader) GetAssetDeletionPolicy() (model.AssetDeletionPolicy, error) {
	policy := model.DefaultAssetDeletionPolicy()
	policy.Normalize()
	return policy, nil
}

type configUploadInput struct {
	userID              uint
	configID            *uint
	assetID             string
	identityFingerprint string
	encryptedBlob       []byte
	vectorClock         string
}

// Upload 保持旧协议兼容，同时允许新版客户端回填稳定 AssetID。
// 已进入 deleted/purged 的资产只能通过显式恢复接口重新激活。
func (s *configService) Upload(userID uint, configID *uint, assetID, identityFingerprint string, encryptedBlob []byte, vectorClock string) (*model.ServerConfig, error) {
	input, err := normalizeConfigUploadInput(
		userID, configID, assetID, identityFingerprint, encryptedBlob, vectorClock,
	)
	if err != nil {
		return nil, err
	}

	existing, err := s.findUploadTarget(input)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		if input.hasConfigID() {
			return nil, ErrConfigNotFound
		}
		return s.createUpload(input)
	}
	return s.updateExistingUpload(existing, input)
}

func normalizeConfigUploadInput(
	userID uint,
	configID *uint,
	assetID, identityFingerprint string,
	encryptedBlob []byte,
	vectorClock string,
) (configUploadInput, error) {
	input := configUploadInput{
		userID:              userID,
		configID:            configID,
		assetID:             strings.TrimSpace(assetID),
		identityFingerprint: strings.ToLower(strings.TrimSpace(identityFingerprint)),
		encryptedBlob:       encryptedBlob,
		vectorClock:         vectorClock,
	}
	if input.userID == 0 || len(input.encryptedBlob) == 0 || !validVectorClock(input.vectorClock) {
		return configUploadInput{}, ErrConfigInvalidInput
	}
	if input.assetID != "" && !validUUID(input.assetID) {
		return configUploadInput{}, ErrConfigInvalidInput
	}
	if input.identityFingerprint != "" && !validHexDigest(input.identityFingerprint) {
		return configUploadInput{}, ErrConfigInvalidInput
	}
	return input, nil
}

func (input configUploadInput) hasConfigID() bool {
	return input.configID != nil && *input.configID != 0
}

func (s *configService) findUploadTarget(input configUploadInput) (*model.ServerConfig, error) {
	if input.hasConfigID() {
		return s.configRepo.FindByIDAndUserID(*input.configID, input.userID)
	}
	if input.assetID != "" {
		return s.configRepo.FindByAssetIDAndUserID(input.assetID, input.userID)
	}
	return nil, nil
}

func (s *configService) createUpload(input configUploadInput) (*model.ServerConfig, error) {
	cfg := &model.ServerConfig{
		UserID:              input.userID,
		AssetID:             input.assetID,
		IdentityFingerprint: input.identityFingerprint,
		EncryptedBlob:       input.encryptedBlob,
		VectorClock:         input.vectorClock,
		State:               model.ServerConfigStateActive,
	}
	createErr := s.configRepo.Create(cfg)
	if createErr == nil {
		return cfg, nil
	}
	if input.assetID == "" {
		return nil, createErr
	}

	// 两台设备可能同时首次上传同一 AssetID。唯一索引只允许一个胜出；
	// 失败方重新读取胜出记录并走相同的向量钟判断，避免返回误导性的 500。
	winner, findErr := s.configRepo.FindByAssetIDAndUserID(input.assetID, input.userID)
	if findErr != nil {
		return nil, findErr
	}
	if winner == nil {
		return nil, createErr
	}
	return s.updateExistingUpload(winner, input)
}

func (s *configService) updateExistingUpload(existing *model.ServerConfig, input configUploadInput) (*model.ServerConfig, error) {

	if existing.IsDeleted() {
		return nil, ErrConfigInvalidState
	}
	if input.assetID != "" && existing.AssetID != "" && existing.AssetID != input.assetID {
		return nil, ErrConfigInvalidInput
	}
	if existing.AssetID == "" && input.assetID != "" {
		bound, err := s.configRepo.FindByAssetIDAndUserID(input.assetID, input.userID)
		if err != nil {
			return nil, err
		}
		if bound != nil && bound.ID != existing.ID {
			return nil, ErrVectorClockConflict
		}
	}
	if existing.AssetID != "" {
		return s.updateAssetBoundConfig(existing.AssetID, input)
	}
	return s.updateLegacyConfig(existing, input)
}

func (s *configService) updateAssetBoundConfig(assetID string, input configUploadInput) (*model.ServerConfig, error) {
	updated, err := s.configRepo.MutateByAssetID(input.userID, assetID, func(current *model.ServerConfig) (bool, error) {
		if current.IsDeleted() {
			return false, ErrConfigInvalidState
		}
		relation, err := compareVectorClock(input.vectorClock, current.VectorClock)
		if err != nil {
			return false, ErrConfigInvalidInput
		}
		switch relation {
		case vectorClockOlder, vectorClockConflict:
			return false, ErrVectorClockConflict
		case vectorClockEqual:
			if bytes.Equal(current.EncryptedBlob, input.encryptedBlob) &&
				(input.identityFingerprint == "" || current.IdentityFingerprint == input.identityFingerprint) {
				return false, nil
			}
			return false, ErrVectorClockConflict
		}
		current.EncryptedBlob = input.encryptedBlob
		current.VectorClock = input.vectorClock
		if input.identityFingerprint != "" {
			current.IdentityFingerprint = input.identityFingerprint
		}
		return true, nil
	})
	if err != nil {
		return nil, err
	}
	if updated == nil {
		return nil, ErrConfigNotFound
	}
	return updated, nil
}

func (s *configService) updateLegacyConfig(existing *model.ServerConfig, input configUploadInput) (*model.ServerConfig, error) {
	relation, err := compareVectorClock(input.vectorClock, existing.VectorClock)
	if err != nil {
		return nil, ErrConfigInvalidInput
	}
	if relation == vectorClockOlder || relation == vectorClockConflict {
		return nil, ErrVectorClockConflict
	}
	if relation == vectorClockEqual {
		if bytes.Equal(existing.EncryptedBlob, input.encryptedBlob) {
			return existing, nil
		}
		return nil, ErrVectorClockConflict
	}

	if existing.AssetID == "" {
		existing.AssetID = input.assetID
	}
	if input.identityFingerprint != "" {
		existing.IdentityFingerprint = input.identityFingerprint
	}
	existing.EncryptedBlob = input.encryptedBlob
	existing.VectorClock = input.vectorClock
	existing.State = model.ServerConfigStateActive
	if err := s.configRepo.Update(existing); err != nil {
		return nil, err
	}
	return existing, nil
}

func (s *configService) FindIdentityMatches(userID uint, fingerprint string) ([]model.ServerConfig, error) {
	fingerprint = strings.ToLower(strings.TrimSpace(fingerprint))
	if userID == 0 || !validHexDigest(fingerprint) {
		return nil, ErrConfigInvalidInput
	}
	return s.configRepo.ListByIdentityFingerprint(userID, fingerprint)
}

// Pull 是旧客户端兼容入口，只返回 active 记录，避免旧版本误把墓碑导入成正常资产。
func (s *configService) Pull(userID uint) ([]model.ServerConfig, error) {
	if userID == 0 {
		return nil, ErrConfigInvalidInput
	}
	return s.configRepo.ListByUserID(userID)
}

// Delete 保留旧版数字 ID 物理删除语义，仅用于客户端迁移期。
func (s *configService) Delete(userID, configID uint) error {
	if userID == 0 || configID == 0 {
		return ErrConfigInvalidInput
	}
	deleted, err := s.configRepo.DeleteByIDAndUserID(configID, userID)
	if err != nil {
		if errors.Is(err, repository.ErrLegacyDeleteProtected) {
			return ErrConfigInvalidState
		}
		return err
	}
	if !deleted {
		return ErrConfigNotFound
	}
	return nil
}
