## 1. Dependencies and Configuration

- [x] 1.1 In `go.mod`, `go.sum`, and `vendor/`, bump `github.com/canonical/authorization-service` by commit SHA (`e761187f9a619954136eabf3cfa3d23ad1e47aea` or latest `main` commit) so `v1.PermissionIngestionServiceClient` is available, then run `go mod tidy` and `make vendor`.
- [x] 1.2 In `internal/config/specs.go`, replace `KafkaEnabled` with `PermissionsPublishingMode` (`envconfig:"permissions_publishing_mode" default:"noop"`), drop unused `KafkaClientID`, and add standalone gRPC and OAuth2 client-credentials settings (`AuthorizationServiceGRPCAddress`, `AuthorizationServiceGRPCTimeout` default `5s`, `AuthorizationServiceGRPCWorkers` default `5`, `AuthorizationServiceTLSEnabled` default `false`, `ServiceTokenURL`, `ServiceClientID`, `ServiceClientSecret`, `ServiceTokenScopes`).

## 2. Standalone gRPC Permission Publisher

- [x] 2.1 In `internal/permissions/publisher.go` and `internal/permissions/interfaces.go`, extract shared `PermissionUpdateEnvelope` construction (defaulting `service` to `"tenant-service"`, attaching OpenTelemetry trace ID as `correlation_id`, and splitting operations into chunks of at most `MaxOperationsPerEnvelope = 100`) and update `Publisher` doc comments to cover both Kafka and gRPC publishing modes.
- [x] 2.2 In `internal/permissions/grpc_publisher.go`, implement `Credentials`, `TokenSource` (with explicit `*http.Client{Timeout: 5 * time.Second}` on `oauth2.HTTPClient`), custom `credentials.PerRPCCredentials` (attaching the OAuth2 access token to the `authorization` metadata header, returning `tlsEnabled` from `RequireTransportSecurity()`, and mapping 400/401 `oauth2.RetrieveError` to `codes.Unauthenticated` and transient token errors to `codes.Unavailable`), `NewGRPCConn`, `DialOptions` (with `UNAVAILABLE` retry service config for `PermissionIngestionService/IngestPermissionUpdate`), and `GRPCPublisher` (with a concurrency-safe bounded worker queue, `permission_events_total` metrics, and graceful drain on `Close()`).
- [x] 2.3 In `internal/permissions/grpc_publisher_test.go` and `internal/permissions/publisher_test.go`, add unit and integration tests covering `TokenSource` caching and refresh, Bearer metadata attachment, `UNAVAILABLE` retries, token failure classification, 100-operation chunking, `permission_events_total` metrics, graceful drain on `Close()`, and concurrent/post-`Close()` `Publish()` calls without panicking.

## 3. Server Wiring

- [x] 3.1 In `cmd/serve.go` and `cmd/serve_test.go`, wire `specs.PermissionsPublishingMode` (`noop`, `platform`, `standalone`) with validation of mode-specific required settings and comma/whitespace scope parsing for `ServiceTokenScopes`, ensure `grpcServer.GracefulStop()` (with deadline fallback to `grpcServer.Stop()`) completes before `publisher.Close()` during shutdown, and add unit tests for publisher mode selection and validation.

## 4. E2E Tests, Documentation, and Canonical Spec Purpose

- [x] 4.1 In `tests/e2e/setup_test.go`, replace `KAFKA_ENABLED` with `PERMISSIONS_PUBLISHING_MODE: "noop"`.
- [x] 4.2 In `start.sh` and `README.md`, replace `KAFKA_ENABLED` with `PERMISSIONS_PUBLISHING_MODE` and document the `standalone` mode gRPC and OAuth2 client-credentials environment variables.
- [x] 4.3 In `openspec/specs/authorization-federation/spec.md`, update the `## Purpose` section so the capability rationale reflects configurable `noop`, `platform` (Kafka), and `standalone` (Authorization Service gRPC ingestion with OAuth2 client-credentials authentication) permission event publishing modes.

## 5. Verification

- [x] 5.1 Run `openspec validate --all --strict` and confirm zero validation errors.
- [x] 5.2 Run `make mocks`, `make build`, `go vet ./...`, and `make test-unit` (`go test ./... -short -race`) and confirm all checks pass.
- [x] 5.3 Run `go vet ./...` in `tests/e2e` to confirm the E2E module compiles cleanly.

## Implementation Notes

- Upgraded `github.com/canonical/authorization-service` to `v1.0.1-0.20261110160538-e761187f9a61` (commit `e761187f9a619954136eabf3cfa3d23ad1e47aea`) and synced `vendor/` in both root and `tests/e2e/` modules (`vendor/` is gitignored).
- Replaced `KafkaEnabled` with `PermissionsPublishingMode` (`default:"noop"`) in `internal/config/specs.go`, removed unused `KafkaClientID` so `envelope.Service` is always `"tenant-service"`, and added `AuthorizationServiceGRPCAddress`, `AuthorizationServiceGRPCTimeout`, `AuthorizationServiceGRPCWorkers` (default `5`), `AuthorizationServiceTLSEnabled`, `ServiceTokenURL`, `ServiceClientID`, `ServiceClientSecret`, and `ServiceTokenScopes`.
- Extracted `buildEnvelopes`, `eventRecorder`, and `NewPublisher` into `internal/permissions/factory.go`, updated `Publisher` doc comments in `internal/permissions/interfaces.go`, moved OAuth2 client-credentials gRPC dialing (`Credentials`, `TokenSource` with `AuthStyleInHeader`, `bearerPerRPCCredentials`, `Dial`) to `internal/grpcutil/oauth2.go`, and implemented `GRPCPublisher` in `internal/permissions/grpc_publisher.go` with tenant-sharded worker queues (default 5 workers), `waitForReady` + `UNAVAILABLE`/`INTERNAL` retries, non-blocking `0s` enqueue, `sync.Once` idempotent `Close()` with a `10s` bounded drain, and `permission_events_total` metrics.
- Added `PermissionsConfig()` and `ParseScopes` to `internal/config/specs.go` (updating default `KAFKA_PERMISSIONS_TOPIC` to `permissions.tenant-service`), wired `permissions.NewPublisher` and concurrent HTTP/gRPC graceful shutdown in `cmd/serve.go`, and updated unit tests in `cmd/serve_test.go`.
- Updated `tests/e2e/setup_test.go`, `start.sh`, `README.md`, and `openspec/specs/authorization-federation/spec.md`.
- Verified with `openspec validate --all --strict` (15/15 passed), `make mocks`, `make build`, `go vet ./...`, `go test ./... -short -race`, and `go vet ./...` in `tests/e2e`.
