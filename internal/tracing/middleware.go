// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package tracing

import (
	"context"
	"net/http"
	"strings"

	"github.com/canonical/tenant-service/internal/logging"
	"github.com/canonical/tenant-service/internal/monitoring"
	chi "github.com/go-chi/chi/v5"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	semconv "go.opentelemetry.io/otel/semconv/v1.18.0"
	"go.opentelemetry.io/otel/trace"
)

// Middleware is the tracing middleware object implementing OpenTelemetry tracing
type Middleware struct {
	service string

	monitor monitoring.MonitorInterface
	logger  logging.LoggerInterface
}

// OpenTelemetry starts a server span for each request. A caller's span context
// stays its parent: Kratos calls the webhooks from inside the cluster.
func (mdw *Middleware) OpenTelemetry(handler http.Handler) http.Handler {
	return otelhttp.NewHandler(mdw.nameAfterRoute(handler), "server")
}

// nameAfterRoute names the span after the chi route that served the request.
// chi records the route in the routing context it finds in the request, so one
// is put there when the router has none yet. A mount's catch-all is left to the
// mounted handler: GatewayRoute names the spans of the API.
func (mdw *Middleware) nameAfterRoute(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rctx := chi.RouteContext(r.Context())
		if rctx == nil {
			rctx = chi.NewRouteContext()
			rctx.Routes, _ = next.(chi.Routes)
			r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
		}

		next.ServeHTTP(w, r)

		if pattern := rctx.RoutePattern(); pattern != "" && !strings.HasSuffix(pattern, "/*") {
			nameSpan(r, pattern)
		}
	})
}

// GatewayRoute names the span after the gateway route that serves the request.
func GatewayRoute(next runtime.HandlerFunc) runtime.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request, pathParams map[string]string) {
		if pattern, ok := runtime.HTTPPattern(r.Context()); ok {
			nameSpan(r, pattern.String())
		}

		next(w, r, pathParams)
	}
}

func nameSpan(r *http.Request, pattern string) {
	span := trace.SpanFromContext(r.Context())
	span.SetName(r.Method + " " + pattern)
	span.SetAttributes(semconv.HTTPRoute(pattern))
}

// NewMiddleware returns a Middleware based on the type of monitor
func NewMiddleware(monitor monitoring.MonitorInterface, logger logging.LoggerInterface) *Middleware {
	mdw := new(Middleware)

	mdw.monitor = monitor

	mdw.logger = logger

	return mdw
}
