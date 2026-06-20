package service

import (
	"strings"

	"orbitterm-server/internal/model"
)

type AssetMutationInput struct {
	AssetID     string
	DeviceID    string
	OperationID string
	VectorClock string
}

type TrashPage struct {
	Items  []model.ServerConfig
	Total  int64
	Limit  int
	Offset int
}

func (s *configService) DeleteAsset(userID uint, input AssetMutationInput) (*model.ServerConfig, error) {
	if userID == 0 || !validAssetMutationInput(input) {
		return nil, ErrConfigInvalidInput
	}
	policy, err := s.policy.GetAssetDeletionPolicy()
	if err != nil {
		return nil, err
	}
	now := s.now()

	config, err := s.configRepo.MutateByAssetID(userID, input.AssetID, func(cfg *model.ServerConfig) (bool, error) {
		if cfg.LastOperationID == input.OperationID && cfg.IsDeleted() {
			return false, nil
		}
		if cfg.State == model.ServerConfigStatePurged {
			return false, ErrConfigPurged
		}

		relation, err := compareVectorClock(input.VectorClock, cfg.VectorClock)
		if err != nil {
			return false, ErrConfigInvalidInput
		}
		if relation == vectorClockOlder || relation == vectorClockEqual {
			return false, ErrVectorClockConflict
		}
		clock := input.VectorClock
		if relation == vectorClockConflict {
			clock, err = mergeVectorClocks(input.VectorClock, cfg.VectorClock)
			if err != nil {
				return false, ErrConfigInvalidInput
			}
		}

		cfg.State = model.ServerConfigStateDeleted
		cfg.VectorClock = clock
		cfg.DeletedByDeviceID = strings.TrimSpace(input.DeviceID)
		cfg.LastOperationID = input.OperationID
		if cfg.DeletedAt == nil {
			cfg.DeletedAt = &now
			purgeAfter := now.AddDate(0, 0, policy.RecentDeletedRetentionDays)
			cfg.PurgeAfter = &purgeAfter
		}
		return true, nil
	})
	if err != nil {
		return nil, err
	}
	if config == nil {
		return nil, ErrConfigNotFound
	}
	return config, nil
}

func (s *configService) ListTrash(userID uint, limit, offset int) (*TrashPage, error) {
	if userID == 0 || offset < 0 {
		return nil, ErrConfigInvalidInput
	}
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	items, total, err := s.configRepo.ListTrashByUserID(userID, limit, offset)
	if err != nil {
		return nil, err
	}
	return &TrashPage{Items: items, Total: total, Limit: limit, Offset: offset}, nil
}

func (s *configService) RestoreAsset(userID uint, input AssetMutationInput) (*model.ServerConfig, error) {
	if userID == 0 || !validAssetMutationInput(input) {
		return nil, ErrConfigInvalidInput
	}
	config, err := s.configRepo.MutateByAssetID(userID, input.AssetID, func(cfg *model.ServerConfig) (bool, error) {
		if cfg.LastOperationID == input.OperationID && cfg.State == model.ServerConfigStateActive {
			return false, nil
		}
		if cfg.State == model.ServerConfigStatePurged {
			return false, ErrConfigPurged
		}
		if cfg.State != model.ServerConfigStateDeleted {
			return false, ErrConfigInvalidState
		}
		relation, err := compareVectorClock(input.VectorClock, cfg.VectorClock)
		if err != nil {
			return false, ErrConfigInvalidInput
		}
		if relation != vectorClockNewer {
			return false, ErrVectorClockConflict
		}

		cfg.State = model.ServerConfigStateActive
		cfg.VectorClock = input.VectorClock
		cfg.DeletedAt = nil
		cfg.PurgeAfter = nil
		cfg.DeletedByDeviceID = ""
		cfg.LastOperationID = input.OperationID
		return true, nil
	})
	if err != nil {
		return nil, err
	}
	if config == nil {
		return nil, ErrConfigNotFound
	}
	return config, nil
}

func (s *configService) PurgeAsset(userID uint, input AssetMutationInput) (*model.ServerConfig, error) {
	if userID == 0 || !validAssetMutationInput(input) {
		return nil, ErrConfigInvalidInput
	}
	config, err := s.configRepo.MutateByAssetID(userID, input.AssetID, func(cfg *model.ServerConfig) (bool, error) {
		if cfg.LastOperationID == input.OperationID && cfg.State == model.ServerConfigStatePurged {
			return false, nil
		}
		if cfg.State == model.ServerConfigStatePurged {
			return false, ErrConfigPurged
		}
		if cfg.State != model.ServerConfigStateDeleted {
			return false, ErrConfigInvalidState
		}
		relation, err := compareVectorClock(input.VectorClock, cfg.VectorClock)
		if err != nil {
			return false, ErrConfigInvalidInput
		}
		if relation != vectorClockNewer {
			return false, ErrVectorClockConflict
		}

		cfg.State = model.ServerConfigStatePurged
		cfg.VectorClock = input.VectorClock
		cfg.EncryptedBlob = []byte{}
		cfg.PurgeAfter = nil
		cfg.LastOperationID = input.OperationID
		return true, nil
	})
	if err != nil {
		return nil, err
	}
	if config == nil {
		return nil, ErrConfigNotFound
	}
	return config, nil
}

func validAssetMutationInput(input AssetMutationInput) bool {
	return validUUID(strings.TrimSpace(input.AssetID)) &&
		validUUID(strings.TrimSpace(input.OperationID)) &&
		validUUID(strings.TrimSpace(input.DeviceID)) &&
		validVectorClock(input.VectorClock)
}
