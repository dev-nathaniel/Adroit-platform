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

	"github.com/dev-nathaniel/Adroit-platform/pkg/config"
	"github.com/dev-nathaniel/Adroit-platform/pkg/logger"
	"github.com/dev-nathaniel/Adroit-platform/pkg/middleware"
	"github.com/dev-nathaniel/Adroit-platform/pkg/response"
)

func main() {
	// 1. Load configuration
	baseCfg := config.LoadBaseConfig(8080)

	// 2. Initialize structured logger
	log := logger.New(logger.Config{
		Level:  baseCfg.LogLevel,
		Format: baseCfg.LogFormat,
	})

	log.Info("Starting Adroit API Gateway",
		slog.String("env", baseCfg.Env),
		slog.Int("port", baseCfg.Port),
	)

	// 3. Setup router
	mux := http.NewServeMux()

	// Health check endpoint
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		response.JSON(w, http.StatusOK, map[string]any{
			"status":    "healthy",
			"service":   "api-gateway",
			"timestamp": time.Now().UTC(),
		})
	})

	// Gateway root info endpoint
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			response.NotFound(w, "Endpoint not found")
			return
		}
		response.JSON(w, http.StatusOK, map[string]any{
			"platform": "Adroit PaaS",
			"version":  "v1.0.0",
			"status":   "operational",
		})
	})

	// 4. Apply middleware pipeline
	var handler http.Handler = mux
	handler = middleware.CORS("*")(handler)
	handler = middleware.Logging(log)(handler)
	handler = middleware.Recovery(log)(handler)
	handler = middleware.RequestID(handler)

	// 5. Configure HTTP server
	addr := fmt.Sprintf(":%d", baseCfg.Port)
	srv := &http.Server{
		Addr:         addr,
		Handler:      handler,
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
