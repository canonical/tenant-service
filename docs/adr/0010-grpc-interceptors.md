# 10. gRPC Interceptors for Middleware Parity

Date: 2026-05-05

## Status

Accepted

## Context

The tenant-service exposes both an HTTP/JSON gateway (via grpc-gateway) and a pure gRPC
interface. The HTTP path applies middleware for transaction management, Prometheus metrics,
and request logging. Direct gRPC calls bypass the HTTP middleware stack entirely, creating
inconsistencies:

- Mutating RPCs over gRPC are not wrapped in a database transaction.
- gRPC calls don't emit Prometheus response time metrics.
- gRPC calls aren't logged per-request.

Tracing is already consistent: `otelgrpc.NewServerHandler()` provides per-RPC OpenTelemetry
spans on the gRPC path, equivalent to the HTTP tracing middleware.

## Decision

### 1. Transaction interceptor applies to unary RPCs only

The interceptor inspects `google.api.http` annotations at startup to build a set of
read-only method names (those mapped to HTTP GET). Mutating unary RPCs are wrapped in
`DBClient.WithTx()`. Read-only unary RPCs bypass the transaction entirely.

**Streaming RPCs are excluded from automatic transaction wrapping.** A single transaction
spanning the full lifetime of a stream is semantically incorrect: streams are unbounded,
may process many independent messages, and have per-message commit semantics that vary
by endpoint. Streaming handlers are expected to manage their own transaction boundaries
explicitly (e.g. per message or per logical batch).

`ReadOnlyMethods` returns `(map[string]bool, error)`. A non-nil error (e.g., proto file
not found in the registry, or unexpected `Metadata` type) is treated as fatal at startup
so mis-configuration is caught before the server accepts traffic.

**Alternatives rejected:**

- **Convention-based skip (Option A):** Skip methods whose name starts with `List` or
  `Get`. Rejected because naming conventions are fragile and may drift.
- **Always wrap with lazy-tx (Option C):** Rely on the `lazyTx` mechanism so no DB
  roundtrip occurs if no writes happen. Rejected because `Statement(ctx)` triggers
  `lazyTx.get()` unconditionally when a `lazyTx` is in the context — this means
  read-only RPCs would start unnecessary transactions (BEGIN/COMMIT around SELECTs),
  inconsistent with the HTTP middleware which skips GET/HEAD entirely.

**Rationale for Option B:**

- Every RPC already has a `google.api.http` annotation (required for the gateway).
- The HTTP verb directly indicates read-only vs mutating — mirrors the HTTP middleware's
  `r.Method == GET` check.
- One-time startup cost to build the method set; zero per-request overhead (map lookup).
- Adding a new RPC with a `get:` annotation automatically gets the correct behaviour.

### 2. Monitoring uses `go-grpc-prometheus` (off-the-shelf)

Use `grpc_prometheus.UnaryServerInterceptor` and `grpc_prometheus.StreamServerInterceptor`
from `github.com/grpc-ecosystem/go-grpc-prometheus` instead of a custom interceptor.

**Rationale:**

- Already an indirect dependency — promote to direct.
- Provides standard metrics (`grpc_server_handling_seconds`, `grpc_server_started_total`,
  `grpc_server_handled_total`) with labels `{grpc_service, grpc_method, grpc_type, grpc_code}`.
- Automatically scraped via `promhttp.Handler()` at `/api/v0/metrics`.
- Industry-standard: Grafana dashboards and alert rules exist out of the box.
- No custom code to maintain.

### 3. Logging is a custom interceptor

A custom interceptor logs method, duration, and gRPC status code. Log level follows
severity: server-side error codes (`Internal`, `Unknown`, `Unavailable`, `DataLoss`,
`DeadlineExceeded`) are logged at **Error** level; all other outcomes at **Debug** level.
The same escalation applies to both unary and streaming interceptors.

The `logger LoggerInterface` parameter is retained in the transaction interceptor even
though it currently only logs on commit failure. This follows the project-wide o11y
constructor convention (`tracer, monitor, logger` as last args) and keeps the door open
for future structured logging without a signature change.

**Rationale:**

- Adding `go-grpc-middleware/v2` for a 5-line function is unnecessary dependency bloat.
- Error-level logging for 5xx-equivalent codes makes server failures visible without
  enabling debug logging in production.

### 4. Interceptor chain order matches HTTP middleware order

```
logging → monitoring → auth → transaction
```

(Outermost to innermost.) Logging and monitoring observe the full lifecycle including
auth failures. Auth rejects before business logic. Transaction wraps only the handler.

### 5. Both unary and streaming interceptors are registered

`ChainUnaryInterceptor` and `ChainStreamInterceptor` receive equivalent chains for
logging, monitoring, and auth. The stream chain intentionally omits the transaction
interceptor — transaction management for streaming handlers is the responsibility of
the handler implementation.

```
unary:   logging → monitoring → auth → transaction
stream:  logging → monitoring → auth
```

The service currently has no streaming RPCs; the stream interceptors are registered
defensively so a future streaming RPC automatically receives logging, metrics, and
authentication without additional wiring.

## Consequences

### Positive

- gRPC and HTTP paths have equivalent observability and transactional guarantees.
- No custom monitoring code: `go-grpc-prometheus` is battle-tested and maintained.
- New RPCs automatically get correct transaction behaviour via their `google.api.http`
  annotation — no manual registration needed.
- Read-only RPCs avoid unnecessary database transactions.
- Startup fails fast if proto descriptor resolution fails, rather than silently
  wrapping all methods in transactions.
- Server-side failures are surfaced at Error log level without requiring debug logging.

### Negative

- The transaction interceptor is coupled to the `google.api.http` annotation. A unary RPC
  without this annotation would be treated as mutating (safe default). This is acceptable
  because all RPCs must have the annotation for the gateway to function.
- Streaming handlers must manage their own transaction boundaries — there is no safety net
  from the interceptor chain.
- gRPC metrics use different label names (`grpc_method`) than HTTP metrics (`route`).
  Dashboards need separate panels for gRPC vs HTTP. This is standard practice.
