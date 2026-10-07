package utils

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"orbitterm-server/internal/session"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var ErrRefreshRejected = errors.New("refresh session rejected")
var ErrRefreshInProgress = errors.New("refresh session already rotated in another request")

// JWTManager 管理 JWT 令牌签发与校验。
type JWTManager struct {
	secretKey           []byte
	issuer              string
	accessExpireMinutes int
	refreshExpireDays   int
	sessions            RefreshSessionStore
	allowLegacyRefresh  bool
}

type RefreshSessionStore interface {
	Create(userID uint, familyID, tokenID string, version int64, expires time.Time) error
	Rotate(userID uint, familyID, oldID, newID string, version int64, expires time.Time) (bool, error)
	Active(userID uint, familyID string, version int64) (bool, error)
	RevokeFamily(userID uint, familyID string) error
}

// CustomClaims 是 OrbitTerm 使用的 JWT 声明。
type CustomClaims struct {
	UserID       uint   `json:"uid"`
	Username     string `json:"username"`
	TokenType    string `json:"typ"`
	TokenVersion int64  `json:"token_version"`
	SessionID    string `json:"sid,omitempty"`
	AuthTime     int64  `json:"auth_time,omitempty"`
	jwt.RegisteredClaims
}

type TokenPair struct {
	AccessToken             string `json:"access_token"`
	RefreshToken            string `json:"refresh_token"`
	AccessExpiresInSeconds  int64  `json:"expires_in_seconds"`
	RefreshExpiresInSeconds int64  `json:"refresh_expires_in_seconds"`
	MustChangePassword      bool   `json:"must_change_password"`
}

func NewJWTManager(secret, issuer string, accessExpireMinutes, refreshExpireDays, legacyExpireHours int) *JWTManager {
	if accessExpireMinutes <= 0 {
		accessExpireMinutes = legacyExpireHours * 60
		if accessExpireMinutes <= 0 {
			accessExpireMinutes = 15
		}
	}
	if refreshExpireDays <= 0 {
		refreshExpireDays = 30
	}
	return &JWTManager{
		secretKey:           []byte(secret),
		issuer:              issuer,
		accessExpireMinutes: accessExpireMinutes,
		refreshExpireDays:   refreshExpireDays,
	}
}

// GenerateTokenPair 为指定用户生成 access + refresh 双令牌。
// TokenVersion 会写入令牌，便于管理端后续实现强制下线和重置密码后失效旧 Token。
func (m *JWTManager) GenerateTokenPair(userID uint, username string, tokenVersion int64) (*TokenPair, error) {
	familyID, err := randomTokenID()
	if err != nil {
		return nil, err
	}
	pair, tokenID, expires, err := m.generateTokenPair(userID, username, tokenVersion, familyID, time.Now().UTC().Unix())
	if err != nil {
		return nil, err
	}
	if m.sessions != nil {
		if err := m.sessions.Create(userID, familyID, tokenID, tokenVersion, expires); err != nil {
			return nil, err
		}
	}
	return pair, nil
}

func (m *JWTManager) SetRefreshSessionStore(store RefreshSessionStore, allowLegacyRefresh bool) {
	m.sessions = store
	m.allowLegacyRefresh = allowLegacyRefresh
}

func (m *JWTManager) RotateTokenPair(claims *CustomClaims) (*TokenPair, error) {
	if claims == nil {
		return nil, errors.New("missing refresh claims")
	}
	if m.sessions == nil {
		pair, _, _, err := m.generateTokenPair(claims.UserID, claims.Username, claims.TokenVersion, claims.SessionID, tokenAuthTime(claims))
		return pair, err
	}
	if claims.SessionID == "" || claims.ID == "" {
		if !m.allowLegacyRefresh {
			return nil, ErrRefreshRejected
		}
		return m.generateLegacyReplacement(claims)
	}
	pair, tokenID, expires, err := m.generateTokenPair(claims.UserID, claims.Username, claims.TokenVersion, claims.SessionID, tokenAuthTime(claims))
	if err != nil {
		return nil, err
	}
	accepted, err := m.sessions.Rotate(claims.UserID, claims.SessionID, claims.ID, tokenID, claims.TokenVersion, expires)
	if err != nil {
		if errors.Is(err, session.ErrConcurrentRefresh) {
			return nil, ErrRefreshInProgress
		}
		return nil, err
	}
	if !accepted {
		return nil, ErrRefreshRejected
	}
	return pair, nil
}

func tokenAuthTime(claims *CustomClaims) int64 {
	if claims.AuthTime > 0 {
		return claims.AuthTime
	}
	if claims.IssuedAt != nil {
		return claims.IssuedAt.Unix()
	}
	return 0
}

func (m *JWTManager) generateLegacyReplacement(claims *CustomClaims) (*TokenPair, error) {
	familyID, err := randomTokenID()
	if err != nil {
		return nil, err
	}
	pair, tokenID, expires, err := m.generateTokenPair(claims.UserID, claims.Username, claims.TokenVersion, familyID, tokenAuthTime(claims))
	if err != nil {
		return nil, err
	}
	if m.sessions != nil {
		if err := m.sessions.Create(claims.UserID, familyID, tokenID, claims.TokenVersion, expires); err != nil {
			return nil, err
		}
	}
	return pair, nil
}

func (m *JWTManager) SessionActive(claims *CustomClaims) (bool, error) {
	if m.sessions == nil || claims.SessionID == "" {
		return true, nil
	}
	return m.sessions.Active(claims.UserID, claims.SessionID, claims.TokenVersion)
}

func (m *JWTManager) RevokeSession(userID uint, familyID string) error {
	if m.sessions == nil || familyID == "" {
		return errors.New("session not available")
	}
	return m.sessions.RevokeFamily(userID, familyID)
}

func randomTokenID() (string, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw[:]), nil
}

func (m *JWTManager) generateTokenPair(userID uint, username string, tokenVersion int64, familyID string, authTime int64) (*TokenPair, string, time.Time, error) {
	now := time.Now().UTC()
	accessExpiresAt := now.Add(time.Duration(m.accessExpireMinutes) * time.Minute)
	refreshExpiresAt := now.Add(time.Duration(m.refreshExpireDays) * 24 * time.Hour)
	tokenID, err := randomTokenID()
	if err != nil {
		return nil, "", time.Time{}, err
	}

	accessClaims := CustomClaims{
		UserID:       userID,
		Username:     username,
		TokenType:    "access",
		TokenVersion: tokenVersion,
		SessionID:    familyID,
		AuthTime:     authTime,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    m.issuer,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(accessExpiresAt),
			Subject:   fmt.Sprintf("user:%d", userID),
		},
	}

	refreshClaims := CustomClaims{
		UserID:       userID,
		Username:     username,
		TokenType:    "refresh",
		TokenVersion: tokenVersion,
		SessionID:    familyID,
		AuthTime:     authTime,
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        tokenID,
			Issuer:    m.issuer,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(refreshExpiresAt),
			Subject:   fmt.Sprintf("user:%d", userID),
		},
	}

	access := jwt.NewWithClaims(jwt.SigningMethodHS256, accessClaims)
	accessToken, err := access.SignedString(m.secretKey)
	if err != nil {
		return nil, "", time.Time{}, err
	}

	refresh := jwt.NewWithClaims(jwt.SigningMethodHS256, refreshClaims)
	refreshToken, err := refresh.SignedString(m.secretKey)
	if err != nil {
		return nil, "", time.Time{}, err
	}

	return &TokenPair{
		AccessToken:             accessToken,
		RefreshToken:            refreshToken,
		AccessExpiresInSeconds:  int64(time.Until(accessExpiresAt).Seconds()),
		RefreshExpiresInSeconds: int64(time.Until(refreshExpiresAt).Seconds()),
	}, tokenID, refreshExpiresAt, nil
}

// ParseAndVerifyToken 解析并验证 JWT：
// 1) 仅允许 HS256；
// 2) 校验签名；
// 3) 校验过期时间与 Issuer。
func (m *JWTManager) ParseAndVerifyToken(tokenString string) (*CustomClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &CustomClaims{}, func(token *jwt.Token) (any, error) {
		if token.Method.Alg() != jwt.SigningMethodHS256.Alg() {
			return nil, errors.New("unexpected signing method")
		}
		return m.secretKey, nil
	}, jwt.WithIssuer(m.issuer), jwt.WithLeeway(5*time.Second))
	if err != nil {
		return nil, err
	}

	claims, ok := token.Claims.(*CustomClaims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid token claims")
	}

	return claims, nil
}

func (m *JWTManager) ParseAndVerifyAccessToken(tokenString string) (*CustomClaims, error) {
	claims, err := m.ParseAndVerifyToken(tokenString)
	if err != nil {
		return nil, err
	}
	if claims.TokenType != "access" {
		return nil, errors.New("invalid token type")
	}
	return claims, nil
}

func (m *JWTManager) ParseAndVerifyRefreshToken(tokenString string) (*CustomClaims, error) {
	claims, err := m.ParseAndVerifyToken(tokenString)
	if err != nil {
		return nil, err
	}
	if claims.TokenType != "refresh" {
		return nil, errors.New("invalid token type")
	}
	return claims, nil
}
