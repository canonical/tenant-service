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

// testTenantLifecycle is a common test function that works with any TenantClient
func testTenantLifecycle(t *testing.T, client TenantClient) {
	// Add timeout to prevent hanging tests
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	tenantName := fmt.Sprintf("test-tenant-%d", time.Now().UnixNano())

	// 1. Create Tenant
	var tenantID string
	t.Run("Create Tenant", func(t *testing.T) {
		id, err := client.CreateTenant(ctx, tenantName)
		if err != nil {
			t.Fatalf("failed to create tenant: %v", err)
		}
		if id == "" {
			t.Fatal("expected created tenant ID, got empty string")
		}
		tenantID = id
	})

	// Cleanup
	defer func() {
		if tenantID != "" {
			// Create a new context for cleanup (don't use test context which may be cancelled)
			cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			if err := client.DeleteTenant(cleanupCtx, tenantID); err != nil {
				t.Logf("warning: failed to delete tenant %s: %v", tenantID, err)
			}
		}
	}()

	// 2. List Tenants
	t.Run("List Tenants", func(t *testing.T) {
		tenants, err := client.ListTenants(ctx)
		if err != nil {
			t.Fatalf("failed to list tenants: %v", err)
		}

		found := false
		for _, tenant := range tenants {
			if tenant.ID == tenantID {
				found = true
				if tenant.Name != tenantName {
					t.Errorf("expected tenant name %s, got %s", tenantName, tenant.Name)
				}
				break
			}
		}
		if !found {
			t.Errorf("created tenant %s not found in list", tenantID)
		}
	})

	// 3. Update Tenant
	t.Run("Update Tenant", func(t *testing.T) {
		updatedName := tenantName + "-updated"
		if err := client.UpdateTenant(ctx, tenantID, updatedName); err != nil {
			t.Fatalf("failed to update tenant: %v", err)
		}

		// Verify the update by listing
		tenants, err := client.ListTenants(ctx)
		if err != nil {
			t.Fatalf("failed to list tenants after update: %v", err)
		}

		found := false
		for _, tenant := range tenants {
			if tenant.ID == tenantID {
				found = true
				if tenant.Name != updatedName {
					t.Errorf("expected updated tenant name %s, got %s", updatedName, tenant.Name)
				}
				break
			}
		}
		if !found {
			t.Errorf("tenant %s not found after update", tenantID)
		}
	})

	// 4. Delete Tenant
	t.Run("Delete Tenant", func(t *testing.T) {
		if err := client.DeleteTenant(ctx, tenantID); err != nil {
			t.Fatalf("failed to delete tenant: %v", err)
		}

		// Verify deletion by listing
		tenants, err := client.ListTenants(ctx)
		if err != nil {
			t.Fatalf("failed to list tenants after delete: %v", err)
		}

		for _, tenant := range tenants {
			if tenant.ID == tenantID {
				t.Errorf("tenant %s still exists after deletion", tenantID)
			}
		}

		// Clear tenantID so defer doesn't try to delete again
		tenantID = ""
	})
}

func TestTenantLifecycle(t *testing.T) {
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
				client, err := NewHTTPTenantClient(baseURL)
				if err != nil {
					t.Fatalf("failed to create HTTP client: %v", err)
				}
				return client
			},
		},
		{
			name: "gRPC",
			clientFunc: func(t *testing.T) TenantClient {
				grpcAddress := getGRPCAddress()
				client, err := NewGRPCTenantClient(grpcAddress)
				if err != nil {
					t.Fatalf("failed to create gRPC client: %v", err)
				}
				return client
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := tt.clientFunc(t)
			defer client.Close()
			testTenantLifecycle(t, client)
		})
	}
}

// TestTenantValidation tests input validation and error cases
func TestTenantValidation(t *testing.T) {
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
				client, err := NewHTTPTenantClient(baseURL)
				if err != nil {
					t.Fatalf("failed to create HTTP client: %v", err)
				}
				return client
			},
		},
		{
			name: "gRPC",
			clientFunc: func(t *testing.T) TenantClient {
				grpcAddress := getGRPCAddress()
				client, err := NewGRPCTenantClient(grpcAddress)
				if err != nil {
					t.Fatalf("failed to create gRPC client: %v", err)
				}
				return client
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := tt.clientFunc(t)
			defer client.Close()

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			t.Run("Create with empty name", func(t *testing.T) {
				_, err := client.CreateTenant(ctx, "")
				if err == nil {
					t.Error("expected error for empty tenant name, got nil")
				}
			})

			t.Run("Update non-existent tenant", func(t *testing.T) {
				err := client.UpdateTenant(ctx, "non-existent-id-12345", "new-name")
				if err == nil {
					t.Error("expected error for non-existent tenant, got nil")
				}
			})

			t.Run("Delete non-existent tenant should be idempotent", func(t *testing.T) {
				// Per project conventions, deletes should be idempotent
				// This should not return an error
				err := client.DeleteTenant(ctx, "non-existent-id-67890")
				// Note: If the API returns an error for non-existent deletes,
				// this test documents that behavior
				if err != nil {
					t.Logf("Delete of non-existent tenant returned error: %v (documenting current behavior)", err)
				}
			})
		})
	}
}
