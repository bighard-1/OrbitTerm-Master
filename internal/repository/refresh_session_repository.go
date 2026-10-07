package repository

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	"orbitterm-server/internal/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type refreshSessionRepository struct{ db *gorm.DB }

func NewRefreshSessionRepository(db *gorm.DB) *refreshSessionRepository {
	return &refreshSessionRepository{db: db}
}

func digestTokenID(id string) string {
	digest := sha256.Sum256([]byte(id))
	return hex.EncodeToString(digest[:])
}

func (r *refreshSessionRepository) Create(userID uint, familyID, tokenID string, version int64, expires time.Time) error {
	return r.db.Create(&model.RefreshSession{UserID: userID, FamilyID: familyID, TokenDigest: digestTokenID(tokenID), TokenVersion: version, ExpiresAt: expires}).Error
}

// Rotate commits replay-family revocation even when the presented token has
// already been used. The caller must not turn replay into a transaction error.
func (r *refreshSessionRepository) Rotate(userID uint, familyID, oldID, newID string, version int64, expires time.Time) (bool, error) {
	accepted := false
	err := r.db.Transaction(func(tx *gorm.DB) error {
		var old model.RefreshSession
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("user_id = ? AND family_id = ? AND token_digest = ?", userID, familyID, digestTokenID(oldID)).First(&old).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		if old.UsedAt != nil || old.RevokedAt != nil {
			return tx.Model(&model.RefreshSession{}).Where("user_id = ? AND family_id = ? AND revoked_at IS NULL", userID, familyID).Update("revoked_at", now).Error
		}
		if old.TokenVersion != version || !old.ExpiresAt.After(now) {
			return nil
		}
		if err := tx.Model(&old).Update("used_at", now).Error; err != nil {
			return err
		}
		if err := tx.Create(&model.RefreshSession{UserID: userID, FamilyID: familyID, TokenDigest: digestTokenID(newID), TokenVersion: version, ExpiresAt: expires}).Error; err != nil {
			return err
		}
		accepted = true
		return nil
	})
	return accepted, err
}

func (r *refreshSessionRepository) Active(userID uint, familyID string, version int64) (bool, error) {
	var count int64
	err := r.db.Model(&model.RefreshSession{}).Where("user_id = ? AND family_id = ? AND token_version = ? AND revoked_at IS NULL AND expires_at > ?", userID, familyID, version, time.Now().UTC()).Count(&count).Error
	return count > 0, err
}

func (r *refreshSessionRepository) RevokeFamily(userID uint, familyID string) error {
	return r.db.Model(&model.RefreshSession{}).Where("user_id = ? AND family_id = ? AND revoked_at IS NULL", userID, familyID).Update("revoked_at", time.Now().UTC()).Error
}

func (r *refreshSessionRepository) PruneExpired(limit int) (int64, error) {
	if limit <= 0 || limit > 1000 {
		limit = 500
	}
	var ids []uint
	if err := r.db.Model(&model.RefreshSession{}).Where("expires_at < ?", time.Now().UTC()).Order("expires_at ASC").Limit(limit).Pluck("id", &ids).Error; err != nil {
		return 0, err
	}
	if len(ids) == 0 {
		return 0, nil
	}
	result := r.db.Where("id IN ?", ids).Delete(&model.RefreshSession{})
	return result.RowsAffected, result.Error
}
