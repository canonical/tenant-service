// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0

package cmd

import (
	"context"
	"fmt"

	v0 "github.com/canonical/tenant-service/v0"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

// getClient returns a client interface and a closure function to close resources if needed.
// It decides whether to return a gRPC or HTTP client based on flags.
func getClient() (func() error, v0.TenantServiceClient, error) {
	// If HTTP endpoint is set, prefer HTTP
	if httpEndpoint != "" {
		return func() error { return nil }, newHTTPTenantClient(httpEndpoint), nil
	}

	// Use gRPC endpoint
	conn, err := grpc.Dial(grpcEndpoint, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to dial gRPC server: %w", err)
	}
	return conn.Close, v0.NewTenantServiceClient(conn), nil
}

func getAuthenticatedContext(ctx context.Context) context.Context {
	if userID != "" {
		md := metadata.New(map[string]string{
			"X-Kratos-Authenticated-Identity-Id": userID,
		})
		return metadata.NewOutgoingContext(ctx, md)
	}
	return ctx
}
