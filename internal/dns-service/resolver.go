package dnsservice

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/dev-nathaniel/Adroit-platform/pkg/types"
)

// DNSResolver abstracts network DNS resolution for verification challenges
type DNSResolver interface {
	LookupCNAME(ctx context.Context, host string) (string, error)
	LookupTXT(ctx context.Context, name string) ([]string, error)
}

// SystemDNSResolver resolves DNS records using the local operating system resolver
type SystemDNSResolver struct {
	resolver *net.Resolver
}

// NewSystemDNSResolver initializes a standard system DNS resolver
func NewSystemDNSResolver() *SystemDNSResolver {
	return &SystemDNSResolver{
		resolver: net.DefaultResolver,
	}
}

func (s *SystemDNSResolver) LookupCNAME(ctx context.Context, host string) (string, error) {
	return s.resolver.LookupCNAME(ctx, host)
}

func (s *SystemDNSResolver) LookupTXT(ctx context.Context, name string) ([]string, error) {
	return s.resolver.LookupTXT(ctx, name)
}

// MockDNSResolver enables hermetic testing and local simulation of DNS records
type MockDNSResolver struct {
	cnameRecords map[string]string
	txtRecords   map[string][]string
}

// NewMockDNSResolver creates a mock resolver pre-populated with test records
func NewMockDNSResolver() *MockDNSResolver {
	return &MockDNSResolver{
		cnameRecords: make(map[string]string),
		txtRecords:   make(map[string][]string),
	}
}

func (m *MockDNSResolver) SetCNAME(host, target string) {
	m.cnameRecords[strings.ToLower(strings.TrimRight(host, "."))] = strings.ToLower(strings.TrimRight(target, "."))
}

func (m *MockDNSResolver) SetTXT(name string, values []string) {
	m.txtRecords[strings.ToLower(strings.TrimRight(name, "."))] = values
}

func (m *MockDNSResolver) LookupCNAME(_ context.Context, host string) (string, error) {
	clean := strings.ToLower(strings.TrimRight(host, "."))
	if target, ok := m.cnameRecords[clean]; ok {
		return target, nil
	}
	return "", &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}
}

func (m *MockDNSResolver) LookupTXT(_ context.Context, name string) ([]string, error) {
	clean := strings.ToLower(strings.TrimRight(name, "."))
	if records, ok := m.txtRecords[clean]; ok {
		return records, nil
	}
	return nil, &net.DNSError{Err: "no such host", Name: name, IsNotFound: true}
}

// GenerateChallengeDetails builds structured instructions for configuring domain DNS
func GenerateChallengeDetails(domain string, verificationType types.VerificationType, token, canonicalCNAME string) types.DNSChallengeDetails {
	cleanDomain := strings.ToLower(strings.TrimSpace(domain))
	cleanCNAME := strings.ToLower(strings.TrimSpace(canonicalCNAME))
	if cleanCNAME == "" {
		cleanCNAME = "cname.adroit.run"
	}

	if verificationType == types.VerificationTypeTXT {
		recordName := fmt.Sprintf("_adroit-challenge.%s", cleanDomain)
		return types.DNSChallengeDetails{
			Domain:            cleanDomain,
			VerificationType:  types.VerificationTypeTXT,
			TargetHost:        token,
			RecordName:        recordName,
			VerificationToken: token,
			Status:            types.DomainStatusPendingVerification,
			Instructions:      fmt.Sprintf("Add a TXT record for '%s' with value '%s'.", recordName, token),
		}
	}

	// Default to CNAME
	return types.DNSChallengeDetails{
		Domain:           cleanDomain,
		VerificationType: types.VerificationTypeCNAME,
		TargetHost:       cleanCNAME,
		RecordName:       cleanDomain,
		Status:           types.DomainStatusPendingVerification,
		Instructions:     fmt.Sprintf("Add a CNAME record for '%s' pointing to '%s'.", cleanDomain, cleanCNAME),
	}
}

// CheckDNSChallenge validates whether the DNS records on the internet match the challenge requirements
func CheckDNSChallenge(ctx context.Context, resolver DNSResolver, domain string, vType types.VerificationType, token, canonicalCNAME string) (bool, string, error) {
	cleanDomain := strings.ToLower(strings.TrimSpace(domain))
	cleanCNAME := strings.ToLower(strings.TrimRight(strings.TrimSpace(canonicalCNAME), "."))
	if cleanCNAME == "" {
		cleanCNAME = "cname.adroit.run"
	}

	// 1. Dev / Local environment bypass
	if strings.HasSuffix(cleanDomain, ".localhost") ||
		strings.HasSuffix(cleanDomain, ".test") ||
		strings.HasSuffix(cleanDomain, ".local") ||
		cleanDomain == "localhost" {
		return true, "Local development domain automatically verified", nil
	}

	// 2. Perform verification with timeout
	ctxWithTimeout, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if vType == types.VerificationTypeTXT {
		recordName := fmt.Sprintf("_adroit-challenge.%s", cleanDomain)
		txts, err := resolver.LookupTXT(ctxWithTimeout, recordName)
		if err != nil {
			return false, fmt.Sprintf("DNS lookup failed for TXT record '%s': %v", recordName, err), nil
		}

		for _, txt := range txts {
			if strings.TrimSpace(txt) == token {
				return true, "TXT challenge verification succeeded", nil
			}
		}

		return false, fmt.Sprintf("TXT record found for '%s', but none matched token '%s'", recordName, token), nil
	}

	// 3. CNAME Verification
	cnameTarget, err := resolver.LookupCNAME(ctxWithTimeout, cleanDomain)
	if err != nil {
		return false, fmt.Sprintf("DNS lookup failed for CNAME record '%s': %v", cleanDomain, err), nil
	}

	cleanTarget := strings.ToLower(strings.TrimRight(strings.TrimSpace(cnameTarget), "."))
	if cleanTarget == cleanCNAME {
		return true, fmt.Sprintf("CNAME points correctly to %s", cleanCNAME), nil
	}

	return false, fmt.Sprintf("CNAME points to '%s', expected '%s'", cleanTarget, cleanCNAME), nil
}
