## Why

The tenant-service exposes both an HTTP/JSON gateway and a pure gRPC interface. The HTTP path applies several middleware layers (transaction management, Prometheus response time, request logging), but direct gRPC calls bypass all of these. This means mutating gRPC RPCs don't get automatic transaction wrapping, gRPC calls don't emit metrics, and gRPC calls aren't logged per-request. This feature adds gRPC unary interceptors to achieve parity with the HTTP middleware stack.

## What Changes

- Add a gRPC transaction interceptor that wraps mutating RPCs in database transactions (commit on success, rollback on error), skipping read-only methods based on HTTP annotation inspection.
- Add a gRPC logging interceptor that logs method, duration, and status code at Debug level.
- Integrate `go-grpc-prometheus` for standard gRPC server metrics (`grpc_server_handling_seconds`, `grpc_server_started_total`, `grpc_server_handled_total`).
- Wire all interceptors in `cmd/serve.go` with proper ordering: logging → monitoring → auth → transaction.
- Add unit tests for transaction interceptor (commit/rollback behavior, read-only skip, error preservation).
- Add unit tests for logging interceptor and annotation helper.

## Capabilities

### New Capabilities
- `grpc-transaction-interceptor`: Automatic database transaction wrapping for mutating gRPC RPCs, with annotation-based read-only detection.
- `grpc-monitoring`: Standard Prometheus metrics for gRPC server calls via `go-grpc-prometheus`.
- `grpc-logging`: Per-request debug logging for gRPC method, duration, and status code.

### Modified Capabilities
- None. No existing capability requirements change; this is a transport-layer enhancement.

## Impact

- **New files**: `internal/db/grpc_interceptor.go`, `internal/logging/grpc_interceptor.go`, `internal/grpcutil/annotations.go`
- **Modified files**: `cmd/serve.go` (interceptor wiring), `go.mod` (new dependency)
- **Dependencies**: `github.com/grpc-ecosystem/go-grpc-prometheus` (promote from indirect to direct)
- **APIs**: No proto or HTTP API changes. gRPC clients get transaction safety and observability automatically.
