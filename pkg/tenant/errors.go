// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package tenant

import (
	"errors"

	"github.com/canonical/tenant-service/internal/logging"
	"github.com/canonical/tenant-service/internal/storage"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// errorDomain is the google.rpc.ErrorInfo domain of the service's errors.
const errorDomain = "tenant-service"

// Error is a service error with its gRPC status code and, when callers branch
// on it, a machine-readable reason.
type Error struct {
	Code    codes.Code
	Reason  string
	Message string
}

func (e *Error) Error() string {
	return e.Message
}

var (
	ErrTenantNotFound   = &Error{Code: codes.NotFound, Message: "tenant not found"}
	ErrMemberNotFound   = &Error{Code: codes.NotFound, Message: "member not found"}
	ErrIdentityNotFound = &Error{Code: codes.NotFound, Message: "identity not found"}
	ErrAlreadyMember    = &Error{Code: codes.AlreadyExists, Message: "already a member of the tenant"}
	ErrInvalidSSOPolicy = &Error{Code: codes.InvalidArgument, Message: "invalid sso policy"}
	ErrConnectionBound  = &Error{Code: codes.FailedPrecondition, Message: "a connection is bound by another tenant"}

	ErrAutoJoinNeedsRequiredAndDomains = &Error{Code: codes.FailedPrecondition, Reason: "AUTO_JOIN_NEEDS_REQUIRED_AND_DOMAINS",
		Message: "auto-join needs domains and REQUIRED enforcement"}
	ErrRequiredNeedsActiveBinding = &Error{Code: codes.FailedPrecondition, Reason: "REQUIRED_NEEDS_ACTIVE_BINDING",
		Message: "REQUIRED enforcement needs an active binding"}
	ErrPersonalTenant = &Error{Code: codes.FailedPrecondition, Reason: "PERSONAL_TENANT",
		Message: "not available for a personal tenant"}
	ErrDomainNotAllowed = &Error{Code: codes.InvalidArgument, Reason: "DOMAIN_NOT_ALLOWED",
		Message: "the address is outside the tenant's domains"}
	ErrNotAdmitted = &Error{Code: codes.FailedPrecondition, Reason: "NOT_ADMITTED",
		Message: "the tenant admits this address by neither a pending invitation nor auto-join"}
	ErrHasTenant = &Error{Code: codes.FailedPrecondition, Reason: "HAS_TENANT",
		Message: "the account belongs to a tenant or has a pending invitation to one: no personal tenant"}
)

// failed logs an unexpected error and returns the gRPC status for err.
func failed(logger logging.LoggerInterface, err error, msg string, keysAndValues ...interface{}) error {
	var e *Error
	if !errors.As(err, &e) {
		logger.Errorw(msg, append(keysAndValues, "error", err)...)
	}
	return toStatus(err, msg)
}

// toStatus returns the gRPC status for err: the Error it wraps, or fallback so
// that no internal detail leaks. A database timeout tells the caller to retry.
func toStatus(err error, fallback string) error {
	var e *Error
	if !errors.As(err, &e) {
		switch {
		case storage.IsLockTimeout(err):
			return status.Error(codes.Aborted, fallback+": the resource is being changed, try again")
		case storage.IsStatementTimeout(err):
			return status.Error(codes.Unavailable, fallback+": the database took too long")
		}
		return status.Error(codes.Internal, fallback)
	}

	// The reason is in the message too: the HTTP gateway drops the details.
	msg := err.Error()
	if e.Reason != "" {
		msg = e.Reason + ": " + msg
	}
	st := status.New(e.Code, msg)
	if e.Reason == "" {
		return st.Err()
	}
	withInfo, detailErr := st.WithDetails(&errdetails.ErrorInfo{Reason: e.Reason, Domain: errorDomain})
	if detailErr != nil {
		return st.Err()
	}
	return withInfo.Err()
}
