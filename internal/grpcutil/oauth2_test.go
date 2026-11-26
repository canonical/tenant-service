// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package grpcutil

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestTokenSource_CachingAndRefresh(t *testing.T) {
	var tokenCalls atomic.Int32
	var authHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call := tokenCalls.Add(1)
		authHeader = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		expiresIn := 3600
		if call == 1 {
			// Already within x/oauth2's 10s expiry delta so the next Token() call refreshes.
			expiresIn = 1
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "tok-" + string(rune('0'+call)),
			"token_type":   "Bearer",
			"expires_in":   expiresIn,
		})
	}))
	defer srv.Close()

	ts := TokenSource(context.Background(), Credentials{
		TokenURL:     srv.URL,
		ClientID:     "cid",
		ClientSecret: "csecret",
		Scopes:       []string{"tenant-service"},
	})

	tok1, err := ts.Token()
	require.NoError(t, err)
	assert.Equal(t, "tok-1", tok1.AccessToken)
	assert.Contains(t, authHeader, "Basic ")
	assert.Equal(t, int32(1), tokenCalls.Load())

	// Second call refreshes because tok-1 is within the expiry delta.
	tok2, err := ts.Token()
	require.NoError(t, err)
	assert.Equal(t, "tok-2", tok2.AccessToken)
	assert.Equal(t, int32(2), tokenCalls.Load())

	// Third call reuses cached tok-2.
	tok3, err := ts.Token()
	require.NoError(t, err)
	assert.Equal(t, "tok-2", tok3.AccessToken)
	assert.Equal(t, int32(2), tokenCalls.Load())
}

func TestBearerPerRPCCredentials_RequireTransportSecurity(t *testing.T) {
	ts := TokenSource(context.Background(), Credentials{
		TokenURL:     "http://localhost:4444/oauth2/token",
		ClientID:     "cid",
		ClientSecret: "csecret",
	})

	plain := &bearerPerRPCCredentials{tokenSource: ts, tlsEnabled: false}
	assert.False(t, plain.RequireTransportSecurity())

	secure := &bearerPerRPCCredentials{tokenSource: ts, tlsEnabled: true}
	assert.True(t, secure.RequireTransportSecurity())
}

func TestBearerPerRPCCredentials_TokenFailureClassification(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		wantCode   codes.Code
	}{
		{name: "400 maps to Unauthenticated", statusCode: http.StatusBadRequest, wantCode: codes.Unauthenticated},
		{name: "401 maps to Unauthenticated", statusCode: http.StatusUnauthorized, wantCode: codes.Unauthenticated},
		{name: "503 maps to Unavailable", statusCode: http.StatusServiceUnavailable, wantCode: codes.Unavailable},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
			}))
			defer srv.Close()

			perRPC := &bearerPerRPCCredentials{
				tokenSource: TokenSource(context.Background(), Credentials{
					TokenURL:     srv.URL,
					ClientID:     "cid",
					ClientSecret: "csecret",
				}),
				tlsEnabled: false,
			}

			_, err := perRPC.GetRequestMetadata(context.Background())
			require.Error(t, err)
			assert.Equal(t, tt.wantCode, status.Code(err))
		})
	}

	t.Run("unreachable endpoint maps to Unavailable", func(t *testing.T) {
		perRPC := &bearerPerRPCCredentials{
			tokenSource: TokenSource(context.Background(), Credentials{
				TokenURL:     "http://127.0.0.1:1/oauth2/token",
				ClientID:     "cid",
				ClientSecret: "csecret",
			}),
			tlsEnabled: false,
		}

		_, err := perRPC.GetRequestMetadata(context.Background())
		require.Error(t, err)
		assert.Equal(t, codes.Unavailable, status.Code(err))
	})

	t.Run("respects per-RPC context deadline when token endpoint stalls", func(t *testing.T) {
		unblock := make(chan struct{})
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			<-unblock
		}))
		defer srv.Close()
		defer close(unblock)

		perRPC := &bearerPerRPCCredentials{
			tokenSource: TokenSource(context.Background(), Credentials{
				TokenURL:     srv.URL,
				ClientID:     "cid",
				ClientSecret: "csecret",
			}),
			tlsEnabled: false,
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
		defer cancel()

		_, err := perRPC.GetRequestMetadata(ctx)
		require.Error(t, err)
		assert.Equal(t, codes.DeadlineExceeded, status.Code(err))
	})
}
