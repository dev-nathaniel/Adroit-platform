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

	"github.com/dev-nathaniel/Adroit-platform/pkg/config"
	"github.com/dev-nathaniel/Adroit-platform/pkg/logger"
	"github.com/dev-nathaniel/Adroit-platform/pkg/middleware"
	"github.com/dev-nathaniel/Adroit-platform/pkg/response"
)

// SetupRouter creates and configures the Chi router and middleware chain
func SetupRouter(log *slog.Logger, rl *middleware.RateLimiter) *chi.Mux {
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

	// 3. Custom 404 & 405 Handlers
	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		response.NotFound(w, fmt.Sprintf("Route %s %s not found", r.Method, r.URL.Path))
	})

	r.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		response.Error(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", fmt.Sprintf("Method %s not allowed on %s", r.Method, r.URL.Path), nil)
	})

	// 4. Platform Health & Diagnostics
	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		response.JSON(w, http.StatusOK, map[string]any{
			"status":    "healthy",
			"service":   "api-gateway",
			"timestamp": time.Now().UTC(),
		})
	})

	// 5. Gateway Root Endpoint
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		response.JSON(w, http.StatusOK, map[string]any{
			"platform": "Adroit PaaS",
			"router":   "chi/v5",
			"version":  "v1.0.0",
			"status":   "operational",
		})
	})

	// 6. Versioned API Subroutes (Placeholder for core microservices proxying)
	r.Route("/api/v1", func(api chi.Router) {
		api.Get("/status", func(w http.ResponseWriter, r *http.Request) {
			response.JSON(w, http.StatusOK, map[string]string{
				"api_version": "v1",
				"status":      "active",
			})
		})
	})

	return r
}

func main() {
	// 1. Load configuration
	baseCfg := config.LoadBaseConfig(8080)
	rateLimitRPS := float64(config.GetEnvInt("RATE_LIMIT_RPS", 50))
	rateLimitBurst := config.GetEnvInt("RATE_LIMIT_BURST", 100)

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
	)

	// 3. Initialize Rate Limiter
	rl := middleware.NewRateLimiter(rateLimitRPS, rateLimitBurst)
	defer rl.Stop()

	// 4. Initialize Router
	router := SetupRouter(log, rl)

	// 5. Configure HTTP server
	addr := fmt.Sprintf(":%d", baseCfg.Port)
	srv := &http.Server{
		Addr:         addr,
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// 5. Start server in goroutine
	go func() {
		log.Info(fmt.Sprintf("API Gateway listening on http://0.0.0.0:%d", baseCfg.Port))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("API Gateway failed to start", slog.String("error", err.Error()))
			os.Exit(1)
		}
	}()

	// 6. Graceful shutdown
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
