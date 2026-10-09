// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package tenant

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/canonical/tenant-service/internal/types"
)

const (
	// maxDomainLength is RFC 1035's limit on a name.
	maxDomainLength = 253
	maxBindings     = 5
	maxDomains      = 50
)

// domainPattern is a name of lower-case labels whose top-level label is at
// least two letters, or an internationalised one in its ASCII form.
var domainPattern = regexp.MustCompile(`^([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)+([a-z]{2,63}|xn--[a-z0-9-]{1,58}[a-z0-9])$`)

// validateBindings checks the bindings of one policy.
func validateBindings(bindings []types.SSOBinding) error {
	if len(bindings) > maxBindings {
		return fmt.Errorf("%w: at most %d bindings", ErrInvalidSSOPolicy, maxBindings)
	}
	seen := make(map[string]struct{}, len(bindings))
	for _, b := range bindings {
		if _, dup := seen[b.ConnectionID]; dup {
			return fmt.Errorf("%w: connection %s is bound twice", ErrInvalidSSOPolicy, b.ConnectionID)
		}
		seen[b.ConnectionID] = struct{}{}
	}

	return nil
}

// normaliseDomains returns the domains of one policy trimmed, lower-cased,
// without the root's trailing dot, de-duplicated and sorted.
func normaliseDomains(domains []string) ([]string, error) {
	normalised := make([]string, 0, len(domains))
	for _, raw := range domains {
		domain := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(raw)), ".")
		if len(domain) > maxDomainLength || !domainPattern.MatchString(domain) {
			return nil, fmt.Errorf("%w: %q is not a domain", ErrInvalidSSOPolicy, domain)
		}
		if !slices.Contains(normalised, domain) {
			normalised = append(normalised, domain)
		}
	}
	if len(normalised) > maxDomains {
		return nil, fmt.Errorf("%w: at most %d domains", ErrInvalidSSOPolicy, maxDomains)
	}
	slices.Sort(normalised)

	return normalised, nil
}

// emailDomain returns the lower-cased domain of an address, or "" when it has
// no single "@" with something on both sides.
func emailDomain(email string) string {
	at := strings.LastIndexByte(email, '@')
	if at <= 0 || at == len(email)-1 || strings.IndexByte(email, '@') != at {
		return ""
	}

	return strings.ToLower(strings.TrimSpace(email[at+1:]))
}
