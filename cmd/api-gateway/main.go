package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"

	apigateway "github.com/dev-nathaniel/Adroit-platform/internal/api-gateway"
	"github.com/dev-nathaniel/Adroit-platform/pkg/config"
	"github.com/dev-nathaniel/Adroit-platform/pkg/logger"
	"github.com/dev-nathaniel/Adroit-platform/pkg/middleware"
	"github.com/dev-nathaniel/Adroit-platform/pkg/ratelimit"
	"github.com/dev-nathaniel/Adroit-platform/pkg/response"
)

// DownstreamConfig holds target service URLs for reverse proxy routing
type DownstreamConfig struct {
	AuthServiceURL         string
	UserServiceURL         string
	NotificationServiceURL string
	ProjectServiceURL      string
	GitServiceURL          string
	BuildServiceURL        string
	OrchestrationServiceURL string
	DNSServiceURL          string
	LogServiceURL          string
}

// SetupRouter creates and configures the Chi router, middleware chain, and service proxies.
// rl can be *middleware.RateLimiter, *ratelimit.TierLimiter, or ratelimit.Limiter.
func SetupRouter(log *slog.Logger, rl any, downstreams DownstreamConfig) *chi.Mux {
	r := chi.NewRouter()

	// 1. Global Middlewares
	r.Use(middleware.RequestID)
	r.Use(middleware.Recovery(log))
	r.Use(middleware.Logging(log))
	r.Use(middleware.CORS("*"))

	// 2. Rate Limiting Middleware
	if rl != nil {
		switch v := rl.(type) {
		case *middleware.RateLimiter:
			r.Use(middleware.RateLimit(v))
		case *ratelimit.TierLimiter:
			r.Use(middleware.TierRateLimit(v))
		case ratelimit.Limiter:
			r.Use(middleware.DistributedRateLimit(v))
		}
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
			r.HandleFunc("/invite", userProxy.ServeHTTP)
			r.HandleFunc("/invite/*", userProxy.ServeHTTP)
			log.Info("Mounted proxy for user-service", slog.String("target", downstreams.UserServiceURL))
		}
	}

	if downstreams.NotificationServiceURL != "" {
		if notifProxy, err := apigateway.CreateServiceProxy("notification-service", downstreams.NotificationServiceURL, log); err == nil {
			r.Mount("/api/v1/notifications", notifProxy)
			log.Info("Mounted proxy for notification-service", slog.String("target", downstreams.NotificationServiceURL))
		}
	}

	if downstreams.BuildServiceURL != "" {
		if buildProxy, err := apigateway.CreateServiceProxy("build-service", downstreams.BuildServiceURL, log); err == nil {
			r.Mount("/api/v1/builds", buildProxy)
			r.HandleFunc("/api/v1/services/{id}/builds", buildProxy.ServeHTTP)
			r.HandleFunc("/api/v1/services/{id}/builds/*", buildProxy.ServeHTTP)
			log.Info("Mounted proxy for build-service", slog.String("target", downstreams.BuildServiceURL))
		}
	}

	if downstreams.ProjectServiceURL != "" {
		if projProxy, err := apigateway.CreateServiceProxy("project-service", downstreams.ProjectServiceURL, log); err == nil {
			r.Mount("/api/v1/projects", projProxy)
			r.Mount("/api/v1/services", projProxy)
			log.Info("Mounted proxy for project-service", slog.String("target", downstreams.ProjectServiceURL))
		}
	}

	if downstreams.GitServiceURL != "" {
		if gitProxy, err := apigateway.CreateServiceProxy("git-integration-service", downstreams.GitServiceURL, log); err == nil {
			r.Mount("/api/v1/git", gitProxy)
			r.Mount("/api/v1/webhooks", gitProxy)
			log.Info("Mounted proxy for git-integration-service", slog.String("target", downstreams.GitServiceURL))
		}
	}

	if downstreams.OrchestrationServiceURL != "" {
		if orchProxy, err := apigateway.CreateServiceProxy("orchestration-service", downstreams.OrchestrationServiceURL, log); err == nil {
			r.Mount("/api/v1/deployments", orchProxy)
			r.HandleFunc("/api/v1/services/{id}/deployments", orchProxy.ServeHTTP)
			r.HandleFunc("/api/v1/services/{id}/deployments/*", orchProxy.ServeHTTP)
			r.HandleFunc("/api/v1/services/{id}/deploy", orchProxy.ServeHTTP)
			r.HandleFunc("/api/v1/services/{id}/scale", orchProxy.ServeHTTP)
			log.Info("Mounted proxy for orchestration-service", slog.String("target", downstreams.OrchestrationServiceURL))
		}
	}

	if downstreams.DNSServiceURL != "" {
		if dnsProxy, err := apigateway.CreateServiceProxy("dns-service", downstreams.DNSServiceURL, log); err == nil {
			r.Mount("/api/v1/domains", dnsProxy)
			r.Mount("/api/v1/ingress", dnsProxy)
			r.HandleFunc("/api/v1/services/{id}/domains", dnsProxy.ServeHTTP)
			r.HandleFunc("/api/v1/services/{id}/domains/*", dnsProxy.ServeHTTP)
			r.HandleFunc("/api/v1/services/{id}/sync-routes", dnsProxy.ServeHTTP)
			log.Info("Mounted proxy for dns-service", slog.String("target", downstreams.DNSServiceURL))
		}
	}

	if downstreams.LogServiceURL != "" {
		if logProxy, err := apigateway.CreateServiceProxy("log-service", downstreams.LogServiceURL, log); err == nil {
			r.Mount("/api/v1/logs", logProxy)
			r.HandleFunc("/api/v1/services/{id}/logs", logProxy.ServeHTTP)
			r.HandleFunc("/api/v1/services/{id}/logs/*", logProxy.ServeHTTP)
			r.HandleFunc("/api/v1/deployments/{id}/logs", logProxy.ServeHTTP)
			r.HandleFunc("/api/v1/deployments/{id}/logs/*", logProxy.ServeHTTP)
			log.Info("Mounted proxy for log-service", slog.String("target", downstreams.LogServiceURL))
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
	redisAddrs := strings.Split(config.GetEnv("REDIS_ADDRS", config.GetEnv("REDIS_ADDR", "localhost:6379")), ",")
	redisPassword := config.GetEnv("REDIS_PASSWORD", "")
	redisDB := config.GetEnvInt("REDIS_DB", 0)
	rateLimitBackend := ratelimit.BackendType(config.GetEnv("RATE_LIMIT_BACKEND", "redis"))
	rateLimitAlgorithm := ratelimit.Algorithm(config.GetEnv("RATE_LIMIT_ALGORITHM", "token_bucket"))
	enableTiers := config.GetEnvBool("RATE_LIMIT_TIERS_ENABLED", true)

	downstreams := DownstreamConfig{
		AuthServiceURL:         config.GetEnv("AUTH_SERVICE_URL", "http://localhost:8081"),
		UserServiceURL:         config.GetEnv("USER_SERVICE_URL", "http://localhost:8082"),
		NotificationServiceURL: config.GetEnv("NOTIFICATION_SERVICE_URL", "http://localhost:8083"),
		ProjectServiceURL:      config.GetEnv("PROJECT_SERVICE_URL", "http://localhost:8084"),
		GitServiceURL:          config.GetEnv("GIT_SERVICE_URL", "http://localhost:8085"),
		BuildServiceURL:        config.GetEnv("BUILD_SERVICE_URL", "http://localhost:8086"),
		OrchestrationServiceURL: config.GetEnv("ORCHESTRATION_SERVICE_URL", "http://localhost:8087"),
		DNSServiceURL:          config.GetEnv("DNS_SERVICE_URL", "http://localhost:8088"),
		LogServiceURL:          config.GetEnv("LOG_SERVICE_URL", "http://localhost:8089"),
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
		slog.String("rate_limit_backend", string(rateLimitBackend)),
		slog.String("rate_limit_algorithm", string(rateLimitAlgorithm)),
		slog.Bool("rate_limit_tiers_enabled", enableTiers),
		slog.String("redis_addrs", strings.Join(redisAddrs, ",")),
		slog.String("auth_service_url", downstreams.AuthServiceURL),
		slog.String("user_service_url", downstreams.UserServiceURL),
		slog.String("notification_service_url", downstreams.NotificationServiceURL),
		slog.String("project_service_url", downstreams.ProjectServiceURL),
		slog.String("git_service_url", downstreams.GitServiceURL),
		slog.String("build_service_url", downstreams.BuildServiceURL),
		slog.String("orchestration_service_url", downstreams.OrchestrationServiceURL),
		slog.String("dns_service_url", downstreams.DNSServiceURL),
		slog.String("log_service_url", downstreams.LogServiceURL),
	)

	// 3. Initialize Distributed / Tier Rate Limiter
	rlCfg := ratelimit.Config{
		Backend:          rateLimitBackend,
		Algorithm:        rateLimitAlgorithm,
		Rate:             rateLimitRPS,
		Burst:            int64(rateLimitBurst),
		Window:           1 * time.Minute,
		RedisAddrs:       redisAddrs,
		RedisPassword:    redisPassword,
		RedisDB:          redisDB,
		FallbackToMemory: true,
		Logger:           log,
	}

	var router *chi.Mux
	if enableTiers {
		tierLimiter, err := ratelimit.NewTierLimiter(rlCfg, nil)
		if err != nil {
			log.Error("Failed to initialize tier rate limiter, falling back to in-memory limiter", slog.String("error", err.Error()))
			fallbackRL := middleware.NewRateLimiter(rateLimitRPS, rateLimitBurst)
			defer fallbackRL.Stop()
			router = SetupRouter(log, fallbackRL, downstreams)
		} else {
			defer tierLimiter.Close()
			router = SetupRouter(log, tierLimiter, downstreams)
		}
	} else {
		distLimiter, err := ratelimit.New(rlCfg)
		if err != nil {
			log.Error("Failed to initialize distributed rate limiter, falling back to in-memory limiter", slog.String("error", err.Error()))
			fallbackRL := middleware.NewRateLimiter(rateLimitRPS, rateLimitBurst)
			defer fallbackRL.Stop()
			router = SetupRouter(log, fallbackRL, downstreams)
		} else {
			defer distLimiter.Close()
			router = SetupRouter(log, distLimiter, downstreams)
		}
	}

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
