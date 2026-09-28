package logger

import (
	"context"
	"io"
	"log/slog"
	"os"
	"strings"
)

type contextKey string

const (
	// RequestIDKey is the context key for tracing HTTP and message requests
	RequestIDKey contextKey = "request_id"
)

// Config defines the configuration for the platform logger
type Config struct {
	Level  string // debug, info, warn, error
	Format string // json, text
	Output io.Writer
}

// Handler wraps slog.Handler to automatically inject context keys (like request_id)
type contextHandler struct {
	slog.Handler
}

// Handle injects context values into the log record
func (h *contextHandler) Handle(ctx context.Context, r slog.Record) error {
	if ctx != nil {
		if reqID, ok := ctx.Value(RequestIDKey).(string); ok && reqID != "" {
			r.AddAttrs(slog.String("request_id", reqID))
		}
	}
	return h.Handler.Handle(ctx, r)
}

// New creates and configures a structured slog.Logger
func New(cfg Config) *slog.Logger {
	var level slog.Level
	switch strings.ToLower(cfg.Level) {
	case "debug":
		level = slog.LevelDebug
	case "warn", "warning":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{
		Level: level,
	}

	out := cfg.Output
	if out == nil {
		out = os.Stdout
	}

	var baseHandler slog.Handler
	if strings.ToLower(cfg.Format) == "text" {
		baseHandler = slog.NewTextHandler(out, opts)
	} else {
		baseHandler = slog.NewJSONHandler(out, opts)
	}

	wrapped := &contextHandler{Handler: baseHandler}
	return slog.New(wrapped)
}

// WithRequestID returns a new context containing the provided request ID
func WithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, RequestIDKey, requestID)
}

// GetRequestID retrieves the request ID from the context if present
func GetRequestID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if reqID, ok := ctx.Value(RequestIDKey).(string); ok {
		return reqID
	}
	return ""
}
