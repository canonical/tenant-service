// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0

package kratos

import (
	"context"
	"fmt"
	"net/http"

	"github.com/canonical/tenant-service/internal/logging"
	"github.com/canonical/tenant-service/internal/monitoring"
	"github.com/canonical/tenant-service/internal/tracing"
	ory "github.com/ory/client-go"
)

type ClientInterface interface {
	GetIdentityIDByEmail(ctx context.Context, email string) (string, error)
	CreateIdentity(ctx context.Context, email string) (string, error)
	GetIdentity(ctx context.Context, id string) (*ory.Identity, error)
	CreateRecoveryLink(ctx context.Context, identityID string, expiresIn string) (string, string, error)
}

type Client struct {
	client  *ory.APIClient
	tracer  tracing.TracingInterface
	monitor monitoring.MonitorInterface
	logger  logging.LoggerInterface
}

func NewClient(kratosAdminURL string, tracer tracing.TracingInterface, monitor monitoring.MonitorInterface, logger logging.LoggerInterface) *Client {
	conf := ory.NewConfiguration()
	conf.Servers = ory.ServerConfigurations{{URL: kratosAdminURL}}
	return &Client{
		client:  ory.NewAPIClient(conf),
		tracer:  tracer,
		monitor: monitor,
		logger:  logger,
	}
}

func (c *Client) GetIdentityIDByEmail(ctx context.Context, email string) (string, error) {
	ctx, span := c.tracer.Start(ctx, "kratos.GetIdentityIDByEmail")
	defer span.End()

	// List identities with credentials_identifier filter (email)
	// This is the standard way to search by email in Kratos Admin API
	// NOTE: we are setting an empty page token because of https://github.com/ory/sdk/issues/461
	// TODO: remove
	ids, r, err := c.client.IdentityAPI.ListIdentities(ctx).CredentialsIdentifier(email).PageToken("").Execute()
	if err != nil {
		if r != nil && r.StatusCode == http.StatusNotFound {
			return "", nil // Not found
		}
		// If list returns empty but no error, it means not found too.
		// However, Kratos list API usually returns 200 OK with empty list if not found.
		return "", fmt.Errorf("failed to list identities: %w", err)
	}

	if len(ids) == 0 {
		return "", nil
	}

	// Assuming uniqueness of email
	return ids[0].Id, nil
}

func (c *Client) CreateIdentity(ctx context.Context, email string) (string, error) {
	ctx, span := c.tracer.Start(ctx, "kratos.CreateIdentity")
	defer span.End()

	traits := map[string]interface{}{
		"email": email,
	}

	createIdentityBody := ory.CreateIdentityBody{
		SchemaId: "default", // default schema
		Traits:   traits,
	}

	identity, _, err := c.client.IdentityAPI.CreateIdentity(ctx).CreateIdentityBody(createIdentityBody).Execute()
	if err != nil {
		return "", fmt.Errorf("failed to create identity: %w", err)
	}

	return identity.Id, nil
}

func (c *Client) GetIdentity(ctx context.Context, id string) (*ory.Identity, error) {
	ctx, span := c.tracer.Start(ctx, "kratos.GetIdentity")
	defer span.End()

	identity, _, err := c.client.IdentityAPI.GetIdentity(ctx, id).Execute()
	if err != nil {
		return nil, fmt.Errorf("failed to get identity: %w", err)
	}

	return identity, nil
}

func (c *Client) CreateRecoveryLink(ctx context.Context, identityID string, expiresIn string) (string, string, error) {
	ctx, span := c.tracer.Start(ctx, "kratos.CreateRecoveryLink")
	defer span.End()

	body := ory.CreateRecoveryCodeForIdentityBody{
		IdentityId: identityID,
		ExpiresIn:  &expiresIn,
	}

	recoveryCode, _, err := c.client.IdentityAPI.CreateRecoveryCodeForIdentity(ctx).CreateRecoveryCodeForIdentityBody(body).Execute()
	if err != nil {
		return "", "", fmt.Errorf("failed to create recovery code: %w", err)
	}

	return recoveryCode.RecoveryLink, recoveryCode.RecoveryCode, nil
}
