package notificationservice

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/dev-nathaniel/Adroit-platform/pkg/events"
	"github.com/dev-nathaniel/Adroit-platform/pkg/logger"
)

func setupTestService(t *testing.T) (*Service, *ConsoleEmailProvider, *HTTPWebhookSender) {
	log := logger.New(logger.Config{Level: "error", Format: "text"})
	emailProv := NewConsoleEmailProvider(log)
	webhookSender := NewHTTPWebhookSender(2*time.Second, log)
	tm, err := NewTemplateManager()
	if err != nil {
		t.Fatalf("failed to create template manager: %v", err)
	}

	svc, err := NewService(emailProv, webhookSender, tm, "http://localhost:3000", log)
	if err != nil {
		t.Fatalf("failed to create notification service: %v", err)
	}

	return svc, emailProv, webhookSender
}

func TestTemplateManagerRendering(t *testing.T) {
	tm, err := NewTemplateManager()
	if err != nil {
		t.Fatalf("failed to initialize template manager: %v", err)
	}

	// 1. Test Org Invitation Template
	data := map[string]any{
		"OrganizationName": "Acme PaaS",
		"InviterName":      "Nathaniel",
		"Role":             "admin",
		"InviteURL":        "http://localhost:3000/invite?token=secret123",
		"ExpiresAt":        "Friday, Oct 05, 2026",
	}

	rendered, err := tm.Render("org_invitation", data)
	if err != nil {
		t.Fatalf("failed to render org_invitation: %v", err)
	}

	if !strings.Contains(rendered.Subject, "Acme PaaS") {
		t.Errorf("subject missing org name: %s", rendered.Subject)
	}
	if !strings.Contains(rendered.HTMLBody, "secret123") {
		t.Errorf("html body missing token link")
	}
	if !strings.Contains(rendered.PlainBody, "Nathaniel has invited you") {
		t.Errorf("plain text missing inviter name")
	}

	// 2. Test Welcome Template
	renderedWelcome, err := tm.Render("welcome", map[string]any{
		"Name":         "Alice",
		"DashboardURL": "http://localhost:3000/dashboard",
	})
	if err != nil {
		t.Fatalf("failed to render welcome: %v", err)
	}
	if !strings.Contains(renderedWelcome.Subject, "Alice") {
		t.Errorf("welcome subject missing name: %s", renderedWelcome.Subject)
	}
}

func TestSendInviteEmail(t *testing.T) {
	svc, emailProv, _ := setupTestService(t)

	payload := events.OrgMemberInvitedPayload{
		InvitationID:     "inv_999",
		OrganizationID:   "org_111",
		OrganizationName: "Stark Industries",
		Email:            "peter.parker@example.com",
		Role:             "admin",
		InvitedByName:    "Tony Stark",
		InviteURL:        "/invite?token=stark_token_abc",
		ExpiresAt:        time.Now().Add(7 * 24 * time.Hour),
	}

	err := svc.SendInviteEmail(context.Background(), payload)
	if err != nil {
		t.Fatalf("SendInviteEmail failed: %v", err)
	}

	messages := emailProv.GetLastMessages()
	if len(messages) != 1 {
		t.Fatalf("expected 1 captured email, got %d", len(messages))
	}

	msg := messages[0]
	if len(msg.To) != 1 || msg.To[0] != "peter.parker@example.com" {
		t.Errorf("expected recipient peter.parker@example.com, got %v", msg.To)
	}
	if !strings.Contains(msg.Subject, "Stark Industries") {
		t.Errorf("expected subject to contain Stark Industries, got %s", msg.Subject)
	}
	if !strings.Contains(msg.HTMLBody, "http://localhost:3000/invite?token=stark_token_abc") {
		t.Errorf("expected absolute invite URL in email body, got %s", msg.HTMLBody)
	}
}

func TestWebhookDeliveryWithHMAC(t *testing.T) {
	log := logger.New(logger.Config{Level: "error", Format: "text"})
	webhookSender := NewHTTPWebhookSender(3*time.Second, log)

	var receivedBody []byte
	var receivedSignature string
	var receivedEvent string
	var receivedHeader string

	secret := "webhook-secret-key-12345"

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedSignature = r.Header.Get("X-Adroit-Signature")
		receivedEvent = r.Header.Get("X-Adroit-Event")
		receivedHeader = r.Header.Get("X-Custom-Header")
		receivedBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	payloadData := map[string]string{"user_id": "u_123", "action": "joined"}
	msg := WebhookMessage{
		URL:   ts.URL,
		Event: "user.joined",
		Headers: map[string]string{
			"X-Custom-Header": "TestValue",
		},
		Payload: payloadData,
		Secret:  secret,
	}

	err := webhookSender.SendWebhook(context.Background(), msg)
	if err != nil {
		t.Fatalf("failed to send webhook: %v", err)
	}

	if receivedEvent != "user.joined" {
		t.Errorf("expected event user.joined, got %s", receivedEvent)
	}
	if receivedHeader != "TestValue" {
		t.Errorf("expected header TestValue, got %s", receivedHeader)
	}

	// Verify HMAC-SHA256 signature
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(receivedBody)
	expectedSig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	if receivedSignature != expectedSig {
		t.Errorf("signature mismatch: expected %s, got %s", expectedSig, receivedSignature)
	}
}

func TestWebhookRetryOnServerError(t *testing.T) {
	log := logger.New(logger.Config{Level: "error", Format: "text"})
	webhookSender := NewHTTPWebhookSender(2*time.Second, log)

	var attempts int
	var mu sync.Mutex

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		attempts++
		current := attempts
		mu.Unlock()

		if current < 2 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	msg := WebhookMessage{
		URL:     ts.URL,
		Event:   "test.retry",
		Payload: map[string]string{"foo": "bar"},
	}

	err := webhookSender.SendWebhook(context.Background(), msg)
	if err != nil {
		t.Fatalf("expected webhook to succeed after retry, got error: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if attempts != 2 {
		t.Errorf("expected 2 attempts, got %d", attempts)
	}
}

func TestEventConsumerIntegration(t *testing.T) {
	svc, emailProv, _ := setupTestService(t)
	inMemBus := events.NewInMemBus()
	defer inMemBus.Close()

	consumer := NewConsumer(svc, inMemBus, logger.New(logger.Config{Level: "error", Format: "text"}))
	if err := consumer.Start(); err != nil {
		t.Fatalf("failed to start consumer: %v", err)
	}
	defer consumer.Stop()

	// Publish OrgMemberInvited event
	payload := events.OrgMemberInvitedPayload{
		InvitationID:     "inv_event_1",
		OrganizationID:   "org_event_1",
		OrganizationName: "Wayne Enterprises",
		Email:            "bruce@waynecorp.com",
		Role:             "owner",
		InvitedByName:    "Alfred",
		InviteURL:        "/invite?token=batman_secret",
		ExpiresAt:        time.Now().Add(7 * 24 * time.Hour),
	}

	err := inMemBus.Publish(context.Background(), events.SubjectOrgMemberInvited, payload)
	if err != nil {
		t.Fatalf("failed to publish event: %v", err)
	}

	// Wait briefly for asynchronous in-memory delivery
	time.Sleep(50 * time.Millisecond)

	messages := emailProv.GetLastMessages()
	if len(messages) != 1 {
		t.Fatalf("expected 1 email received via event consumer, got %d", len(messages))
	}

	if messages[0].To[0] != "bruce@waynecorp.com" {
		t.Errorf("expected recipient bruce@waynecorp.com, got %s", messages[0].To[0])
	}
}

func TestHTTPHandlers(t *testing.T) {
	svc, _, _ := setupTestService(t)
	h := NewHandler(svc)

	r := chi.NewRouter()
	h.RegisterRoutes(r)

	// 1. HealthCheck
	reqHealth := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	recHealth := httptest.NewRecorder()
	r.ServeHTTP(recHealth, reqHealth)

	if recHealth.Code != http.StatusOK {
		t.Errorf("expected 200 on /healthz, got %d", recHealth.Code)
	}

	// 2. Direct Email Dispatch
	emailReq := SendEmailRequest{
		To:       []string{"developer@example.com"},
		Subject:  "Hello from Adroit",
		Template: "welcome",
		Data: map[string]any{
			"Name":         "Dev",
			"DashboardURL": "http://localhost:3000",
		},
	}
	emailBody, _ := json.Marshal(emailReq)
	reqEmail := httptest.NewRequest(http.MethodPost, "/api/v1/notifications/email", bytes.NewReader(emailBody))
	recEmail := httptest.NewRecorder()
	r.ServeHTTP(recEmail, reqEmail)

	if recEmail.Code != http.StatusOK {
		t.Errorf("expected 200 on /api/v1/notifications/email, got %d: %s", recEmail.Code, recEmail.Body.String())
	}

	// 3. Test Invite Endpoint
	reqInvite := httptest.NewRequest(http.MethodPost, "/api/v1/notifications/test-invite", bytes.NewReader([]byte(`{"email":"test@adroit.run"}`)))
	recInvite := httptest.NewRecorder()
	r.ServeHTTP(recInvite, reqInvite)

	if recInvite.Code != http.StatusOK {
		t.Errorf("expected 200 on /api/v1/notifications/test-invite, got %d: %s", recInvite.Code, recInvite.Body.String())
	}
}
