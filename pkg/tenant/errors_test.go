// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package tenant

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func reasonOf(err error) string {
	st, _ := status.FromError(err)
	for _, d := range st.Details() {
		if info, ok := d.(*errdetails.ErrorInfo); ok && info.Domain == errorDomain {
			return info.Reason
		}
	}
	return ""
}

func TestToStatus(t *testing.T) {
	testCases := []struct {
		err        error
		wantCode   codes.Code
		wantReason string
	}{
		{fmt.Errorf("%w: detail", ErrDomainNotAllowed), codes.InvalidArgument, "DOMAIN_NOT_ALLOWED"},
		{ErrNotAdmitted, codes.FailedPrecondition, "NOT_ADMITTED"},
		{ErrPersonalTenant, codes.FailedPrecondition, "PERSONAL_TENANT"},
		{ErrRequiredNeedsActiveBinding, codes.FailedPrecondition, "REQUIRED_NEEDS_ACTIVE_BINDING"},
		{ErrAutoJoinNeedsRequiredAndDomains, codes.FailedPrecondition, "AUTO_JOIN_NEEDS_REQUIRED_AND_DOMAINS"},
		{ErrTenantNotFound, codes.NotFound, ""},
		{fmt.Errorf("%w: x", ErrMemberNotFound), codes.NotFound, ""},
		{ErrConnectionBound, codes.FailedPrecondition, ""},
		{errors.New("db exploded at 10.0.0.3"), codes.Internal, ""},
		{fmt.Errorf("failed to write: %w", &pgconn.PgError{Code: "55P03"}), codes.Aborted, ""},
		{fmt.Errorf("failed to read: %w", &pgconn.PgError{Code: "57014"}), codes.Unavailable, ""},
	}
	for _, tc := range testCases {
		err := toStatus(tc.err, "failed")
		if status.Code(err) != tc.wantCode || reasonOf(err) != tc.wantReason {
			t.Errorf("%v: got %v / %q", tc.err, status.Code(err), reasonOf(err))
		}
	}
	if st, _ := status.FromError(toStatus(errors.New("secret"), "failed")); st.Message() != "failed" {
		t.Errorf("an internal error must not leak: %q", st.Message())
	}
	timeout := fmt.Errorf("failed to write: %w", &pgconn.PgError{Code: "55P03", Message: "canceling statement due to lock timeout", TableName: "secret"})
	if st, _ := status.FromError(toStatus(timeout, "failed")); strings.Contains(st.Message(), "secret") || strings.Contains(st.Message(), "canceling") {
		t.Errorf("a database error must not leak: %q", st.Message())
	}
}
