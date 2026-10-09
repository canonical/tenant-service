// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package cmd

import (
	"testing"

	v0 "github.com/canonical/identity-platform-api/v0/tenant"
	"google.golang.org/grpc"
)

// Every RPC either writes, and runs in a transaction, or only reads, and runs
// without one: a new RPC has to be put on one side here.
func TestGRPCReadOnlyMethods(t *testing.T) {
	readOnly, err := grpcReadOnlyMethods()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	transactional := map[string]bool{
		v0.TenantService_InviteMember_FullMethodName:                    true,
		v0.TenantService_CreateTenant_FullMethodName:                    true,
		v0.TenantService_UpdateTenant_FullMethodName:                    true,
		v0.TenantService_DeleteTenant_FullMethodName:                    true,
		v0.TenantService_ProvisionUser_FullMethodName:                   true,
		v0.TenantService_RemoveTenantUser_FullMethodName:                true,
		v0.TenantService_PutTenantMFAPolicy_FullMethodName:              true,
		v0.TenantSignInService_JoinTenant_FullMethodName:                true,
		v0.TenantSignInService_CreatePersonalTenant_FullMethodName:      true,
		v0.TenantSSOPolicyService_PutTenantSSOPolicy_FullMethodName:     true,
		v0.TenantSSOPolicyService_SetTenantSSODomains_FullMethodName:    true,
		v0.TenantSSOPolicyService_RemoveTenantSSOBinding_FullMethodName: true,
	}

	for _, sd := range []grpc.ServiceDesc{
		v0.TenantService_ServiceDesc,
		v0.TenantSignInService_ServiceDesc,
		v0.TenantSSOPolicyService_ServiceDesc,
	} {
		for _, m := range sd.Methods {
			method := "/" + sd.ServiceName + "/" + m.MethodName
			if readOnly[method] == transactional[method] {
				t.Errorf("%s: read-only %v, transactional %v", method, readOnly[method], transactional[method])
			}
		}
	}
}
