// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0

package hydra

import (
	"context"
	"fmt"
	"net/http"

	"github.com/canonical/tenant-service/internal/logging"
	"github.com/canonical/tenant-service/internal/monitoring"
	"github.com/canonical/tenant-service/internal/tracing"
	client "github.com/ory/hydra-client-go/v2"
)

// ClientInterface defines the operations for managing Hydra OAuth2 clients.
type ClientInterface interface {
	CreateOAuth2Client(ctx context.Context, clientID string, metadata map[string]interface{}) (string, error)
	DeleteOAuth2Client(ctx context.Context, clientID string) error
}

// Client wraps the Hydra admin SDK with tracing, monitoring, and logging.
type Client struct {
	client  *client.APIClient
	tracer  tracing.TracingInterface
	monitor monitoring.MonitorInterface
	logger  logging.LoggerInterface
}

// NewClient creates a new Hydra admin client wrapper.
func NewClient(hydraAdminURL string, tracer tracing.TracingInterface, monitor monitoring.MonitorInterface, logger logging.LoggerInterface) *Client {
	conf := client.NewConfiguration()
	conf.Servers = client.ServerConfigurations{{URL: hydraAdminURL}}
	return &Client{
		client:  client.NewAPIClient(conf),
		tracer:  tracer,
		monitor: monitor,
		logger:  logger,
	}
}

// CreateOAuth2Client creates a new client_credentials OAuth2 client in Hydra
// with a caller-chosen client ID. The metadata map is stored on the client for
// auditability (e.g., {"tenant_id": "..."}).
// Returns the client_secret.
func (c *Client) CreateOAuth2Client(ctx context.Context, clientID string, metadata map[string]interface{}) (string, error) {
	ctx, span := c.tracer.Start(ctx, "hydra.CreateOAuth2Client")
	defer span.End()

	body := client.NewOAuth2Client()
	body.SetClientId(clientID)
	body.SetGrantTypes([]string{"client_credentials"})
	body.SetTokenEndpointAuthMethod("client_secret_basic")
	body.SetAccessTokenStrategy("jwt")
	body.SetMetadata(metadata)

	result, r, err := c.client.OAuth2API.CreateOAuth2Client(ctx).OAuth2Client(*body).Execute()
	if err != nil {
		statusCode := 0
		if r != nil {
			statusCode = r.StatusCode
		}
		return "", fmt.Errorf("failed to create OAuth2 client (status %d): %w", statusCode, err)
	}

	clientSecret := result.GetClientSecret()

	c.logger.Infow("created OAuth2 client in Hydra",
		"client_id", clientID,
	)

	return clientSecret, nil
}

// DeleteOAuth2Client deletes an OAuth2 client from Hydra by its client ID.
func (c *Client) DeleteOAuth2Client(ctx context.Context, clientID string) error {
	ctx, span := c.tracer.Start(ctx, "hydra.DeleteOAuth2Client")
	defer span.End()

	r, err := c.client.OAuth2API.DeleteOAuth2Client(ctx, clientID).Execute()
	if err != nil {
		if r != nil && r.StatusCode == http.StatusNotFound {
			// Client already deleted, treat as idempotent.
			c.logger.Warnw("OAuth2 client not found in Hydra during deletion",
				"client_id", clientID,
			)
			return nil
		}
		return fmt.Errorf("failed to delete OAuth2 client %s: %w", clientID, err)
	}

	c.logger.Infow("deleted OAuth2 client from Hydra",
		"client_id", clientID,
	)

	return nil
}
