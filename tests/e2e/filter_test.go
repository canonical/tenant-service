// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0

package e2e

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"
)

// TestFilters_HTTP runs all filter test scenarios against the HTTP/REST client.
func TestFilters_HTTP(t *testing.T) {
	baseURL := os.Getenv("HTTP_BASE_URL")
	if baseURL == "" {
		if testEnv != nil {
			baseURL = testEnv.BaseURL
		} else {
			baseURL = defaultBaseURL
		}
	}

	client, err := NewHTTPTenantClient(baseURL)
	if err != nil {
		t.Fatalf("failed to create HTTP client: %v", err)
	}
	defer client.Close()

	t.Run("TenantFilter/Enabled", func(t *testing.T) { testFilterByEnabled(t, client) })
	t.Run("TenantUserFilter/Role", func(t *testing.T) { testFilterUsersByRole(t, client) })
	t.Run("TenantUserFilter/Email", func(t *testing.T) { testFilterUsersByEmail(t, client) })
	t.Run("TenantUserFilter/IdentityID", func(t *testing.T) { testFilterUsersByIdentityID(t, client) })
}

// TestFilters_GRPC runs the same filter test scenarios against the gRPC client.
func TestFilters_GRPC(t *testing.T) {
	client, err := NewGRPCTenantClient(getGRPCAddress())
	if err != nil {
		t.Fatalf("failed to create gRPC client: %v", err)
	}
	defer client.Close()

	t.Run("TenantFilter/Enabled", func(t *testing.T) { testFilterByEnabled(t, client) })
	t.Run("TenantUserFilter/Role", func(t *testing.T) { testFilterUsersByRole(t, client) })
	t.Run("TenantUserFilter/Email", func(t *testing.T) { testFilterUsersByEmail(t, client) })
	t.Run("TenantUserFilter/IdentityID", func(t *testing.T) { testFilterUsersByIdentityID(t, client) })
}

// testFilterByEnabled verifies that filter.enabled=true/false works correctly:
//   - With enabled=true, only enabled tenants are returned.
//   - With enabled=false, only disabled tenants are returned.
//   - The two result sets are disjoint.
func testFilterByEnabled(t *testing.T, client TenantClient) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	prefix := fmt.Sprintf("e2e-filter-enabled-%d-", time.Now().UnixNano())

	// Create an enabled tenant (all new tenants are enabled by default).
	enabledName := prefix + "enabled"
	enabledID, err := client.CreateTenant(ctx, enabledName)
	if err != nil {
		t.Fatalf("CreateTenant(%q): %v", enabledName, err)
	}
	t.Cleanup(func() {
		cleanCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = client.DeleteTenant(cleanCtx, enabledID)
	})

	// Create a second tenant and then disable it via UpdateTenant (sets enabled=false).
	disabledName := prefix + "disabled"
	disabledID, err := client.CreateTenant(ctx, disabledName)
	if err != nil {
		t.Fatalf("CreateTenant(%q): %v", disabledName, err)
	}
	t.Cleanup(func() {
		cleanCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = client.DeleteTenant(cleanCtx, disabledID)
	})

	// Disable the second tenant.
	if err := client.DisableTenant(ctx, disabledID); err != nil {
		t.Fatalf("DisableTenant(%s): %v", disabledID, err)
	}

	trueVal := true
	falseVal := false

	// filter.enabled=true must include the enabled tenant, not the disabled one.
	enabledTenants, err := client.ListTenantsFiltered(ctx, TenantFilterOptions{Enabled: &trueVal})
	if err != nil {
		t.Fatalf("ListTenantsFiltered(enabled=true): %v", err)
	}
	assertContainsTenant(t, enabledTenants, enabledID, "enabled=true results must include enabled tenant")
	assertNotContainsTenant(t, enabledTenants, disabledID, "enabled=true results must not include disabled tenant")

	// filter.enabled=false must include the disabled tenant, not the enabled one.
	disabledTenants, err := client.ListTenantsFiltered(ctx, TenantFilterOptions{Enabled: &falseVal})
	if err != nil {
		t.Fatalf("ListTenantsFiltered(enabled=false): %v", err)
	}
	assertContainsTenant(t, disabledTenants, disabledID, "enabled=false results must include disabled tenant")
	assertNotContainsTenant(t, disabledTenants, enabledID, "enabled=false results must not include enabled tenant")
}

// testFilterUsersByRole verifies that filter.role restricts ListTenantUsers results
// to only members with the matching role.
func testFilterUsersByRole(t *testing.T, client TenantClient) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	tenantName := fmt.Sprintf("e2e-filter-role-%d", time.Now().UnixNano())
	tenantID, err := client.CreateTenant(ctx, tenantName)
	if err != nil {
		t.Fatalf("CreateTenant(%q): %v", tenantName, err)
	}
	t.Cleanup(func() {
		cleanCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = client.DeleteTenant(cleanCtx, tenantID)
	})

	ownerEmail := fmt.Sprintf("e2e-owner-%d@example.com", time.Now().UnixNano())
	memberEmail := fmt.Sprintf("e2e-member-%d@example.com", time.Now().UnixNano())

	if err := client.ProvisionTenantUser(ctx, tenantID, ownerEmail, "owner"); err != nil {
		t.Fatalf("ProvisionTenantUser(owner): %v", err)
	}
	if err := client.ProvisionTenantUser(ctx, tenantID, memberEmail, "member"); err != nil {
		t.Fatalf("ProvisionTenantUser(member): %v", err)
	}

	// Filter by role=owner: only the owner must appear.
	owners, err := client.ListTenantUsersFiltered(ctx, tenantID, TenantUserFilterOptions{Role: "owner"})
	if err != nil {
		t.Fatalf("ListTenantUsersFiltered(role=owner): %v", err)
	}
	assertContainsUser(t, owners, ownerEmail, "role=owner must include the owner")
	assertNotContainsUser(t, owners, memberEmail, "role=owner must not include the member")

	// Filter by role=member: only the member must appear.
	members, err := client.ListTenantUsersFiltered(ctx, tenantID, TenantUserFilterOptions{Role: "member"})
	if err != nil {
		t.Fatalf("ListTenantUsersFiltered(role=member): %v", err)
	}
	assertContainsUser(t, members, memberEmail, "role=member must include the member")
	assertNotContainsUser(t, members, ownerEmail, "role=member must not include the owner")
}

// testFilterUsersByEmail verifies that filter.email restricts ListTenantUsers to
// the single member whose email matches.
func testFilterUsersByEmail(t *testing.T, client TenantClient) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	tenantName := fmt.Sprintf("e2e-filter-email-%d", time.Now().UnixNano())
	tenantID, err := client.CreateTenant(ctx, tenantName)
	if err != nil {
		t.Fatalf("CreateTenant(%q): %v", tenantName, err)
	}
	t.Cleanup(func() {
		cleanCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = client.DeleteTenant(cleanCtx, tenantID)
	})

	now := time.Now().UnixNano()
	aliceEmail := fmt.Sprintf("e2e-alice-%d@example.com", now)
	bobEmail := fmt.Sprintf("e2e-bob-%d@example.com", now)

	if err := client.ProvisionTenantUser(ctx, tenantID, aliceEmail, "member"); err != nil {
		t.Fatalf("ProvisionTenantUser(alice): %v", err)
	}
	if err := client.ProvisionTenantUser(ctx, tenantID, bobEmail, "member"); err != nil {
		t.Fatalf("ProvisionTenantUser(bob): %v", err)
	}

	// filter.email=alice: only alice must be returned.
	users, err := client.ListTenantUsersFiltered(ctx, tenantID, TenantUserFilterOptions{Email: aliceEmail})
	if err != nil {
		t.Fatalf("ListTenantUsersFiltered(email=alice): %v", err)
	}
	if len(users) != 1 {
		t.Errorf("expected exactly 1 user, got %d", len(users))
	}
	assertContainsUser(t, users, aliceEmail, "email filter must return alice")
	assertNotContainsUser(t, users, bobEmail, "email filter must not return bob")

	// filter.email for an unknown address must return an empty list (not an error).
	unknown := fmt.Sprintf("e2e-nobody-%d@example.com", now)
	none, err := client.ListTenantUsersFiltered(ctx, tenantID, TenantUserFilterOptions{Email: unknown})
	if err != nil {
		t.Fatalf("ListTenantUsersFiltered(unknown email) returned error: %v", err)
	}
	if len(none) != 0 {
		t.Errorf("expected empty result for unknown email, got %d users", len(none))
	}
}

// testFilterUsersByIdentityID verifies that filter.identity_id restricts
// ListTenantUsers to the single member whose Kratos identity ID matches.
func testFilterUsersByIdentityID(t *testing.T, client TenantClient) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	tenantName := fmt.Sprintf("e2e-filter-identityid-%d", time.Now().UnixNano())
	tenantID, err := client.CreateTenant(ctx, tenantName)
	if err != nil {
		t.Fatalf("CreateTenant(%q): %v", tenantName, err)
	}
	t.Cleanup(func() {
		cleanCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = client.DeleteTenant(cleanCtx, tenantID)
	})

	now := time.Now().UnixNano()
	aliceEmail := fmt.Sprintf("e2e-alice-id-%d@example.com", now)
	bobEmail := fmt.Sprintf("e2e-bob-id-%d@example.com", now)

	if err := client.ProvisionTenantUser(ctx, tenantID, aliceEmail, "member"); err != nil {
		t.Fatalf("ProvisionTenantUser(alice): %v", err)
	}
	if err := client.ProvisionTenantUser(ctx, tenantID, bobEmail, "member"); err != nil {
		t.Fatalf("ProvisionTenantUser(bob): %v", err)
	}

	// List all users to resolve Alice's identity ID.
	all, err := client.ListTenantUsersFiltered(ctx, tenantID, TenantUserFilterOptions{})
	if err != nil {
		t.Fatalf("ListTenantUsersFiltered (all): %v", err)
	}
	var aliceID string
	for _, u := range all {
		if u.Email == aliceEmail {
			aliceID = u.UserID
			break
		}
	}
	if aliceID == "" {
		t.Fatalf("could not find alice's identity ID after provisioning")
	}

	// Filter by alice's identity_id: only alice must appear.
	filtered, err := client.ListTenantUsersFiltered(ctx, tenantID, TenantUserFilterOptions{IdentityID: aliceID})
	if err != nil {
		t.Fatalf("ListTenantUsersFiltered(identity_id=alice): %v", err)
	}
	if len(filtered) != 1 {
		t.Errorf("expected exactly 1 user, got %d", len(filtered))
	}
	assertContainsUser(t, filtered, aliceEmail, "identity_id filter must return alice")
	assertNotContainsUser(t, filtered, bobEmail, "identity_id filter must not return bob")
}

// ---- assertion helpers ----

func assertContainsTenant(t *testing.T, tenants []Tenant, id, msg string) {
	t.Helper()
	for _, tenant := range tenants {
		if tenant.ID == id {
			return
		}
	}
	t.Errorf("%s: tenant %s not found in results", msg, id)
}

func assertNotContainsTenant(t *testing.T, tenants []Tenant, id, msg string) {
	t.Helper()
	for _, tenant := range tenants {
		if tenant.ID == id {
			t.Errorf("%s: tenant %s unexpectedly found in results", msg, id)
			return
		}
	}
}

func assertContainsUser(t *testing.T, users []TenantUser, email, msg string) {
	t.Helper()
	for _, u := range users {
		if u.Email == email {
			return
		}
	}
	t.Errorf("%s: user %s not found in results", msg, email)
}

func assertNotContainsUser(t *testing.T, users []TenantUser, email, msg string) {
	t.Helper()
	for _, u := range users {
		if u.Email == email {
			t.Errorf("%s: user %s unexpectedly found in results", msg, email)
			return
		}
	}
}
