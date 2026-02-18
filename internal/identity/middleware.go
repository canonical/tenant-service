// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0

package identity

import (
	"context"
	"net/http"
	"strings"

	"github.com/canonical/tenant-service/internal/logging"
	"github.com/canonical/tenant-service/internal/monitoring"
	"github.com/canonical/tenant-service/internal/tracing"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

const (
	// HeaderName is the header used to pass the authenticated identity ID
	HeaderName = "X-Kratos-Authenticated-Identity-Id"
	// ContextKey is the key used to store the user ID in the context
	ContextKey = "user_id"
)

type Middleware struct {
	tracer  tracing.TracingInterface
	monitor monitoring.MonitorInterface
	logger  logging.LoggerInterface
}

func NewMiddleware(tracer tracing.TracingInterface, monitor monitoring.MonitorInterface, logger logging.LoggerInterface) *Middleware {
	return &Middleware{
		tracer:  tracer,
		monitor: monitor,
		logger:  logger,
	}
}

func (m *Middleware) HTTPMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, span := m.tracer.Start(r.Context(), "identity.Middleware.HTTPMiddleware")
		defer span.End()

		userID := r.Header.Get(HeaderName)

		ctx = context.WithValue(ctx, ContextKey, userID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (m *Middleware) GRPCInterceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	ctx, span := m.tracer.Start(ctx, "identity.Middleware.GRPCInterceptor")
	defer span.End()

	// gRPC method names are like /package.Service/Method
	// If we need allowlisting for gRPC, we can add logic here.
	// For now, assuming all gRPC calls through this port are protected.

	// Metadata keys are lowercased
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		values := md.Get(strings.ToLower(HeaderName))
		if len(values) > 0 && values[0] != "" {
			ctx = context.WithValue(ctx, ContextKey, values[0])
		}
	}

	return handler(ctx, req)
}
