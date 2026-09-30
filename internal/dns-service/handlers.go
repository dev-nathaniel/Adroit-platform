package dnsservice

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/dev-nathaniel/Adroit-platform/pkg/middleware"
	"github.com/dev-nathaniel/Adroit-platform/pkg/response"
)

// Handler exposes the REST HTTP API for the Ingress & DNS Service
type Handler struct {
	svc *Service
}

// NewHandler creates an initialized Handler
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// RegisterRoutes registers HTTP routes with the Chi router
func (h *Handler) RegisterRoutes(r chi.Router, jwtSecret string) {
	// Public health probe
	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		response.JSON(w, http.StatusOK, map[string]any{
			"service": "dns-service",
			"status":  "healthy",
		})
	})

	// Protected routes
	r.Group(func(protected chi.Router) {
		if jwtSecret != "" {
			protected.Use(middleware.RequireAuth(jwtSecret))
		}

		// Service domain management
		protected.Post("/services/{id}/domains", h.handleCreateDomain)
		protected.Get("/services/{id}/domains", h.handleListServiceDomains)
		protected.Post("/services/{id}/sync-routes", h.handleSyncServiceRoutes)

		// Domain lifecycle & verification
		protected.Get("/domains/{id}", h.handleGetDomain)
		protected.Post("/domains/{id}/verify", h.handleVerifyDomain)
		protected.Post("/domains/{id}/sync", h.handleSyncDomainRoute)
		protected.Delete("/domains/{id}", h.handleDeleteDomain)

		// Edge ingress status & routes
		protected.Get("/ingress/routes", h.handleListAllRoutes)
	})
}

// handleCreateDomain registers a new custom domain for a service
func (h *Handler) handleCreateDomain(w http.ResponseWriter, r *http.Request) {
	serviceID := chi.URLParam(r, "id")
	if serviceID == "" {
		response.BadRequest(w, "Service ID is required", nil)
		return
	}

	var req CreateDomainRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "Invalid JSON request body", nil)
		return
	}

	res, err := h.svc.AddCustomDomain(r.Context(), serviceID, req)
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidDomainName):
			response.BadRequest(w, err.Error(), nil)
		case errors.Is(err, ErrDomainAlreadyExists):
			response.Conflict(w, err.Error(), nil)
		case errors.Is(err, ErrServiceNotFound):
			response.NotFound(w, err.Error())
		default:
			response.InternalServerError(w, "Failed to register custom domain")
		}
		return
	}

	response.JSON(w, http.StatusCreated, res)
}

// handleListServiceDomains lists all domains registered for a service
func (h *Handler) handleListServiceDomains(w http.ResponseWriter, r *http.Request) {
	serviceID := chi.URLParam(r, "id")
	if serviceID == "" {
		response.BadRequest(w, "Service ID is required", nil)
		return
	}

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	perPage, _ := strconv.Atoi(r.URL.Query().Get("per_page"))
	if perPage <= 0 || perPage > 100 {
		perPage = 20
	}

	domains, total, err := h.svc.ListServiceDomains(r.Context(), serviceID, page, perPage)
	if err != nil {
		response.InternalServerError(w, "Failed to list custom domains")
		return
	}

	totalPages := 0
	if perPage > 0 {
		totalPages = (total + perPage - 1) / perPage
	}
	meta := &response.Pagination{
		Page:       page,
		PerPage:    perPage,
		TotalItems: total,
		TotalPages: totalPages,
	}
	response.JSONWithMeta(w, http.StatusOK, domains, meta)
}

// handleGetDomain retrieves single domain details and challenge instructions
func (h *Handler) handleGetDomain(w http.ResponseWriter, r *http.Request) {
	domainID := chi.URLParam(r, "id")
	if domainID == "" {
		response.BadRequest(w, "Domain ID is required", nil)
		return
	}

	d, err := h.svc.GetDomainDetails(r.Context(), domainID)
	if err != nil {
		if errors.Is(err, ErrDomainNotFound) {
			response.NotFound(w, "Domain not found")
			return
		}
		response.InternalServerError(w, "Failed to get domain details")
		return
	}

	response.JSON(w, http.StatusOK, d)
}

// handleVerifyDomain triggers an immediate DNS challenge verification
func (h *Handler) handleVerifyDomain(w http.ResponseWriter, r *http.Request) {
	domainID := chi.URLParam(r, "id")
	if domainID == "" {
		response.BadRequest(w, "Domain ID is required", nil)
		return
	}

	res, err := h.svc.VerifyCustomDomain(r.Context(), domainID)
	if err != nil {
		if errors.Is(err, ErrDomainNotFound) {
			response.NotFound(w, "Domain not found")
			return
		}
		response.InternalServerError(w, "DNS challenge verification failed")
		return
	}

	statusCode := http.StatusOK
	if !res.Verified {
		statusCode = http.StatusUnprocessableEntity
	}
	response.JSON(w, statusCode, res)
}

// handleSyncDomainRoute synchronizes Caddy edge proxying for a verified domain
func (h *Handler) handleSyncDomainRoute(w http.ResponseWriter, r *http.Request) {
	domainID := chi.URLParam(r, "id")
	if domainID == "" {
		response.BadRequest(w, "Domain ID is required", nil)
		return
	}

	var req SyncRouteRequest
	_ = json.NewDecoder(r.Body).Decode(&req)

	route, err := h.svc.SyncEdgeRoute(r.Context(), domainID, req)
	if err != nil {
		switch {
		case errors.Is(err, ErrDomainNotFound):
			response.NotFound(w, "Domain not found")
		case errors.Is(err, ErrDomainNotVerified):
			response.BadRequest(w, "Cannot sync edge route: domain must be verified first", nil)
		case errors.Is(err, ErrNoActiveDeployment):
			response.BadRequest(w, "Cannot sync edge route: no active deployment found for this service", nil)
		default:
			response.InternalServerError(w, "Failed to synchronize edge route with Caddy")
		}
		return
	}

	response.JSON(w, http.StatusOK, route)
}

// handleDeleteDomain removes a custom domain and deprovisions its Caddy edge route
func (h *Handler) handleDeleteDomain(w http.ResponseWriter, r *http.Request) {
	domainID := chi.URLParam(r, "id")
	if domainID == "" {
		response.BadRequest(w, "Domain ID is required", nil)
		return
	}

	err := h.svc.DeleteCustomDomain(r.Context(), domainID)
	if err != nil {
		if errors.Is(err, ErrDomainNotFound) {
			response.NotFound(w, "Domain not found")
			return
		}
		response.InternalServerError(w, "Failed to delete custom domain")
		return
	}

	response.JSON(w, http.StatusOK, map[string]any{
		"message": "Custom domain removed and edge route deprovisioned",
	})
}

// handleSyncServiceRoutes forces Caddy synchronization for all routes attached to a service
func (h *Handler) handleSyncServiceRoutes(w http.ResponseWriter, r *http.Request) {
	serviceID := chi.URLParam(r, "id")
	if serviceID == "" {
		response.BadRequest(w, "Service ID is required", nil)
		return
	}

	var req SyncRouteRequest
	_ = json.NewDecoder(r.Body).Decode(&req)

	result, err := h.svc.SyncServiceRoutesForDeployment(r.Context(), serviceID, req.UpstreamURL)
	if err != nil {
		if errors.Is(err, ErrServiceNotFound) {
			response.NotFound(w, "Service not found")
			return
		}
		response.InternalServerError(w, "Failed to synchronize service routes")
		return
	}

	response.JSON(w, http.StatusOK, result)
}

// handleListAllRoutes returns all registered edge routes
func (h *Handler) handleListAllRoutes(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	perPage, _ := strconv.Atoi(r.URL.Query().Get("per_page"))
	if perPage <= 0 || perPage > 100 {
		perPage = 50
	}

	routes, total, err := h.svc.ListAllRoutes(r.Context(), page, perPage)
	if err != nil {
		response.InternalServerError(w, "Failed to list ingress routes")
		return
	}

	totalPages := 0
	if perPage > 0 {
		totalPages = (total + perPage - 1) / perPage
	}
	meta := &response.Pagination{
		Page:       page,
		PerPage:    perPage,
		TotalItems: total,
		TotalPages: totalPages,
	}
	response.JSONWithMeta(w, http.StatusOK, routes, meta)
}
