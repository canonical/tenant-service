// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package grpcutil

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

const defaultTokenHTTPTimeout = 5 * time.Second

// Credentials configures OAuth2 client-credentials authentication for outgoing gRPC requests.
type Credentials struct {
	TokenURL     string
	ClientID     string
	ClientSecret string
	Scopes       []string
}

// Validate checks that the required OAuth2 client-credentials fields are populated.
func (c Credentials) Validate() error {
	if strings.TrimSpace(c.TokenURL) == "" {
		return errors.New("service token URL cannot be empty")
	}
	if strings.TrimSpace(c.ClientID) == "" {
		return errors.New("service client ID cannot be empty")
	}
	if strings.TrimSpace(c.ClientSecret) == "" {
		return errors.New("service client secret cannot be empty")
	}
	return nil
}

// TokenSource returns a caching oauth2.TokenSource using the client-credentials grant
// with AuthStyleInHeader and an explicit HTTP client timeout so token requests never fall
// back to http.DefaultClient or double-probe with body credentials on failure.
func TokenSource(baseCtx context.Context, creds Credentials) oauth2.TokenSource {
	if baseCtx == nil {
		baseCtx = context.Background()
	}
	cfg := clientcredentials.Config{
		ClientID:     creds.ClientID,
		ClientSecret: creds.ClientSecret,
		TokenURL:     creds.TokenURL,
		Scopes:       creds.Scopes,
		AuthStyle:    oauth2.AuthStyleInHeader,
	}
	httpClient := &http.Client{Timeout: defaultTokenHTTPTimeout}
	ctx := context.WithValue(baseCtx, oauth2.HTTPClient, httpClient)
	return cfg.TokenSource(ctx)
}

type bearerPerRPCCredentials struct {
	tokenSource oauth2.TokenSource
	tlsEnabled  bool
}

var _ credentials.PerRPCCredentials = (*bearerPerRPCCredentials)(nil)

func (c *bearerPerRPCCredentials) GetRequestMetadata(ctx context.Context, _ ...string) (map[string]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, status.FromContextError(err).Err()
	}

	type tokenResult struct {
		token *oauth2.Token
		err   error
	}
	ch := make(chan tokenResult, 1)
	go func() {
		tok, err := c.tokenSource.Token()
		ch <- tokenResult{token: tok, err: err}
	}()

	var token *oauth2.Token
	var err error
	select {
	case <-ctx.Done():
		return nil, status.FromContextError(ctx.Err()).Err()
	case res := <-ch:
		token, err = res.token, res.err
	}

	if err != nil {
		var retrieveErr *oauth2.RetrieveError
		if errors.As(err, &retrieveErr) && retrieveErr.Response != nil &&
			(retrieveErr.Response.StatusCode == http.StatusBadRequest ||
				retrieveErr.Response.StatusCode == http.StatusUnauthorized) {
			return nil, status.Errorf(codes.Unauthenticated, "failed to retrieve oauth2 token: %v", err)
		}
		return nil, status.Errorf(codes.Unavailable, "oauth2 token endpoint unavailable: %v", err)
	}
	return map[string]string{
		"authorization": "Bearer " + token.AccessToken,
	}, nil
}

func (c *bearerPerRPCCredentials) RequireTransportSecurity() bool {
	return c.tlsEnabled
}

// Dial creates a gRPC ClientConn authenticated via OAuth2 client-credentials bearer tokens.
func Dial(
	baseCtx context.Context,
	address string,
	creds Credentials,
	tlsEnabled bool,
	serviceConfig string,
	extraOpts ...grpc.DialOption,
) (*grpc.ClientConn, error) {
	if strings.TrimSpace(address) == "" {
		return nil, errors.New("gRPC address cannot be empty")
	}
	if err := creds.Validate(); err != nil {
		return nil, err
	}

	var transportCreds credentials.TransportCredentials
	if tlsEnabled {
		transportCreds = credentials.NewTLS(&tls.Config{MinVersion: tls.VersionTLS12})
	} else {
		transportCreds = insecure.NewCredentials()
	}

	perRPCCreds := &bearerPerRPCCredentials{
		tokenSource: TokenSource(baseCtx, creds),
		tlsEnabled:  tlsEnabled,
	}

	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(transportCreds),
		grpc.WithPerRPCCredentials(perRPCCreds),
		grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
	}
	if serviceConfig != "" {
		opts = append(opts, grpc.WithDefaultServiceConfig(serviceConfig))
	}
	opts = append(opts, extraOpts...)

	conn, err := grpc.NewClient(address, opts...)
	if err != nil {
		return nil, fmt.Errorf("dial gRPC %q: %w", address, err)
	}
	return conn, nil
}
