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

	runtimeservice "github.com/dev-nathaniel/Adroit-platform/internal/runtime-service"
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
	baseCfg := config.LoadBaseConfig(8090)
	nomadAddress := config.GetEnv("NOMAD_ADDR", "http://localhost:4646")
	nomadDatacenter := config.GetEnv("NOMAD_DATACENTER", "dc1")
	nomadToken := config.GetEnv("NOMAD_TOKEN", "")
	driverType := config.GetEnv("ORCHESTRATOR_DRIVER_TYPE", "nomad")
	prometheusURL := config.GetEnv("PROMETHEUS_URL", "http://localhost:9090")
	reconcileIntervalSec := config.GetEnvInt("AUTOSCALE_INTERVAL_SECONDS", 15)

	log := logger.New(logger.Config{
		Level:  baseCfg.LogLevel,
		Format: baseCfg.LogFormat,
	})

	log.Info("Starting Adroit Runtime & Auto-Scaling Service",
		slog.String("env", baseCfg.Env),
		slog.Int("port", baseCfg.Port),
		slog.String("driver_type", driverType),
		slog.String("nomad_addr", nomadAddress),
		slog.String("prometheus_url", prometheusURL),
		slog.Int("reconcile_interval_s", reconcileIntervalSec),
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

	repo := runtimeservice.NewPostgresRepository(db)

	// 3. Connect to NATS JetStream or fallback to InMemBus
	natsURL := config.GetEnv("NATS_URL", "nats://localhost:4222")
	var eventBus events.EventBus
	natsBus, err := events.NewNATSBus(events.NATSConfig{
		URL:        natsURL,
		ClientName: "runtime-service",
		Timeout:    3 * time.Second,
	}, log)
	if err != nil {
		log.Warn("NATS unreachable, running runtime-service with InMemBus fallback", slog.Any("error", err))
		eventBus = events.NewInMemBus()
	} else {
		log.Info("Connected to NATS JetStream backbone", slog.String("url", natsURL))
		eventBus = natsBus
	}
	defer eventBus.Close()

	// 4. Initialize Orchestrator Driver
	var driver orchestrator.Driver
	if driverType == "nomad" && nomadAddress != "" {
		log.Info("Using live Nomad HTTP Client driver", slog.String("addr", nomadAddress))
		driver = orchestrator.NewNomadHTTPClient(orchestrator.NomadClientConfig{
			Address:    nomadAddress,
			Token:      nomadToken,
			Datacenter: nomadDatacenter,
		})
	} else {
		log.Info("Using Simulated Nomad Driver for runtime-service")
		driver = orchestrator.NewSimulatedDriver(10 * time.Millisecond)
	}

	// 5. Initialize Components
	collector := runtimeservice.NewClusterMetricsCollector(prometheusURL, driver, log)
	evaluator := runtimeservice.NewPolicyEvaluator()
	interval := time.Duration(reconcileIntervalSec) * time.Second
	controller := runtimeservice.NewAutoScaleController(repo, collector, evaluator, driver, eventBus, log, interval)

	// Start background reconciliation loop
	loopCtx, stopLoop := context.WithCancel(context.Background())
	defer stopLoop()
	controller.Start(loopCtx)

	handler := runtimeservice.NewHandler(repo, collector, controller, log)

	// 6. Router & Middleware Chain
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.Recovery(log))
	r.Use(middleware.Logging(log))
	r.Use(middleware.CORS("*"))

	handler.RegisterRoutes(r)

	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		response.NotFound(w, fmt.Sprintf("Route %s %s not found on runtime-service", r.Method, r.URL.Path))
	})

	// 7. HTTP Server Setup
	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", baseCfg.Port),
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// 8. Graceful Shutdown Listener
	shutdownCh := make(chan os.Signal, 1)
	signal.Notify(shutdownCh, os.Interrupt, syscall.SIGTERM)

	go func() {
		log.Info("Runtime Service listening", slog.String("addr", server.Addr))
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("HTTP server failed", slog.String("error", err.Error()))
			os.Exit(1)
		}
	}()

	<-shutdownCh
	log.Info("Shutting down Runtime Service...")
	controller.Stop()
	stopLoop()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Error("Server forced to shutdown", slog.String("error", err.Error()))
	}
	log.Info("Runtime Service stopped gracefully")
}
