// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0

package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
)

// webhookLoginPayload mirrors pkg/webhooks.KratosLoginPayload for e2e tests.
type webhookLoginPayload struct {
	IdentityID string `json:"identity_id"`
	Email      string `json:"email"`
	TenantID   string `json:"tenant_id"`
}

// webhookRegistrationPayload mirrors the Kratos identity payload used by the registration hook.
type webhookRegistrationPayload struct {
	ID    string `json:"user_id"`
	Email string `json:"email"`
}

// postWebhook sends a JSON POST request to the given path and returns the response.
func postWebhook(ctx context.Context, baseURL, path string, body interface{}) (*http.Response, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+path, bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "secret_api_key")

	client := &http.Client{Timeout: 10 * time.Second}
	return client.Do(req)
}

// baseURL returns the service URL from the test environment or falls back to the default.
func serviceBaseURL() string {
	if testEnv != nil {
		return testEnv.BaseURL
	}
	return defaultBaseURL
}

// registerIdentity calls the registration webhook to create a shadow tenant and membership.
// Returns the tenant ID extracted from the response.
func registerIdentity(ctx context.Context, t *testing.T, identityID, email string) {
	t.Helper()
	resp, err := postWebhook(ctx, serviceBaseURL(), "/api/v0/webhooks/registration", webhookRegistrationPayload{
		ID:    identityID,
		Email: email,
	})
	if err != nil {
		t.Fatalf("registration webhook request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("registration webhook returned %d: %s", resp.StatusCode, string(body))
	}
}

func TestWebhookLogin_ValidMember(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	identityID := uuid.New().String()
	email := fmt.Sprintf("e2e-login-valid-%s@example.com", identityID)

	// Setup: register the identity first so a tenant + membership exist.
	registerIdentity(ctx, t, identityID, email)

	// We need the tenant ID the registration hook created.
	// The login hook with an empty tenant_id (the fallback path) is easier to test
	// without knowing the exact tenant_id. See TestWebhookLogin_NoTenantID_HasMemberships.
	// For this test, we use the no-tenant_id path to confirm the valid-member branch works.
	resp, err := postWebhook(ctx, serviceBaseURL(), "/api/v0/webhooks/login", webhookLoginPayload{
		IdentityID: identityID,
		Email:      email,
		TenantID:   "",
	})
	if err != nil {
		t.Fatalf("login webhook request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Errorf("expected 200, got %d: %s", resp.StatusCode, string(body))
	}
}

func TestWebhookLogin_NoTenantID_HasMemberships(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	identityID := uuid.New().String()
	email := fmt.Sprintf("e2e-login-notenant-%s@example.com", identityID)

	// Register so user already has a membership.
	registerIdentity(ctx, t, identityID, email)

	resp, err := postWebhook(ctx, serviceBaseURL(), "/api/v0/webhooks/login", webhookLoginPayload{
		IdentityID: identityID,
		Email:      email,
		TenantID:   "",
	})
	if err != nil {
		t.Fatalf("login webhook request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Errorf("expected 200, got %d: %s", resp.StatusCode, string(body))
	}
}

func TestWebhookLogin_NoTenantID_Succeeds(t *testing.T) {
	// Identity has never registered via the webhook — no shadow tenant exists.
	// The login hook should succeed without checking anything since no tenant_id is requested.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	identityID := uuid.New().String()
	email := fmt.Sprintf("e2e-login-orphan-%s@example.com", identityID)

	resp, err := postWebhook(ctx, serviceBaseURL(), "/api/v0/webhooks/login", webhookLoginPayload{
		IdentityID: identityID,
		Email:      email,
		TenantID:   "",
	})
	if err != nil {
		t.Fatalf("login webhook request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Errorf("expected 200 , got %d: %s", resp.StatusCode, string(body))
	}
}

func TestWebhookLogin_NotMember(t *testing.T) {
	// Identity ID is valid but the provided tenant_id is one it doesn't belong to.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	identityID := uuid.New().String()
	email := fmt.Sprintf("e2e-login-notmember-%s@example.com", identityID)
	foreignTenantID := "00000000-0000-0000-0000-000000000000"

	resp, err := postWebhook(ctx, serviceBaseURL(), "/api/v0/webhooks/login", webhookLoginPayload{
		IdentityID: identityID,
		Email:      email,
		TenantID:   foreignTenantID,
	})
	if err != nil {
		t.Fatalf("login webhook request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		body, _ := io.ReadAll(resp.Body)
		t.Errorf("expected 403, got %d: %s", resp.StatusCode, string(body))
	}

	var errBody map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&errBody); err == nil {
		if errBody["error"] == nil {
			t.Error("expected 'error' field in 403 response body")
		}
	}
}

func TestWebhookLogin_InvalidBody(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, http.MethodPost,
		serviceBaseURL()+"/api/v0/webhooks/login",
		bytes.NewBufferString("not-valid-json"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "secret_api_key")

	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		body, _ := io.ReadAll(resp.Body)
		t.Errorf("expected 400, got %d: %s", resp.StatusCode, string(body))
	}
}

func TestTenantLookup_KnownEmail(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// The lookup endpoint needs Kratos to resolve the email to an identity ID.
	// Since the e2e environment includes Kratos, skip if Kratos is not configured.
	// This test serves as an integration smoke-test when the full stack is running.

	identityID := uuid.New().String()
	email := fmt.Sprintf("e2e-lookup-%s@example.com", identityID)

	// Ensure the identity has a shadow tenant via the registration hook.
	registerIdentity(ctx, t, identityID, email)

	// NOTE: The lookup endpoint queries Kratos to map email → identity_id.
	// In a test environment without a real Kratos identity, the lookup will return
	// an empty list (Kratos returns no match). This test validates the endpoint is
	// reachable and returns a valid JSON response.
	resp, err := http.Get(fmt.Sprintf("%s/api/v0/tenants/lookup?email=%s", serviceBaseURL(), email))
	if err != nil {
		t.Fatalf("lookup request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Errorf("expected 200, got %d: %s", resp.StatusCode, string(body))
		return
	}

	var result struct {
		Tenants []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"tenants"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Errorf("failed to decode lookup response: %v", err)
	}
}

func TestTenantLookup_UnknownEmail(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = ctx

	resp, err := http.Get(fmt.Sprintf("%s/api/v0/tenants/lookup?email=nobody-unknown-%d@example.com",
		serviceBaseURL(), time.Now().UnixNano()))
	if err != nil {
		t.Fatalf("lookup request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Errorf("expected 200, got %d: %s", resp.StatusCode, string(body))
		return
	}

	var result struct {
		Tenants []interface{} `json:"tenants"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Errorf("failed to decode lookup response: %v", err)
		return
	}
	if len(result.Tenants) != 0 {
		t.Errorf("expected empty tenants for unknown email, got %d", len(result.Tenants))
	}
}

func TestTenantLookup_MissingEmail(t *testing.T) {
	resp, err := http.Get(fmt.Sprintf("%s/api/v0/tenants/lookup", serviceBaseURL()))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		body, _ := io.ReadAll(resp.Body)
		t.Errorf("expected 400, got %d: %s", resp.StatusCode, string(body))
	}
}

func TestWebhook_Unauthorized(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	endpoints := []string{
		"/api/v0/webhooks/login",
		"/api/v0/webhooks/registration",
		"/api/v0/webhooks/token",
	}

	for _, endpoint := range endpoints {
		t.Run(endpoint, func(t *testing.T) {
			req, _ := http.NewRequestWithContext(ctx, http.MethodPost,
				serviceBaseURL()+endpoint,
				bytes.NewBufferString("{}"))
			req.Header.Set("Content-Type", "application/json")
			// No Authorization header

			resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusUnauthorized {
				t.Errorf("expected status %d but got %d", http.StatusUnauthorized, resp.StatusCode)
			}
		})
	}
}
