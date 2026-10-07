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

	buildservice "github.com/dev-nathaniel/Adroit-platform/internal/build-service"
	"github.com/dev-nathaniel/Adroit-platform/pkg/builder"
	"github.com/dev-nathaniel/Adroit-platform/pkg/config"
	"github.com/dev-nathaniel/Adroit-platform/pkg/database"
	"github.com/dev-nathaniel/Adroit-platform/pkg/events"
	"github.com/dev-nathaniel/Adroit-platform/pkg/logger"
	"github.com/dev-nathaniel/Adroit-platform/pkg/middleware"
	"github.com/dev-nathaniel/Adroit-platform/pkg/response"
)

func main() {
	// 1. Config & Logger
	baseCfg := config.LoadBaseConfig(8086)
	jwtSecret := config.GetEnv("JWT_SECRET", "super-secret-development-jwt-signing-key-change-in-production")
	registryHost := config.GetEnv("REGISTRY_HOST", "registry.adroit.run")
	nomadAddress := config.GetEnv("NOMAD_ADDR", "http://localhost:4646")
	nomadDatacenter := config.GetEnv("NOMAD_DATACENTER", "dc1")
	runnerType := config.GetEnv("BUILD_RUNNER_TYPE", "nomad")

	log := logger.New(logger.Config{
		Level:  baseCfg.LogLevel,
		Format: baseCfg.LogFormat,
	})

	log.Info("Starting Adroit Build Service",
		slog.String("env", baseCfg.Env),
		slog.Int("port", baseCfg.Port),
		slog.String("registry_host", registryHost),
		slog.String("runner_type", runnerType),
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

	repo := buildservice.NewPostgresRepository(db)

	// 3. Connect to NATS JetStream or fallback to InMemBus
	natsURL := config.GetEnv("NATS_URL", "nats://localhost:4222")
	var eventBus events.EventBus
	natsBus, err := events.NewNATSBus(events.NATSConfig{
		URL:        natsURL,
		ClientName: "build-service",
		Timeout:    3 * time.Second,
	}, log)
	if err != nil {
		log.Warn("NATS unreachable, running build-service with InMemBus fallback", slog.Any("error", err))
		eventBus = events.NewInMemBus()
	} else {
		log.Info("Connected to NATS JetStream backbone", slog.String("url", natsURL))
		eventBus = natsBus
	}
	defer eventBus.Close()

	// 4. Initialize Build Execution Engine
	localRunner := builder.NewLocalDevBuilder(50 * time.Millisecond)
	var buildEngine builder.Builder

	if runnerType == "nomad" && nomadAddress != "" {
		log.Info("Using Nomad Batch Builder", slog.String("nomad_addr", nomadAddress), slog.String("dc", nomadDatacenter))
		buildEngine = builder.NewNomadBatchBuilder(nomadAddress, nomadDatacenter, localRunner)
	} else {
		log.Info("Using Local Dev Builder engine")
		buildEngine = localRunner
	}

	// 5. Service & Handler
	svc := buildservice.NewService(
		repo,
		buildEngine,
		buildservice.ServiceConfig{
			RegistryHost:        registryHost,
			DefaultBuildTimeout: 15 * time.Minute,
			MaxConcurrentBuilds: 5,
		},
		buildservice.WithEventBus(eventBus),
		buildservice.WithLogger(log),
	)

	// Start background NATS listener for adroit.build.requested events
	if err := svc.StartEventListener(context.Background()); err != nil {
		log.Warn("Failed to start event listener for build requests", slog.Any("error", err))
	}

	handler := buildservice.NewHandler(svc)

	// 6. Chi Router
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
			"service": "build-service",
			"status":  "healthy",
		})
	})

	// Mount versioned subroutes
	r.Route("/api/v1", func(api chi.Router) {
		handler.RegisterRoutes(api, jwtSecret)
	})

	// Also allow direct hits if accessed directly on :8086
	handler.RegisterRoutes(r, jwtSecret)

	// 7. Server
	addr := fmt.Sprintf(":%d", baseCfg.Port)
	srv := &http.Server{
		Addr:         addr,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Info("Build Service HTTP server listening", slog.String("addr", addr))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("HTTP server failed", slog.String("error", err.Error()))
			os.Exit(1)
		}
	}()

	// 8. Graceful Shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info("Shutting down Build Service...")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error("Server forced to shutdown", slog.String("error", err.Error()))
	}

	log.Info("Build Service exited cleanly")
}
