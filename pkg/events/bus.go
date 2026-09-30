package events

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
)

// Common bus errors
var (
	ErrBusClosed        = errors.New("event bus is closed")
	ErrInvalidSubject   = errors.New("invalid event subject")
	ErrNilHandler       = errors.New("event handler cannot be nil")
	ErrNATSUnavailable = errors.New("nats server is unreachable")
)

// Publisher defines the contract for emitting platform domain events
type Publisher interface {
	Publish(ctx context.Context, subject string, data any) error
}

// HandlerFunc processes an incoming raw event payload
type HandlerFunc func(ctx context.Context, subject string, rawData []byte) error

// Subscriber defines the contract for listening to platform domain events
type Subscriber interface {
	Subscribe(subject string, handler HandlerFunc) error
	Close() error
}

// EventBus unites both publishing and subscription capabilities
type EventBus interface {
	Publisher
	Subscriber
}

// ============================================================================
// NoopPublisher
// ============================================================================

// NoopPublisher silently drops published events (used as default or in unit tests)
type NoopPublisher struct{}

// NewNoopPublisher creates a no-op publisher
func NewNoopPublisher() *NoopPublisher {
	return &NoopPublisher{}
}

// Publish drops the event without error
func (n *NoopPublisher) Publish(ctx context.Context, subject string, data any) error {
	return nil
}

// ============================================================================
// InMemBus (Thread-Safe In-Memory Pub/Sub for Testing & Local Fallback)
// ============================================================================

type subscription struct {
	subject string
	handler HandlerFunc
}

// InMemBus provides a synchronized in-memory event bus with pattern matching
type InMemBus struct {
	mu     sync.RWMutex
	subs   []subscription
	closed bool
}

// NewInMemBus creates an in-memory event bus
func NewInMemBus() *InMemBus {
	return &InMemBus{
		subs: make([]subscription, 0),
	}
}

// Publish dispatches the event payload to all matching registered handlers
func (b *InMemBus) Publish(ctx context.Context, subject string, data any) error {
	b.mu.RLock()
	if b.closed {
		b.mu.RUnlock()
		return ErrBusClosed
	}

	raw, err := json.Marshal(data)
	if err != nil {
		b.mu.RUnlock()
		return fmt.Errorf("failed to marshal event data: %w", err)
	}

	// Match subscribers
	var matchedHandlers []HandlerFunc
	for _, sub := range b.subs {
		if matchSubject(sub.subject, subject) {
			matchedHandlers = append(matchedHandlers, sub.handler)
		}
	}
	b.mu.RUnlock()

	// Dispatch concurrently to each handler
	for _, handler := range matchedHandlers {
		h := handler
		go func() {
			_ = h(context.Background(), subject, raw)
		}()
	}

	return nil
}

// Subscribe registers a handler for the given subject or wildcard (e.g. "adroit.>")
func (b *InMemBus) Subscribe(subject string, handler HandlerFunc) error {
	if handler == nil {
		return ErrNilHandler
	}
	if strings.TrimSpace(subject) == "" {
		return ErrInvalidSubject
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return ErrBusClosed
	}

	b.subs = append(b.subs, subscription{
		subject: subject,
		handler: handler,
	})
	return nil
}

// Close closes the in-memory bus
func (b *InMemBus) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.closed = true
	b.subs = nil
	return nil
}

// ============================================================================
// NATS JetStream EventBus
// ============================================================================

// NATSBus manages connection and publishing/subscribing via NATS JetStream
type NATSBus struct {
	nc     *nats.Conn
	js     nats.JetStreamContext
	log    *slog.Logger
	subs   []*nats.Subscription
	mu     sync.Mutex
	closed bool
}

// NATSConfig defines connection parameters for NATS
type NATSConfig struct {
	URL        string
	ClientName string
	Timeout    time.Duration
}

// NewNATSBus establishes connection to NATS and ensures JetStream stream setup
func NewNATSBus(cfg NATSConfig, log *slog.Logger) (*NATSBus, error) {
	if cfg.URL == "" {
		cfg.URL = nats.DefaultURL
	}
	if cfg.ClientName == "" {
		cfg.ClientName = "adroit-service"
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 5 * time.Second
	}
	if log == nil {
		log = slog.Default()
	}

	opts := []nats.Option{
		nats.Name(cfg.ClientName),
		nats.Timeout(cfg.Timeout),
		nats.ReconnectWait(2 * time.Second),
		nats.MaxReconnects(-1), // Unlimited reconnect attempts
		nats.DisconnectErrHandler(func(nc *nats.Conn, err error) {
			log.Warn("NATS connection disconnected", slog.Any("error", err))
		}),
		nats.ReconnectHandler(func(nc *nats.Conn) {
			log.Info("NATS connection re-established", slog.String("url", nc.ConnectedUrl()))
		}),
	}

	nc, err := nats.Connect(cfg.URL, opts...)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrNATSUnavailable, err)
	}

	js, err := nc.JetStream()
	if err != nil {
		nc.Close()
		return nil, fmt.Errorf("failed to initialize JetStream context: %w", err)
	}

	// Ensure ADROIT_EVENTS stream exists covering adroit.> subjects
	streamInfo, err := js.StreamInfo(StreamName)
	if err != nil || streamInfo == nil {
		_, err = js.AddStream(&nats.StreamConfig{
			Name:        StreamName,
			Description: "Adroit Platform Universal Event Backbone",
			Subjects:    []string{"adroit.>"},
			Storage:     nats.FileStorage,
			Retention:   nats.InterestPolicy,
			MaxAge:      7 * 24 * time.Hour, // Retain for 7 days
		})
		if err != nil && !strings.Contains(err.Error(), "already in use") {
			// If file storage is not supported in dev or already exists, attempt memory storage
			_, _ = js.AddStream(&nats.StreamConfig{
				Name:        StreamName,
				Description: "Adroit Platform Universal Event Backbone",
				Subjects:    []string{"adroit.>"},
				Storage:     nats.MemoryStorage,
			})
		}
	}

	return &NATSBus{
		nc:   nc,
		js:   js,
		log:  log,
		subs: make([]*nats.Subscription, 0),
	}, nil
}

// Publish emits an event to NATS JetStream
func (n *NATSBus) Publish(ctx context.Context, subject string, data any) error {
	n.mu.Lock()
	if n.closed {
		n.mu.Unlock()
		return ErrBusClosed
	}
	n.mu.Unlock()

	raw, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to encode event data: %w", err)
	}

	msg := &nats.Msg{
		Subject: subject,
		Data:    raw,
		Header:  make(nats.Header),
	}
	msg.Header.Set("Nats-Msg-Id", fmt.Sprintf("msg_%d", time.Now().UnixNano()))

	_, err = n.js.PublishMsg(msg, nats.Context(ctx))
	if err != nil {
		// Fallback to standard NATS publish if JetStream stream rejects transiently
		return n.nc.Publish(subject, raw)
	}
	return nil
}

// Subscribe listens to events on the given subject
func (n *NATSBus) Subscribe(subject string, handler HandlerFunc) error {
	if handler == nil {
		return ErrNilHandler
	}

	n.mu.Lock()
	defer n.mu.Unlock()
	if n.closed {
		return ErrBusClosed
	}

	sub, err := n.nc.Subscribe(subject, func(msg *nats.Msg) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		if err := handler(ctx, msg.Subject, msg.Data); err != nil {
			n.log.Error("Error handling event message",
				slog.String("subject", msg.Subject),
				slog.String("error", err.Error()),
			)
		}
	})
	if err != nil {
		return fmt.Errorf("failed to subscribe to subject %s: %w", subject, err)
	}

	n.subs = append(n.subs, sub)
	return nil
}

// Close gracefully closes subscriptions and the NATS connection
func (n *NATSBus) Close() error {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.closed {
		return nil
	}
	n.closed = true

	for _, sub := range n.subs {
		_ = sub.Unsubscribe()
	}

	_ = n.nc.Drain()
	n.nc.Close()
	return nil
}

// Helper: matches subject wildcards e.g. "adroit.>" matches "adroit.org.member_invited"
func matchSubject(pattern, subject string) bool {
	if pattern == subject || pattern == ">" || pattern == "*" {
		return true
	}
	if strings.HasSuffix(pattern, ".>") {
		prefix := strings.TrimSuffix(pattern, ".>")
		return strings.HasPrefix(subject, prefix+".") || subject == prefix
	}
	if strings.HasSuffix(pattern, ".*") {
		prefix := strings.TrimSuffix(pattern, ".*")
		if strings.HasPrefix(subject, prefix+".") {
			rest := strings.TrimPrefix(subject, prefix+".")
			return !strings.Contains(rest, ".")
		}
	}
	return false
}
