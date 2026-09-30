package dnsservice

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/dev-nathaniel/Adroit-platform/pkg/events"
	"github.com/dev-nathaniel/Adroit-platform/pkg/ingress"
	"github.com/dev-nathaniel/Adroit-platform/pkg/types"
)

var (
	ErrInvalidDomainName   = errors.New("invalid domain name: must be a valid fully-qualified domain name")
	ErrDomainNotVerified   = errors.New("domain is not verified yet")
	ErrNoActiveDeployment  = errors.New("no active deployment found for this service to route traffic to")
	domainRegex            = regexp.MustCompile(`^([a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?\.)+[a-zA-Z]{2,}$`)
)

// ServiceConfig holds operational parameters for the DNS & Ingress Service
type ServiceConfig struct {
	PlatformDomain string // Default platform domain suffix, e.g. "adroit.run"
	CanonicalCNAME string // Ingress target for CNAME challenges, e.g. "cname.adroit.run"
}

// Service coordinates domain verification and dynamic edge reverse proxy configuration
type Service struct {
	repo          Repository
	ingressDriver ingress.Driver
	dnsResolver   DNSResolver
	eventBus      events.EventBus
	logger        *slog.Logger
	cfg           ServiceConfig
}

// NewService creates an initialized Ingress & DNS Service
func NewService(
	repo Repository,
	ingressDriver ingress.Driver,
	dnsResolver DNSResolver,
	eventBus events.EventBus,
	logger *slog.Logger,
	cfg ServiceConfig,
) *Service {
	if cfg.PlatformDomain == "" {
		cfg.PlatformDomain = "adroit.run"
	}
	if cfg.CanonicalCNAME == "" {
		cfg.CanonicalCNAME = "cname.adroit.run"
	}

	return &Service{
		repo:          repo,
		ingressDriver: ingressDriver,
		dnsResolver:   dnsResolver,
		eventBus:      eventBus,
		logger:        logger,
		cfg:           cfg,
	}
}

// AddCustomDomain registers a new domain name to point to a service
func (s *Service) AddCustomDomain(ctx context.Context, serviceID string, req CreateDomainRequest) (*DomainResponse, error) {
	cleanDomain := strings.ToLower(strings.TrimSpace(req.Domain))
	if cleanDomain == "" || (!domainRegex.MatchString(cleanDomain) && !strings.HasSuffix(cleanDomain, ".localhost") && !strings.HasSuffix(cleanDomain, ".test")) {
		return nil, ErrInvalidDomainName
	}

	// 1. Verify service exists
	svc, err := s.repo.GetService(ctx, serviceID)
	if err != nil {
		return nil, err
	}

	// 2. Generate cryptographically secure verification token
	tokenBytes := make([]byte, 16)
	_, _ = rand.Read(tokenBytes)
	verificationToken := "adr_verify_" + hex.EncodeToString(tokenBytes)

	vType := req.VerificationType
	if vType == "" {
		vType = types.VerificationTypeCNAME
	}

	d := &types.CustomDomain{
		ServiceID:         svc.ID,
		Domain:            cleanDomain,
		Status:            types.DomainStatusPendingVerification,
		VerificationType:  vType,
		VerificationToken: verificationToken,
		IsPrimary:         req.IsPrimary,
		SSLStatus:         types.SSLStatusPending,
	}

	if err := s.repo.CreateDomain(ctx, d); err != nil {
		return nil, err
	}

	// If marked as primary, update other domains
	if d.IsPrimary {
		_ = s.repo.SetPrimaryDomain(ctx, svc.ID, d.ID)
	}

	// 3. Emit domain.created event
	if s.eventBus != nil {
		evt := events.New(events.SubjectDomainCreated, "dns-service", events.DomainCreatedPayload{
			DomainID:         d.ID,
			ServiceID:        d.ServiceID,
			Domain:           d.Domain,
			VerificationType: string(d.VerificationType),
		})
		_ = s.eventBus.Publish(ctx, events.SubjectDomainCreated, evt)
	}

	challenge := GenerateChallengeDetails(d.Domain, d.VerificationType, d.VerificationToken, s.cfg.CanonicalCNAME)
	challenge.Status = d.Status

	s.logger.Info("Registered new custom domain",
		slog.String("domain", d.Domain),
		slog.String("service_id", d.ServiceID),
		slog.String("verification_type", string(d.VerificationType)),
	)

	return &DomainResponse{
		Domain:           *d,
		ChallengeDetails: challenge,
	}, nil
}

// ListServiceDomains retrieves all domains attached to a given service
func (s *Service) ListServiceDomains(ctx context.Context, serviceID string, page, perPage int) ([]DomainResponse, int, error) {
	if page < 1 {
		page = 1
	}
	if perPage < 1 || perPage > 100 {
		perPage = 20
	}
	offset := (page - 1) * perPage

	domains, total, err := s.repo.ListDomainsByService(ctx, serviceID, perPage, offset)
	if err != nil {
		return nil, 0, err
	}

	responses := make([]DomainResponse, len(domains))
	for i, d := range domains {
		ch := GenerateChallengeDetails(d.Domain, d.VerificationType, d.VerificationToken, s.cfg.CanonicalCNAME)
		ch.Status = d.Status
		responses[i] = DomainResponse{
			Domain:           d,
			ChallengeDetails: ch,
		}
	}

	return responses, total, nil
}

// GetDomainDetails returns domain status and challenge details by domain UUID
func (s *Service) GetDomainDetails(ctx context.Context, domainID string) (*DomainResponse, error) {
	d, err := s.repo.GetDomainByID(ctx, domainID)
	if err != nil {
		return nil, err
	}

	ch := GenerateChallengeDetails(d.Domain, d.VerificationType, d.VerificationToken, s.cfg.CanonicalCNAME)
	ch.Status = d.Status

	return &DomainResponse{
		Domain:           *d,
		ChallengeDetails: ch,
	}, nil
}

// VerifyCustomDomain triggers DNS verification for a domain
func (s *Service) VerifyCustomDomain(ctx context.Context, domainID string) (*VerifyDomainResponse, error) {
	d, err := s.repo.GetDomainByID(ctx, domainID)
	if err != nil {
		return nil, err
	}

	passed, message, err := CheckDNSChallenge(ctx, s.dnsResolver, d.Domain, d.VerificationType, d.VerificationToken, s.cfg.CanonicalCNAME)
	if err != nil {
		s.logger.Error("DNS challenge check error", slog.String("domain", d.Domain), slog.Any("error", err))
		return nil, err
	}

	challenge := GenerateChallengeDetails(d.Domain, d.VerificationType, d.VerificationToken, s.cfg.CanonicalCNAME)

	if !passed {
		_ = s.repo.UpdateDomainStatus(ctx, d.ID, types.DomainStatusFailedVerification, types.SSLStatusPending, message, nil)
		d.Status = types.DomainStatusFailedVerification
		challenge.Status = d.Status

		return &VerifyDomainResponse{
			Verified:         false,
			Status:           d.Status,
			Message:          message,
			Domain:           *d,
			ChallengeDetails: challenge,
		}, nil
	}

	// Passed verification
	now := time.Now().UTC()
	d.Status = types.DomainStatusActive
	d.SSLStatus = types.SSLStatusIssuing
	d.VerifiedAt = &now
	_ = s.repo.UpdateDomainStatus(ctx, d.ID, d.Status, d.SSLStatus, "", d.VerifiedAt)

	challenge.Status = d.Status

	// Emit domain.verified event
	if s.eventBus != nil {
		evt := events.New(events.SubjectDomainVerified, "dns-service", events.DomainVerifiedPayload{
			DomainID:  d.ID,
			ServiceID: d.ServiceID,
			Domain:    d.Domain,
		})
		_ = s.eventBus.Publish(ctx, events.SubjectDomainVerified, evt)
	}

	s.logger.Info("Custom domain successfully verified",
		slog.String("domain", d.Domain),
		slog.String("service_id", d.ServiceID),
	)

	// If there's an active deployment for this service, immediately synchronize Caddy route
	activeDep, err := s.repo.GetActiveDeploymentForService(ctx, d.ServiceID)
	if err == nil && activeDep != nil {
		targetDial := extractUpstreamDial(activeDep.ActiveURL)
		if targetDial != "" {
			routeID := ingress.GenerateRouteID(d.Domain)
			routeSpec := ingress.RouteSpec{
				ID:          routeID,
				HostPattern: d.Domain,
				Upstreams:   []ingress.UpstreamTarget{{Dial: targetDial}},
				EnableTLS:   true,
			}
			if err := s.ingressDriver.UpsertRoute(ctx, routeSpec); err == nil {
				_ = s.repo.UpsertIngressRoute(ctx, &types.IngressRoute{
					ServiceID:   d.ServiceID,
					DomainID:    &d.ID,
					HostPattern: d.Domain,
					UpstreamURL: targetDial,
					IsActive:    true,
				})
				_ = s.repo.UpdateDomainStatus(ctx, d.ID, types.DomainStatusActive, types.SSLStatusActive, "", d.VerifiedAt)
				d.SSLStatus = types.SSLStatusActive
			}
		}
	}

	return &VerifyDomainResponse{
		Verified:         true,
		Status:           d.Status,
		Message:          message,
		Domain:           *d,
		ChallengeDetails: challenge,
	}, nil
}

// SyncEdgeRoute forces synchronization of Caddy route for a verified domain
func (s *Service) SyncEdgeRoute(ctx context.Context, domainID string, req SyncRouteRequest) (*types.IngressRoute, error) {
	d, err := s.repo.GetDomainByID(ctx, domainID)
	if err != nil {
		return nil, err
	}

	if d.Status != types.DomainStatusActive {
		return nil, ErrDomainNotVerified
	}

	targetDial := req.UpstreamURL
	if targetDial == "" {
		activeDep, err := s.repo.GetActiveDeploymentForService(ctx, d.ServiceID)
		if err != nil || activeDep == nil {
			return nil, ErrNoActiveDeployment
		}
		targetDial = extractUpstreamDial(activeDep.ActiveURL)
		if targetDial == "" {
			return nil, ErrNoActiveDeployment
		}
	}

	routeID := ingress.GenerateRouteID(d.Domain)
	spec := ingress.RouteSpec{
		ID:          routeID,
		HostPattern: d.Domain,
		Upstreams:   []ingress.UpstreamTarget{{Dial: targetDial}},
		EnableTLS:   true,
	}

	if err := s.ingressDriver.UpsertRoute(ctx, spec); err != nil {
		s.logger.Error("Failed to upsert Caddy edge route", slog.String("domain", d.Domain), slog.Any("error", err))
		return nil, err
	}

	ingressRoute := &types.IngressRoute{
		ServiceID:   d.ServiceID,
		DomainID:    &d.ID,
		HostPattern: d.Domain,
		UpstreamURL: targetDial,
		IsActive:    true,
	}
	if err := s.repo.UpsertIngressRoute(ctx, ingressRoute); err != nil {
		return nil, err
	}

	// Emit route.updated event
	if s.eventBus != nil {
		evt := events.New(events.SubjectRouteUpdated, "dns-service", events.RouteUpdatedPayload{
			ServiceID:   d.ServiceID,
			HostPattern: d.Domain,
			UpstreamURL: targetDial,
			Domains:     []string{d.Domain},
		})
		_ = s.eventBus.Publish(ctx, events.SubjectRouteUpdated, evt)
	}

	return ingressRoute, nil
}

// DeleteCustomDomain removes a domain and its corresponding Caddy edge route
func (s *Service) DeleteCustomDomain(ctx context.Context, domainID string) error {
	d, err := s.repo.GetDomainByID(ctx, domainID)
	if err != nil {
		return err
	}

	// 1. Teardown route in Caddy
	routeID := ingress.GenerateRouteID(d.Domain)
	_ = s.ingressDriver.DeleteRoute(ctx, routeID)
	_ = s.repo.DeleteIngressRoute(ctx, d.Domain)

	// 2. Delete database record
	if err := s.repo.DeleteDomain(ctx, d.ID); err != nil {
		return err
	}

	// 3. Emit domain.deleted event
	if s.eventBus != nil {
		evt := events.New(events.SubjectDomainDeleted, "dns-service", events.DomainDeletedPayload{
			DomainID:  d.ID,
			ServiceID: d.ServiceID,
			Domain:    d.Domain,
		})
		_ = s.eventBus.Publish(ctx, events.SubjectDomainDeleted, evt)
	}

	s.logger.Info("Custom domain deleted", slog.String("domain", d.Domain), slog.String("id", domainID))
	return nil
}

// SyncServiceRoutesForDeployment synchronizes Caddy reverse proxy upstreams when a deployment becomes healthy
func (s *Service) SyncServiceRoutesForDeployment(ctx context.Context, serviceID, activeURL string) (*ServiceRouteSyncResult, error) {
	svc, err := s.repo.GetService(ctx, serviceID)
	if err != nil {
		return nil, err
	}

	targetDial := extractUpstreamDial(activeURL)
	if targetDial == "" {
		// Fallback to internal service port
		targetDial = fmt.Sprintf("127.0.0.1:%d", svc.Port)
	}

	result := &ServiceRouteSyncResult{
		ServiceID:    svc.ID,
		UpstreamURL:  targetDial,
		SyncedHosts:  []string{},
		LastSyncedAt: time.Now().UTC(),
	}

	// 1. Synchronize default platform subdomain: <slug>.<platformDomain>
	platformHost := fmt.Sprintf("%s.%s", svc.Slug, s.cfg.PlatformDomain)
	platformRouteID := ingress.GenerateRouteID(platformHost)
	platformSpec := ingress.RouteSpec{
		ID:          platformRouteID,
		HostPattern: platformHost,
		Upstreams:   []ingress.UpstreamTarget{{Dial: targetDial}},
		EnableTLS:   true,
	}

	if err := s.ingressDriver.UpsertRoute(ctx, platformSpec); err != nil {
		s.logger.Error("Failed to upsert platform route", slog.String("host", platformHost), slog.Any("error", err))
		result.FailedHosts = append(result.FailedHosts, platformHost)
	} else {
		_ = s.repo.UpsertIngressRoute(ctx, &types.IngressRoute{
			ServiceID:   svc.ID,
			HostPattern: platformHost,
			UpstreamURL: targetDial,
			IsActive:    true,
		})
		result.SyncedHosts = append(result.SyncedHosts, platformHost)
	}

	// 2. Synchronize all active custom domains for this service
	customDomains, _, err := s.repo.ListDomainsByService(ctx, svc.ID, 100, 0)
	if err == nil {
		for _, d := range customDomains {
			if d.Status != types.DomainStatusActive {
				continue
			}

			cRouteID := ingress.GenerateRouteID(d.Domain)
			cSpec := ingress.RouteSpec{
				ID:          cRouteID,
				HostPattern: d.Domain,
				Upstreams:   []ingress.UpstreamTarget{{Dial: targetDial}},
				EnableTLS:   true,
			}

			if err := s.ingressDriver.UpsertRoute(ctx, cSpec); err != nil {
				s.logger.Error("Failed to upsert custom domain route", slog.String("domain", d.Domain), slog.Any("error", err))
				result.FailedHosts = append(result.FailedHosts, d.Domain)
			} else {
				_ = s.repo.UpsertIngressRoute(ctx, &types.IngressRoute{
					ServiceID:   svc.ID,
					DomainID:    &d.ID,
					HostPattern: d.Domain,
					UpstreamURL: targetDial,
					IsActive:    true,
				})
				result.SyncedHosts = append(result.SyncedHosts, d.Domain)
			}
		}
	}

	// 3. Emit route.updated event
	if s.eventBus != nil {
		evt := events.New(events.SubjectRouteUpdated, "dns-service", events.RouteUpdatedPayload{
			ServiceID:   svc.ID,
			HostPattern: platformHost,
			UpstreamURL: targetDial,
			Domains:     result.SyncedHosts,
		})
		_ = s.eventBus.Publish(ctx, events.SubjectRouteUpdated, evt)
	}

	s.logger.Info("Synchronized ingress routes for service",
		slog.String("service_id", svc.ID),
		slog.String("upstream", targetDial),
		slog.Int("synced_count", len(result.SyncedHosts)),
	)

	return result, nil
}

// ListAllRoutes retrieves all edge routes registered across services
func (s *Service) ListAllRoutes(ctx context.Context, page, perPage int) ([]IngressRouteResponse, int, error) {
	if page < 1 {
		page = 1
	}
	if perPage < 1 || perPage > 100 {
		perPage = 50
	}
	offset := (page - 1) * perPage

	routes, total, err := s.repo.ListAllIngressRoutes(ctx, perPage, offset)
	if err != nil {
		return nil, 0, err
	}

	resp := make([]IngressRouteResponse, len(routes))
	for i, r := range routes {
		resp[i] = IngressRouteResponse{
			ID:           r.ID,
			ServiceID:    r.ServiceID,
			DomainID:     r.DomainID,
			HostPattern:  r.HostPattern,
			UpstreamURL:  r.UpstreamURL,
			IsActive:     r.IsActive,
			LastSyncedAt: r.LastSyncedAt,
		}
	}

	return resp, total, nil
}

// StartEventSubscribers registers background NATS event listeners
func (s *Service) StartEventSubscribers(ctx context.Context) error {
	if s.eventBus == nil {
		return nil
	}

	// 1. Listen for healthy deployments to switch Caddy edge traffic
	err := s.eventBus.Subscribe(events.SubjectDeployHealthy, func(subCtx context.Context, subject string, msg []byte) error {
		var evt events.Event[events.DeployHealthyPayload]
		if err := json.Unmarshal(msg, &evt); err != nil {
			s.logger.Error("Failed to unmarshal deploy.healthy event", slog.Any("error", err))
			return nil
		}

		s.logger.Info("Received deploy.healthy event, synchronizing edge routes",
			slog.String("service_id", evt.Data.ServiceID),
			slog.String("deployment_id", evt.Data.DeploymentID),
			slog.String("active_url", evt.Data.ActiveURL),
		)

		_, syncErr := s.SyncServiceRoutesForDeployment(subCtx, evt.Data.ServiceID, evt.Data.ActiveURL)
		if syncErr != nil {
			s.logger.Error("Failed to synchronize routes for deployment", slog.String("service_id", evt.Data.ServiceID), slog.Any("error", syncErr))
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to subscribe to %s: %w", events.SubjectDeployHealthy, err)
	}

	// 2. Listen for service deletion to cleanup routes and domains
	err = s.eventBus.Subscribe(events.SubjectServiceDeleted, func(subCtx context.Context, subject string, msg []byte) error {
		var evt events.Event[struct {
			ServiceID string `json:"service_id"`
		}]
		if err := json.Unmarshal(msg, &evt); err != nil {
			return nil
		}

		s.logger.Info("Received service.deleted event, cleaning up routes", slog.String("service_id", evt.Data.ServiceID))

		// List and teardown all routes for service
		routes, err := s.repo.ListIngressRoutesByService(subCtx, evt.Data.ServiceID)
		if err == nil {
			for _, r := range routes {
				_ = s.ingressDriver.DeleteRoute(subCtx, ingress.GenerateRouteID(r.HostPattern))
				_ = s.repo.DeleteIngressRoute(subCtx, r.HostPattern)
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to subscribe to %s: %w", events.SubjectServiceDeleted, err)
	}

	return nil
}

// extractUpstreamDial converts a full URL (e.g. "http://127.0.0.1:8080" or "http://node-1:24819") to "host:port"
func extractUpstreamDial(rawURL string) string {
	if rawURL == "" {
		return ""
	}
	if !strings.Contains(rawURL, "://") {
		rawURL = "http://" + rawURL
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	if u.Host != "" {
		return u.Host
	}
	return ""
}
