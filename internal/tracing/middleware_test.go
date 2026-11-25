// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package tracing

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/canonical/tenant-service/internal/logging"
	"github.com/canonical/tenant-service/internal/monitoring"
	chi "github.com/go-chi/chi/v5"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	semconv "go.opentelemetry.io/otel/semconv/v1.18.0"
)

func TestOpenTelemetry_SpanNamedAfterRoute(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter)))
	t.Cleanup(func() { otel.SetTracerProvider(previous) })

	gateway := runtime.NewServeMux(runtime.WithMiddlewares(GatewayRoute))
	if err := gateway.HandlePath(http.MethodGet, "/api/v0/tenants/{tenant_id}/users", func(w http.ResponseWriter, _ *http.Request, _ map[string]string) {}); err != nil {
		t.Fatal(err)
	}

	router := chi.NewRouter()
	router.Post("/api/v0/webhooks/{name}", func(http.ResponseWriter, *http.Request) {})
	router.Mount("/", gateway)

	logger := logging.NewNoopLogger()
	handler := NewMiddleware(monitoring.NewNoopMonitor("test", logger), logger).OpenTelemetry(router)

	tests := []struct {
		method, target, route string
	}{
		{http.MethodPost, "/api/v0/webhooks/registration", "/api/v0/webhooks/{name}"},
		{http.MethodGet, "/api/v0/tenants/abc/users?page_token=x", "/api/v0/tenants/{tenant_id=*}/users"},
	}

	for _, tt := range tests {
		t.Run(tt.route, func(t *testing.T) {
			exporter.Reset()
			handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(tt.method, tt.target, nil))

			spans := exporter.GetSpans()
			if len(spans) != 1 {
				t.Fatalf("expected one span, got %d", len(spans))
			}
			if got, want := spans[0].Name, tt.method+" "+tt.route; got != want {
				t.Errorf("span name: got %q, want %q", got, want)
			}

			route := ""
			for _, attr := range spans[0].Attributes {
				if attr.Key == semconv.HTTPRouteKey {
					route = attr.Value.AsString()
				}
			}
			if route != tt.route {
				t.Errorf("http.route: got %q, want %q", route, tt.route)
			}
		})
	}
}
