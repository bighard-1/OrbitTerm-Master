package service

import (
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
	Upload(userID uint, configID *uint, assetID string, encryptedBlob []byte, vectorClock string) (*model.ServerConfig, error)
	Pull(userID uint) ([]model.ServerConfig, error)
	Delete(userID, configID uint) error
	DeleteAsset(userID uint, input AssetMutationInput) (*model.ServerConfig, error)
	ListTrash(userID uint, limit, offset int) (*TrashPage, error)
	RestoreAsset(userID uint, input AssetMutationInput) (*model.ServerConfig, error)
	PurgeAsset(userID uint, input AssetMutationInput) (*model.ServerConfig, error)
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

// Upload 保持旧协议兼容，同时允许新版客户端回填稳定 AssetID。
// 已进入 deleted/purged 的资产只能通过显式恢复接口重新激活。
func (s *configService) Upload(userID uint, configID *uint, assetID string, encryptedBlob []byte, vectorClock string) (*model.ServerConfig, error) {
	assetID = strings.TrimSpace(assetID)
	if userID == 0 || len(encryptedBlob) == 0 || !validVectorClock(vectorClock) {
		return nil, ErrConfigInvalidInput
	}
	if assetID != "" && !validUUID(assetID) {
		return nil, ErrConfigInvalidInput
	}

	var existing *model.ServerConfig
	var err error
	if configID != nil && *configID != 0 {
		existing, err = s.configRepo.FindByIDAndUserID(*configID, userID)
	} else if assetID != "" {
		existing, err = s.configRepo.FindByAssetIDAndUserID(assetID, userID)
	}
	if err != nil {
		return nil, err
	}

	if existing == nil {
		if configID != nil && *configID != 0 {
			return nil, ErrConfigNotFound
		}
		cfg := &model.ServerConfig{
			UserID:        userID,
			AssetID:       assetID,
			EncryptedBlob: encryptedBlob,
			VectorClock:   vectorClock,
			State:         model.ServerConfigStateActive,
		}
		if err := s.configRepo.Create(cfg); err != nil {
			return nil, err
		}
		return cfg, nil
	}

	if existing.IsDeleted() {
		return nil, ErrConfigInvalidState
	}
	if assetID != "" && existing.AssetID != "" && existing.AssetID != assetID {
		return nil, ErrConfigInvalidInput
	}
	if existing.AssetID == "" && assetID != "" {
		bound, err := s.configRepo.FindByAssetIDAndUserID(assetID, userID)
		if err != nil {
			return nil, err
		}
		if bound != nil && bound.ID != existing.ID {
			return nil, ErrVectorClockConflict
		}
	}

	relation, err := compareVectorClock(vectorClock, existing.VectorClock)
	if err != nil {
		return nil, ErrConfigInvalidInput
	}
	if relation == vectorClockOlder || relation == vectorClockConflict {
		return nil, ErrVectorClockConflict
	}

	if existing.AssetID == "" {
		existing.AssetID = assetID
	}
	existing.EncryptedBlob = encryptedBlob
	existing.VectorClock = vectorClock
	existing.State = model.ServerConfigStateActive
	if err := s.configRepo.Update(existing); err != nil {
		return nil, err
	}
	return existing, nil
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
		return err
	}
	if !deleted {
		return ErrConfigNotFound
	}
	return nil
}
