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

	notificationservice "github.com/dev-nathaniel/Adroit-platform/internal/notification-service"
	"github.com/dev-nathaniel/Adroit-platform/pkg/config"
	"github.com/dev-nathaniel/Adroit-platform/pkg/events"
	"github.com/dev-nathaniel/Adroit-platform/pkg/logger"
	"github.com/dev-nathaniel/Adroit-platform/pkg/middleware"
)

func main() {
	// 1. Config & Logger
	baseCfg := config.LoadBaseConfig(8083)
	natsURL := config.GetEnv("NATS_URL", "nats://localhost:4222")
	providerType := config.GetEnv("NOTIFICATION_PROVIDER", "console") // "console" or "smtp"
	appURL := config.GetEnv("APP_URL", "http://localhost:8080")

	log := logger.New(logger.Config{
		Level:  baseCfg.LogLevel,
		Format: baseCfg.LogFormat,
	})

	log.Info("Starting Adroit Notification Service",
		slog.String("env", baseCfg.Env),
		slog.Int("port", baseCfg.Port),
		slog.String("provider", providerType),
	)

	// 2. Initialize Email Provider
	var emailProv notificationservice.EmailProvider
	if providerType == "smtp" {
		smtpCfg := notificationservice.SMTPConfig{
			Host:       config.GetEnv("SMTP_HOST", "localhost"),
			Port:       config.GetEnvInt("SMTP_PORT", 1025), // Default to local Mailpit/Mailhog port
			Username:   config.GetEnv("SMTP_USER", ""),
			Password:   config.GetEnv("SMTP_PASSWORD", ""),
			From:       config.GetEnv("SMTP_FROM", "Adroit Platform <no-reply@adroit.run>"),
			SkipVerify: config.GetEnvBool("SMTP_SKIP_VERIFY", false),
		}
		emailProv = notificationservice.NewSMTPEmailProvider(smtpCfg, log)
		log.Info("Configured SMTP Email Provider", slog.String("host", smtpCfg.Host), slog.Int("port", smtpCfg.Port))
	} else {
		emailProv = notificationservice.NewConsoleEmailProvider(log)
		log.Info("Configured Console/Terminal Email Provider for local development")
	}

	// 3. Initialize Webhook Sender & Template Manager
	webhookSender := notificationservice.NewHTTPWebhookSender(10*time.Second, log)
	tm, err := notificationservice.NewTemplateManager()
	if err != nil {
		log.Error("Failed to initialize template manager", slog.Any("error", err))
		os.Exit(1)
	}

	// 4. Construct Notification Service
	svc, err := notificationservice.NewService(emailProv, webhookSender, tm, appURL, log)
	if err != nil {
		log.Error("Failed to instantiate notification service", slog.Any("error", err))
		os.Exit(1)
	}

	// 5. Connect to NATS JetStream or fallback to InMemBus
	var eventBus events.EventBus
	natsBus, err := events.NewNATSBus(events.NATSConfig{
		URL:        natsURL,
		ClientName: "notification-service",
		Timeout:    3 * time.Second,
	}, log)

	if err != nil {
		log.Warn("NATS unreachable, falling back to InMemBus for local operation", slog.Any("error", err))
		eventBus = events.NewInMemBus()
	} else {
		log.Info("Connected to NATS JetStream backbone", slog.String("url", natsURL))
		eventBus = natsBus
	}
	defer eventBus.Close()

	// 6. Start Event Consumer
	consumer := notificationservice.NewConsumer(svc, eventBus, log)
	if err := consumer.Start(); err != nil {
		log.Error("Failed to start event consumer", slog.Any("error", err))
		os.Exit(1)
	}
	defer func() {
		_ = consumer.Stop()
	}()

	// 7. HTTP API Router & Diagnostics
	r := chi.NewRouter()
	rl := middleware.NewRateLimiter(50.0, 100)
	defer rl.Stop()

	r.Use(middleware.RequestID)
	r.Use(middleware.Recovery(log))
	r.Use(middleware.Logging(log))
	r.Use(middleware.CORS("*"))
	r.Use(middleware.RateLimit(rl))

	handler := notificationservice.NewHandler(svc)
	handler.RegisterRoutes(r)

	// 8. Start HTTP Server
	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", baseCfg.Port),
		Handler:      r,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Info("Notification Service listening", slog.String("addr", server.Addr))
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("Server error", slog.String("error", err.Error()))
			os.Exit(1)
		}
	}()

	// 9. Graceful Shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info("Shutting down Notification Service...")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Error("Server forced shutdown", slog.String("error", err.Error()))
	}
	log.Info("Notification Service gracefully stopped")
}
