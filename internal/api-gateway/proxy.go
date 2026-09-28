package apigateway

import (
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/dev-nathaniel/Adroit-platform/pkg/logger"
	"github.com/dev-nathaniel/Adroit-platform/pkg/response"
)

// ReverseProxyConfig holds configuration for forwarding traffic to a downstream microservice
type ReverseProxyConfig struct {
	ServiceName string
	TargetURL   *url.URL
	Log         *slog.Logger
}

// NewReverseProxy creates a reverse proxy handler that forwards traffic to a downstream microservice
func NewReverseProxy(cfg ReverseProxyConfig) (http.Handler, error) {
	if cfg.TargetURL == nil {
		return nil, fmt.Errorf("target URL is required for service %s", cfg.ServiceName)
	}

	proxy := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(cfg.TargetURL)
			pr.SetXForwarded()

			// Propagate Request ID
			reqID := logger.GetRequestID(pr.In.Context())
			if reqID != "" {
				pr.Out.Header.Set("X-Request-ID", reqID)
			}
		},
	}

	// Custom error handler returns standard 502 Bad Gateway response
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		cfg.Log.ErrorContext(r.Context(), "Downstream service unavailable",
			slog.String("service", cfg.ServiceName),
			slog.String("target_url", cfg.TargetURL.String()),
			slog.String("path", r.URL.Path),
			slog.String("error", err.Error()),
		)

		response.Error(
			w,
			http.StatusBadGateway,
			response.ErrCodeServiceUnavailable,
			fmt.Sprintf("Downstream service '%s' is unavailable", cfg.ServiceName),
			map[string]string{
				"service": cfg.ServiceName,
				"error":   err.Error(),
			},
		)
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxy.ServeHTTP(w, r)
	}), nil
}

// CreateServiceProxy is a helper to parse URL string and initialize reverse proxy
func CreateServiceProxy(serviceName, targetURLStr string, log *slog.Logger) (http.Handler, error) {
	targetURLStr = strings.TrimSpace(targetURLStr)
	if !strings.HasPrefix(targetURLStr, "http://") && !strings.HasPrefix(targetURLStr, "https://") {
		targetURLStr = "http://" + targetURLStr
	}

	targetURL, err := url.Parse(targetURLStr)
	if err != nil {
		return nil, fmt.Errorf("invalid target URL for %s: %w", serviceName, err)
	}

	return NewReverseProxy(ReverseProxyConfig{
		ServiceName: serviceName,
		TargetURL:   targetURL,
		Log:         log,
	})
}
