package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dev-nathaniel/Adroit-platform/pkg/identity"
	"github.com/dev-nathaniel/Adroit-platform/pkg/middleware"
)

func TestRequireAuth(t *testing.T) {
	jwtSecret := "test-secret-key-32-chars-long-12345"
	userID := "usr_test_123"
	email := "test@adroit.run"

	validToken, err := identity.GenerateJWT(userID, email, jwtSecret, 1*time.Hour)
	if err != nil {
		t.Fatalf("failed to generate valid token: %v", err)
	}

	handler := middleware.RequireAuth(jwtSecret)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctxUserID := middleware.GetUserID(r.Context())
		ctxEmail := middleware.GetUserEmail(r.Context())

		if ctxUserID != userID {
			t.Errorf("expected userID %s, got %s", userID, ctxUserID)
		}
		if ctxEmail != email {
			t.Errorf("expected email %s, got %s", email, ctxEmail)
		}

		w.WriteHeader(http.StatusOK)
	}))

	// 1. Missing header
	rec1 := httptest.NewRecorder()
	req1 := httptest.NewRequest(http.MethodGet, "/protected", nil)
	handler.ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for missing header, got %d", rec1.Code)
	}

	// 2. Malformed header
	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req2.Header.Set("Authorization", "Basic 12345")
	handler.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for invalid header format, got %d", rec2.Code)
	}

	// 3. Invalid token string
	rec3 := httptest.NewRecorder()
	req3 := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req3.Header.Set("Authorization", "Bearer invalid.jwt.string")
	handler.ServeHTTP(rec3, req3)
	if rec3.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for invalid token, got %d", rec3.Code)
	}

	// 4. Valid token succeeds
	rec4 := httptest.NewRecorder()
	req4 := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req4.Header.Set("Authorization", "Bearer "+validToken)
	handler.ServeHTTP(rec4, req4)
	if rec4.Code != http.StatusOK {
		t.Errorf("expected 200 for valid token, got %d", rec4.Code)
	}
}
