// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package kratos

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/canonical/tenant-service/internal/logging"
	"github.com/canonical/tenant-service/internal/monitoring"
	"github.com/canonical/tenant-service/internal/tracing"
)

func TestCreateIdentityUsesKratosDefaultSchema(t *testing.T) {
	tests := []struct {
		name  string
		email string
	}{
		{name: "invited address", email: "invitee@example.com"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var body map[string]any
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || r.URL.Path != "/admin/identities" {
					t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Errorf("cannot decode the request body: %v", err)
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusCreated)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"id":         "identity-id",
					"schema_id":  "social_user_v0",
					"schema_url": "",
					"traits":     map[string]any{"email": tc.email},
				})
			}))
			defer srv.Close()

			logger := logging.NewNoopLogger()
			c := NewClient(srv.URL, tracing.NewNoopTracer(), monitoring.NewNoopMonitor("test", logger), logger)

			id, err := c.CreateIdentity(context.Background(), tc.email)
			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			if id != "identity-id" {
				t.Errorf("expected identity-id, got %q", id)
			}
			if schemaID, _ := body["schema_id"].(string); schemaID != "" {
				t.Errorf("expected no schema id (Kratos's default schema), got %q", schemaID)
			}
			if traits, _ := body["traits"].(map[string]any); traits["email"] != tc.email {
				t.Errorf("expected the email trait %q, got %v", tc.email, body["traits"])
			}
		})
	}
}
