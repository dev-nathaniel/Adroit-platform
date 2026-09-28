package middleware

import (
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"time"

	"github.com/dev-nathaniel/Adroit-platform/pkg/logger"
	"github.com/dev-nathaniel/Adroit-platform/pkg/response"
)

// ResponseWriter wrapper to capture HTTP status code
type statusResponseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *statusResponseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// RequestID middleware assigns or propagates an X-Request-ID header
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqID := r.Header.Get("X-Request-ID")
		if reqID == "" {
			bytes := make([]byte, 16)
			if _, err := rand.Read(bytes); err == nil {
				reqID = hex.EncodeToString(bytes)
			} else {
				reqID = "req-" + time.Now().Format("20060102150405.000000")
			}
		}

		w.Header().Set("X-Request-ID", reqID)
		ctx := logger.WithRequestID(r.Context(), reqID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// Logging middleware logs each HTTP request and duration with slog
func Logging(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			wrapped := &statusResponseWriter{ResponseWriter: w, statusCode: http.StatusOK}

			next.ServeHTTP(wrapped, r)

			duration := time.Since(start)
			reqID := logger.GetRequestID(r.Context())

			log.InfoContext(r.Context(), "HTTP Request",
				slog.String("request_id", reqID),
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", wrapped.statusCode),
				slog.Duration("duration", duration),
				slog.String("remote_addr", r.RemoteAddr),
				slog.String("user_agent", r.UserAgent()),
			)
		})
	}
}

// Recovery middleware catches panics and returns 500 JSON response
func Recovery(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					log.ErrorContext(r.Context(), "Unhandled panic recovered",
						slog.Any("panic", rec),
						slog.String("path", r.URL.Path),
					)
					response.InternalServerError(w, "Internal server error")
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// CORS middleware handles cross-origin requests for dashboard and mobile clients
func CORS(allowedOrigins string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if allowedOrigins == "*" || allowedOrigins == "" {
				w.Header().Set("Access-Control-Allow-Origin", "*")
			} else if origin != "" {
				w.Header().Set("Access-Control-Allow-Origin", origin)
			}

			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Accept, Authorization, Content-Type, X-Request-ID")
			w.Header().Set("Access-Control-Expose-Headers", "X-Request-ID")
			w.Header().Set("Access-Control-Allow-Credentials", "true")

			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
