## Context

The tenant-service exposes both an HTTP/JSON gateway (via gRPC-Gateway) and a pure gRPC interface on separate ports. The HTTP path applies several middleware layers from `pkg/web/router.go`:

1. **Transaction middleware** (`db.TransactionMiddleware`) — wraps mutating requests in DB transactions
2. **Response time metrics** (`monitoring.NewMiddleware(...).ResponseTime()`) — Prometheus histogram observations
3. **Request logging** (`middleware.RequestLogger(...)`) — per-request structured logging

Direct gRPC calls (port 8001 by default) bypass all of these. The gRPC server in `cmd/serve.go` currently only has the auth interceptor (`authMiddleware.GRPCInterceptor`) and the OpenTelemetry stats handler (`otelgrpc.NewServerHandler()`). This creates an observability and consistency gap between the HTTP and gRPC transports.

Additionally, several files have already been partially implemented in the codebase:
- `internal/db/grpc_interceptor.go` — transaction interceptor (complete)
- `internal/logging/grpc_interceptor.go` — logging interceptor (complete)
- `internal/grpcutil/annotations.go` — annotation helper for read-only method detection (complete with tests)
- `go.mod` already has `go-grpc-prometheus` as an indirect dependency

The remaining work is to promote the Prometheus dependency to direct, wire all interceptors in `cmd/serve.go`, and add comprehensive unit tests.

## Goals / Non-Goals

**Goals:**
- Achieve parity between HTTP and gRPC transports for transactions, metrics, and logging
- Wrap mutating gRPC RPCs in database transactions with proper commit/rollback semantics
- Emit standard Prometheus metrics for all gRPC calls
- Log gRPC calls at Debug level with method, duration, and status code
- Use proto HTTP annotations to identify read-only methods (avoiding transaction overhead for GET-equivalent RPCs)
- Maintain backward compatibility — no API changes

**Non-Goals:**
- No transaction interceptor for streaming RPCs — streaming handlers manage their own transaction boundaries
- No changes to HTTP middleware stack
- No changes to proto definitions
- No custom monitoring interceptor — use `go-grpc-prometheus` instead
- No additional tracing spans — `otelgrpc.NewServerHandler()` already handles this

## Decisions

### 1. Read-Only Detection: Annotation-Based (Option B)

**Decision:** Use `google.api.http` annotations to identify read-only methods (those with `get:` rules).

**Rationale:**
- Option A (naming convention) is fragile — naming can drift
- Option C (always wrap with lazy-tx) is incorrect — `Statement(ctx)` triggers `lazyTx.get()` unconditionally, causing unnecessary BEGIN/COMMIT around SELECTs
- Option B uses existing annotations, mirrors the HTTP middleware's `r.Method == GET` check, and has zero per-request overhead (pre-built set at startup)

**Implementation:** `grpcutil.ReadOnlyMethods()` inspects the proto service descriptor at server construction time and builds a `map[string]bool` of full method names (e.g., `/identity.platform.api.tenant.TenantService/ListTenants`) that have HTTP GET annotations.

### 2. Commit/Rollback Signal: Handler Error

**Decision:** Check the `error` returned by the handler.
- `err == nil` → commit (if tx was started)
- `err != nil` → rollback

**Rationale:** This is the natural gRPC equivalent of HTTP's "status >= 400" check. The transaction interceptor returns `handlerErr` directly on error, preserving the original gRPC status code. Only if `txErr != nil` and `handlerErr == nil` does it wrap with `codes.Internal`.

### 3. Interceptor Ordering

**Decision:** `grpc.ChainUnaryInterceptor(loggingInterceptor, monitoringInterceptor, authInterceptor, txInterceptor)`

**Rationale (outermost to innermost):**
1. **Logging** — captures full request lifecycle including auth failures
2. **Monitoring** — measures total response time including auth overhead
3. **Auth** — rejects unauthenticated requests before hitting business logic
4. **Transaction** — wraps the handler call in a DB transaction

This matches the HTTP middleware ordering: `ResponseTime` → `RequestLogger` → `Authenticate` → `Transaction`.

### 4. Monitoring: `go-grpc-prometheus`

**Decision:** Use `grpc_prometheus.UnaryServerInterceptor` and `grpc_prometheus.Register(grpcServer)`.

**Rationale:**
- Already an indirect dependency — promote to direct
- Provides standard metrics: `grpc_server_handling_seconds`, `grpc_server_started_total`, `grpc_server_handled_total`
- Rich labels: `{grpc_service, grpc_method, grpc_type, grpc_code}`
- Automatically scraped at `/api/v0/metrics` via existing `promhttp.Handler()`
- Industry-standard dashboards and alerts exist out of the box

### 5. Logging: Custom Lightweight Interceptor

**Decision:** Custom 5-line interceptor in `internal/logging/grpc_interceptor.go`.

**Rationale:** Adding `go-grpc-middleware/v2` for a trivial debug log would be overkill. The interceptor logs `method`, `code`, and `duration` at Debug level, matching HTTP logging behavior.

### 6. Streaming Interceptors

**Decision:** Register `ChainStreamInterceptor` with logging, Prometheus metrics, and auth. The transaction interceptor is **intentionally excluded** from the stream chain.

**Rationale:** The service currently has no streaming RPCs, but registering stream interceptors defensively ensures any future streaming RPC automatically receives logging, metrics, and authentication without additional wiring changes. A single transaction wrapping the full lifetime of a stream is semantically incorrect — streams are unbounded and have per-message semantics that vary by endpoint. Streaming handlers must manage their own transaction boundaries explicitly.

```
unary:   logging → monitoring → auth → transaction
stream:  logging → monitoring → auth
```

## Risks / Trade-offs

| Risk | Mitigation |
|------|-----------|
| Read-only method detection fails if proto annotations are missing or incorrect | Fallback: if `ReadOnlyMethods` returns empty, all methods are treated as mutating (safe fail-closed). The HTTP GET annotations are already required for gRPC-Gateway routing, so they will be present. |
| Transaction wrapping adds latency to gRPC calls | Only mutating methods are wrapped. Read-only methods bypass transactions entirely. The HTTP path already has this overhead, so this is parity, not regression. |
| `go-grpc-prometheus` metric names differ from existing custom HTTP metrics | Acceptable — standard names are better long-term. Both will be scraped by the same endpoint. |
| Prometheus dependency promotion causes version conflicts | Current indirect version is `v1.2.0`. This is stable and widely used. Run `go mod tidy` after wiring to ensure consistency. |
| Handler error wrapped by transaction commit failure masks original error | Implementation preserves `handlerErr` and returns it directly. Only returns `Internal` if `txErr != nil && handlerErr == nil` (commit failed after successful handler). |

## Migration Plan

No special migration needed. This is a additive transport-layer enhancement:

1. `go get github.com/grpc-ecosystem/go-grpc-prometheus` (promote to direct dependency)
2. Wire interceptors in `cmd/serve.go`
3. Add unit tests for new interceptors
4. Run `make test` and `go vet ./...`
5. Verify metrics appear at `/api/v0/metrics` after deployment

Rollback: revert `cmd/serve.go` to previous `grpc.NewServer()` call.

## Open Questions

None. The existing spec file (`docs/specs/grpc-transaction-interceptor/spec.md`) and partially implemented files provide complete technical clarity.
