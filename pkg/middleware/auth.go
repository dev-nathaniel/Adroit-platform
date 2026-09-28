package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/dev-nathaniel/Adroit-platform/pkg/identity"
	"github.com/dev-nathaniel/Adroit-platform/pkg/response"
)

type userContextKey string

const (
	userIDKey    userContextKey = "auth_user_id"
	userEmailKey userContextKey = "auth_user_email"
)

// RequireAuth middleware enforces valid JWT bearer authentication
func RequireAuth(jwtSecret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				response.Unauthorized(w, "Missing Authorization header")
				return
			}

			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
				response.Unauthorized(w, "Invalid Authorization header format. Expected 'Bearer <token>'")
				return
			}

			tokenStr := parts[1]
			claims, err := identity.ValidateJWT(tokenStr, jwtSecret)
			if err != nil {
				if err == identity.ErrExpiredToken {
					response.Unauthorized(w, "Token has expired")
				} else {
					response.Unauthorized(w, "Invalid authentication token")
				}
				return
			}

			// Inject user identity into context
			ctx := context.WithValue(r.Context(), userIDKey, claims.UserID)
			ctx = context.WithValue(ctx, userEmailKey, claims.Email)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// GetUserID retrieves the authenticated user ID from context
func GetUserID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if id, ok := ctx.Value(userIDKey).(string); ok {
		return id
	}
	return ""
}

// GetUserEmail retrieves the authenticated user email from context
func GetUserEmail(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if email, ok := ctx.Value(userEmailKey).(string); ok {
		return email
	}
	return ""
}
