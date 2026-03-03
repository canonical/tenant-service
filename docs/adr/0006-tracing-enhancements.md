# ADR 0006: Tracing Enhancements — Error Recording and gRPC Trace Propagation

Date: 2026-03-03

## Status

Accepted

## Context

OpenTelemetry tracing was already wired throughout the service; every method in
`pkg/tenant/service.go`, `pkg/webhooks/service.go`, and
`pkg/authentication/middleware.go` started a span with `tracer.Start(...)` and
deferred `span.End()`. However, two gaps made the traces effectively silent in
the face of errors:

**Gap 1 — Spans never recorded errors.**
Error paths returned errors to the caller but never called `span.RecordError(err)`
or `span.SetStatus(codes.Error, ...)`. From the perspective of any OTel backend
(e.g. Jaeger), every completed span appeared successful, making it impossible to
identify failing operations in traces without correlating with logs.

**Gap 2 — Inbound gRPC calls could not propagate trace context.**
The gRPC server in `cmd/serve.go` was created with a single unary interceptor for
authentication:

```go
grpc.NewServer(
    grpc.UnaryInterceptor(authMiddleware.GRPCInterceptor),
)
```

There was no mechanism to extract W3C `traceparent` / Jaeger trace headers from
incoming gRPC metadata, so traces originating in a gRPC client could not be
composed with server-side spans. Every inbound gRPC call started a disconnected
root span.

## Decision

### Decision A — Record errors on all span error paths

On every error-returning path in all service and middleware methods, add:

```go
span.RecordError(err)
span.SetStatus(codes.Error, err.Error())
```

immediately before the return statement. The `go.opentelemetry.io/otel/codes`
package was already vendored; no new dependency is required for this change.

The scope covers:
- All 10 methods in `pkg/tenant/service.go`
- Both methods in `pkg/webhooks/service.go`
- Both interceptors in `pkg/authentication/middleware.go` (`Authenticate` HTTP
  middleware and `GRPCInterceptor`), including auth-failure paths that do not
  have a natural `error` variable (where a local `err` is constructed before
  recording)

The **original error** (not the opaque, user-facing wrapper) is recorded on the
span so that the trace backend receives the full diagnostic detail, consistent
with the pattern of recording the real cause while returning a redacted message
to the caller.

### Decision B — Wire `otelgrpc.NewServerHandler` as a gRPC stats handler

Add `go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc`
at **v0.65.0** (aligned with the existing `otelhttp` v0.65.0 dependency) and
register its server handler via `grpc.StatsHandler`:

```go
grpc.NewServer(
    grpc.StatsHandler(otelgrpc.NewServerHandler()),
    grpc.UnaryInterceptor(authMiddleware.GRPCInterceptor),
)
```

The stats-handler approach (rather than a unary interceptor) is preferred because:
- It is the recommended approach in otelgrpc v0.65.0+; the older
  `UnaryServerInterceptor` / `StreamServerInterceptor` functions were removed in
  favour of this API.
- It handles both unary and streaming RPCs from a single registration point.
- It operates at the transport layer, extracting trace context before any
  application-level interceptor runs, so authentication failures appear as
  child events of the inbound RPC span rather than as orphaned spans.

## Consequences

- **Traces now surface errors.** Any span whose operation fails is marked with
  `StatusCode=ERROR` and carries the error event, making failing operations
  immediately visible in Jaeger / any OTel-compatible backend without cross-
  referencing logs.
- **Distributed traces compose end-to-end.** gRPC callers that propagate a
  `traceparent` header (W3C Trace Context) or Jaeger `uber-trace-id` will have
  their server-side spans correctly nested under their client span.
- **New vendored dependency.** `go.opentelemetry.io/contrib/instrumentation/
  google.golang.org/grpc/otelgrpc v0.65.0` is added to `go.mod` and vendored.
  It is a first-party OTel contrib package from the `open-telemetry` organisation,
  consistent with all existing OTel dependencies.
- **Auth spans are now correctly scoped.** The `authentication.Middleware.
  GRPCInterceptor` span runs as a child of the inbound RPC span created by the
  stats handler, providing accurate latency attribution.
