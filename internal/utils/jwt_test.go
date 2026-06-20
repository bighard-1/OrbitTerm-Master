package utils

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestGenerateTokenPairIncludesTokenVersion(t *testing.T) {
	manager := NewJWTManager("test-secret-that-is-long-enough", "orbitterm-test", 15, 30, 24)

	pair, err := manager.GenerateTokenPair(42, "alice", 7)
	if err != nil {
		t.Fatalf("GenerateTokenPair failed: %v", err)
	}

	accessClaims, err := manager.ParseAndVerifyAccessToken(pair.AccessToken)
	if err != nil {
		t.Fatalf("ParseAndVerifyAccessToken failed: %v", err)
	}
	if accessClaims.UserID != 42 || accessClaims.Username != "alice" || accessClaims.TokenVersion != 7 {
		t.Fatalf("unexpected access claims: %+v", accessClaims)
	}

	refreshClaims, err := manager.ParseAndVerifyRefreshToken(pair.RefreshToken)
	if err != nil {
		t.Fatalf("ParseAndVerifyRefreshToken failed: %v", err)
	}
	if refreshClaims.UserID != 42 || refreshClaims.Username != "alice" || refreshClaims.TokenVersion != 7 {
		t.Fatalf("unexpected refresh claims: %+v", refreshClaims)
	}
}

func TestJWTManagerRejectsOtherHMACAlgorithms(t *testing.T) {
	secret := "test-secret-that-is-long-enough"
	manager := NewJWTManager(secret, "orbitterm-test", 15, 30, 24)
	claims := CustomClaims{
		UserID: 1, Username: "alice", TokenType: "access",
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer: "orbitterm-test", ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS512, claims)
	signed, err := token.SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("sign HS512 test token: %v", err)
	}
	if _, err := manager.ParseAndVerifyToken(signed); err == nil {
		t.Fatal("HS512 token must be rejected when only HS256 is configured")
	}
}
