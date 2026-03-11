// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0

package e2e

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

// decodeJWTClaims decodes the payload of a JWT without verifying the signature.
// This is sufficient for e2e tests where we trust the issuer.
func decodeJWTClaims(token string) (map[string]interface{}, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid JWT: expected 3 parts, got %d", len(parts))
	}

	// Base64url decode the payload (2nd part)
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("failed to decode JWT payload: %w", err)
	}

	var claims map[string]interface{}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, fmt.Errorf("failed to unmarshal JWT claims: %w", err)
	}

	return claims, nil
}

// testClientLifecycle tests the full lifecycle of tenant OAuth2 clients:
// Create Tenant → Create Client → List Clients → Delete Client → Delete Tenant
func testClientLifecycle(t *testing.T, client TenantClient) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	tenantName := fmt.Sprintf("test-client-tenant-%d", time.Now().UnixNano())

	// 1. Create a tenant to associate clients with
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

	defer func() {
		if tenantID != "" {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := client.DeleteTenant(cleanupCtx, tenantID); err != nil {
				t.Logf("warning: failed to delete tenant %s: %v", tenantID, err)
			}
		}
	}()

	// 2. Create Client
	var clientID, clientSecret string
	t.Run("Create Client", func(t *testing.T) {
		id, secret, err := client.CreateTenantClient(ctx, tenantID)
		if err != nil {
			t.Fatalf("failed to create tenant client: %v", err)
		}
		if id == "" {
			t.Fatal("expected client ID, got empty string")
		}
		if secret == "" {
			t.Fatal("expected client secret, got empty string")
		}
		clientID = id
		clientSecret = secret
		_ = clientSecret // secret is only returned on creation
	})

	defer func() {
		if clientID != "" && tenantID != "" {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := client.DeleteTenantClient(cleanupCtx, tenantID, clientID); err != nil {
				t.Logf("warning: failed to delete client %s: %v", clientID, err)
			}
		}
	}()

	// 3. Validate token claims
	t.Run("Client Token Contains Tenant", func(t *testing.T) {
		// Exchange client credentials for an access token from Hydra
		accessToken, err := getJWTToken(ctx, clientID, clientSecret)
		if err != nil {
			t.Fatalf("failed to get access token for client: %v", err)
		}
		if accessToken == "" {
			t.Fatal("expected non-empty access token")
		}

		// Decode the JWT and check the tenants claim
		claims, err := decodeJWTClaims(accessToken)
		if err != nil {
			t.Fatalf("failed to decode access token: %v", err)
		}

		tenants, ok := claims["tenants"].([]interface{})
		if !ok {
			t.Fatalf("expected 'tenants' array in claims: %v", claims)
		}

		found := false
		for _, tid := range tenants {
			if tid == tenantID {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("tenant %s not found in access token tenants claim: %v", tenantID, tenants)
		}
	})

	// 4. List Clients
	t.Run("List Clients", func(t *testing.T) {
		clients, err := client.ListTenantClients(ctx, tenantID)
		if err != nil {
			t.Fatalf("failed to list tenant clients: %v", err)
		}

		found := false
		for _, c := range clients {
			if c.ClientID == clientID {
				found = true
				if c.CreatedAt == "" {
					t.Error("expected non-empty created_at timestamp")
				}
				break
			}
		}
		if !found {
			t.Errorf("created client %s not found in list (got %d clients)", clientID, len(clients))
		}
	})

	// 5. Create a second client to verify multiple clients per tenant
	var secondClientID string
	t.Run("Create Second Client", func(t *testing.T) {
		id, secret, err := client.CreateTenantClient(ctx, tenantID)
		if err != nil {
			t.Fatalf("failed to create second tenant client: %v", err)
		}
		if id == "" {
			t.Fatal("expected second client ID, got empty string")
		}
		if secret == "" {
			t.Fatal("expected second client secret, got empty string")
		}
		if id == clientID {
			t.Error("second client should have a different ID")
		}
		secondClientID = id
	})

	defer func() {
		if secondClientID != "" && tenantID != "" {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := client.DeleteTenantClient(cleanupCtx, tenantID, secondClientID); err != nil {
				t.Logf("warning: failed to delete second client %s: %v", secondClientID, err)
			}
		}
	}()

	// 6. List should now show both clients
	t.Run("List Multiple Clients", func(t *testing.T) {
		clients, err := client.ListTenantClients(ctx, tenantID)
		if err != nil {
			t.Fatalf("failed to list tenant clients: %v", err)
		}

		if len(clients) < 2 {
			t.Fatalf("expected at least 2 clients, got %d", len(clients))
		}

		foundFirst, foundSecond := false, false
		for _, c := range clients {
			if c.ClientID == clientID {
				foundFirst = true
			}
			if c.ClientID == secondClientID {
				foundSecond = true
			}
		}
		if !foundFirst {
			t.Errorf("first client %s not found in list", clientID)
		}
		if !foundSecond {
			t.Errorf("second client %s not found in list", secondClientID)
		}
	})

	// 7. Delete first client
	t.Run("Delete Client", func(t *testing.T) {
		if err := client.DeleteTenantClient(ctx, tenantID, clientID); err != nil {
			t.Fatalf("failed to delete tenant client: %v", err)
		}

		// Verify deletion
		clients, err := client.ListTenantClients(ctx, tenantID)
		if err != nil {
			t.Fatalf("failed to list clients after delete: %v", err)
		}

		for _, c := range clients {
			if c.ClientID == clientID {
				t.Errorf("client %s still exists after deletion", clientID)
			}
		}

		// Second client should still exist
		found := false
		for _, c := range clients {
			if c.ClientID == secondClientID {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("second client %s should still exist after deleting first", secondClientID)
		}

		// Clear so deferred cleanup doesn't try again
		clientID = ""
	})

	// 8. Delete second client
	t.Run("Delete Second Client", func(t *testing.T) {
		if err := client.DeleteTenantClient(ctx, tenantID, secondClientID); err != nil {
			t.Fatalf("failed to delete second tenant client: %v", err)
		}

		// Verify empty list
		clients, err := client.ListTenantClients(ctx, tenantID)
		if err != nil {
			t.Fatalf("failed to list clients after deleting all: %v", err)
		}
		if len(clients) != 0 {
			t.Errorf("expected 0 clients after deleting all, got %d", len(clients))
		}

		secondClientID = ""
	})
}

func TestClientLifecycle(t *testing.T) {
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
			testClientLifecycle(t, client)
		})
	}
}
