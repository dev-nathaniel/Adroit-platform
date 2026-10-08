package runtimeservice

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/dev-nathaniel/Adroit-platform/pkg/response"
)

// Handler handles HTTP requests for runtime and autoscaling operations
type Handler struct {
	repo       Repository
	collector  MetricsCollector
	controller *AutoScaleController
	logger     *slog.Logger
}

// NewHandler creates an initialized Handler
func NewHandler(
	repo Repository,
	collector MetricsCollector,
	controller *AutoScaleController,
	logger *slog.Logger,
) *Handler {
	return &Handler{
		repo:       repo,
		collector:  collector,
		controller: controller,
		logger:     logger,
	}
}

// RegisterRoutes mounts all runtime-service endpoints onto a Chi router
func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Get("/healthz", h.handleHealthz)

	r.Route("/api/v1/services/{id}", func(sr chi.Router) {
		// Autoscaling policy management
		sr.Get("/autoscale", h.handleGetAutoscalePolicy)
		sr.Post("/autoscale", h.handleUpdateAutoscalePolicy)
		sr.Put("/autoscale", h.handleUpdateAutoscalePolicy)

		// Live and historical workload telemetry
		sr.Get("/metrics", h.handleGetLiveMetrics)
		sr.Get("/metrics/history", h.handleGetMetricsHistory)

		// Scaling audit log
		sr.Get("/scaling-events", h.handleGetScalingEvents)

		// Scale triggers and simulations
		sr.Post("/trigger-scale", h.handleTriggerScale)
		sr.Post("/metrics/simulate", h.handleSimulateMetrics)
	})
}

func (h *Handler) handleHealthz(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, http.StatusOK, map[string]any{
		"service":   "runtime-service",
		"status":    "healthy",
		"timestamp": time.Now().UTC(),
	})
}

func (h *Handler) handleGetAutoscalePolicy(w http.ResponseWriter, r *http.Request) {
	serviceID := chi.URLParam(r, "id")
	policy, err := h.repo.GetAutoscalePolicy(r.Context(), serviceID)
	if err != nil {
		response.Error(w, http.StatusNotFound, "POLICY_NOT_FOUND", err.Error(), nil)
		return
	}
	response.JSON(w, http.StatusOK, policy)
}

func (h *Handler) handleUpdateAutoscalePolicy(w http.ResponseWriter, r *http.Request) {
	serviceID := chi.URLParam(r, "id")
	var req UpdateAutoscalePolicyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid JSON body", nil)
		return
	}

	policy, err := h.repo.UpdateAutoscalePolicy(r.Context(), serviceID, req)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "UPDATE_FAILED", err.Error(), nil)
		return
	}

	h.logger.Info("Autoscale policy updated",
		slog.String("service_id", serviceID),
		slog.Bool("enabled", policy.AutoScalingEnabled),
		slog.Int("min", policy.MinReplicas),
		slog.Int("max", policy.MaxReplicas),
	)

	// Trigger immediate reconciliation cycle to apply new bounds if necessary
	go func() {
		_, _ = h.controller.ReconcileService(context.Background(), serviceID)
	}()

	response.JSON(w, http.StatusOK, policy)
}

func (h *Handler) handleGetLiveMetrics(w http.ResponseWriter, r *http.Request) {
	serviceID := chi.URLParam(r, "id")
	svc, err := h.repo.GetService(r.Context(), serviceID)
	if err != nil {
		response.Error(w, http.StatusNotFound, "SERVICE_NOT_FOUND", err.Error(), nil)
		return
	}

	metrics, err := h.collector.CollectServiceMetrics(r.Context(), svc)
	if err != nil {
		// Fallback to latest persisted sample
		persisted, pErr := h.repo.GetLatestMetrics(r.Context(), serviceID)
		if pErr == nil {
			response.JSON(w, http.StatusOK, LiveMetricsResponse{Metrics: *persisted})
			return
		}
		response.Error(w, http.StatusInternalServerError, "METRICS_FETCH_FAILED", err.Error(), nil)
		return
	}

	// Persist snapshot asynchronously
	go func() {
		_ = h.repo.RecordMetricsSample(context.Background(), metrics)
	}()

	response.JSON(w, http.StatusOK, LiveMetricsResponse{Metrics: *metrics})
}

func (h *Handler) handleGetMetricsHistory(w http.ResponseWriter, r *http.Request) {
	serviceID := chi.URLParam(r, "id")
	limitStr := r.URL.Query().Get("limit")
	limit := 50
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}

	samples, err := h.repo.GetMetricsHistory(r.Context(), serviceID, limit)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "HISTORY_FETCH_FAILED", err.Error(), nil)
		return
	}

	response.JSON(w, http.StatusOK, MetricsHistoryResponse{
		ServiceID: serviceID,
		Samples:   samples,
		Count:     len(samples),
	})
}

func (h *Handler) handleGetScalingEvents(w http.ResponseWriter, r *http.Request) {
	serviceID := chi.URLParam(r, "id")
	limitStr := r.URL.Query().Get("limit")
	limit := 20
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}

	events, err := h.repo.ListScalingEvents(r.Context(), serviceID, limit)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "EVENTS_FETCH_FAILED", err.Error(), nil)
		return
	}

	response.JSON(w, http.StatusOK, ScalingEventsResponse{
		ServiceID: serviceID,
		Events:    events,
		Count:     len(events),
	})
}

func (h *Handler) handleTriggerScale(w http.ResponseWriter, r *http.Request) {
	serviceID := chi.URLParam(r, "id")

	// Trigger immediate reconciliation cycle
	decision, err := h.controller.ReconcileService(r.Context(), serviceID)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "RECONCILE_FAILED", err.Error(), nil)
		return
	}

	response.JSON(w, http.StatusOK, map[string]any{
		"service_id": serviceID,
		"decision":   decision,
		"timestamp":  time.Now().UTC(),
	})
}

func (h *Handler) handleSimulateMetrics(w http.ResponseWriter, r *http.Request) {
	serviceID := chi.URLParam(r, "id")

	var req struct {
		CPUPercent    float64 `json:"cpu_percent"`
		MemoryPercent float64 `json:"memory_percent"`
		RPS           float64 `json:"rps"`
		Clear         bool    `json:"clear"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid JSON body", nil)
		return
	}

	if req.Clear {
		h.collector.ClearSimulatedMetrics(serviceID)
		response.JSON(w, http.StatusOK, map[string]any{
			"service_id": serviceID,
			"simulated":  false,
			"message":    "cleared simulated metrics overlay",
		})
		return
	}

	h.collector.InjectSimulatedMetrics(serviceID, req.CPUPercent, req.MemoryPercent, req.RPS)

	// Trigger reconciliation right away to immediately observe autoscaler response
	decision, _ := h.controller.ReconcileService(r.Context(), serviceID)

	response.JSON(w, http.StatusOK, map[string]any{
		"service_id":     serviceID,
		"simulated":      true,
		"cpu_percent":    req.CPUPercent,
		"memory_percent": req.MemoryPercent,
		"rps":            req.RPS,
		"decision":       decision,
	})
}
