package notificationservice

import (
	"time"
)

// EmailMessage holds fields required to dispatch an email
type EmailMessage struct {
	To        []string          `json:"to"`
	Subject   string            `json:"subject"`
	HTMLBody  string            `json:"html_body"`
	PlainBody string            `json:"plain_body"`
	From      string            `json:"from,omitempty"`
	Headers   map[string]string `json:"headers,omitempty"`
}

// WebhookMessage holds fields required to execute an outbound webhook request
type WebhookMessage struct {
	URL     string            `json:"url"`
	Event   string            `json:"event"`
	Headers map[string]string `json:"headers,omitempty"`
	Payload any               `json:"payload"`
	Secret  string            `json:"secret,omitempty"`
}

// SendEmailRequest is the HTTP request payload for direct email dispatch
type SendEmailRequest struct {
	To        []string       `json:"to"`
	Subject   string         `json:"subject"`
	Template  string         `json:"template,omitempty"`
	Data      map[string]any `json:"data,omitempty"`
	HTMLBody  string         `json:"html_body,omitempty"`
	PlainBody string         `json:"plain_body,omitempty"`
}

// SendWebhookRequest is the HTTP request payload for direct webhook dispatch
type SendWebhookRequest struct {
	URL     string            `json:"url"`
	Event   string            `json:"event"`
	Headers map[string]string `json:"headers,omitempty"`
	Payload any               `json:"payload"`
	Secret  string            `json:"secret,omitempty"`
}

// NotificationDeliveryStatus tracks outgoing delivery outcomes
type NotificationDeliveryStatus struct {
	ID        string    `json:"id"`
	Channel   string    `json:"channel"` // "email" or "webhook"
	Recipient string    `json:"recipient"`
	Status    string    `json:"status"` // "delivered", "failed", "simulated"
	Error     string    `json:"error,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}
