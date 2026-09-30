package notificationservice

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/dev-nathaniel/Adroit-platform/pkg/events"
)

// Consumer listens for platform events and dispatches notifications
type Consumer struct {
	svc *Service
	sub events.Subscriber
	log *slog.Logger
}

// NewConsumer constructs an event consumer
func NewConsumer(svc *Service, sub events.Subscriber, log *slog.Logger) *Consumer {
	if log == nil {
		log = slog.Default()
	}
	return &Consumer{
		svc: svc,
		sub: sub,
		log: log,
	}
}

// Start registers subscriptions for notification domain subjects
func (c *Consumer) Start() error {
	// 1. Organization Member Invitation Event
	err := c.sub.Subscribe(events.SubjectOrgMemberInvited, func(ctx context.Context, subject string, rawData []byte) error {
		c.log.Info("Received event: member invited", slog.String("subject", subject))

		// Try unmarshaling as Event envelope first, fallback to direct payload
		var envelope events.Event[events.OrgMemberInvitedPayload]
		var payload events.OrgMemberInvitedPayload

		if err := json.Unmarshal(rawData, &envelope); err == nil && envelope.Data.Email != "" {
			payload = envelope.Data
		} else if err := json.Unmarshal(rawData, &payload); err != nil {
			return fmt.Errorf("failed to decode OrgMemberInvitedPayload: %w", err)
		}

		return c.svc.SendInviteEmail(ctx, payload)
	})
	if err != nil {
		return fmt.Errorf("failed to subscribe to %s: %w", events.SubjectOrgMemberInvited, err)
	}

	// 2. Direct Email Notification Event
	err = c.sub.Subscribe(events.SubjectNotificationEmailSend, func(ctx context.Context, subject string, rawData []byte) error {
		c.log.Info("Received event: email send requested", slog.String("subject", subject))

		var envelope events.Event[events.EmailNotificationPayload]
		var payload events.EmailNotificationPayload

		if err := json.Unmarshal(rawData, &envelope); err == nil && len(envelope.Data.To) > 0 {
			payload = envelope.Data
		} else if err := json.Unmarshal(rawData, &payload); err != nil {
			return fmt.Errorf("failed to decode EmailNotificationPayload: %w", err)
		}

		req := SendEmailRequest{
			To:        payload.To,
			Subject:   payload.Subject,
			Template:  payload.Template,
			Data:      payload.Data,
			PlainBody: payload.PlainBody,
			HTMLBody:  payload.HTMLBody,
		}
		return c.svc.SendEmail(ctx, req)
	})
	if err != nil {
		return fmt.Errorf("failed to subscribe to %s: %w", events.SubjectNotificationEmailSend, err)
	}

	// 3. Direct Webhook Notification Event
	err = c.sub.Subscribe(events.SubjectNotificationWebhookSend, func(ctx context.Context, subject string, rawData []byte) error {
		c.log.Info("Received event: webhook send requested", slog.String("subject", subject))

		var envelope events.Event[events.WebhookNotificationPayload]
		var payload events.WebhookNotificationPayload

		if err := json.Unmarshal(rawData, &envelope); err == nil && envelope.Data.URL != "" {
			payload = envelope.Data
		} else if err := json.Unmarshal(rawData, &payload); err != nil {
			return fmt.Errorf("failed to decode WebhookNotificationPayload: %w", err)
		}

		req := SendWebhookRequest{
			URL:     payload.URL,
			Event:   payload.Event,
			Headers: payload.Headers,
			Payload: payload.Payload,
			Secret:  payload.Secret,
		}
		return c.svc.SendWebhook(ctx, req)
	})
	if err != nil {
		return fmt.Errorf("failed to subscribe to %s: %w", events.SubjectNotificationWebhookSend, err)
	}

	c.log.Info("Notification Service event consumer registered successfully")
	return nil
}

// Stop gracefully terminates event subscriptions
func (c *Consumer) Stop() error {
	if c.sub != nil {
		return c.sub.Close()
	}
	return nil
}
