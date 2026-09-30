package notificationservice

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/dev-nathaniel/Adroit-platform/pkg/events"
	"github.com/dev-nathaniel/Adroit-platform/pkg/response"
)

// Handler handles HTTP requests for the notification service
type Handler struct {
	svc *Service
}

// NewHandler constructs a new Handler
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// RegisterRoutes registers endpoints on a Chi router
func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Get("/healthz", h.HealthCheck)

	r.Route("/api/v1/notifications", func(api chi.Router) {
		api.Post("/email", h.SendEmail)
		api.Post("/webhook", h.SendWebhook)
		api.Post("/test-invite", h.SendTestInvite)
	})
}

// HealthCheck verifies service operational status
func (h *Handler) HealthCheck(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, http.StatusOK, map[string]any{
		"status":         "healthy",
		"service":        "notification-service",
		"email_provider": h.svc.GetEmailProvider().Name(),
		"webhook_sender": h.svc.GetWebhookSender().Name(),
		"timestamp":      time.Now().UTC(),
	})
}

// SendEmail handles manual/programmatic email dispatch
func (h *Handler) SendEmail(w http.ResponseWriter, r *http.Request) {
	var req SendEmailRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "Invalid JSON payload: "+err.Error(), nil)
		return
	}

	if len(req.To) == 0 {
		response.BadRequest(w, "At least one recipient ('to') is required", nil)
		return
	}

	if req.Template == "" && req.PlainBody == "" && req.HTMLBody == "" {
		response.BadRequest(w, "Either 'template' or ('html_body' / 'plain_body') must be provided", nil)
		return
	}

	if err := h.svc.SendEmail(r.Context(), req); err != nil {
		response.InternalServerError(w, "Failed to send email: "+err.Error())
		return
	}

	response.JSON(w, http.StatusOK, map[string]any{
		"dispatched": true,
		"provider":   h.svc.GetEmailProvider().Name(),
		"recipients": req.To,
	})
}

// SendWebhook handles manual/programmatic webhook dispatch
func (h *Handler) SendWebhook(w http.ResponseWriter, r *http.Request) {
	var req SendWebhookRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "Invalid JSON payload: "+err.Error(), nil)
		return
	}

	if strings.TrimSpace(req.URL) == "" {
		response.BadRequest(w, "Webhook 'url' is required", nil)
		return
	}

	if err := h.svc.SendWebhook(r.Context(), req); err != nil {
		response.InternalServerError(w, "Failed to deliver webhook: "+err.Error())
		return
	}

	response.JSON(w, http.StatusOK, map[string]any{
		"dispatched": true,
		"url":        req.URL,
		"event":      req.Event,
	})
}

// SendTestInvite triggers an invite email simulation
func (h *Handler) SendTestInvite(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email            string `json:"email"`
		OrganizationName string `json:"organization_name"`
		InvitedByName    string `json:"invited_by_name"`
		Role             string `json:"role"`
	}

	_ = json.NewDecoder(r.Body).Decode(&req)
	if req.Email == "" {
		req.Email = "developer@example.com"
	}
	if req.OrganizationName == "" {
		req.OrganizationName = "Adroit Devs"
	}
	if req.InvitedByName == "" {
		req.InvitedByName = "Platform Administrator"
	}
	if req.Role == "" {
		req.Role = "member"
	}

	payload := events.OrgMemberInvitedPayload{
		InvitationID:     "inv_test_123",
		OrganizationID:   "org_test_456",
		OrganizationName: req.OrganizationName,
		Email:            req.Email,
		Role:             req.Role,
		InvitedByName:    req.InvitedByName,
		InviteURL:        "/invite?token=sample_verification_token_12345",
		ExpiresAt:        time.Now().Add(7 * 24 * time.Hour),
	}

	if err := h.svc.SendInviteEmail(r.Context(), payload); err != nil {
		response.InternalServerError(w, "Failed to dispatch test invite: "+err.Error())
		return
	}

	response.JSON(w, http.StatusOK, map[string]any{
		"message":   "Test invitation email dispatched successfully",
		"recipient": req.Email,
		"provider":  h.svc.GetEmailProvider().Name(),
	})
}
