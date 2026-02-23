// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0

package e2e

import (
	"context"
	"os"
	"testing"
	"time"

	v0 "github.com/canonical/tenant-service/v0"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

// getGRPCAddress returns the gRPC server address for tests
func getGRPCAddress() string {
	if addr := os.Getenv("GRPC_ADDRESS"); addr != "" {
		return addr
	}
	if testEnv != nil {
		// When running in full test environment, use localhost with default gRPC port
		return "localhost:50051"
	}
	return "localhost:50051"
}

// newTestGRPCClient creates a new gRPC client configured for the test environment
func newTestGRPCClient(t *testing.T) (v0.TenantServiceClient, *grpc.ClientConn) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	grpcAddress := getGRPCAddress()
	conn, err := grpc.DialContext(
		ctx,
		grpcAddress,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		t.Fatalf("failed to connect to gRPC server at %s: %v", grpcAddress, err)
	}

	client := v0.NewTenantServiceClient(conn)
	return client, conn
}

// grpcAuthContext returns a context with JWT Bearer token for authentication
func grpcAuthContext(ctx context.Context) (context.Context, error) {
	token, err := getAuthToken(ctx)
	if err != nil {
		return nil, err
	}

	// Add authorization metadata to context
	md := metadata.Pairs("authorization", "Bearer "+token)
	return metadata.NewOutgoingContext(ctx, md), nil
}

// TestGRPCAuthentication tests that gRPC endpoints require authentication
func TestGRPCAuthentication(t *testing.T) {
	client, conn := newTestGRPCClient(t)
	defer conn.Close()

	ctx := context.Background()

	t.Run("Request Without Auth Should Fail", func(t *testing.T) {
		// Try to list tenants without authentication
		_, err := client.ListTenants(ctx, &v0.ListTenantsRequest{})
		if err == nil {
			t.Error("expected error when calling without authentication, got nil")
		}
	})

	t.Run("Request With Valid Auth Should Succeed", func(t *testing.T) {
		authCtx, err := grpcAuthContext(ctx)
		if err != nil {
			t.Fatalf("failed to setup auth context: %v", err)
		}

		_, err = client.ListTenants(authCtx, &v0.ListTenantsRequest{})
		if err != nil {
			t.Errorf("expected success with valid auth, got error: %v", err)
		}
	})
}
