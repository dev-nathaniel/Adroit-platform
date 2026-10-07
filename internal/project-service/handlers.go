package projectservice

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/dev-nathaniel/Adroit-platform/pkg/middleware"
	"github.com/dev-nathaniel/Adroit-platform/pkg/response"
)

// Handler handles HTTP requests for projects, services, and environment variables
type Handler struct {
	service *Service
}

// NewHandler creates a new project-service HTTP handler
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// RegisterRoutes registers endpoints onto Chi routers
func (h *Handler) RegisterRoutes(r chi.Router, jwtSecret string) {
	// Public / worker download endpoint for BuildKit container runner
	r.Get("/services/{id}/source/download", h.HandleDownloadSource)

	r.Group(func(auth chi.Router) {
		auth.Use(middleware.RequireAuth(jwtSecret))

		// Projects
		auth.Route("/projects", func(pr chi.Router) {
			pr.Post("/", h.HandleCreateProject)
			pr.Get("/", h.HandleListProjects)
			pr.Get("/{id}", h.HandleGetProject)
			pr.Patch("/{id}", h.HandleUpdateProject)
			pr.Delete("/{id}", h.HandleDeleteProject)

			// Project-nested routes
			pr.Get("/{id}/services", h.HandleListProjectServices)
			pr.Post("/{id}/services", h.HandleCreateProjectService)
			pr.Get("/{id}/env", h.HandleGetProjectEnv)
			pr.Post("/{id}/env", h.HandleSetProjectEnv)
			pr.Delete("/{id}/env/{key}", h.HandleDeleteProjectEnv)
			pr.Post("/{id}/env/{key}/reveal", h.HandleRevealProjectSecret)
		})

		// Services
		auth.Route("/services", func(sr chi.Router) {
			sr.Post("/", h.HandleCreateService)
			sr.Get("/", h.HandleListServices)
			sr.Get("/{id}", h.HandleGetService)
			sr.Patch("/{id}", h.HandleUpdateService)
			sr.Post("/{id}/move", h.HandleMoveService)
			sr.Delete("/{id}", h.HandleDeleteService)

			// Source bundle upload & download
			sr.Post("/{id}/upload", h.HandleUploadSource)
			sr.Post("/{id}/source", h.HandleUploadSource)
			sr.Get("/{id}/source/download", h.HandleDownloadSource)

			// Service-nested routes
			sr.Get("/{id}/env", h.HandleGetServiceEnv)
			sr.Post("/{id}/env", h.HandleSetServiceEnv)
			sr.Delete("/{id}/env/{key}", h.HandleDeleteServiceEnv)
			sr.Post("/{id}/env/{key}/reveal", h.HandleRevealServiceSecret)
		})
	})
}

// ============================================================================
// PROJECT HANDLERS
// ============================================================================

// HandleCreateProject creates a new project
func (h *Handler) HandleCreateProject(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	var req CreateProjectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "Invalid JSON body", nil)
		return
	}

	project, err := h.service.CreateProject(r.Context(), userID, req)
	if err != nil {
		h.handleError(w, err)
		return
	}
	response.JSON(w, http.StatusCreated, project)
}

// HandleListProjects lists projects (optionally filtered by ?organization_id=)
func (h *Handler) HandleListProjects(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	orgID := r.URL.Query().Get("organization_id")

	projects, err := h.service.ListProjects(r.Context(), userID, orgID)
	if err != nil {
		h.handleError(w, err)
		return
	}
	response.JSON(w, http.StatusOK, projects)
}

// HandleGetProject retrieves project by ID
func (h *Handler) HandleGetProject(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	projectID := chi.URLParam(r, "id")

	detail, err := h.service.GetProject(r.Context(), userID, projectID)
	if err != nil {
		h.handleError(w, err)
		return
	}
	response.JSON(w, http.StatusOK, detail)
}

// HandleUpdateProject modifies project details
func (h *Handler) HandleUpdateProject(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	projectID := chi.URLParam(r, "id")

	var req UpdateProjectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "Invalid JSON body", nil)
		return
	}

	updated, err := h.service.UpdateProject(r.Context(), userID, projectID, req)
	if err != nil {
		h.handleError(w, err)
		return
	}
	response.JSON(w, http.StatusOK, updated)
}

// HandleDeleteProject removes a project
func (h *Handler) HandleDeleteProject(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	projectID := chi.URLParam(r, "id")

	if err := h.service.DeleteProject(r.Context(), userID, projectID); err != nil {
		h.handleError(w, err)
		return
	}
	response.JSON(w, http.StatusOK, map[string]string{
		"message": "Project deleted successfully",
	})
}

// HandleListProjectServices lists services inside a specific project
func (h *Handler) HandleListProjectServices(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	projectID := chi.URLParam(r, "id")

	project, err := h.service.GetProject(r.Context(), userID, projectID)
	if err != nil {
		h.handleError(w, err)
		return
	}

	services, err := h.service.ListServices(r.Context(), userID, project.OrganizationID, &projectID, false)
	if err != nil {
		h.handleError(w, err)
		return
	}
	response.JSON(w, http.StatusOK, services)
}

// HandleCreateProjectService creates a service directly inside the specified project
func (h *Handler) HandleCreateProjectService(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	projectID := chi.URLParam(r, "id")

	project, err := h.service.GetProject(r.Context(), userID, projectID)
	if err != nil {
		h.handleError(w, err)
		return
	}

	var req CreateServiceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "Invalid JSON body", nil)
		return
	}

	req.OrganizationID = project.OrganizationID
	req.ProjectID = &projectID

	svc, err := h.service.CreateService(r.Context(), userID, req)
	if err != nil {
		h.handleError(w, err)
		return
	}
	response.JSON(w, http.StatusCreated, svc)
}

// ============================================================================
// SERVICE HANDLERS
// ============================================================================

// HandleCreateService creates a service (standalone or inside project)
func (h *Handler) HandleCreateService(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	var req CreateServiceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "Invalid JSON body", nil)
		return
	}

	svc, err := h.service.CreateService(r.Context(), userID, req)
	if err != nil {
		h.handleError(w, err)
		return
	}
	response.JSON(w, http.StatusCreated, svc)
}

// HandleListServices lists services in an organization (standalone or project-scoped)
func (h *Handler) HandleListServices(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	orgID := r.URL.Query().Get("organization_id")
	if strings.TrimSpace(orgID) == "" {
		response.BadRequest(w, "organization_id query parameter is required", nil)
		return
	}

	standalone := r.URL.Query().Get("standalone") == "true"
	var projectID *string
	if pid := r.URL.Query().Get("project_id"); pid != "" {
		projectID = &pid
	}

	services, err := h.service.ListServices(r.Context(), userID, orgID, projectID, standalone)
	if err != nil {
		h.handleError(w, err)
		return
	}
	response.JSON(w, http.StatusOK, services)
}

// HandleGetService retrieves service by ID
func (h *Handler) HandleGetService(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	serviceID := chi.URLParam(r, "id")

	svc, err := h.service.GetService(r.Context(), userID, serviceID)
	if err != nil {
		h.handleError(w, err)
		return
	}
	response.JSON(w, http.StatusOK, svc)
}

// HandleUpdateService modifies mutable fields of a service
func (h *Handler) HandleUpdateService(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	serviceID := chi.URLParam(r, "id")

	var req UpdateServiceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "Invalid JSON body", nil)
		return
	}

	updated, err := h.service.UpdateService(r.Context(), userID, serviceID, req)
	if err != nil {
		h.handleError(w, err)
		return
	}
	response.JSON(w, http.StatusOK, updated)
}

// HandleMoveService moves a service to another project or detaches it to standalone
func (h *Handler) HandleMoveService(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	serviceID := chi.URLParam(r, "id")

	var req MoveServiceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "Invalid JSON body", nil)
		return
	}

	moved, err := h.service.MoveService(r.Context(), userID, serviceID, req.ProjectID)
	if err != nil {
		h.handleError(w, err)
		return
	}
	response.JSON(w, http.StatusOK, moved)
}

// HandleDeleteService removes a service
func (h *Handler) HandleDeleteService(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	serviceID := chi.URLParam(r, "id")

	if err := h.service.DeleteService(r.Context(), userID, serviceID); err != nil {
		h.handleError(w, err)
		return
	}
	response.JSON(w, http.StatusOK, map[string]string{
		"message": "Service deleted successfully",
	})
}

// ============================================================================
// ENVIRONMENT VARIABLES HANDLERS
// ============================================================================

// HandleGetServiceEnv returns environment variables for a service
func (h *Handler) HandleGetServiceEnv(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	serviceID := chi.URLParam(r, "id")

	if r.URL.Query().Get("resolved") == "true" {
		resolved, err := h.service.GetResolvedServiceEnv(r.Context(), userID, serviceID)
		if err != nil {
			h.handleError(w, err)
			return
		}
		response.JSON(w, http.StatusOK, ResolvedEnvResponse{Variables: resolved})
		return
	}

	vars, err := h.service.GetServiceEnvVars(r.Context(), userID, serviceID)
	if err != nil {
		h.handleError(w, err)
		return
	}
	response.JSON(w, http.StatusOK, vars)
}

// HandleSetServiceEnv upserts environment variables on a service
func (h *Handler) HandleSetServiceEnv(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	serviceID := chi.URLParam(r, "id")

	var req SetEnvVarsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "Invalid JSON body", nil)
		return
	}

	if err := h.service.SetServiceEnvVars(r.Context(), userID, serviceID, req.Variables); err != nil {
		h.handleError(w, err)
		return
	}

	vars, _ := h.service.GetServiceEnvVars(r.Context(), userID, serviceID)
	response.JSON(w, http.StatusOK, vars)
}

// HandleDeleteServiceEnv removes an environment variable from a service
func (h *Handler) HandleDeleteServiceEnv(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	serviceID := chi.URLParam(r, "id")
	key := chi.URLParam(r, "key")

	if err := h.service.DeleteServiceEnvVar(r.Context(), userID, serviceID, key); err != nil {
		h.handleError(w, err)
		return
	}
	response.JSON(w, http.StatusOK, map[string]string{
		"message": "Environment variable deleted successfully",
	})
}

// HandleGetProjectEnv returns environment variables for a project
func (h *Handler) HandleGetProjectEnv(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	projectID := chi.URLParam(r, "id")

	vars, err := h.service.GetProjectEnvVars(r.Context(), userID, projectID)
	if err != nil {
		h.handleError(w, err)
		return
	}
	response.JSON(w, http.StatusOK, vars)
}

// HandleSetProjectEnv upserts environment variables on a project
func (h *Handler) HandleSetProjectEnv(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	projectID := chi.URLParam(r, "id")

	var req SetEnvVarsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "Invalid JSON body", nil)
		return
	}

	if err := h.service.SetProjectEnvVars(r.Context(), userID, projectID, req.Variables); err != nil {
		h.handleError(w, err)
		return
	}

	vars, _ := h.service.GetProjectEnvVars(r.Context(), userID, projectID)
	response.JSON(w, http.StatusOK, vars)
}

// HandleDeleteProjectEnv removes an environment variable from a project
func (h *Handler) HandleDeleteProjectEnv(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	projectID := chi.URLParam(r, "id")
	key := chi.URLParam(r, "key")

	if err := h.service.DeleteProjectEnvVar(r.Context(), userID, projectID, key); err != nil {
		h.handleError(w, err)
		return
	}
	response.JSON(w, http.StatusOK, map[string]string{
		"message": "Project environment variable deleted successfully",
	})
}

// HandleRevealServiceSecret returns the unmasked secret value for a service
func (h *Handler) HandleRevealServiceSecret(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	serviceID := chi.URLParam(r, "id")
	key := chi.URLParam(r, "key")

	resp, err := h.service.RevealServiceSecret(r.Context(), userID, serviceID, key)
	if err != nil {
		h.handleError(w, err)
		return
	}
	response.JSON(w, http.StatusOK, resp)
}

// HandleRevealProjectSecret returns the unmasked secret value for a project
func (h *Handler) HandleRevealProjectSecret(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	projectID := chi.URLParam(r, "id")
	key := chi.URLParam(r, "key")

	resp, err := h.service.RevealProjectSecret(r.Context(), userID, projectID, key)
	if err != nil {
		h.handleError(w, err)
		return
	}
	response.JSON(w, http.StatusOK, resp)
}

// HandleUploadSource processes uploaded source archives (zip or tar.gz)
func (h *Handler) HandleUploadSource(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	serviceID := chi.URLParam(r, "id")
	if strings.TrimSpace(serviceID) == "" {
		response.BadRequest(w, "service_id is required in URL path", nil)
		return
	}

	triggerBuild := true
	if tb := r.URL.Query().Get("trigger_build"); tb == "false" || tb == "0" {
		triggerBuild = false
	}

	var filename string
	var reader io.Reader

	contentType := r.Header.Get("Content-Type")
	if strings.HasPrefix(contentType, "multipart/form-data") {
		// Max upload size 200MB
		if err := r.ParseMultipartForm(200 << 20); err != nil {
			response.BadRequest(w, "Failed to parse multipart form data: "+err.Error(), nil)
			return
		}
		var fileHeader *multipart.FileHeader
		for _, formKey := range []string{"file", "archive", "source", "code", "bundle"} {
			if f, fh, err := r.FormFile(formKey); err == nil {
				defer f.Close()
				fileHeader = fh
				reader = f
				break
			}
		}
		if fileHeader == nil {
			response.BadRequest(w, "No archive file found in form data (expected field 'file', 'archive', or 'source')", nil)
			return
		}
		filename = fileHeader.Filename
	} else {
		// Raw binary stream body
		filename = r.URL.Query().Get("filename")
		if filename == "" {
			if strings.Contains(contentType, "zip") {
				filename = "source.zip"
			} else {
				filename = "source.tar.gz"
			}
		}
		reader = r.Body
	}

	res, err := h.service.UploadServiceSource(r.Context(), userID, serviceID, filename, reader, triggerBuild)
	if err != nil {
		h.handleError(w, err)
		return
	}

	response.JSON(w, http.StatusCreated, res)
}

// HandleDownloadSource streams the source code archive for BuildKit or client download
func (h *Handler) HandleDownloadSource(w http.ResponseWriter, r *http.Request) {
	serviceID := chi.URLParam(r, "id")
	if strings.TrimSpace(serviceID) == "" {
		response.BadRequest(w, "service_id is required in URL path", nil)
		return
	}

	userID := middleware.GetUserID(r.Context())
	rc, size, filename, err := h.service.DownloadServiceSource(r.Context(), userID, serviceID)
	if err != nil {
		h.handleError(w, err)
		return
	}
	defer rc.Close()

	w.Header().Set("Content-Type", "application/gzip")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	if size > 0 {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", size))
	}
	_, _ = io.Copy(w, rc)
}

// handleError maps internal errors to HTTP responses
func (h *Handler) handleError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		response.NotFound(w, err.Error())
	case errors.Is(err, ErrConflict):
		response.Conflict(w, err.Error(), nil)
	case errors.Is(err, ErrForbidden):
		response.Forbidden(w, err.Error())
	case errors.Is(err, ErrUnauthorized):
		response.Unauthorized(w, err.Error())
	case errors.Is(err, ErrInvalidRequest):
		response.BadRequest(w, err.Error(), nil)
	default:
		response.InternalServerError(w, err.Error())
	}
}
