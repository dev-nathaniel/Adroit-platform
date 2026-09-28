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

	userservice "github.com/dev-nathaniel/Adroit-platform/internal/user-service"
	"github.com/dev-nathaniel/Adroit-platform/pkg/config"
	"github.com/dev-nathaniel/Adroit-platform/pkg/database"
	"github.com/dev-nathaniel/Adroit-platform/pkg/logger"
	"github.com/dev-nathaniel/Adroit-platform/pkg/middleware"
	"github.com/dev-nathaniel/Adroit-platform/pkg/response"
)

func main() {
	// 1. Config & Logger
	baseCfg := config.LoadBaseConfig(8082)
	jwtSecret := config.GetEnv("JWT_SECRET", "super-secret-development-jwt-signing-key-change-in-production")

	log := logger.New(logger.Config{
		Level:  baseCfg.LogLevel,
		Format: baseCfg.LogFormat,
	})

	log.Info("Starting Adroit User & Organization Service",
		slog.String("env", baseCfg.Env),
		slog.Int("port", baseCfg.Port),
	)

	// 2. Database Connection
	var repo userservice.Repository
	dbCfg := database.LoadConfig()
	dbCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	db, err := database.Connect(dbCtx, dbCfg)
	if err != nil {
		log.Warn("PostgreSQL connection failed; running with in-memory repository", slog.String("error", err.Error()))
		repo = userservice.NewMemoryRepository()
	} else {
		log.Info("Connected to PostgreSQL successfully")
		defer db.Close()
		repo = userservice.NewPostgresRepository(db)
	}

	// 3. Service & Handlers
	svc := userservice.NewService(repo)
	handler := userservice.NewHandler(svc)

	// 4. Chi Router
	r := chi.NewRouter()
	rl := middleware.NewRateLimiter(50.0, 100)
	defer rl.Stop()

	r.Use(middleware.RequestID)
	r.Use(middleware.Recovery(log))
	r.Use(middleware.Logging(log))
	r.Use(middleware.CORS("*"))
	r.Use(middleware.RateLimit(rl))

	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		response.JSON(w, http.StatusOK, map[string]any{
			"service": "user-service",
			"status":  "healthy",
		})
	})

	r.Route("/api/v1/users", func(userRouter chi.Router) {
		handler.RegisterRoutes(userRouter, jwtSecret)
	})

	// 5. Server
	addr := fmt.Sprintf(":%d", baseCfg.Port)
	srv := &http.Server{
		Addr:         addr,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Info(fmt.Sprintf("User Service listening on http://0.0.0.0:%d", baseCfg.Port))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("User Service failed to start", slog.String("error", err.Error()))
			os.Exit(1)
		}
	}()

	// 6. Graceful Shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info("Shutting down User Service gracefully...")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error("User Service forced to shutdown", slog.String("error", err.Error()))
	} else {
		log.Info("User Service exited cleanly")
	}
}
