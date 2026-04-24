package service

import (
	"encoding/json"
	"errors"

	"orbitterm-server/internal/model"
	"orbitterm-server/internal/repository"
)

var (
	ErrConfigInvalidInput  = errors.New("配置参数不合法")
	ErrConfigNotFound      = errors.New("配置不存在")
	ErrVectorClockConflict = errors.New("版本冲突，请先拉取最新配置后再上传")
)

// ConfigService 提供 ServerConfig 云同步业务能力。
type ConfigService interface {
	Upload(userID uint, configID *uint, encryptedBlob []byte, vectorClock string) (*model.ServerConfig, error)
	Pull(userID uint) ([]model.ServerConfig, error)
	Delete(userID, configID uint) error
}

type configService struct {
	configRepo repository.ServerConfigRepository
}

func NewConfigService(configRepo repository.ServerConfigRepository) ConfigService {
	return &configService{configRepo: configRepo}
}

// Upload 上传密文配置：
// 1) 新配置直接创建；
// 2) 已存在配置则基于 Vector Clock 判断是否允许覆盖；
// 3) 后端不解密，仅存储密文。
func (s *configService) Upload(userID uint, configID *uint, encryptedBlob []byte, vectorClock string) (*model.ServerConfig, error) {
	if userID == 0 || len(encryptedBlob) == 0 || vectorClock == "" {
		return nil, ErrConfigInvalidInput
	}

	if !json.Valid([]byte(vectorClock)) {
		return nil, ErrConfigInvalidInput
	}

	if configID == nil || *configID == 0 {
		cfg := &model.ServerConfig{
			UserID:        userID,
			EncryptedBlob: encryptedBlob,
			VectorClock:   vectorClock,
		}
		if err := s.configRepo.Create(cfg); err != nil {
			return nil, err
		}
		return cfg, nil
	}

	existing, err := s.configRepo.FindByIDAndUserID(*configID, userID)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, ErrConfigNotFound
	}

	relation, err := compareVectorClock(vectorClock, existing.VectorClock)
	if err != nil {
		return nil, ErrConfigInvalidInput
	}

	// 仅当上传版本“新于或等于”当前版本时才允许覆盖。
	if relation == vectorClockOlder || relation == vectorClockConflict {
		return nil, ErrVectorClockConflict
	}

	existing.EncryptedBlob = encryptedBlob
	existing.VectorClock = vectorClock
	if err := s.configRepo.Update(existing); err != nil {
		return nil, err
	}
	return existing, nil
}

// Pull 拉取当前用户全部配置。
func (s *configService) Pull(userID uint) ([]model.ServerConfig, error) {
	if userID == 0 {
		return nil, ErrConfigInvalidInput
	}
	return s.configRepo.ListByUserID(userID)
}

// Delete 删除指定配置。
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

type vectorClockRelation int

const (
	vectorClockEqual vectorClockRelation = iota
	vectorClockNewer
	vectorClockOlder
	vectorClockConflict
)

// compareVectorClock 比较上传版本与服务端版本：
// 返回值语义：
// - vectorClockNewer: incoming 可以覆盖 current
// - vectorClockEqual: incoming 与 current 相同，也允许幂等覆盖
// - vectorClockOlder/conflict: 拒绝覆盖，避免回滚或并发冲突
func compareVectorClock(incomingJSON, currentJSON string) (vectorClockRelation, error) {
	incoming, err := parseVectorClockJSON(incomingJSON)
	if err != nil {
		return vectorClockConflict, err
	}
	current, err := parseVectorClockJSON(currentJSON)
	if err != nil {
		return vectorClockConflict, err
	}

	allKeys := make(map[string]struct{}, len(incoming)+len(current))
	for k := range incoming {
		allKeys[k] = struct{}{}
	}
	for k := range current {
		allKeys[k] = struct{}{}
	}

	incomingGreater := false
	currentGreater := false
	for k := range allKeys {
		i := incoming[k]
		c := current[k]
		if i > c {
			incomingGreater = true
		}
		if i < c {
			currentGreater = true
		}
	}

	switch {
	case !incomingGreater && !currentGreater:
		return vectorClockEqual, nil
	case incomingGreater && !currentGreater:
		return vectorClockNewer, nil
	case !incomingGreater && currentGreater:
		return vectorClockOlder, nil
	default:
		return vectorClockConflict, nil
	}
}

func parseVectorClockJSON(raw string) (map[string]int64, error) {
	parsed := make(map[string]int64)
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return nil, err
	}
	return parsed, nil
}
