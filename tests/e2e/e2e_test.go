// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0

package e2e

import (
	"context"
	"io"
	"net/http"
	"os"
	"testing"
	"time"

	httpclient "github.com/canonical/tenant-service/client/http"
)

// authRequestEditor returns a RequestEditorFn that adds Bearer token authentication
func authRequestEditor(ctx context.Context) httpclient.RequestEditorFn {
	return func(ctx context.Context, req *http.Request) error {
		token, err := getAuthToken(ctx)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+token)
		return nil
	}
}

// newTestClient creates a new HTTP client configured for the test environment
func newTestClient(t *testing.T) *httpclient.Client {
	baseURL := os.Getenv("HTTP_BASE_URL")
	if baseURL == "" {
		if testEnv != nil {
			baseURL = testEnv.BaseURL
		} else {
			baseURL = defaultBaseURL
		}
	}

	client, err := httpclient.NewClient(baseURL, httpclient.WithHTTPClient(&http.Client{
		Timeout: 10 * time.Second,
	}))
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	return client
}

// TestHTTPAuthentication tests that HTTP endpoints require authentication
func TestHTTPAuthentication(t *testing.T) {
	baseURL := os.Getenv("HTTP_BASE_URL")
	if baseURL == "" {
		if testEnv != nil {
			baseURL = testEnv.BaseURL
		} else {
			baseURL = defaultBaseURL
		}
	}

	client, err := httpclient.NewClient(baseURL, httpclient.WithHTTPClient(&http.Client{
		Timeout: 10 * time.Second,
	}))
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	ctx := context.Background()

	t.Run("Request Without Auth Should Fail", func(t *testing.T) {
		// Try to list tenants without authentication
		resp, err := client.TenantServiceListTenants(ctx)
		if err != nil {
			// Connection error is acceptable
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			t.Error("expected non-OK status when calling without authentication")
		}
	})

	t.Run("Request With Valid Auth Should Succeed", func(t *testing.T) {
		authEditor := authRequestEditor(ctx)
		resp, err := client.TenantServiceListTenants(ctx, authEditor)
		if err != nil {
			t.Fatalf("expected success with valid auth, got error: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			t.Errorf("expected status OK with valid auth, got %d: %s", resp.StatusCode, string(body))
		}
	})
}
