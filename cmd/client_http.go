// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package cmd

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	v0 "github.com/canonical/identity-platform-api/v0/tenant"
	httpclient "github.com/canonical/tenant-service/client/http"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

type httpTenantClient struct {
	client *httpclient.Client
}

// Ensure interface compliance
var _ v0.TenantServiceClient = (*httpTenantClient)(nil)

func newHTTPTenantClient(endpoint string) v0.TenantServiceClient {
	if !strings.HasPrefix(endpoint, "http") {
		endpoint = "http://" + endpoint
	}
	endpoint = strings.TrimSuffix(endpoint, "/")

	opts := []httpclient.ClientOption{}
	if authToken != "" {
		opts = append(opts, httpclient.WithRequestEditorFn(func(ctx context.Context, req *http.Request) error {
			token := authToken
			if !strings.HasPrefix(token, "Bearer ") {
				token = "Bearer " + token
			}
			req.Header.Set("Authorization", token)
			return nil
		}))
	}

	client, err := httpclient.NewClient(endpoint, opts...)
	if err != nil {
		panic(fmt.Errorf("failed to create http client: %w", err))
	}

	return &httpTenantClient{
		client: client,
	}
}

// Helper to make requests and parse responses using protojson
func (c *httpTenantClient) handleRequest(resp *http.Response, err error, out proto.Message) error {
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("api error (status %d): %s", resp.StatusCode, string(body))
	}

	if out != nil {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("failed to read response body: %w", err)
		}
		if err := protojson.Unmarshal(body, out); err != nil {
			return fmt.Errorf("failed to unmarshal response: %w", err)
		}
	}
	return nil
}

// Implement v0.TenantServiceClient interface

func (c *httpTenantClient) ListMyTenants(ctx context.Context, in *v0.ListMyTenantsRequest, opts ...grpc.CallOption) (*v0.ListMyTenantsResponse, error) {
	out := new(v0.ListMyTenantsResponse)
	params := &httpclient.TenantServiceListMyTenantsParams{}
	if in != nil {
		if in.Enabled != nil {
			params.Enabled = in.Enabled
		}
	}
	resp, err := c.client.TenantServiceListMyTenants(ctx, params)
	if err := c.handleRequest(resp, err, out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *httpTenantClient) ListTenants(ctx context.Context, in *v0.ListTenantsRequest, opts ...grpc.CallOption) (*v0.ListTenantsResponse, error) {
	out := new(v0.ListTenantsResponse)
	params := &httpclient.TenantServiceListTenantsParams{}
	if in != nil {
		if in.PageToken != "" {
			pageToken := in.PageToken
			params.PageToken = &pageToken
		}
		if in.PageSize > 0 {
			pageSize := in.PageSize
			params.PageSize = &pageSize
		}
		if in.Enabled != nil {
			params.Enabled = in.Enabled
		}
	}
	resp, err := c.client.TenantServiceListTenants(ctx, params)
	if err := c.handleRequest(resp, err, out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *httpTenantClient) InviteMember(ctx context.Context, in *v0.InviteMemberRequest, opts ...grpc.CallOption) (*v0.InviteMemberResponse, error) {
	out := new(v0.InviteMemberResponse)
	bodyBytes, err := protojson.Marshal(in)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}
	resp, err := c.client.TenantServiceInviteMemberWithBody(ctx, in.GetTenantId(), "application/json", bytes.NewReader(bodyBytes))
	if err := c.handleRequest(resp, err, out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *httpTenantClient) ListUserTenants(ctx context.Context, in *v0.ListUserTenantsRequest, opts ...grpc.CallOption) (*v0.ListUserTenantsResponse, error) {
	out := new(v0.ListUserTenantsResponse)
	params := &httpclient.TenantServiceListUserTenantsParams{}
	if in != nil {
		if in.Enabled != nil {
			params.Enabled = in.Enabled
		}
	}
	resp, err := c.client.TenantServiceListUserTenants(ctx, in.GetUserId(), params)
	if err := c.handleRequest(resp, err, out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *httpTenantClient) CreateTenant(ctx context.Context, in *v0.CreateTenantRequest, opts ...grpc.CallOption) (*v0.CreateTenantResponse, error) {
	out := new(v0.CreateTenantResponse)
	bodyBytes, err := protojson.Marshal(in)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}
	resp, err := c.client.TenantServiceCreateTenantWithBody(ctx, "application/json", bytes.NewReader(bodyBytes))
	if err := c.handleRequest(resp, err, out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *httpTenantClient) UpdateTenant(ctx context.Context, in *v0.UpdateTenantRequest, opts ...grpc.CallOption) (*v0.UpdateTenantResponse, error) {
	out := new(v0.UpdateTenantResponse)
	if in.Tenant == nil {
		return nil, fmt.Errorf("tenant is required")
	}
	bodyBytes, err := protojson.Marshal(in)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}
	resp, err := c.client.TenantServiceUpdateTenantWithBody(ctx, in.GetTenantId(), "application/json", bytes.NewReader(bodyBytes))
	if err := c.handleRequest(resp, err, out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *httpTenantClient) DeleteTenant(ctx context.Context, in *v0.DeleteTenantRequest, opts ...grpc.CallOption) (*v0.DeleteTenantResponse, error) {
	out := new(v0.DeleteTenantResponse)
	resp, err := c.client.TenantServiceDeleteTenant(ctx, in.GetTenantId())
	if err := c.handleRequest(resp, err, out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *httpTenantClient) ProvisionUser(ctx context.Context, in *v0.ProvisionUserRequest, opts ...grpc.CallOption) (*v0.ProvisionUserResponse, error) {
	out := new(v0.ProvisionUserResponse)
	bodyBytes, err := protojson.Marshal(in)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}
	resp, err := c.client.TenantServiceProvisionUserWithBody(ctx, in.GetTenantId(), "application/json", bytes.NewReader(bodyBytes))
	if err := c.handleRequest(resp, err, out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *httpTenantClient) ListTenantUsers(ctx context.Context, in *v0.ListTenantUsersRequest, opts ...grpc.CallOption) (*v0.ListTenantUsersResponse, error) {
	out := new(v0.ListTenantUsersResponse)
	params := &httpclient.TenantServiceListTenantUsersParams{}
	if in != nil {
		if in.PageToken != "" {
			pageToken := in.PageToken
			params.PageToken = &pageToken
		}
		if in.PageSize > 0 {
			pageSize := in.PageSize
			params.PageSize = &pageSize
		}
		if in.Role != nil {
			params.Role = in.Role
		}
		if in.Email != nil {
			params.Email = in.Email
		}
		if in.IdentityId != nil {
			params.IdentityId = in.IdentityId
		}
		if in.IncludeEmails {
			includeEmails := in.IncludeEmails
			params.IncludeEmails = &includeEmails
		}
	}
	resp, err := c.client.TenantServiceListTenantUsers(ctx, in.GetTenantId(), params)
	if err := c.handleRequest(resp, err, out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *httpTenantClient) UpdateTenantUser(ctx context.Context, in *v0.UpdateTenantUserRequest, opts ...grpc.CallOption) (*v0.UpdateTenantUserResponse, error) {
	out := new(v0.UpdateTenantUserResponse)
	bodyBytes, err := protojson.Marshal(in)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}
	resp, err := c.client.TenantServiceUpdateTenantUserWithBody(ctx, in.GetTenantId(), in.GetUserId(), "application/json", bytes.NewReader(bodyBytes))
	if err := c.handleRequest(resp, err, out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *httpTenantClient) LookupTenants(ctx context.Context, in *v0.LookupTenantsRequest, opts ...grpc.CallOption) (*v0.LookupTenantsResponse, error) {
	out := new(v0.LookupTenantsResponse)
	params := &httpclient.TenantServiceLookupTenantsParams{}
	if in != nil {
		if in.Email != "" {
			email := in.Email
			params.Email = &email
		}
		if in.IdentityId != "" {
			identityID := in.IdentityId
			params.IdentityId = &identityID
		}
	}
	resp, err := c.client.TenantServiceLookupTenants(ctx, params)
	if err := c.handleRequest(resp, err, out); err != nil {
		return nil, err
	}
	return out, nil
}
