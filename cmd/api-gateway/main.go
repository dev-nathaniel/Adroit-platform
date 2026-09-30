package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"

	apigateway "github.com/dev-nathaniel/Adroit-platform/internal/api-gateway"
	"github.com/dev-nathaniel/Adroit-platform/pkg/config"
	"github.com/dev-nathaniel/Adroit-platform/pkg/logger"
	"github.com/dev-nathaniel/Adroit-platform/pkg/middleware"
	"github.com/dev-nathaniel/Adroit-platform/pkg/response"
)

// DownstreamConfig holds target service URLs for reverse proxy routing
type DownstreamConfig struct {
	AuthServiceURL         string
	UserServiceURL         string
	NotificationServiceURL string
	ProjectServiceURL      string
}

// SetupRouter creates and configures the Chi router, middleware chain, and service proxies
func SetupRouter(log *slog.Logger, rl *middleware.RateLimiter, downstreams DownstreamConfig) *chi.Mux {
	r := chi.NewRouter()

	// 1. Global Middlewares
	r.Use(middleware.RequestID)
	r.Use(middleware.Recovery(log))
	r.Use(middleware.Logging(log))
	r.Use(middleware.CORS("*"))

	// 2. Rate Limiting Middleware
	if rl != nil {
		r.Use(middleware.RateLimit(rl))
	}

	// 3. Platform Diagnostics
	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		response.JSON(w, http.StatusOK, map[string]any{
			"status":    "healthy",
			"service":   "api-gateway",
			"timestamp": time.Now().UTC(),
		})
	})

	// 4. Gateway Root Endpoint
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		response.JSON(w, http.StatusOK, map[string]any{
			"platform": "Adroit PaaS",
			"router":   "chi/v5",
			"version":  "v1.0.0",
			"status":   "operational",
		})
	})

	// 5. Versioned API Subroutes
	r.Route("/api/v1", func(api chi.Router) {
		api.Get("/status", func(w http.ResponseWriter, r *http.Request) {
			response.JSON(w, http.StatusOK, map[string]string{
				"api_version": "v1",
				"status":      "active",
			})
		})
	})

	// 6. Reverse Proxy Routes for Downstream Microservices
	if downstreams.AuthServiceURL != "" {
		if authProxy, err := apigateway.CreateServiceProxy("auth-service", downstreams.AuthServiceURL, log); err == nil {
			r.Mount("/api/v1/auth", authProxy)
			log.Info("Mounted proxy for auth-service", slog.String("target", downstreams.AuthServiceURL))
		}
	}

	if downstreams.UserServiceURL != "" {
		if userProxy, err := apigateway.CreateServiceProxy("user-service", downstreams.UserServiceURL, log); err == nil {
			r.Mount("/api/v1/users", userProxy)
			log.Info("Mounted proxy for user-service", slog.String("target", downstreams.UserServiceURL))
		}
	}

	if downstreams.NotificationServiceURL != "" {
		if notifProxy, err := apigateway.CreateServiceProxy("notification-service", downstreams.NotificationServiceURL, log); err == nil {
			r.Mount("/api/v1/notifications", notifProxy)
			log.Info("Mounted proxy for notification-service", slog.String("target", downstreams.NotificationServiceURL))
		}
	}

	if downstreams.ProjectServiceURL != "" {
		if projProxy, err := apigateway.CreateServiceProxy("project-service", downstreams.ProjectServiceURL, log); err == nil {
			r.Mount("/api/v1/projects", projProxy)
			r.Mount("/api/v1/services", projProxy)
			log.Info("Mounted proxy for project-service", slog.String("target", downstreams.ProjectServiceURL))
		}
	}

	// 7. Custom 404 & 405 Handlers
	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		response.NotFound(w, fmt.Sprintf("Route %s %s not found", r.Method, r.URL.Path))
	})

	r.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		response.Error(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", fmt.Sprintf("Method %s not allowed on %s", r.Method, r.URL.Path), nil)
	})

	return r
}

func main() {
	// 1. Load configuration
	baseCfg := config.LoadBaseConfig(8080)
	rateLimitRPS := float64(config.GetEnvInt("RATE_LIMIT_RPS", 50))
	rateLimitBurst := config.GetEnvInt("RATE_LIMIT_BURST", 100)

	downstreams := DownstreamConfig{
		AuthServiceURL:         config.GetEnv("AUTH_SERVICE_URL", "http://localhost:8081"),
		UserServiceURL:         config.GetEnv("USER_SERVICE_URL", "http://localhost:8082"),
		NotificationServiceURL: config.GetEnv("NOTIFICATION_SERVICE_URL", "http://localhost:8083"),
		ProjectServiceURL:      config.GetEnv("PROJECT_SERVICE_URL", "http://localhost:8084"),
	}

	// 2. Initialize structured logger
	log := logger.New(logger.Config{
		Level:  baseCfg.LogLevel,
		Format: baseCfg.LogFormat,
	})

	log.Info("Starting Adroit API Gateway with Chi Router",
		slog.String("env", baseCfg.Env),
		slog.Int("port", baseCfg.Port),
		slog.Float64("rate_limit_rps", rateLimitRPS),
		slog.Int("rate_limit_burst", rateLimitBurst),
		slog.String("auth_service_url", downstreams.AuthServiceURL),
		slog.String("user_service_url", downstreams.UserServiceURL),
		slog.String("notification_service_url", downstreams.NotificationServiceURL),
		slog.String("project_service_url", downstreams.ProjectServiceURL),
	)

	// 3. Initialize Rate Limiter
	rl := middleware.NewRateLimiter(rateLimitRPS, rateLimitBurst)
	defer rl.Stop()

	// 4. Initialize Router
	router := SetupRouter(log, rl, downstreams)

	// 5. Configure HTTP server
	addr := fmt.Sprintf(":%d", baseCfg.Port)
	srv := &http.Server{
		Addr:         addr,
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// 6. Start server in goroutine
	go func() {
		log.Info(fmt.Sprintf("API Gateway listening on http://0.0.0.0:%d", baseCfg.Port))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("API Gateway failed to start", slog.String("error", err.Error()))
			os.Exit(1)
		}
	}()

	// 7. Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info("Shutting down API Gateway gracefully...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Error("API Gateway forced to shutdown", slog.String("error", err.Error()))
	} else {
		log.Info("API Gateway exited cleanly")
	}
}
