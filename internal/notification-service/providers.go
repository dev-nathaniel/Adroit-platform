package notificationservice

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net"
	"net/http"
	"net/mail"
	"net/smtp"
	"net/textproto"
	"strings"
	"sync"
	"time"
)

var (
	ErrNoRecipients = errors.New("email must have at least one recipient")
	ErrEmptySubject = errors.New("email subject cannot be empty")
	ErrEmptyURL     = errors.New("webhook url cannot be empty")
)

// EmailProvider abstracts delivery to outbound email services
type EmailProvider interface {
	SendEmail(ctx context.Context, msg EmailMessage) error
	Name() string
}

// WebhookSender abstracts delivery to external webhooks
type WebhookSender interface {
	SendWebhook(ctx context.Context, msg WebhookMessage) error
	Name() string
}

// ============================================================================
// Console / Mock Email Provider (Default for Local Development & Testing)
// ============================================================================

// ConsoleEmailProvider logs sent emails to slog and stores recent messages in-memory for testing
type ConsoleEmailProvider struct {
	log          *slog.Logger
	mu           sync.Mutex
	lastMessages []EmailMessage
}

// NewConsoleEmailProvider creates a console email provider
func NewConsoleEmailProvider(log *slog.Logger) *ConsoleEmailProvider {
	if log == nil {
		log = slog.Default()
	}
	return &ConsoleEmailProvider{
		log:          log,
		lastMessages: make([]EmailMessage, 0),
	}
}

func (c *ConsoleEmailProvider) Name() string {
	return "console"
}

func (c *ConsoleEmailProvider) SendEmail(ctx context.Context, msg EmailMessage) error {
	if len(msg.To) == 0 {
		return ErrNoRecipients
	}
	if strings.TrimSpace(msg.Subject) == "" {
		return ErrEmptySubject
	}

	c.mu.Lock()
	c.lastMessages = append(c.lastMessages, msg)
	c.mu.Unlock()

	recipients := strings.Join(msg.To, ", ")
	c.log.Info("✉ [EMAIL DISPATCHED] (Console Provider)",
		slog.String("to", recipients),
		slog.String("subject", msg.Subject),
	)

	// In development, display clear ASCII preview
	fmt.Println()
	fmt.Println("================================================================================")
	fmt.Printf(" [DEVELOPMENT EMAIL] To: %s | Subject: %s\n", recipients, msg.Subject)
	fmt.Println("--------------------------------------------------------------------------------")
	if msg.PlainBody != "" {
		fmt.Println(msg.PlainBody)
	} else {
		fmt.Printf("(HTML Content: %d bytes)\n", len(msg.HTMLBody))
	}
	fmt.Println("================================================================================")
	fmt.Println()

	return nil
}

// GetLastMessages returns a copy of captured messages (for unit tests)
func (c *ConsoleEmailProvider) GetLastMessages() []EmailMessage {
	c.mu.Lock()
	defer c.mu.Unlock()
	res := make([]EmailMessage, len(c.lastMessages))
	copy(res, c.lastMessages)
	return res
}

// ============================================================================
// SMTP Email Provider
// ============================================================================

// SMTPConfig contains SMTP credentials and server connection details
type SMTPConfig struct {
	Host       string
	Port       int
	Username   string
	Password   string
	From       string
	SkipVerify bool
}

// SMTPEmailProvider sends emails via standard SMTP server (Mailpit, SES, SendGrid, etc.)
type SMTPEmailProvider struct {
	cfg SMTPConfig
	log *slog.Logger
}

// NewSMTPEmailProvider creates an SMTP email provider
func NewSMTPEmailProvider(cfg SMTPConfig, log *slog.Logger) *SMTPEmailProvider {
	if log == nil {
		log = slog.Default()
	}
	if cfg.Port == 0 {
		cfg.Port = 587
	}
	return &SMTPEmailProvider{
		cfg: cfg,
		log: log,
	}
}

func (s *SMTPEmailProvider) Name() string {
	return "smtp"
}

func (s *SMTPEmailProvider) SendEmail(ctx context.Context, msg EmailMessage) error {
	if len(msg.To) == 0 {
		return ErrNoRecipients
	}
	if strings.TrimSpace(msg.Subject) == "" {
		return ErrEmptySubject
	}

	from := msg.From
	if from == "" {
		from = s.cfg.From
	}
	if from == "" {
		from = "no-reply@adroit.run"
	}

	addr := fmt.Sprintf("%s:%d", s.cfg.Host, s.cfg.Port)

	// Build MIME multipart/alternative message (plain text + HTML)
	buf := new(bytes.Buffer)
	buf.WriteString(fmt.Sprintf("From: %s\r\n", from))
	buf.WriteString(fmt.Sprintf("To: %s\r\n", strings.Join(msg.To, ", ")))
	buf.WriteString(fmt.Sprintf("Subject: %s\r\n", msg.Subject))
	buf.WriteString("MIME-Version: 1.0\r\n")

	for k, v := range msg.Headers {
		buf.WriteString(fmt.Sprintf("%s: %s\r\n", k, v))
	}

	writer := multipart.NewWriter(buf)
	buf.WriteString(fmt.Sprintf("Content-Type: multipart/alternative; boundary=\"%s\"\r\n\r\n", writer.Boundary()))

	// 1. Plain text part
	if msg.PlainBody != "" {
		partHeader := make(textproto.MIMEHeader)
		partHeader.Set("Content-Type", "text/plain; charset=UTF-8")
		partHeader.Set("Content-Transfer-Encoding", "7bit")
		part, err := writer.CreatePart(partHeader)
		if err != nil {
			return err
		}
		if _, err := part.Write([]byte(msg.PlainBody)); err != nil {
			return err
		}
	}

	// 2. HTML part
	if msg.HTMLBody != "" {
		partHeader := make(textproto.MIMEHeader)
		partHeader.Set("Content-Type", "text/html; charset=UTF-8")
		partHeader.Set("Content-Transfer-Encoding", "7bit")
		part, err := writer.CreatePart(partHeader)
		if err != nil {
			return err
		}
		if _, err := part.Write([]byte(msg.HTMLBody)); err != nil {
			return err
		}
	}
	_ = writer.Close()

	// Extract bare email address for SMTP envelope MAIL FROM
	envelopeFrom := from
	if parsed, err := mail.ParseAddress(from); err == nil {
		envelopeFrom = parsed.Address
	}

	// Extract bare email addresses for SMTP envelope RCPT TO
	envelopeTo := make([]string, 0, len(msg.To))
	for _, to := range msg.To {
		if parsed, err := mail.ParseAddress(to); err == nil {
			envelopeTo = append(envelopeTo, parsed.Address)
		} else {
			envelopeTo = append(envelopeTo, strings.TrimSpace(to))
		}
	}

	var auth smtp.Auth
	if s.cfg.Username != "" && s.cfg.Password != "" {
		auth = smtp.PlainAuth("", s.cfg.Username, s.cfg.Password, s.cfg.Host)
	}

	var conn net.Conn
	var err error

	if s.cfg.Port == 465 {
		tlsConfig := &tls.Config{
			ServerName:         s.cfg.Host,
			InsecureSkipVerify: s.cfg.SkipVerify,
		}
		conn, err = tls.Dial("tcp", addr, tlsConfig)
		if err != nil {
			return fmt.Errorf("failed to dial SMTP TLS %s: %w", addr, err)
		}
	} else {
		conn, err = net.DialTimeout("tcp", addr, 10*time.Second)
		if err != nil {
			return fmt.Errorf("failed to connect to SMTP server %s: %w", addr, err)
		}
	}
	defer conn.Close()

	c, err := smtp.NewClient(conn, s.cfg.Host)
	if err != nil {
		return fmt.Errorf("failed to create SMTP client: %w", err)
	}
	defer c.Quit()

	if s.cfg.Port != 465 {
		if ok, _ := c.Extension("STARTTLS"); ok {
			tlsConfig := &tls.Config{
				ServerName:         s.cfg.Host,
				InsecureSkipVerify: s.cfg.SkipVerify,
			}
			if err := c.StartTLS(tlsConfig); err != nil {
				return fmt.Errorf("smtp starttls failed: %w", err)
			}
		}
	}

	if auth != nil {
		if err := c.Auth(auth); err != nil {
			return fmt.Errorf("smtp auth failed: %w", err)
		}
	}

	if err := c.Mail(envelopeFrom); err != nil {
		return fmt.Errorf("smtp MAIL FROM failed: %w", err)
	}
	for _, to := range envelopeTo {
		if err := c.Rcpt(to); err != nil {
			return fmt.Errorf("smtp RCPT TO failed: %w", err)
		}
	}
	w, err := c.Data()
	if err != nil {
		return fmt.Errorf("smtp DATA command failed: %w", err)
	}
	if _, err := w.Write(buf.Bytes()); err != nil {
		return fmt.Errorf("smtp write message failed: %w", err)
	}
	return w.Close()
}

// ============================================================================
// Webhook Dispatcher with HMAC Signature & Retries
// ============================================================================

// HTTPWebhookSender dispatches webhooks via HTTP POST with exponential backoff
type HTTPWebhookSender struct {
	client     *http.Client
	log        *slog.Logger
	maxRetries int
}

// NewHTTPWebhookSender creates an HTTP webhook dispatcher
func NewHTTPWebhookSender(timeout time.Duration, log *slog.Logger) *HTTPWebhookSender {
	if timeout == 0 {
		timeout = 10 * time.Second
	}
	if log == nil {
		log = slog.Default()
	}
	return &HTTPWebhookSender{
		client: &http.Client{
			Timeout: timeout,
		},
		log:        log,
		maxRetries: 3,
	}
}

func (h *HTTPWebhookSender) Name() string {
	return "http-webhook"
}

func (h *HTTPWebhookSender) SendWebhook(ctx context.Context, msg WebhookMessage) error {
	if strings.TrimSpace(msg.URL) == "" {
		return ErrEmptyURL
	}

	payloadBytes, err := json.Marshal(msg.Payload)
	if err != nil {
		return fmt.Errorf("failed to marshal webhook payload: %w", err)
	}

	var lastErr error
	backoff := 200 * time.Millisecond

	for attempt := 1; attempt <= h.maxRetries; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, msg.URL, bytes.NewReader(payloadBytes))
		if err != nil {
			return fmt.Errorf("failed to create webhook request: %w", err)
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", "Adroit-Notification-Service/1.0")
		if msg.Event != "" {
			req.Header.Set("X-Adroit-Event", msg.Event)
		}

		// Optional HMAC-SHA256 signature for payload verification
		if msg.Secret != "" {
			mac := hmac.New(sha256.New, []byte(msg.Secret))
			mac.Write(payloadBytes)
			signature := hex.EncodeToString(mac.Sum(nil))
			req.Header.Set("X-Adroit-Signature", "sha256="+signature)
		}

		for k, v := range msg.Headers {
			req.Header.Set(k, v)
		}

		resp, err := h.client.Do(req)
		if err == nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()

			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				h.log.Info("Webhook delivered successfully",
					slog.String("url", msg.URL),
					slog.String("event", msg.Event),
					slog.Int("status_code", resp.StatusCode),
				)
				return nil
			}

			// Don't retry client errors (4xx) except 429
			if resp.StatusCode >= 400 && resp.StatusCode < 500 && resp.StatusCode != http.StatusTooManyRequests {
				return fmt.Errorf("webhook rejected by endpoint with HTTP %d", resp.StatusCode)
			}
			lastErr = fmt.Errorf("webhook endpoint returned status %d", resp.StatusCode)
		} else {
			lastErr = err
		}

		h.log.Warn("Webhook dispatch failed, retrying...",
			slog.String("url", msg.URL),
			slog.Int("attempt", attempt),
			slog.Any("error", lastErr),
		)

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
			backoff *= 2
		}
	}

	return fmt.Errorf("failed to deliver webhook after %d attempts: %w", h.maxRetries, lastErr)
}
