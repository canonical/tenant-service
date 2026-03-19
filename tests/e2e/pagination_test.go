// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0

package e2e

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

const (
	// paginationPageSize is intentionally small so we exercise multiple pages and
	// verify that page boundaries don't skip or duplicate rows (off-by-one guard).
	paginationPageSize = 2
	// paginationNumTenants creates an odd number of items so both full and
	// partial (remainder) pages are tested.
	paginationNumTenants = 5
	// paginationNumUsers mirrors the same approach for the user-list endpoint.
	paginationNumUsers = 5
)

// testListTenantsPagination verifies cursor-based pagination for ListTenants:
//   - Each page (except the last) contains exactly paginationPageSize items.
//   - No tenant ID is returned more than once across all pages.
//   - Every tenant created by this test appears exactly once in the results.
//
// This directly guards against the off-by-one bug where
// nextPageToken = results[pageSize].ID instead of results[pageSize-1].ID,
// which causes one item to be silently skipped at every page boundary.
func testListTenantsPagination(t *testing.T, client TenantClient) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Unique name prefix so we can filter our tenants from any pre-existing ones.
	prefix := fmt.Sprintf("e2e-pg-%d-", time.Now().UnixNano())

	// Create N tenants and record their IDs.
	createdIDs := make(map[string]struct{}, paginationNumTenants)
	for i := 0; i < paginationNumTenants; i++ {
		name := fmt.Sprintf("%s%03d", prefix, i+1)
		id, err := client.CreateTenant(ctx, name)
		if err != nil {
			t.Fatalf("setup: CreateTenant(%q): %v", name, err)
		}
		createdIDs[id] = struct{}{}

		t.Cleanup(func() {
			cleanCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := client.DeleteTenant(cleanCtx, id); err != nil {
				t.Logf("cleanup: DeleteTenant(%s): %v", id, err)
			}
		})
	}

	// Walk every page and collect tenant IDs that belong to this test run.
	seen := make(map[string]struct{})
	pageToken := ""
	for pageNum := 1; ; pageNum++ {
		tenants, nextToken, err := client.ListTenantsPaged(ctx, pageToken, paginationPageSize)
		if err != nil {
			t.Fatalf("page %d: ListTenantsPaged(%q, %d): %v", pageNum, pageToken, paginationPageSize, err)
		}

		// A non-final page must be exactly full.
		if nextToken != "" && len(tenants) != paginationPageSize {
			t.Errorf("page %d: got %d item(s) with next_page_token set; want exactly %d",
				pageNum, len(tenants), paginationPageSize)
		}

		for _, tenant := range tenants {
			if !strings.HasPrefix(tenant.Name, prefix) {
				continue // from a different test / pre-existing data
			}
			if _, dup := seen[tenant.ID]; dup {
				t.Errorf("page %d: duplicate tenant ID %s", pageNum, tenant.ID)
			}
			seen[tenant.ID] = struct{}{}
		}

		if nextToken == "" {
			break
		}
		pageToken = nextToken
	}

	// Every created tenant must appear exactly once.
	for id := range createdIDs {
		if _, ok := seen[id]; !ok {
			t.Errorf("tenant %s was created but never returned by pagination", id)
		}
	}
	// Nothing extra from our prefix group.
	for id := range seen {
		if _, ok := createdIDs[id]; !ok {
			t.Errorf("tenant %s appeared in results but was not created by this test", id)
		}
	}
}

// testListTenantUsersPagination verifies cursor-based pagination for ListTenantUsers.
// The tenant-scoped list is simpler: there are no pre-existing rows to filter out,
// so we verify that total count and set membership match exactly.
func testListTenantUsersPagination(t *testing.T, client TenantClient) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Create the tenant that will own the users.
	tenantName := fmt.Sprintf("e2e-user-pg-%d", time.Now().UnixNano())
	tenantID, err := client.CreateTenant(ctx, tenantName)
	if err != nil {
		t.Fatalf("setup: CreateTenant(%q): %v", tenantName, err)
	}
	t.Cleanup(func() {
		cleanCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := client.DeleteTenant(cleanCtx, tenantID); err != nil {
			t.Logf("cleanup: DeleteTenant(%s): %v", tenantID, err)
		}
	})

	// Provision N users with unique e-mail addresses.
	createdEmails := make(map[string]struct{}, paginationNumUsers)
	ts := time.Now().UnixNano()
	for i := 0; i < paginationNumUsers; i++ {
		email := fmt.Sprintf("user%03d-%d@e2e.example.com", i+1, ts)
		role := "member"
		if i == 0 {
			role = "owner"
		}
		if err := client.ProvisionTenantUser(ctx, tenantID, email, role); err != nil {
			t.Fatalf("setup: ProvisionTenantUser(%q, %q): %v", email, role, err)
		}
		createdEmails[email] = struct{}{}
	}

	// Walk every page.
	seen := make(map[string]struct{})
	pageToken := ""
	for pageNum := 1; ; pageNum++ {
		users, nextToken, err := client.ListTenantUsersPaged(ctx, tenantID, pageToken, paginationPageSize)
		if err != nil {
			t.Fatalf("page %d: ListTenantUsersPaged(%q, %q, %d): %v",
				pageNum, tenantID, pageToken, paginationPageSize, err)
		}

		// A non-final page must be exactly full.
		if nextToken != "" && len(users) != paginationPageSize {
			t.Errorf("page %d: got %d item(s) with next_page_token set; want exactly %d",
				pageNum, len(users), paginationPageSize)
		}

		for _, u := range users {
			if _, dup := seen[u.Email]; dup {
				t.Errorf("page %d: duplicate email %s", pageNum, u.Email)
			}
			seen[u.Email] = struct{}{}
		}

		if nextToken == "" {
			break
		}
		pageToken = nextToken
	}

	// Every provisioned email must appear exactly once.
	for email := range createdEmails {
		if _, ok := seen[email]; !ok {
			t.Errorf("user %s was provisioned but never returned by pagination", email)
		}
	}
	if want, got := len(createdEmails), len(seen); want != got {
		t.Errorf("total users: want %d, got %d", want, got)
	}
}

// TestPagination runs the pagination suite over both HTTP and gRPC transports.
func TestPagination(t *testing.T) {
	tests := []struct {
		name       string
		clientFunc func(t *testing.T) TenantClient
	}{
		{
			name: "HTTP",
			clientFunc: func(t *testing.T) TenantClient {
				baseURL := os.Getenv("HTTP_BASE_URL")
				if baseURL == "" {
					if testEnv != nil {
						baseURL = testEnv.BaseURL
					} else {
						baseURL = defaultBaseURL
					}
				}
				c, err := NewHTTPTenantClient(baseURL)
				if err != nil {
					t.Fatalf("failed to create HTTP client: %v", err)
				}
				return c
			},
		},
		{
			name: "gRPC",
			clientFunc: func(t *testing.T) TenantClient {
				c, err := NewGRPCTenantClient(getGRPCAddress())
				if err != nil {
					t.Fatalf("failed to create gRPC client: %v", err)
				}
				return c
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := tt.clientFunc(t)
			defer client.Close()

			t.Run("ListTenants", func(t *testing.T) {
				testListTenantsPagination(t, client)
			})
			t.Run("ListTenantUsers", func(t *testing.T) {
				testListTenantUsersPagination(t, client)
			})
		})
	}
}
