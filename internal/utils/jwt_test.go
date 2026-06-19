package utils

import "testing"

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
