package service

import (
	"encoding/json"
	"strings"

	"orbitterm-server/internal/model"
	"orbitterm-server/internal/repository"
)

type AssetDeletionPolicyReader interface {
	GetAssetDeletionPolicy() (model.AssetDeletionPolicy, error)
}

type assetDeletionPolicyService struct {
	settingRepo repository.SystemSettingRepository
}

func NewAssetDeletionPolicyService(settingRepo repository.SystemSettingRepository) AssetDeletionPolicyReader {
	return &assetDeletionPolicyService{settingRepo: settingRepo}
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
