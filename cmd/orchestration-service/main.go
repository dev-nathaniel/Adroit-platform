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

	orchestrationservice "github.com/dev-nathaniel/Adroit-platform/internal/orchestration-service"
	"github.com/dev-nathaniel/Adroit-platform/pkg/config"
	"github.com/dev-nathaniel/Adroit-platform/pkg/database"
	"github.com/dev-nathaniel/Adroit-platform/pkg/events"
	"github.com/dev-nathaniel/Adroit-platform/pkg/logger"
	"github.com/dev-nathaniel/Adroit-platform/pkg/middleware"
	"github.com/dev-nathaniel/Adroit-platform/pkg/orchestrator"
	"github.com/dev-nathaniel/Adroit-platform/pkg/response"
)

func main() {
	// 1. Config & Logger
	baseCfg := config.LoadBaseConfig(8087)
	jwtSecret := config.GetEnv("JWT_SECRET", "super-secret-development-jwt-signing-key-change-in-production")
	nomadAddress := config.GetEnv("NOMAD_ADDR", "")
	nomadDatacenter := config.GetEnv("NOMAD_DATACENTER", "dc1")
	nomadToken := config.GetEnv("NOMAD_TOKEN", "")
	driverType := config.GetEnv("ORCHESTRATOR_DRIVER_TYPE", "simulated")
	baseDomain := config.GetEnv("BASE_DOMAIN", "adroit.run")

	log := logger.New(logger.Config{
		Level:  baseCfg.LogLevel,
		Format: baseCfg.LogFormat,
	})

	log.Info("Starting Adroit Orchestration & Runtime Service",
		slog.String("env", baseCfg.Env),
		slog.Int("port", baseCfg.Port),
		slog.String("driver_type", driverType),
		slog.String("base_domain", baseDomain),
		slog.String("datacenter", nomadDatacenter),
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

	repo := orchestrationservice.NewPostgresRepository(db)

	// 3. Connect to NATS JetStream or fallback to InMemBus
	natsURL := config.GetEnv("NATS_URL", "nats://localhost:4222")
	var eventBus events.EventBus
	natsBus, err := events.NewNATSBus(events.NATSConfig{
		URL:        natsURL,
		ClientName: "orchestration-service",
		Timeout:    3 * time.Second,
	}, log)
	if err != nil {
		log.Warn("NATS unreachable, running orchestration-service with InMemBus fallback", slog.Any("error", err))
		eventBus = events.NewInMemBus()
	} else {
		log.Info("Connected to NATS JetStream backbone", slog.String("url", natsURL))
		eventBus = natsBus
	}
	defer eventBus.Close()

	// 4. Initialize Nomad Driver (Live Nomad HTTP Client or High-Fidelity Simulator)
	var driver orchestrator.Driver
	if (driverType == "nomad" || nomadAddress != "") && nomadAddress != "" {
		log.Info("Using live Nomad HTTP Client driver",
			slog.String("nomad_addr", nomadAddress),
			slog.String("dc", nomadDatacenter),
		)
		driver = orchestrator.NewNomadHTTPClient(orchestrator.NomadClientConfig{
			Address:    nomadAddress,
			Token:      nomadToken,
			Datacenter: nomadDatacenter,
			Timeout:    10 * time.Second,
		})
	} else {
		log.Info("Using High-Fidelity Simulated Nomad Driver (in-memory scheduler)")
		driver = orchestrator.NewSimulatedDriver(500 * time.Millisecond)
	}

	// 5. Initialize Service & Handler
	svc := orchestrationservice.NewService(
		repo,
		driver,
		orchestrationservice.ServiceConfig{
			Datacenter:         nomadDatacenter,
			BaseDomain:         baseDomain,
			RolloutTimeout:     5 * time.Minute,
			HealthPollInterval: 1 * time.Second,
			AutoPromoteCanary:  true,
		},
		orchestrationservice.WithEventBus(eventBus),
		orchestrationservice.WithLogger(log),
	)

	// Start background NATS listener for adroit.deploy.requested events
	if err := svc.StartEventListener(context.Background()); err != nil {
		log.Warn("Failed to start event listener for deploy requests", slog.Any("error", err))
	}

	handler := orchestrationservice.NewHandler(svc)

	// 6. Chi Router & Middlewares
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
			"service": "orchestration-service",
			"status":  "healthy",
		})
	})

	// Mount versioned subroutes and top-level direct routes
	r.Route("/api/v1", func(api chi.Router) {
		handler.RegisterRoutes(api, jwtSecret)
	})
	handler.RegisterRoutes(r, jwtSecret)

	// 7. Start HTTP Server
	addr := fmt.Sprintf(":%d", baseCfg.Port)
	srv := &http.Server{
		Addr:         addr,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Info("Orchestration Service HTTP server listening", slog.String("addr", addr))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("HTTP server failed", slog.String("error", err.Error()))
			os.Exit(1)
		}
	}()

	// 8. Graceful Shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info("Shutting down Orchestration Service...")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error("Server forced to shutdown", slog.String("error", err.Error()))
	}

	log.Info("Orchestration Service exited cleanly")
}
