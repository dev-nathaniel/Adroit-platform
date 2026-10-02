package notificationservice

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/dev-nathaniel/Adroit-platform/pkg/events"
)

var (
	ErrNilProvider = errors.New("notification provider cannot be nil")
)

// Service coordinates email and webhook notification delivery
type Service struct {
	emailProv     EmailProvider
	webhookSender WebhookSender
	tm            *TemplateManager
	appURL        string
	log           *slog.Logger
}

// NewService constructs a notification service
func NewService(
	emailProv EmailProvider,
	webhookSender WebhookSender,
	tm *TemplateManager,
	appURL string,
	log *slog.Logger,
) (*Service, error) {
	if emailProv == nil {
		return nil, ErrNilProvider
	}
	if webhookSender == nil {
		return nil, ErrNilProvider
	}
	if tm == nil {
		var err error
		tm, err = NewTemplateManager()
		if err != nil {
			return nil, err
		}
	}
	if log == nil {
		log = slog.Default()
	}
	if appURL == "" {
		appURL = "http://localhost:8080"
	}

	return &Service{
		emailProv:     emailProv,
		webhookSender: webhookSender,
		tm:            tm,
		appURL:        strings.TrimRight(appURL, "/"),
		log:           log,
	}, nil
}

// SendInviteEmail renders and sends the organization member invitation email
func (s *Service) SendInviteEmail(ctx context.Context, p events.OrgMemberInvitedPayload) error {
	inviteURL := p.InviteURL
	if strings.HasPrefix(inviteURL, "/") {
		inviteURL = s.appURL + inviteURL
	}

	orgName := p.OrganizationName
	if orgName == "" {
		orgName = "Workspace"
	}

	inviterName := p.InvitedByName
	if inviterName == "" {
		inviterName = "A team administrator"
	}

	role := p.Role
	if role == "" {
		role = "member"
	}

	expiresStr := p.ExpiresAt.Format("Monday, Jan 02, 2006 at 15:04 MST")
	if p.ExpiresAt.IsZero() {
		expiresStr = time.Now().Add(7 * 24 * time.Hour).Format("Monday, Jan 02, 2006 at 15:04 MST")
	}

	data := map[string]any{
		"OrganizationName": orgName,
		"InvitedByName":    inviterName,
		"InviterName":      inviterName,
		"InvitedByEmail":   p.InvitedByEmail,
		"RecipientEmail":   p.Email,
		"Role":             role,
		"InviteURL":        inviteURL,
		"ExpiresAt":        expiresStr,
	}

	rendered, err := s.tm.Render("org_invitation", data)
	if err != nil {
		return fmt.Errorf("failed to render org invitation template: %w", err)
	}

	msg := EmailMessage{
		To:        []string{p.Email},
		Subject:   rendered.Subject,
		HTMLBody:  rendered.HTMLBody,
		PlainBody: rendered.PlainBody,
	}

	if err := s.emailProv.SendEmail(ctx, msg); err != nil {
		s.log.Error("Failed to send invitation email",
			slog.String("recipient", p.Email),
			slog.String("org_id", p.OrganizationID),
			slog.Any("error", err),
		)
		return err
	}

	s.log.Info("Successfully delivered organization invitation email",
		slog.String("recipient", p.Email),
		slog.String("org_name", orgName),
		slog.String("invitation_id", p.InvitationID),
	)
	return nil
}

// SendEmail delivers a general or templated email
func (s *Service) SendEmail(ctx context.Context, req SendEmailRequest) error {
	var subject, htmlBody, plainBody string

	if req.Template != "" {
		rendered, err := s.tm.Render(req.Template, req.Data)
		if err != nil {
			return fmt.Errorf("failed to render template '%s': %w", req.Template, err)
		}
		subject = rendered.Subject
		htmlBody = rendered.HTMLBody
		plainBody = rendered.PlainBody
	} else {
		subject = req.Subject
		htmlBody = req.HTMLBody
		plainBody = req.PlainBody
	}

	if req.Subject != "" {
		subject = req.Subject // Override subject if explicitly provided
	}

	msg := EmailMessage{
		To:        req.To,
		Subject:   subject,
		HTMLBody:  htmlBody,
		PlainBody: plainBody,
	}

	return s.emailProv.SendEmail(ctx, msg)
}

// SendWebhook delivers an external HTTP webhook notification
func (s *Service) SendWebhook(ctx context.Context, req SendWebhookRequest) error {
	msg := WebhookMessage{
		URL:     req.URL,
		Event:   req.Event,
		Headers: req.Headers,
		Payload: req.Payload,
		Secret:  req.Secret,
	}
	return s.webhookSender.SendWebhook(ctx, msg)
}

// GetEmailProvider returns the configured email provider
func (s *Service) GetEmailProvider() EmailProvider {
	return s.emailProv
}

// GetWebhookSender returns the configured webhook sender
func (s *Service) GetWebhookSender() WebhookSender {
	return s.webhookSender
}
