package utils

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// JWTManager 管理 JWT 令牌签发与校验。
type JWTManager struct {
	secretKey   []byte
	issuer      string
	expireHours int
}

// CustomClaims 是 OrbitTerm 使用的 JWT 声明。
type CustomClaims struct {
	UserID   uint   `json:"uid"`
	Username string `json:"username"`
	jwt.RegisteredClaims
}

func NewJWTManager(secret, issuer string, expireHours int) *JWTManager {
	return &JWTManager{
		secretKey:   []byte(secret),
		issuer:      issuer,
		expireHours: expireHours,
	}
}

// GenerateToken 为指定用户生成签名后的 JWT。
func (m *JWTManager) GenerateToken(userID uint, username string) (string, error) {
	now := time.Now().UTC()
	expiresAt := now.Add(time.Duration(m.expireHours) * time.Hour)

	claims := CustomClaims{
		UserID:   userID,
		Username: username,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    m.issuer,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			Subject:   fmt.Sprintf("user:%d", userID),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(m.secretKey)
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
