## Context

See `proposal.md` for motivation. Today `tenant-service` defines `permissions.Publisher` (`internal/permissions/interfaces.go`) with `KafkaPublisher` and `NoopPublisher`, selected via `KAFKA_ENABLED` (`internal/config/specs.go`, `cmd/serve.go`). Callers (`pkg/tenant/service.go`, `pkg/webhooks/service.go`) invoke `publisher.Publish(ctx, tenantID, ops...)` on the critical path of tenant registration, member provisioning/invitation, and tenant deletion, expecting non-blocking asynchronous dispatch.

Meanwhile, `sso-service` (`internal/tenants/grpc.go`) established the Canonical Identity pattern for service-to-service gRPC connections authenticated with Ory Hydra OAuth2 client credentials (`golang.org/x/oauth2/clientcredentials` + `oauth2.ReuseTokenSource` + a unary `bearerInterceptor`).

## Goals / Non-Goals

**Goals:**
- Provide a unified `PERMISSIONS_PUBLISHING_MODE` (`noop`, `platform`, `standalone`) replacing `KAFKA_ENABLED`.
- Implement an asynchronous `GRPCPublisher` in `internal/permissions` that dispatches `PermissionUpdateEnvelope` chunks to `v1.PermissionIngestionServiceClient.IngestPermissionUpdate` off the request critical path while preserving FIFO ordering and draining safely on shutdown without channel-close data races.
- Authenticate outgoing gRPC requests using Ory Hydra OAuth2 client credentials with automatic token caching, explicit HTTP timeout, and gRPC retry compatibility.

**Non-Goals:**
- Changing the `permissions.Publisher` interface signature or caller call sites in `pkg/tenant` and `pkg/webhooks`.
- Adding persistent local disk queueing in `tenant-service` for failed gRPC deliveries.

## Decisions

### 1. Explicit `PERMISSIONS_PUBLISHING_MODE` Replacing `KAFKA_ENABLED`
- **Decision**: Replace `KAFKA_ENABLED` with `PERMISSIONS_PUBLISHING_MODE` (`default:"noop"`), accepting `noop`, `platform`, or `standalone`.
- **Rationale**: Permission enforcement itself is not disabled when publishing is turned off in isolated local development; only upstream permission event publishing is bypassed. Using `PERMISSIONS_PUBLISHING_MODE` makes the scope explicit while aligning mode names (`platform`, `standalone`) with `authorization-service#116`.
- **Alternatives considered**: Keeping `KAFKA_ENABLED` alongside a mode flag (rejected: redundant and ambiguous when both are set; the platform is not yet deployed in production so breaking `KAFKA_ENABLED` now is clean).

### 2. Asynchronous Tenant-Sharded Worker Pool in `GRPCPublisher` with Concurrency-Safe Shutdown
- **Decision**: `GRPCPublisher.Publish` builds `PermissionUpdateEnvelope` chunks synchronously (setting `envelope.Service` to `"tenant-service"`, capturing the caller's OpenTelemetry trace ID as `correlation_id`, and splitting at `MaxOperationsPerEnvelope = 100`), then hashes `tenantID` (32-bit FNV-1a) onto one of `N` bounded in-memory worker channels (configurable via `AUTHORIZATION_SERVICE_GRPC_WORKERS`, default `5`, buffer size 1024 per worker shard), each consumed by a dedicated background worker goroutine.
  - **Concurrency safety (`Publish` vs. `Close`)**: Guard `closed` state and channel enqueueing with a `sync.RWMutex` (read-locked during `Publish` enqueue, write-locked in `Close` before setting `closed = true` and closing all shard channels). `Publish` enqueues non-blockingly (`0s` wait); if `closed` is true or the tenant shard's queue is full, `Publish` logs an error and records `permission_events_total{result="failure", stage="write"}` without blocking request handlers or panicking on a closed channel.
  - **Server shutdown ordering in `cmd/serve.go`**: Stop `grpcServer` (`GracefulStop()` with deadline fallback to `Stop()`) concurrently with `srv.Shutdown(ctx)` before `serve()` returns and triggers `defer publisher.Close()`, which drains queued envelopes across all workers up to a bounded `10s` deadline before cancelling remaining in-flight RPCs and OAuth2 token requests.
- **Rationale**: Matches `KafkaPublisher`'s `kafka.Writer{Async: true}` + `kafka.Hash` behavior so network round-trips, OAuth2 token refreshes, and multi-envelope tenant deletions do not block HTTP/gRPC request handlers. Sharding by `tenantID` across multiple worker goroutines parallelizes steady-state delivery and shutdown draining across tenants while strictly preserving per-tenant FIFO ordering.
- **Observability**: Enqueue failures record `permission_events_total{result="failure", stage="write"}`. RPC completions record `permission_events_total{result="success"|"failure", stage="delivery"}`, matching `KafkaPublisher` metrics.
- **Alternatives considered**: Synchronous unary gRPC calls inside `Publish()` (rejected: adds network + token fetch latency to the critical path, especially on large tenant deletions with multiple 100-op envelopes).

### 3. Hydra OAuth2 Client Credentials via Custom `credentials.PerRPCCredentials`
- **Decision**: Build an `oauth2.TokenSource` via `clientcredentials.Config` (with `AuthStyle: oauth2.AuthStyleInHeader`), injecting an explicit `*http.Client{Timeout: 5 * time.Second}` via `context.WithValue(baseCtx, oauth2.HTTPClient, ...)` in `internal/grpcutil` so token refreshes never fall back to `http.DefaultClient` and cancel cleanly on shutdown. Wrap the `oauth2.TokenSource` in a custom `credentials.PerRPCCredentials` implementation whose `RequireTransportSecurity()` returns `tlsEnabled` and whose `GetRequestMetadata` respects per-RPC `ctx.Done()` before attaching the OAuth2 access token to the `authorization` metadata header, mapping HTTP 400/401 `oauth2.RetrieveError`s to `codes.Unauthenticated` and wrapping transient token-endpoint errors as `codes.Unavailable`. Configure gRPC service config retries on `UNAVAILABLE` and `INTERNAL` with `waitForReady: true` (up to 3 attempts with exponential backoff) for `authorization.service.api.v1.PermissionIngestionService/IngestPermissionUpdate`.
- **Rationale**: Standard `google.golang.org/grpc/credentials/oauth.TokenSource` hardcodes `RequireTransportSecurity() == true` and refuses plaintext connections, whereas a custom `UnaryClientInterceptor` runs outside gRPC's `serviceConfig` retry loop. Custom `credentials.PerRPCCredentials` works over both plaintext and TLS, executes on every retry attempt, and allows transient Hydra token-endpoint blips (`codes.Unavailable`) to participate in gRPC retries. `IngestPermissionUpdate` is idempotent by `(service, idempotency_key)`, making automatic retry on `UNAVAILABLE` and `INTERNAL` safe.
- **Configuration**: Reuse the `sso-service` environment variable convention (`SERVICE_TOKEN_URL`, `SERVICE_CLIENT_ID`, `SERVICE_CLIENT_SECRET`, `SERVICE_TOKEN_SCOPES` parsed in `internal/config`) paired with `AUTHORIZATION_SERVICE_GRPC_ADDRESS`, `AUTHORIZATION_SERVICE_GRPC_TIMEOUT` (default `5s`), `AUTHORIZATION_SERVICE_GRPC_WORKERS` (default `5`), and `AUTHORIZATION_SERVICE_TLS_ENABLED` (default `false`, with a startup warning when disabled).

## Risks / Trade-offs

- **[Risk] In-memory queue drop on ungraceful process crash (`SIGKILL`)** → Mitigation: Same trade-off as `kafka.Writer{Async: true}`; on normal `SIGINT`/`SIGTERM`, `cmd/serve.go` drains the HTTP and gRPC servers concurrently first and then calls `publisher.Close()`, which drains the sharded worker queues in parallel up to a bounded `10s` deadline before exiting.
- **[Risk] Bounded queue backpressure if `authorization-service` is unreachable for an extended period** → Mitigation: Buffer up to 1024 envelopes per worker shard, bound each RPC by `AUTHORIZATION_SERVICE_GRPC_TIMEOUT` (default `5s`), and drop non-blockingly (`0s`) when a shard queue is saturated, logging and incrementing `permission_events_total{result="failure", stage="write"}`.
