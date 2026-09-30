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

	gitintegrationservice "github.com/dev-nathaniel/Adroit-platform/internal/git-integration-service"
	"github.com/dev-nathaniel/Adroit-platform/pkg/config"
	"github.com/dev-nathaniel/Adroit-platform/pkg/database"
	"github.com/dev-nathaniel/Adroit-platform/pkg/events"
	"github.com/dev-nathaniel/Adroit-platform/pkg/logger"
	"github.com/dev-nathaniel/Adroit-platform/pkg/middleware"
	"github.com/dev-nathaniel/Adroit-platform/pkg/response"
)

func main() {
	// 1. Config & Logger
	baseCfg := config.LoadBaseConfig(8085)
	jwtSecret := config.GetEnv("JWT_SECRET", "super-secret-development-jwt-signing-key-change-in-production")
	githubSecret := config.GetEnv("GITHUB_WEBHOOK_SECRET", "")
	gitlabSecret := config.GetEnv("GITLAB_WEBHOOK_SECRET", "")
	platformURL := config.GetEnv("PLATFORM_URL", "http://localhost:8080")

	log := logger.New(logger.Config{
		Level:  baseCfg.LogLevel,
		Format: baseCfg.LogFormat,
	})

	log.Info("Starting Adroit Git Integration Service",
		slog.String("env", baseCfg.Env),
		slog.Int("port", baseCfg.Port),
		slog.String("platform_url", platformURL),
	)

	// 2. Database Connection
	dbCfg := database.LoadConfig()
	dbCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	db, err := database.Connect(dbCtx, dbCfg)
	if err != nil {
		log.Error("Failed to connect to PostgreSQL", slog.String("error", err.Error()))
		os.Exit(1)
	}
	defer db.Close()
	log.Info("Connected to PostgreSQL successfully")

	repo := gitintegrationservice.NewPostgresRepository(db)

	// 3. Connect to NATS JetStream or fallback to InMemBus
	natsURL := config.GetEnv("NATS_URL", "nats://localhost:4222")
	var eventBus events.EventBus
	natsBus, err := events.NewNATSBus(events.NATSConfig{
		URL:        natsURL,
		ClientName: "git-integration-service",
		Timeout:    3 * time.Second,
	}, log)
	if err != nil {
		log.Warn("NATS unreachable, running git-integration-service with InMemBus fallback", slog.Any("error", err))
		eventBus = events.NewInMemBus()
	} else {
		log.Info("Connected to NATS JetStream backbone", slog.String("url", natsURL))
		eventBus = natsBus
	}
	defer eventBus.Close()

	// 4. Service & Handler
	svc := gitintegrationservice.NewService(
		repo,
		gitintegrationservice.ServiceConfig{
			GitHubWebhookSecret: githubSecret,
			GitLabWebhookSecret: gitlabSecret,
			PlatformURL:         platformURL,
		},
		gitintegrationservice.WithEventBus(eventBus),
		gitintegrationservice.WithLogger(log),
	)
	handler := gitintegrationservice.NewHandler(svc)

	// 5. Chi Router
	r := chi.NewRouter()
	rl := middleware.NewRateLimiter(100.0, 200)
	defer rl.Stop()

	r.Use(middleware.RequestID)
	r.Use(middleware.Recovery(log))
	r.Use(middleware.Logging(log))
	r.Use(middleware.CORS("*"))
	r.Use(middleware.RateLimit(rl))

	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		response.JSON(w, http.StatusOK, map[string]any{
			"service": "git-integration-service",
			"status":  "healthy",
		})
	})

	// Mount versioned subroutes
	r.Route("/api/v1", func(api chi.Router) {
		handler.RegisterRoutes(api, jwtSecret)
	})

	// Also allow direct /webhooks hits if accessed directly on :8085
	r.Route("/webhooks", func(wr chi.Router) {
		wr.Post("/github", handler.HandleGitHubWebhook)
		wr.Post("/gitlab", handler.HandleGitLabWebhook)
	})

	// 6. Server
	addr := fmt.Sprintf(":%d", baseCfg.Port)
	srv := &http.Server{
		Addr:         addr,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Info(fmt.Sprintf("Git Integration Service listening on http://0.0.0.0:%d", baseCfg.Port))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("Git Integration Service failed to start", slog.String("error", err.Error()))
			os.Exit(1)
		}
	}()

	// 7. Graceful Shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info("Shutting down Git Integration Service gracefully...")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error("Forced shutdown on Git Integration Service", slog.String("error", err.Error()))
	}
	log.Info("Git Integration Service stopped successfully")
}
