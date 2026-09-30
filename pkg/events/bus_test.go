package events

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestInMemBusPublishSubscribe(t *testing.T) {
	bus := NewInMemBus()
	defer bus.Close()

	var wg sync.WaitGroup
	wg.Add(1)

	var receivedSubject string
	var receivedPayload OrgMemberInvitedPayload

	err := bus.Subscribe("adroit.org.member_invited", func(ctx context.Context, subject string, rawData []byte) error {
		defer wg.Done()
		receivedSubject = subject
		return nil
	})
	if err != nil {
		t.Fatalf("failed to subscribe: %v", err)
	}

	testPayload := OrgMemberInvitedPayload{
		InvitationID:     "inv_123",
		OrganizationID:   "org_456",
		OrganizationName: "Acme Corp",
		Email:            "alice@example.com",
		Role:             "admin",
		InvitedByName:    "Admin",
		InviteURL:        "http://localhost:3000/invite?token=abc",
		ExpiresAt:        time.Now().Add(7 * 24 * time.Hour),
	}

	err = bus.Publish(context.Background(), "adroit.org.member_invited", testPayload)
	if err != nil {
		t.Fatalf("failed to publish: %v", err)
	}

	wg.Wait()

	if receivedSubject != "adroit.org.member_invited" {
		t.Errorf("expected subject adroit.org.member_invited, got %s", receivedSubject)
	}
	_ = receivedPayload
}

func TestInMemBusWildcard(t *testing.T) {
	bus := NewInMemBus()
	defer bus.Close()

	var wg sync.WaitGroup
	wg.Add(2)

	var received []string
	var mu sync.Mutex

	err := bus.Subscribe("adroit.org.>", func(ctx context.Context, subject string, rawData []byte) error {
		mu.Lock()
		received = append(received, subject)
		mu.Unlock()
		wg.Done()
		return nil
	})
	if err != nil {
		t.Fatalf("failed to subscribe: %v", err)
	}

	_ = bus.Publish(context.Background(), "adroit.org.member_invited", map[string]string{"foo": "bar"})
	_ = bus.Publish(context.Background(), "adroit.org.member_joined", map[string]string{"foo": "bar"})

	wg.Wait()

	mu.Lock()
	defer mu.Unlock()
	if len(received) != 2 {
		t.Fatalf("expected 2 received events, got %d", len(received))
	}
}

func TestNoopPublisher(t *testing.T) {
	noop := NewNoopPublisher()
	if err := noop.Publish(context.Background(), "test", nil); err != nil {
		t.Errorf("noop publisher failed: %v", err)
	}
}
