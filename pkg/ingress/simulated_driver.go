package ingress

import (
	"context"
	"strings"
	"sync"
	"time"
)

// SimulatedIngressDriver provides an in-memory implementation of the Driver interface
type SimulatedIngressDriver struct {
	mu            sync.RWMutex
	routes        map[string]RouteInfo
	simulateError bool
	errToReturn   error
}

// NewSimulatedIngressDriver creates an initialized in-memory ingress driver
func NewSimulatedIngressDriver() *SimulatedIngressDriver {
	return &SimulatedIngressDriver{
		routes: make(map[string]RouteInfo),
	}
}

// SetSimulateError configures whether future driver calls should return an injected error
func (s *SimulatedIngressDriver) SetSimulateError(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.simulateError = (err != nil)
	s.errToReturn = err
}

// UpsertRoute stores or updates an in-memory route
func (s *SimulatedIngressDriver) UpsertRoute(ctx context.Context, spec RouteSpec) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.simulateError && s.errToReturn != nil {
		return s.errToReturn
	}

	if spec.HostPattern == "" {
		return ErrInvalidHostPattern
	}
	if len(spec.Upstreams) == 0 {
		return ErrNoUpstreams
	}
	if spec.ID == "" {
		spec.ID = GenerateRouteID(spec.HostPattern)
	}

	s.routes[spec.ID] = RouteInfo{
		ID:          spec.ID,
		HostPattern: strings.ToLower(strings.TrimSpace(spec.HostPattern)),
		Upstreams:   spec.Upstreams,
		Active:      true,
		SyncedAt:    time.Now().UTC(),
	}

	return nil
}

// DeleteRoute removes a route by route ID
func (s *SimulatedIngressDriver) DeleteRoute(ctx context.Context, routeID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.simulateError && s.errToReturn != nil {
		return s.errToReturn
	}

	if _, exists := s.routes[routeID]; !exists {
		return ErrRouteNotFound
	}

	delete(s.routes, routeID)
	return nil
}

// GetRoute returns route information for a route ID
func (s *SimulatedIngressDriver) GetRoute(ctx context.Context, routeID string) (*RouteInfo, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.simulateError && s.errToReturn != nil {
		return nil, s.errToReturn
	}

	r, exists := s.routes[routeID]
	if !exists {
		return nil, ErrRouteNotFound
	}

	return &r, nil
}

// ListRoutes returns a slice of all stored routes
func (s *SimulatedIngressDriver) ListRoutes(ctx context.Context) ([]RouteInfo, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.simulateError && s.errToReturn != nil {
		return nil, s.errToReturn
	}

	list := make([]RouteInfo, 0, len(s.routes))
	for _, r := range s.routes {
		list = append(list, r)
	}
	return list, nil
}

// CheckHealth verifies simulator state
func (s *SimulatedIngressDriver) CheckHealth(ctx context.Context) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.simulateError && s.errToReturn != nil {
		return s.errToReturn
	}
	return nil
}
