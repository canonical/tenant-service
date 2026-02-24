// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"time"

	httpclient "github.com/canonical/tenant-service/client/http"
	v0 "github.com/canonical/tenant-service/v0"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
)

var (
	cachedToken string
	tokenExpiry time.Time
	tokenMutex  sync.RWMutex
)

// getAuthToken returns a JWT token either from environment or by exchanging client credentials.
// Tokens are cached to avoid unnecessary token endpoint requests.
func getAuthToken(ctx context.Context) (string, error) {
	// Check cache first (read lock)
	tokenMutex.RLock()
	if cachedToken != "" && time.Now().Before(tokenExpiry) {
		defer tokenMutex.RUnlock()
		return cachedToken, nil
	}
	tokenMutex.RUnlock()

	// Acquire write lock for token refresh
	tokenMutex.Lock()
	defer tokenMutex.Unlock()

	// Double-check after acquiring write lock (another goroutine may have refreshed)
	if cachedToken != "" && time.Now().Before(tokenExpiry) {
		return cachedToken, nil
	}

	// Use JWT token from environment if provided
	if token := os.Getenv("JWT_TOKEN"); token != "" {
		// JWT tokens from env should also be cached, set reasonable cache duration
		cachedToken = token
		tokenExpiry = time.Now().Add(5 * time.Minute)
		return token, nil
	}

	// Otherwise, use client credentials from env or test globals
	cID := os.Getenv("CLIENT_ID")
	if cID == "" {
		cID = clientId
	}
	cSecret := os.Getenv("CLIENT_SECRET")
	if cSecret == "" {
		cSecret = clientSecret
	}

	if cID == "" || cSecret == "" {
		return "", fmt.Errorf("no authentication credentials available")
	}

	// Exchange for token
	token, expiresIn, err := getJWTTokenWithExpiry(ctx, cID, cSecret)
	if err != nil {
		return "", err
	}

	// Cache with safety margin (refresh 60 seconds before actual expiry)
	cachedToken = token
	safetyMargin := 60
	if expiresIn > safetyMargin {
		tokenExpiry = time.Now().Add(time.Duration(expiresIn-safetyMargin) * time.Second)
	} else {
		tokenExpiry = time.Now().Add(time.Duration(expiresIn) * time.Second)
	}

	return token, nil
}

// Tenant represents a minimal tenant structure for E2E testing.
type Tenant struct {
	ID   string
	Name string
}

// TenantClient abstracts tenant operations across HTTP and gRPC protocols.
// Implementations must handle authentication and protocol-specific details.
type TenantClient interface {
	// CreateTenant creates a new tenant with the given name.
	// Returns the created tenant's ID or an error.
	CreateTenant(ctx context.Context, name string) (string, error)

	// ListTenants retrieves all tenants the authenticated user has access to.
	ListTenants(ctx context.Context) ([]Tenant, error)

	// UpdateTenant modifies the tenant with the given ID.
	UpdateTenant(ctx context.Context, id, name string) error

	// DeleteTenant removes the tenant with the given ID.
	// Implementations should be idempotent per project conventions.
	DeleteTenant(ctx context.Context, id string) error

	// Close releases any resources held by the client.
	Close() error
}

// HTTPTenantClient implements TenantClient using the HTTP/REST API.
type HTTPTenantClient struct {
	client   *httpclient.Client
	getToken func(context.Context) (string, error)
}

// NewHTTPTenantClient creates a new HTTP client for tenant operations.
func NewHTTPTenantClient(baseURL string) (*HTTPTenantClient, error) {
	client, err := httpclient.NewClient(baseURL, httpclient.WithHTTPClient(&http.Client{
		Timeout: 10 * time.Second,
	}))
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP client: %w", err)
	}
	return &HTTPTenantClient{
		client:   client,
		getToken: getAuthToken, // Inject token getter for testability
	}, nil
}

// authEditor returns a RequestEditorFn that adds Bearer token authentication.
func (c *HTTPTenantClient) authEditor(ctx context.Context) (httpclient.RequestEditorFn, error) {
	token, err := c.getToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get auth token: %w", err)
	}
	return func(ctx context.Context, req *http.Request) error {
		req.Header.Set("Authorization", "Bearer "+token)
		return nil
	}, nil
}

func (c *HTTPTenantClient) CreateTenant(ctx context.Context, name string) (string, error) {
	authEditor, err := c.authEditor(ctx)
	if err != nil {
		return "", err
	}

	resp, err := c.client.TenantServiceCreateTenant(ctx, httpclient.TenantServiceCreateTenantJSONRequestBody{
		Name: &name,
	}, authEditor)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			return "", fmt.Errorf("unexpected status %d (failed to read body: %w)", resp.StatusCode, readErr)
		}
		return "", fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Tenant struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"tenant"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	return result.Tenant.ID, nil
}

func (c *HTTPTenantClient) ListTenants(ctx context.Context) ([]Tenant, error) {
	authEditor, err := c.authEditor(ctx)
	if err != nil {
		return nil, err
	}

	resp, err := c.client.TenantServiceListTenants(ctx, authEditor)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			return nil, fmt.Errorf("unexpected status %d (failed to read body: %w)", resp.StatusCode, readErr)
		}
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Tenants []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"tenants"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	tenants := make([]Tenant, len(result.Tenants))
	for i, t := range result.Tenants {
		tenants[i] = Tenant{ID: t.ID, Name: t.Name}
	}
	return tenants, nil
}

func (c *HTTPTenantClient) UpdateTenant(ctx context.Context, id, name string) error {
	authEditor, err := c.authEditor(ctx)
	if err != nil {
		return err
	}

	// Create update request
	updateMask := "name"
	updateReq := httpclient.TenantServiceUpdateTenantJSONRequestBody{
		Tenant: &struct {
			CreatedAt *string `json:"createdAt,omitempty"`
			Enabled   *bool   `json:"enabled,omitempty"`
			Name      *string `json:"name,omitempty"`
		}{
			Name: &name,
		},
		UpdateMask: &updateMask,
	}

	resp, err := c.client.TenantServiceUpdateTenant(ctx, id, updateReq, authEditor)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			return fmt.Errorf("unexpected status %d (failed to read body: %w)", resp.StatusCode, readErr)
		}
		return fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

func (c *HTTPTenantClient) DeleteTenant(ctx context.Context, id string) error {
	authEditor, err := c.authEditor(ctx)
	if err != nil {
		return err
	}

	resp, err := c.client.TenantServiceDeleteTenant(ctx, id, authEditor)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			return fmt.Errorf("unexpected status %d (failed to read body: %w)", resp.StatusCode, readErr)
		}
		return fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

func (c *HTTPTenantClient) Close() error {
	return nil
}

// GRPCTenantClient implements TenantClient using the gRPC API.
type GRPCTenantClient struct {
	client   v0.TenantServiceClient
	conn     *grpc.ClientConn
	getToken func(context.Context) (string, error)
}

// NewGRPCTenantClient creates a new gRPC client for tenant operations.
func NewGRPCTenantClient(address string) (*GRPCTenantClient, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(
		ctx,
		address,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to gRPC server at %s: %w", address, err)
	}

	return &GRPCTenantClient{
		client:   v0.NewTenantServiceClient(conn),
		conn:     conn,
		getToken: getAuthToken, // Inject token getter for testability
	}, nil
}

// authContext returns a context with Bearer token authentication metadata.
func (c *GRPCTenantClient) authContext(ctx context.Context) (context.Context, error) {
	token, err := c.getToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get auth token: %w", err)
	}
	md := metadata.Pairs("authorization", "Bearer "+token)
	return metadata.NewOutgoingContext(ctx, md), nil
}

func (c *GRPCTenantClient) CreateTenant(ctx context.Context, name string) (string, error) {
	authCtx, err := c.authContext(ctx)
	if err != nil {
		return "", err
	}

	resp, err := c.client.CreateTenant(authCtx, &v0.CreateTenantRequest{
		Name: name,
	})
	if err != nil {
		return "", err
	}

	if resp.Tenant == nil {
		return "", fmt.Errorf("nil tenant in response")
	}

	return resp.Tenant.Id, nil
}

func (c *GRPCTenantClient) ListTenants(ctx context.Context) ([]Tenant, error) {
	authCtx, err := c.authContext(ctx)
	if err != nil {
		return nil, err
	}

	resp, err := c.client.ListTenants(authCtx, &v0.ListTenantsRequest{})
	if err != nil {
		return nil, err
	}

	tenants := make([]Tenant, len(resp.Tenants))
	for i, t := range resp.Tenants {
		tenants[i] = Tenant{ID: t.Id, Name: t.Name}
	}
	return tenants, nil
}

func (c *GRPCTenantClient) UpdateTenant(ctx context.Context, id, name string) error {
	authCtx, err := c.authContext(ctx)
	if err != nil {
		return err
	}

	_, err = c.client.UpdateTenant(authCtx, &v0.UpdateTenantRequest{
		Tenant: &v0.Tenant{
			Id:   id,
			Name: name,
		},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"name"}},
	})
	return err
}

func (c *GRPCTenantClient) DeleteTenant(ctx context.Context, id string) error {
	authCtx, err := c.authContext(ctx)
	if err != nil {
		return err
	}

	_, err = c.client.DeleteTenant(authCtx, &v0.DeleteTenantRequest{
		TenantId: id,
	})
	return err
}

func (c *GRPCTenantClient) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}
