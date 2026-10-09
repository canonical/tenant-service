// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

// TODO: Go convention prefers black-box tests (package types_test); using white-box here to match project convention.
package types

import (
	"testing"
)

func TestResolvePageSize(t *testing.T) {
	tests := []struct {
		name     string
		opts     ListOptions
		wantSize uint64
	}{
		{
			name:     "zero returns default",
			opts:     ListOptions{PageSize: 0},
			wantSize: 100,
		},
		{
			name:     "negative returns default",
			opts:     ListOptions{PageSize: -1},
			wantSize: 100,
		},
		{
			name:     "one returns one",
			opts:     ListOptions{PageSize: 1},
			wantSize: 1,
		},
		{
			name:     "mid-range value passes through",
			opts:     ListOptions{PageSize: 50},
			wantSize: 50,
		},
		{
			name:     "exactly max passes through",
			opts:     ListOptions{PageSize: 100},
			wantSize: 100,
		},
		{
			name:     "above max clamped to max not default",
			opts:     ListOptions{PageSize: 101},
			wantSize: 100,
		},
		{
			name:     "large value clamped to max",
			opts:     ListOptions{PageSize: 10000},
			wantSize: 100,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.opts.ResolvePageSize()
			if got != tt.wantSize {
				t.Errorf("ResolvePageSize() = %d, want %d", got, tt.wantSize)
			}
		})
	}
}

func TestTenantSSOPolicy_Derived(t *testing.T) {
	active := []SSOBinding{{ConnectionID: "a", Active: true}, {ConnectionID: "b"}}
	inactive := []SSOBinding{{ConnectionID: "b"}}

	testCases := []struct {
		name        string
		p           *TenantSSOPolicy
		enforcement string
		ids         int
		applies     map[string]bool
		autoJoin    map[string]bool
		invitation  map[string]bool
	}{
		{name: "no policy", p: nil, enforcement: EnforcementOff, applies: map[string]bool{"x.example": true, "": true}, autoJoin: map[string]bool{"x.example": false}, invitation: map[string]bool{"x.example": true, "": true}},
		{name: "stored required, nothing active", p: &TenantSSOPolicy{Enforcement: EnforcementRequired, AutoJoin: true, Domains: []string{"x.example"}, Bindings: inactive},
			enforcement: EnforcementOff, applies: map[string]bool{"x.example": true, "y.example": false}, autoJoin: map[string]bool{"x.example": false}, invitation: map[string]bool{"x.example": true, "y.example": true}},
		{name: "optional, no domains", p: &TenantSSOPolicy{Enforcement: EnforcementOptional, Bindings: active},
			enforcement: EnforcementOptional, ids: 1, applies: map[string]bool{"y.example": true, "": true}, autoJoin: map[string]bool{"y.example": false}, invitation: map[string]bool{"y.example": true}},
		{name: "optional with domains", p: &TenantSSOPolicy{Enforcement: EnforcementOptional, Domains: []string{"x.example"}, Bindings: active},
			enforcement: EnforcementOptional, ids: 1, applies: map[string]bool{"x.example": true, "y.example": false}, autoJoin: map[string]bool{"x.example": false}, invitation: map[string]bool{"x.example": true, "y.example": true}},
		{name: "required, no domains", p: &TenantSSOPolicy{Enforcement: EnforcementRequired, Bindings: active},
			enforcement: EnforcementRequired, ids: 1, applies: map[string]bool{"y.example": true}, autoJoin: map[string]bool{"y.example": false}, invitation: map[string]bool{"y.example": true}},
		{name: "required with auto-join", p: &TenantSSOPolicy{Enforcement: EnforcementRequired, AutoJoin: true, Domains: []string{"x.example"}, Bindings: active},
			enforcement: EnforcementRequired, ids: 1, applies: map[string]bool{"x.example": true, "y.example": false, "": false}, autoJoin: map[string]bool{"x.example": true, "y.example": false}, invitation: map[string]bool{"x.example": true, "y.example": false, "": false}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.p.EffectiveEnforcement(); got != tc.enforcement {
				t.Errorf("enforcement: want %q, got %q", tc.enforcement, got)
			}
			if got := len(tc.p.ActiveConnectionIDs()); got != tc.ids {
				t.Errorf("active ids: want %d, got %d", tc.ids, got)
			}
			for domain, want := range tc.applies {
				if got := tc.p.AppliesToDomain(domain); got != want {
					t.Errorf("applies to %q: want %v, got %v", domain, want, got)
				}
			}
			for domain, want := range tc.autoJoin {
				if got := tc.p.AutoJoinAdmitsDomain(domain); got != want {
					t.Errorf("auto-join admits %q: want %v, got %v", domain, want, got)
				}
			}
			for domain, want := range tc.invitation {
				if got := tc.p.InvitationAdmitsDomain(domain); got != want {
					t.Errorf("an invitation admits %q: want %v, got %v", domain, want, got)
				}
			}
		})
	}
}
