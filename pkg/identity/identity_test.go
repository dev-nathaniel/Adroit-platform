package identity_test

import (
	"testing"
	"time"

	"github.com/dev-nathaniel/Adroit-platform/pkg/identity"
)

func TestPasswordHashing(t *testing.T) {
	password := "SecretP@ssw0rd!123"

	hash, err := identity.HashPassword(password)
	if err != nil {
		t.Fatalf("failed to hash password: %v", err)
	}

	if hash == password {
		t.Errorf("hash should not equal plain password")
	}

	if !identity.CheckPassword(password, hash) {
		t.Errorf("CheckPassword expected true for correct password")
	}

	if identity.CheckPassword("wrongPassword", hash) {
		t.Errorf("CheckPassword expected false for incorrect password")
	}
}

func TestJWTGenerationAndValidation(t *testing.T) {
	secret := "test-jwt-secret-key-32-chars-long!"
	userID := "usr_550e8400-e29b-41d4-a716-446655440000"
	email := "developer@adroit.run"

	tokenStr, err := identity.GenerateJWT(userID, email, secret, 1*time.Hour)
	if err != nil {
		t.Fatalf("failed to generate JWT: %v", err)
	}

	claims, err := identity.ValidateJWT(tokenStr, secret)
	if err != nil {
		t.Fatalf("failed to validate JWT: %v", err)
	}

	if claims.UserID != userID {
		t.Errorf("expected userID %s, got %s", userID, claims.UserID)
	}

	if claims.Email != email {
		t.Errorf("expected email %s, got %s", email, claims.Email)
	}

	// Validate with wrong secret must fail
	_, errWrongSecret := identity.ValidateJWT(tokenStr, "wrong-secret-key")
	if errWrongSecret == nil {
		t.Errorf("expected error with wrong secret, got nil")
	}

	// Validate expired token
	expiredToken, _ := identity.GenerateJWT(userID, email, secret, -1*time.Hour)
	_, errExpired := identity.ValidateJWT(expiredToken, secret)
	if errExpired != identity.ErrExpiredToken {
		t.Errorf("expected ErrExpiredToken, got %v", errExpired)
	}
}
