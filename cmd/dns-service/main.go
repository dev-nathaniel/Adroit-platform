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

	dnsservice "github.com/dev-nathaniel/Adroit-platform/internal/dns-service"
	"github.com/dev-nathaniel/Adroit-platform/pkg/config"
	"github.com/dev-nathaniel/Adroit-platform/pkg/database"
	"github.com/dev-nathaniel/Adroit-platform/pkg/events"
	"github.com/dev-nathaniel/Adroit-platform/pkg/ingress"
	"github.com/dev-nathaniel/Adroit-platform/pkg/logger"
	"github.com/dev-nathaniel/Adroit-platform/pkg/middleware"
)

func main() {
	// 1. Load configuration
	baseCfg := config.LoadBaseConfig(8088)
	dbCfg := database.LoadConfig()
	platformDomain := config.GetEnv("PLATFORM_DOMAIN", "adroit.run")
	canonicalCNAME := config.GetEnv("CANONICAL_CNAME", "cname.adroit.run")
	caddyAdminURL := config.GetEnv("CADDY_ADMIN_URL", "http://localhost:2019")
	ingressDriverType := config.GetEnv("INGRESS_DRIVER", "auto")
	natsURL := config.GetEnv("NATS_URL", "nats://localhost:4222")
	jwtSecret := config.GetEnv("JWT_SECRET", "adroit-super-secret-development-jwt-signing-key-32b!")

	// 2. Structured logger
	log := logger.New(logger.Config{
		Level:  baseCfg.LogLevel,
		Format: baseCfg.LogFormat,
	})

	log.Info("Starting Adroit Ingress & DNS Service",
		slog.String("env", baseCfg.Env),
		slog.Int("port", baseCfg.Port),
		slog.String("platform_domain", platformDomain),
		slog.String("canonical_cname", canonicalCNAME),
		slog.String("caddy_admin_url", caddyAdminURL),
		slog.String("ingress_driver", ingressDriverType),
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

	// 4. Initialize Ingress Driver (Caddy Admin Client or Simulated Driver)
	var ingressDriver ingress.Driver
	if ingressDriverType == "simulated" {
		log.Info("Using simulated in-memory ingress driver")
		ingressDriver = ingress.NewSimulatedIngressDriver()
	} else {
		caddyClient := ingress.NewCaddyClient(caddyAdminURL, "adroit_edge")
		probeCtx, probeCancel := context.WithTimeout(context.Background(), 2*time.Second)
		if probeErr := caddyClient.CheckHealth(probeCtx); probeErr != nil {
			probeCancel()
			log.Warn("Caddy Admin API unreachable, falling back to simulated ingress driver", slog.Any("error", probeErr))
			ingressDriver = ingress.NewSimulatedIngressDriver()
		} else {
			probeCancel()
			log.Info("Connected to live Caddy edge server via Admin API", slog.String("url", caddyAdminURL))
			ingressDriver = caddyClient
		}
	}

	// 5. Initialize DNS Resolver
	dnsResolver := dnsservice.NewSystemDNSResolver()

	// 6. Connect to NATS JetStream Event Bus
	var eventBus events.EventBus
	natsBus, err := events.NewNATSBus(events.NATSConfig{
		URL:        natsURL,
		ClientName: "dns-service",
		Timeout:    3 * time.Second,
	}, log)
	if err != nil {
		log.Warn("NATS unreachable, running dns-service with InMemBus fallback", slog.Any("error", err))
		eventBus = events.NewInMemBus()
	} else {
		eventBus = natsBus
		log.Info("Connected to NATS JetStream event bus", slog.String("url", natsURL))
	}
	defer eventBus.Close()

	// 7. Initialize Repository & Service
	repo := dnsservice.NewPostgresRepository(db)
	svc := dnsservice.NewService(repo, ingressDriver, dnsResolver, eventBus, log, dnsservice.ServiceConfig{
		PlatformDomain: platformDomain,
		CanonicalCNAME: canonicalCNAME,
	})

	// 8. Start background event subscribers
	subCtx, subCancel := context.WithCancel(context.Background())
	defer subCancel()
	if err := svc.StartEventSubscribers(subCtx); err != nil {
		log.Warn("Failed to initialize background event subscribers", slog.Any("error", err))
	} else {
		log.Info("DNS Service background event subscribers active")
	}

	// 9. HTTP Router
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.Logging(log))
	r.Use(middleware.Recovery(log))
	r.Use(middleware.CORS("*"))

	handler := dnsservice.NewHandler(svc)
	r.Route("/api/v1", func(api chi.Router) {
		handler.RegisterRoutes(api, jwtSecret)
	})
	handler.RegisterRoutes(r, jwtSecret)

	// 10. Start HTTP server
	addr := fmt.Sprintf(":%d", baseCfg.Port)
	server := &http.Server{
		Addr:         addr,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Info(fmt.Sprintf("Ingress & DNS Service listening on http://0.0.0.0:%d", baseCfg.Port))
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("DNS Service failed to start", slog.String("error", err.Error()))
			os.Exit(1)
		}
	}()

	// 11. Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info("Shutting down Ingress & DNS Service gracefully...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Error("DNS Service forced to shutdown", slog.String("error", err.Error()))
	} else {
		log.Info("DNS Service exited cleanly")
	}
}
