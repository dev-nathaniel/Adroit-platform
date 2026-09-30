package ingress

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGenerateRouteID(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"api.example.com", "route_api_example_com"},
		{"*.adroit.run", "route_wildcard_adroit_run"},
		{"MY-APP.COM", "route_my-app_com"},
		{"service.local:8080", "route_service_local_8080"},
	}

	for _, tt := range tests {
		got := GenerateRouteID(tt.input)
		if got != tt.expected {
			t.Errorf("GenerateRouteID(%q) = %q; want %q", tt.input, got, tt.expected)
		}
	}
}

func TestSimulatedIngressDriver(t *testing.T) {
	ctx := context.Background()
	driver := NewSimulatedIngressDriver()

	// 1. Health check
	if err := driver.CheckHealth(ctx); err != nil {
		t.Fatalf("expected healthy, got %v", err)
	}

	// 2. Validation errors
	err := driver.UpsertRoute(ctx, RouteSpec{HostPattern: ""})
	if !errors.Is(err, ErrInvalidHostPattern) {
		t.Errorf("expected ErrInvalidHostPattern, got %v", err)
	}

	err = driver.UpsertRoute(ctx, RouteSpec{HostPattern: "example.com", Upstreams: []UpstreamTarget{}})
	if !errors.Is(err, ErrNoUpstreams) {
		t.Errorf("expected ErrNoUpstreams, got %v", err)
	}

	// 3. Upsert valid route
	spec := RouteSpec{
		HostPattern: "app.adroit.run",
		Upstreams:   []UpstreamTarget{{Dial: "127.0.0.1:8080"}},
	}
	if err := driver.UpsertRoute(ctx, spec); err != nil {
		t.Fatalf("UpsertRoute failed: %v", err)
	}

	expectedID := GenerateRouteID(spec.HostPattern)
	r, err := driver.GetRoute(ctx, expectedID)
	if err != nil {
		t.Fatalf("GetRoute failed: %v", err)
	}
	if r.HostPattern != "app.adroit.run" {
		t.Errorf("expected host app.adroit.run, got %s", r.HostPattern)
	}
	if len(r.Upstreams) != 1 || r.Upstreams[0].Dial != "127.0.0.1:8080" {
		t.Errorf("unexpected upstreams: %+v", r.Upstreams)
	}

	// 4. List routes
	list, err := driver.ListRoutes(ctx)
	if err != nil {
		t.Fatalf("ListRoutes failed: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("expected 1 route, got %d", len(list))
	}

	// 5. Delete route
	if err := driver.DeleteRoute(ctx, expectedID); err != nil {
		t.Fatalf("DeleteRoute failed: %v", err)
	}

	_, err = driver.GetRoute(ctx, expectedID)
	if !errors.Is(err, ErrRouteNotFound) {
		t.Errorf("expected ErrRouteNotFound, got %v", err)
	}

	// 6. Delete non-existent route
	err = driver.DeleteRoute(ctx, "nonexistent")
	if !errors.Is(err, ErrRouteNotFound) {
		t.Errorf("expected ErrRouteNotFound, got %v", err)
	}

	// 7. Simulated Error Injection
	injectedErr := errors.New("simulated network timeout")
	driver.SetSimulateError(injectedErr)
	if err := driver.CheckHealth(ctx); !errors.Is(err, injectedErr) {
		t.Errorf("expected injected error, got %v", err)
	}
}

func TestCaddyClient_WithMockServer(t *testing.T) {
	routesStore := make(map[string]CaddyRouteConfig)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/config/apps/http/servers/adroit_edge":
			if r.Method == http.MethodGet {
				w.WriteHeader(http.StatusOK)
				_ = json.NewEncoder(w).Encode(map[string]any{"listen": []string{":80"}})
				return
			}
			w.WriteHeader(http.StatusOK)
		case r.URL.Path == "/config/apps/http/servers/adroit_edge/routes":
			if r.Method == http.MethodGet {
				var list []CaddyRouteConfig
				for _, route := range routesStore {
					list = append(list, route)
				}
				w.WriteHeader(http.StatusOK)
				_ = json.NewEncoder(w).Encode(list)
				return
			}
			if r.Method == http.MethodPost {
				var route CaddyRouteConfig
				if err := json.NewDecoder(r.Body).Decode(&route); err != nil {
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				routesStore[route.ID] = route
				w.WriteHeader(http.StatusOK)
				return
			}
		case len(r.URL.Path) > 4 && r.URL.Path[:4] == "/id/":
			id := r.URL.Path[4:]
			if r.Method == http.MethodPut {
				var route CaddyRouteConfig
				if err := json.NewDecoder(r.Body).Decode(&route); err != nil {
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				if _, ok := routesStore[id]; !ok {
					w.WriteHeader(http.StatusNotFound)
					return
				}
				routesStore[id] = route
				w.WriteHeader(http.StatusOK)
				return
			}
			if r.Method == http.MethodGet {
				route, ok := routesStore[id]
				if !ok {
					w.WriteHeader(http.StatusNotFound)
					return
				}
				w.WriteHeader(http.StatusOK)
				_ = json.NewEncoder(w).Encode(route)
				return
			}
			if r.Method == http.MethodDelete {
				if _, ok := routesStore[id]; !ok {
					w.WriteHeader(http.StatusNotFound)
					return
				}
				delete(routesStore, id)
				w.WriteHeader(http.StatusOK)
				return
			}
		case r.URL.Path == "/config/":
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	ctx := context.Background()
	client := NewCaddyClient(ts.URL, "adroit_edge")

	if err := client.CheckHealth(ctx); err != nil {
		t.Fatalf("CheckHealth failed: %v", err)
	}

	spec := RouteSpec{
		HostPattern: "api.customer.io",
		Upstreams:   []UpstreamTarget{{Dial: "10.0.0.1:8080"}},
	}
	if err := client.UpsertRoute(ctx, spec); err != nil {
		t.Fatalf("UpsertRoute failed: %v", err)
	}

	routeID := GenerateRouteID(spec.HostPattern)
	info, err := client.GetRoute(ctx, routeID)
	if err != nil {
		t.Fatalf("GetRoute failed: %v", err)
	}
	if info.HostPattern != "api.customer.io" {
		t.Errorf("expected api.customer.io, got %s", info.HostPattern)
	}

	routes, err := client.ListRoutes(ctx)
	if err != nil {
		t.Fatalf("ListRoutes failed: %v", err)
	}
	if len(routes) != 1 {
		t.Errorf("expected 1 route, got %d", len(routes))
	}

	if err := client.DeleteRoute(ctx, routeID); err != nil {
		t.Fatalf("DeleteRoute failed: %v", err)
	}

	_, err = client.GetRoute(ctx, routeID)
	if !errors.Is(err, ErrRouteNotFound) {
		t.Errorf("expected ErrRouteNotFound, got %v", err)
	}
}
