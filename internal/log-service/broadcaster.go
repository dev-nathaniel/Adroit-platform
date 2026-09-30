package logservice

import (
	"sync"

	"github.com/dev-nathaniel/Adroit-platform/pkg/types"
)

const defaultChannelBuffer = 512

// Broadcaster manages live pub-sub fan-out of log entries to active subscribers
type Broadcaster struct {
	mu          sync.RWMutex
	subscribers map[string]map[chan types.AppLogEntry]struct{}
	closed      bool
}

// NewBroadcaster creates a new Broadcaster instance
func NewBroadcaster() *Broadcaster {
	return &Broadcaster{
		subscribers: make(map[string]map[chan types.AppLogEntry]struct{}),
	}
}

// Subscribe registers a listener for a specific service ID.
// It returns a read-only channel and an unsubscribe cleanup function.
func (b *Broadcaster) Subscribe(serviceID string) (<-chan types.AppLogEntry, func()) {
	b.mu.Lock()
	defer b.mu.Unlock()

	ch := make(chan types.AppLogEntry, defaultChannelBuffer)
	if b.closed {
		close(ch)
		return ch, func() {}
	}

	if _, exists := b.subscribers[serviceID]; !exists {
		b.subscribers[serviceID] = make(map[chan types.AppLogEntry]struct{})
	}
	b.subscribers[serviceID][ch] = struct{}{}

	var once sync.Once
	unsubscribe := func() {
		once.Do(func() {
			b.mu.Lock()
			defer b.mu.Unlock()

			if subs, exists := b.subscribers[serviceID]; exists {
				delete(subs, ch)
				if len(subs) == 0 {
					delete(b.subscribers, serviceID)
				}
			}
			close(ch)
		})
	}

	return ch, unsubscribe
}

// Broadcast distributes a log entry to all active subscribers of the entry's service
func (b *Broadcaster) Broadcast(entry types.AppLogEntry) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if b.closed {
		return
	}

	subs, exists := b.subscribers[entry.ServiceID]
	if !exists {
		return
	}

	for ch := range subs {
		select {
		case ch <- entry:
		default:
			// If buffer is full, drop to prevent slow clients from stalling the ingestion pipeline
		}
	}
}

// ActiveSubscribers returns the number of active listeners for a service
func (b *Broadcaster) ActiveSubscribers(serviceID string) int {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if subs, exists := b.subscribers[serviceID]; exists {
		return len(subs)
	}
	return 0
}

// Close closes the broadcaster and terminates all active subscriber channels
func (b *Broadcaster) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return
	}
	b.closed = true

	for _, subs := range b.subscribers {
		for ch := range subs {
			close(ch)
		}
	}
	b.subscribers = make(map[string]map[chan types.AppLogEntry]struct{})
}
