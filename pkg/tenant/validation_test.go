// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package tenant

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/canonical/tenant-service/internal/types"
)

func TestNormaliseDomains(t *testing.T) {
	many := func(n int) []string {
		out := make([]string, n)
		for i := range out {
			out[i] = fmt.Sprintf("d%d.hooli.example", i)
		}
		return out
	}

	testCases := []struct {
		name    string
		domains []string
		want    []string
		wantErr error
	}{
		{name: "none", want: []string{}},
		{name: "trimmed, lower-cased, de-duplicated and sorted",
			domains: []string{" Hooli.EXAMPLE\t", "acme.example.", "hooli.example", "ACME.example"},
			want:    []string{"acme.example", "hooli.example"}},
		{name: "punycode label", domains: []string{"XN--Bcher-kva.example"}, want: []string{"xn--bcher-kva.example"}},
		{name: "internationalised top-level label", domains: []string{"hooli.xn--p1ai"}, want: []string{"hooli.xn--p1ai"}},
		{name: "as many as allowed", domains: many(maxDomains), want: slices.Sorted(slices.Values(many(maxDomains)))},
		{name: "duplicates do not count", domains: append(many(maxDomains), "d0.hooli.example"), want: slices.Sorted(slices.Values(many(maxDomains)))},
		{name: "too many", domains: many(maxDomains + 1), wantErr: ErrInvalidSSOPolicy},
		{name: "empty", domains: []string{""}, wantErr: ErrInvalidSSOPolicy},
		{name: "blank", domains: []string{"   "}, wantErr: ErrInvalidSSOPolicy},
		{name: "a single label", domains: []string{"hooli"}, wantErr: ErrInvalidSSOPolicy},
		{name: "a one-letter top-level label", domains: []string{"hooli.e"}, wantErr: ErrInvalidSSOPolicy},
		{name: "a numeric top-level label", domains: []string{"hooli.123"}, wantErr: ErrInvalidSSOPolicy},
		{name: "a leading dot", domains: []string{".hooli.example"}, wantErr: ErrInvalidSSOPolicy},
		{name: "an empty label", domains: []string{"hooli..example"}, wantErr: ErrInvalidSSOPolicy},
		{name: "a label that starts with a hyphen", domains: []string{"-hooli.example"}, wantErr: ErrInvalidSSOPolicy},
		{name: "a label that ends with a hyphen", domains: []string{"hooli-.example"}, wantErr: ErrInvalidSSOPolicy},
		{name: "longer than a name may be", domains: []string{strings.Repeat("a.", 126) + "example"}, wantErr: ErrInvalidSSOPolicy},
		{name: "not ASCII", domains: []string{"bücher.example"}, wantErr: ErrInvalidSSOPolicy},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := normaliseDomains(tc.domains)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("want %v, got %v, %v", tc.wantErr, got, err)
				}
				return
			}
			if err != nil || !slices.Equal(got, tc.want) {
				t.Fatalf("want %v, got %v, %v", tc.want, got, err)
			}
		})
	}
}

func TestValidateBindings(t *testing.T) {
	many := func(n int) []types.SSOBinding {
		out := make([]types.SSOBinding, n)
		for i := range out {
			out[i] = types.SSOBinding{ConnectionID: fmt.Sprintf("0190a000-0000-7000-8000-%012d", i)}
		}
		return out
	}

	if err := validateBindings(nil); err != nil {
		t.Errorf("none: %v", err)
	}
	if err := validateBindings(many(maxBindings)); err != nil {
		t.Errorf("as many as allowed: %v", err)
	}
	if err := validateBindings(many(maxBindings + 1)); !errors.Is(err, ErrInvalidSSOPolicy) {
		t.Errorf("too many: want ErrInvalidSSOPolicy, got %v", err)
	}
	twice := []types.SSOBinding{{ConnectionID: tConnection, Active: true}, {ConnectionID: tConnection}}
	if err := validateBindings(twice); !errors.Is(err, ErrInvalidSSOPolicy) {
		t.Errorf("a connection bound twice: want ErrInvalidSSOPolicy, got %v", err)
	}
}

func TestEmailDomain(t *testing.T) {
	testCases := map[string]string{
		"hank@Hooli.Example": "hooli.example",
		"hank@hooli":         "hooli",
		"hank":               "",
		"@hooli.example":     "",
		"hank@":              "",
		"a@b@hooli.example":  "",
	}
	for email, want := range testCases {
		if got := emailDomain(email); got != want {
			t.Errorf("emailDomain(%q) = %q, want %q", email, got, want)
		}
	}
}
