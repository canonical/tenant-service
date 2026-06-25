// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package grpcutil

import (
	"testing"

	v0 "github.com/canonical/identity-platform-api/v0/tenant"
	"google.golang.org/grpc"
)

func TestReadOnlyMethods(t *testing.T) {
	readOnly, err := ReadOnlyMethods(v0.TenantService_ServiceDesc)
	if err != nil {
		t.Fatalf("ReadOnlyMethods returned unexpected error: %v", err)
	}

	// Methods annotated with HTTP GET should be in the read-only set.
	expectedReadOnly := []string{
		"/identity.platform.api.tenant.TenantService/ListMyTenants",
		"/identity.platform.api.tenant.TenantService/ListTenants",
		"/identity.platform.api.tenant.TenantService/ListUserTenants",
		"/identity.platform.api.tenant.TenantService/ListTenantUsers",
		"/identity.platform.api.tenant.TenantService/LookupTenants",
	}

	for _, method := range expectedReadOnly {
		if !readOnly[method] {
			t.Errorf("expected %q to be read-only, but it was not", method)
		}
	}

	// Methods annotated with HTTP POST/PATCH/DELETE should NOT be in the read-only set.
	expectedMutating := []string{
		"/identity.platform.api.tenant.TenantService/CreateTenant",
		"/identity.platform.api.tenant.TenantService/UpdateTenant",
		"/identity.platform.api.tenant.TenantService/DeleteTenant",
		"/identity.platform.api.tenant.TenantService/ProvisionUser",
		"/identity.platform.api.tenant.TenantService/UpdateTenantUser",
		"/identity.platform.api.tenant.TenantService/InviteMember",
	}

	for _, method := range expectedMutating {
		if readOnly[method] {
			t.Errorf("expected %q to be mutating, but it was marked read-only", method)
		}
	}
}

func TestReadOnlyMethods_InvalidMetadata(t *testing.T) {
	sd := grpc.ServiceDesc{
		ServiceName: "test.Service",
		Metadata:    42, // not a string — should return an error, not panic
	}

	_, err := ReadOnlyMethods(sd)
	if err == nil {
		t.Error("expected an error for non-string metadata, got nil")
	}
}

func TestReadOnlyMethods_UnknownFile(t *testing.T) {
	sd := grpc.ServiceDesc{
		ServiceName: "test.Service",
		Metadata:    "nonexistent/file.proto",
	}

	_, err := ReadOnlyMethods(sd)
	if err == nil {
		t.Error("expected an error for unknown proto file, got nil")
	}
}
