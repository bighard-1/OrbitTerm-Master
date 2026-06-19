package utils

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// JWTManager 管理 JWT 令牌签发与校验。
type JWTManager struct {
	secretKey           []byte
	issuer              string
	accessExpireMinutes int
	refreshExpireDays   int
}

// CustomClaims 是 OrbitTerm 使用的 JWT 声明。
type CustomClaims struct {
	UserID       uint   `json:"uid"`
	Username     string `json:"username"`
	TokenType    string `json:"typ"`
	TokenVersion int64  `json:"token_version"`
	jwt.RegisteredClaims
}

type TokenPair struct {
	AccessToken             string
	RefreshToken            string
	AccessExpiresInSeconds  int64
	RefreshExpiresInSeconds int64
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
	now := time.Now().UTC()
	accessExpiresAt := now.Add(time.Duration(m.accessExpireMinutes) * time.Minute)
	refreshExpiresAt := now.Add(time.Duration(m.refreshExpireDays) * 24 * time.Hour)

	accessClaims := CustomClaims{
		UserID:       userID,
		Username:     username,
		TokenType:    "access",
		TokenVersion: tokenVersion,
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
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    m.issuer,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(refreshExpiresAt),
			Subject:   fmt.Sprintf("user:%d", userID),
		},
	}

	access := jwt.NewWithClaims(jwt.SigningMethodHS256, accessClaims)
	accessToken, err := access.SignedString(m.secretKey)
	if err != nil {
		return nil, err
	}

	refresh := jwt.NewWithClaims(jwt.SigningMethodHS256, refreshClaims)
	refreshToken, err := refresh.SignedString(m.secretKey)
	if err != nil {
		return nil, err
	}

	return &TokenPair{
		AccessToken:             accessToken,
		RefreshToken:            refreshToken,
		AccessExpiresInSeconds:  int64(time.Until(accessExpiresAt).Seconds()),
		RefreshExpiresInSeconds: int64(time.Until(refreshExpiresAt).Seconds()),
	}, nil
}

// ParseAndVerifyToken 解析并验证 JWT：
// 1) 仅允许 HS256；
// 2) 校验签名；
// 3) 校验过期时间与 Issuer。
func (m *JWTManager) ParseAndVerifyToken(tokenString string) (*CustomClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &CustomClaims{}, func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
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
