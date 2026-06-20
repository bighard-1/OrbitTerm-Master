package service

import (
	"strings"
	"time"

	"orbitterm-server/internal/model"
)

type SyncPullPage struct {
	Items         []model.ServerConfig
	NextCursor    uint64
	HasMore       bool
	ResetRequired bool
}

type SyncAcknowledgementInput struct {
	DeviceID      string
	Revision      uint64
	Platform      string
	ClientVersion string
}

func (s *configService) PullChanges(userID uint, afterRevision uint64, limit int) (*SyncPullPage, error) {
	if userID == 0 {
		return nil, ErrConfigInvalidInput
	}
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}

	maxRevision, err := s.configRepo.MaxRevisionByUserID(userID)
	if err != nil {
		return nil, err
	}
	if afterRevision > maxRevision {
		return &SyncPullPage{NextCursor: 0, ResetRequired: true}, nil
	}

	items, hasMore, err := s.configRepo.ListChangedByUserID(userID, afterRevision, limit)
	if err != nil {
		return nil, err
	}
	nextCursor := afterRevision
	if len(items) > 0 {
		nextCursor = items[len(items)-1].ServerRevision
	}
	return &SyncPullPage{
		Items:      items,
		NextCursor: nextCursor,
		HasMore:    hasMore,
	}, nil
}

func (s *configService) AcknowledgeSync(userID uint, input SyncAcknowledgementInput) error {
	input.DeviceID = strings.TrimSpace(input.DeviceID)
	input.Platform = strings.TrimSpace(input.Platform)
	input.ClientVersion = strings.TrimSpace(input.ClientVersion)
	if userID == 0 || !validUUID(input.DeviceID) || len(input.Platform) > 32 || len(input.ClientVersion) > 32 {
		return ErrConfigInvalidInput
	}

	maxRevision, err := s.configRepo.MaxRevisionByUserID(userID)
	if err != nil {
		return err
	}
	if input.Revision > maxRevision {
		return ErrConfigInvalidInput
	}
	return s.configRepo.AcknowledgeDevice(
		userID,
		input.DeviceID,
		input.Revision,
		input.Platform,
		input.ClientVersion,
		s.now().UTC().Truncate(time.Millisecond),
	)
}
