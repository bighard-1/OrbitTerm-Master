package utils

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestTokenPairJSONUsesStableSnakeCaseKeys(t *testing.T) {
	encoded, err := json.Marshal(TokenPair{AccessToken: "access", RefreshToken: "refresh", AccessExpiresInSeconds: 10, RefreshExpiresInSeconds: 20})
	if err != nil {
		t.Fatalf("marshal token pair: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(encoded, &payload); err != nil {
		t.Fatalf("unmarshal token pair: %v", err)
	}
	if payload["access_token"] != "access" || payload["refresh_token"] != "refresh" {
		t.Fatalf("unexpected token wire payload: %s", encoded)
	}
	if _, leaked := payload["AccessToken"]; leaked {
		t.Fatalf("PascalCase token key must not be emitted: %s", encoded)
	}
}

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
