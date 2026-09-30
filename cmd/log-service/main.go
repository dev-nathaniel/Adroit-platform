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

	logservice "github.com/dev-nathaniel/Adroit-platform/internal/log-service"
	"github.com/dev-nathaniel/Adroit-platform/pkg/config"
	"github.com/dev-nathaniel/Adroit-platform/pkg/database"
	"github.com/dev-nathaniel/Adroit-platform/pkg/events"
	"github.com/dev-nathaniel/Adroit-platform/pkg/logger"
	"github.com/dev-nathaniel/Adroit-platform/pkg/middleware"
)

func main() {
	// 1. 12-factor configuration
	baseCfg := config.LoadBaseConfig(8089)
	dbCfg := database.LoadConfig()
	natsURL := config.GetEnv("NATS_URL", "nats://localhost:4222")
	nomadAddr := config.GetEnv("NOMAD_ADDR", "http://localhost:4646")
	nomadToken := config.GetEnv("NOMAD_TOKEN", "")
	tailerDriverType := config.GetEnv("LOG_TAILER_DRIVER", "auto")
	jwtSecret := config.GetEnv("JWT_SECRET", "adroit-super-secret-development-jwt-signing-key-32b!")

	// 2. Structured JSON logger
	log := logger.New(logger.Config{
		Level:  baseCfg.LogLevel,
		Format: baseCfg.LogFormat,
	})

	log.Info("Starting Adroit Application Log Streaming Service",
		slog.String("env", baseCfg.Env),
		slog.Int("port", baseCfg.Port),
		slog.String("nomad_addr", nomadAddr),
		slog.String("tailer_driver", tailerDriverType),
		slog.String("nats_url", natsURL),
	)

	// 3. Connect to PostgreSQL
	dbCtx, dbCancel := context.WithTimeout(context.Background(), 5*time.Second)
	db, err := database.Connect(dbCtx, dbCfg)
	dbCancel()
	if err != nil {
		log.Error("Database connection failed", slog.String("error", err.Error()))
		os.Exit(1)
	}
	defer db.Close()
	log.Info("Connected to PostgreSQL successfully")

	// 4. Initialize Log Tailer (Nomad client agent or simulated tailer)
	var logTailer logservice.LogTailer
	if tailerDriverType == "simulated" {
		log.Info("Using simulated in-memory log tailer")
		logTailer = logservice.NewSimulatedLogTailer(1*time.Second, true)
	} else {
		// Probe Nomad
		probeClient := &http.Client{Timeout: 2 * time.Second}
		nomadProbeReq, _ := http.NewRequest(http.MethodGet, nomadAddr+"/v1/status/leader", nil)
		if nomadToken != "" {
			nomadProbeReq.Header.Set("X-Nomad-Token", nomadToken)
		}
		resp, probeErr := probeClient.Do(nomadProbeReq)
		if probeErr != nil || (resp != nil && resp.StatusCode != http.StatusOK) {
			log.Warn("Nomad cluster unreachable, falling back to simulated log tailer", slog.Any("error", probeErr))
			logTailer = logservice.NewSimulatedLogTailer(1*time.Second, true)
		} else {
			resp.Body.Close()
			log.Info("Connected to Nomad cluster for live log tailing", slog.String("addr", nomadAddr))
			logTailer = logservice.NewNomadLogTailer(logservice.NomadTailerConfig{
				NomadAddress: nomadAddr,
				NomadToken:   nomadToken,
			})
		}
	}

	// 5. Connect to NATS JetStream Event Bus
	var eventBus events.EventBus
	natsBus, err := events.NewNATSBus(events.NATSConfig{
		URL:        natsURL,
		ClientName: "log-service",
		Timeout:    3 * time.Second,
	}, log)
	if err != nil {
		log.Warn("NATS unreachable, running log-service with InMemBus fallback", slog.Any("error", err))
		eventBus = events.NewInMemBus()
	} else {
		eventBus = natsBus
		log.Info("Connected to NATS JetStream event bus", slog.String("url", natsURL))
	}
	defer eventBus.Close()

	// 6. Initialize Service, Broadcaster, and Repository
	repo := logservice.NewPostgresRepository(db)
	broadcaster := logservice.NewBroadcaster()
	defer broadcaster.Close()

	svc := logservice.NewService(repo, broadcaster, logTailer, eventBus, log)

	// 7. Start background event subscribers
	subCtx, subCancel := context.WithCancel(context.Background())
	defer subCancel()
	if err := svc.StartEventSubscribers(subCtx); err != nil {
		log.Warn("Failed to initialize background event subscribers", slog.Any("error", err))
	} else {
		log.Info("Log Service background event subscribers active")
	}

	// 8. HTTP Router
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.Logging(log))
	r.Use(middleware.Recovery(log))
	r.Use(middleware.CORS("*"))

	handler := logservice.NewHandler(svc)
	handler.RegisterRoutes(r, jwtSecret)

	// 9. Start HTTP Server
	addr := fmt.Sprintf(":%d", baseCfg.Port)
	server := &http.Server{
		Addr:         addr,
		Handler:      r,
		ReadTimeout:  0, // No read timeout for long-lived WebSocket connections
		WriteTimeout: 0, // No write timeout for SSE and WebSocket streaming
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Info(fmt.Sprintf("Log Streaming Service listening on http://0.0.0.0:%d", baseCfg.Port))
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("Log Service failed to start", slog.String("error", err.Error()))
			os.Exit(1)
		}
	}()

	// 10. Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info("Shutting down Log Streaming Service gracefully...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Error("Log Service forced to shutdown", slog.String("error", err.Error()))
	} else {
		log.Info("Log Service exited cleanly")
	}
}
