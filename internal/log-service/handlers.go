package logservice

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"

	"github.com/dev-nathaniel/Adroit-platform/pkg/middleware"
	"github.com/dev-nathaniel/Adroit-platform/pkg/response"
	"github.com/dev-nathaniel/Adroit-platform/pkg/types"
)

// Handler manages HTTP, SSE, and WebSocket endpoints for the log service
type Handler struct {
	svc      *Service
	upgrader websocket.Upgrader
}

// NewHandler initializes a new Handler
func NewHandler(svc *Service) *Handler {
	return &Handler{
		svc: svc,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true // Allow cross-origin connections from dashboard and web clients
			},
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
		},
	}
}

// RegisterRoutes registers all log service routes on the Chi router
func (h *Handler) RegisterRoutes(r chi.Router, jwtSecret string) {
	// 1. Health check probe
	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		response.JSON(w, http.StatusOK, map[string]any{
			"service": "log-service",
			"status":  "healthy",
		})
	})

	// 2. Direct log ingestion endpoints (Used by container agents/sidecars)
	r.Post("/api/v1/services/{id}/logs", h.handleIngestServiceLogs)
	r.Post("/api/v1/logs/ingest", h.handleBatchIngestLogs)

	// 3. Protected querying and streaming endpoints
	r.Group(func(protected chi.Router) {
		if jwtSecret != "" {
			protected.Use(middleware.RequireAuth(jwtSecret))
		}

		// REST query endpoints
		protected.Get("/api/v1/services/{id}/logs", h.handleGetLogs)
		protected.Get("/api/v1/deployments/{id}/logs", h.handleGetDeploymentLogs)
		protected.Delete("/api/v1/services/{id}/logs", h.handlePurgeLogs)

		// Real-time Server-Sent Events (SSE) streaming
		protected.Get("/api/v1/services/{id}/logs/stream", h.handleStreamLogsSSE)
		protected.Get("/api/v1/deployments/{id}/logs/stream", h.handleStreamDeploymentLogsSSE)

		// Real-time WebSocket streaming
		protected.Get("/api/v1/services/{id}/logs/ws", h.handleStreamLogsWS)
	})
}

// handleIngestServiceLogs processes incoming logs for a specific service ID
func (h *Handler) handleIngestServiceLogs(w http.ResponseWriter, r *http.Request) {
	serviceID := chi.URLParam(r, "id")
	if serviceID == "" {
		response.BadRequest(w, "Service ID is required", nil)
		return
	}

	var req IngestLogsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "Invalid JSON payload: "+err.Error(), nil)
		return
	}

	if err := h.svc.IngestLogs(r.Context(), serviceID, req); err != nil {
		if errors.Is(err, ErrServiceNotFound) {
			response.NotFound(w, "Service not found")
			return
		}
		response.Error(w, http.StatusInternalServerError, "INGEST_FAILED", err.Error(), nil)
		return
	}

	response.JSON(w, http.StatusAccepted, map[string]any{
		"accepted": true,
		"count":    len(req.Logs),
	})
}

// handleBatchIngestLogs handles global log ingestion with service_id in the body
func (h *Handler) handleBatchIngestLogs(w http.ResponseWriter, r *http.Request) {
	var req IngestLogsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "Invalid JSON payload: "+err.Error(), nil)
		return
	}

	if req.ServiceID == "" {
		response.BadRequest(w, "service_id is required in request body", nil)
		return
	}

	if err := h.svc.IngestLogs(r.Context(), req.ServiceID, req); err != nil {
		if errors.Is(err, ErrServiceNotFound) {
			response.NotFound(w, "Service not found")
			return
		}
		response.Error(w, http.StatusInternalServerError, "INGEST_FAILED", err.Error(), nil)
		return
	}

	response.JSON(w, http.StatusAccepted, map[string]any{
		"accepted": true,
		"count":    len(req.Logs),
	})
}

// handleGetLogs queries paginated historical logs for a service
func (h *Handler) handleGetLogs(w http.ResponseWriter, r *http.Request) {
	serviceID := chi.URLParam(r, "id")
	if serviceID == "" {
		response.BadRequest(w, "Service ID is required", nil)
		return
	}

	userID := middleware.GetUserID(r.Context())
	filter := parseQueryFilter(r, serviceID)

	res, err := h.svc.GetLogs(r.Context(), userID, filter)
	if err != nil {
		if errors.Is(err, ErrServiceNotFound) {
			response.NotFound(w, "Service not found")
			return
		}
		if errors.Is(err, ErrUnauthorized) {
			response.Unauthorized(w, "You do not have access to this service's logs")
			return
		}
		response.Error(w, http.StatusInternalServerError, "QUERY_FAILED", err.Error(), nil)
		return
	}

	response.JSON(w, http.StatusOK, res)
}

// handleGetDeploymentLogs queries historical logs for a deployment
func (h *Handler) handleGetDeploymentLogs(w http.ResponseWriter, r *http.Request) {
	deploymentID := chi.URLParam(r, "id")
	if deploymentID == "" {
		response.BadRequest(w, "Deployment ID is required", nil)
		return
	}

	userID := middleware.GetUserID(r.Context())
	filter := parseQueryFilter(r, "")
	filter.DeploymentID = deploymentID

	res, err := h.svc.GetDeploymentLogs(r.Context(), userID, deploymentID, filter)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "QUERY_FAILED", err.Error(), nil)
		return
	}

	response.JSON(w, http.StatusOK, res)
}

// handleStreamLogsSSE streams live application logs over Server-Sent Events (SSE)
func (h *Handler) handleStreamLogsSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		response.Error(w, http.StatusInternalServerError, "STREAMING_UNSUPPORTED", "HTTP streaming unsupported by server", nil)
		return
	}

	serviceID := chi.URLParam(r, "id")
	if serviceID == "" {
		response.BadRequest(w, "Service ID is required", nil)
		return
	}

	streamFilter := r.URL.Query().Get("stream")
	searchFilter := r.URL.Query().Get("search")
	tailCount := parseTailCount(r.URL.Query().Get("tail"), 50)

	// Set standard SSE response headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	// 1. Send historical backlog if requested
	if tailCount > 0 {
		userID := middleware.GetUserID(r.Context())
		histLogs, err := h.svc.GetLogs(r.Context(), userID, types.AppLogsQueryFilter{
			ServiceID: serviceID,
			Stream:    streamFilter,
			Search:    searchFilter,
			Limit:     tailCount,
		})
		if err == nil && histLogs != nil {
			for _, entry := range histLogs.Logs {
				data, _ := json.Marshal(entry)
				fmt.Fprintf(w, "data: %s\n\n", data)
			}
			flusher.Flush()
		}
	}

	// 2. Open live pub-sub subscription
	subCh, unsubscribe := h.svc.Subscribe(serviceID)
	defer unsubscribe()

	keepAliveTicker := time.NewTicker(15 * time.Second)
	defer keepAliveTicker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-keepAliveTicker.C:
			fmt.Fprintf(w, ": keep-alive\n\n")
			flusher.Flush()
		case entry, ok := <-subCh:
			if !ok {
				return
			}
			if streamFilter != "" && streamFilter != "all" && entry.Stream != streamFilter {
				continue
			}
			if searchFilter != "" && !strings.Contains(strings.ToLower(entry.Message), strings.ToLower(searchFilter)) {
				continue
			}

			data, err := json.Marshal(entry)
			if err != nil {
				continue
			}
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
	}
}

// handleStreamDeploymentLogsSSE streams live logs filtered by deployment ID over SSE
func (h *Handler) handleStreamDeploymentLogsSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		response.Error(w, http.StatusInternalServerError, "STREAMING_UNSUPPORTED", "HTTP streaming unsupported", nil)
		return
	}

	deploymentID := chi.URLParam(r, "id")
	if deploymentID == "" {
		response.BadRequest(w, "Deployment ID is required", nil)
		return
	}

	streamFilter := r.URL.Query().Get("stream")
	searchFilter := r.URL.Query().Get("search")
	tailCount := parseTailCount(r.URL.Query().Get("tail"), 50)

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	userID := middleware.GetUserID(r.Context())
	var serviceID string

	if tailCount > 0 {
		histLogs, err := h.svc.GetDeploymentLogs(r.Context(), userID, deploymentID, types.AppLogsQueryFilter{
			DeploymentID: deploymentID,
			Stream:       streamFilter,
			Search:       searchFilter,
			Limit:        tailCount,
		})
		if err == nil && histLogs != nil {
			serviceID = histLogs.ServiceID
			for _, entry := range histLogs.Logs {
				data, _ := json.Marshal(entry)
				fmt.Fprintf(w, "data: %s\n\n", data)
			}
			flusher.Flush()
		}
	}

	if serviceID == "" {
		// If no historical logs exist to deduce serviceID, fallback to query param or direct deploy lookup
		serviceID = r.URL.Query().Get("service_id")
	}

	if serviceID == "" {
		fmt.Fprintf(w, "event: error\ndata: %s\n\n", `{"message":"unable to resolve service_id for deployment"}`)
		flusher.Flush()
		return
	}

	subCh, unsubscribe := h.svc.Subscribe(serviceID)
	defer unsubscribe()

	keepAliveTicker := time.NewTicker(15 * time.Second)
	defer keepAliveTicker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-keepAliveTicker.C:
			fmt.Fprintf(w, ": keep-alive\n\n")
			flusher.Flush()
		case entry, ok := <-subCh:
			if !ok {
				return
			}
			if entry.DeploymentID != nil && *entry.DeploymentID != deploymentID {
				continue
			}
			if streamFilter != "" && streamFilter != "all" && entry.Stream != streamFilter {
				continue
			}
			if searchFilter != "" && !strings.Contains(strings.ToLower(entry.Message), strings.ToLower(searchFilter)) {
				continue
			}

			data, err := json.Marshal(entry)
			if err != nil {
				continue
			}
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
	}
}

// handleStreamLogsWS streams live application logs over WebSocket
func (h *Handler) handleStreamLogsWS(w http.ResponseWriter, r *http.Request) {
	serviceID := chi.URLParam(r, "id")
	if serviceID == "" {
		response.BadRequest(w, "Service ID is required", nil)
		return
	}

	streamFilter := r.URL.Query().Get("stream")
	searchFilter := r.URL.Query().Get("search")
	tailCount := parseTailCount(r.URL.Query().Get("tail"), 50)

	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	// Initial greeting frame
	_ = conn.WriteJSON(WebSocketServerMessage{
		Type: "connected",
		Text: fmt.Sprintf("Connected to live log stream for service %s", serviceID),
	})

	// Historical backlog
	if tailCount > 0 {
		userID := middleware.GetUserID(r.Context())
		histLogs, err := h.svc.GetLogs(r.Context(), userID, types.AppLogsQueryFilter{
			ServiceID: serviceID,
			Stream:    streamFilter,
			Search:    searchFilter,
			Limit:     tailCount,
		})
		if err == nil && histLogs != nil {
			for _, entry := range histLogs.Logs {
				_ = conn.WriteJSON(WebSocketServerMessage{
					Type: "log",
					Log:  &entry,
				})
			}
		}
	}

	subCh, unsubscribe := h.svc.Subscribe(serviceID)
	defer unsubscribe()

	doneCh := make(chan struct{})
	go func() {
		defer close(doneCh)
		for {
			var msg WebSocketClientMessage
			if err := conn.ReadJSON(&msg); err != nil {
				return
			}
			if msg.Action == "ping" {
				_ = conn.WriteJSON(WebSocketServerMessage{Type: "pong"})
			}
		}
	}()

	for {
		select {
		case <-doneCh:
			return
		case <-r.Context().Done():
			return
		case entry, ok := <-subCh:
			if !ok {
				return
			}
			if streamFilter != "" && streamFilter != "all" && entry.Stream != streamFilter {
				continue
			}
			if searchFilter != "" && !strings.Contains(strings.ToLower(entry.Message), strings.ToLower(searchFilter)) {
				continue
			}

			if err := conn.WriteJSON(WebSocketServerMessage{
				Type: "log",
				Log:  &entry,
			}); err != nil {
				return
			}
		}
	}
}

// handlePurgeLogs deletes logs older than the specified timestamp
func (h *Handler) handlePurgeLogs(w http.ResponseWriter, r *http.Request) {
	serviceID := chi.URLParam(r, "id")
	if serviceID == "" {
		response.BadRequest(w, "Service ID is required", nil)
		return
	}

	beforeStr := r.URL.Query().Get("before")
	if beforeStr == "" {
		response.BadRequest(w, "Query parameter 'before' (RFC3339 timestamp) is required", nil)
		return
	}

	// In case '+' in timezone offset was decoded to space by URL parser
	if strings.Contains(beforeStr, " ") && !strings.Contains(beforeStr, "+") {
		beforeStr = strings.Replace(beforeStr, " ", "+", 1)
	}

	before, err := time.Parse(time.RFC3339, beforeStr)
	if err != nil {
		response.BadRequest(w, "Invalid timestamp format for 'before'. Expected RFC3339 (e.g. 2026-09-30T00:00:00Z)", nil)
		return
	}

	userID := middleware.GetUserID(r.Context())
	count, err := h.svc.PurgeLogs(r.Context(), userID, serviceID, before)
	if err != nil {
		if errors.Is(err, ErrServiceNotFound) {
			response.NotFound(w, "Service not found")
			return
		}
		if errors.Is(err, ErrUnauthorized) {
			response.Unauthorized(w, "You do not have access to manage this service's logs")
			return
		}
		response.Error(w, http.StatusInternalServerError, "PURGE_FAILED", err.Error(), nil)
		return
	}

	response.JSON(w, http.StatusOK, map[string]any{
		"deleted_count": count,
		"service_id":    serviceID,
	})
}

func parseQueryFilter(r *http.Request, serviceID string) types.AppLogsQueryFilter {
	q := r.URL.Query()
	limit := 100
	if l := q.Get("limit"); l != "" {
		if val, err := strconv.Atoi(l); err == nil && val > 0 {
			limit = val
		}
	}

	offset := 0
	if o := q.Get("offset"); o != "" {
		if val, err := strconv.Atoi(o); err == nil && val >= 0 {
			offset = val
		}
	}

	filter := types.AppLogsQueryFilter{
		ServiceID:    serviceID,
		DeploymentID: q.Get("deployment_id"),
		AllocationID: q.Get("allocation_id"),
		Stream:       q.Get("stream"),
		Search:       q.Get("search"),
		Limit:        limit,
		Offset:       offset,
	}

	if sinceStr := q.Get("since"); sinceStr != "" {
		if t, err := time.Parse(time.RFC3339, sinceStr); err == nil {
			filter.Since = &t
		}
	}

	if untilStr := q.Get("until"); untilStr != "" {
		if t, err := time.Parse(time.RFC3339, untilStr); err == nil {
			filter.Until = &t
		}
	}

	return filter
}

func parseTailCount(str string, def int) int {
	if str == "" {
		return def
	}
	if val, err := strconv.Atoi(str); err == nil && val >= 0 {
		return val
	}
	return def
}
