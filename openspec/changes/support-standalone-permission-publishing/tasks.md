## 1. Dependencies and Configuration

- [ ] 1.1 In `go.mod`, `go.sum`, and `vendor/`, bump `github.com/canonical/authorization-service` by commit SHA (`e761187f9a619954136eabf3cfa3d23ad1e47aea` or latest `main` commit) so `v1.PermissionIngestionServiceClient` is available, then run `go mod tidy` and `make vendor`.
- [ ] 1.2 In `internal/config/specs.go`, replace `KafkaEnabled` with `PermissionsPublishingMode` (`envconfig:"permissions_publishing_mode" default:"noop"`) and add standalone gRPC and OAuth2 client-credentials settings (`AuthorizationServiceGRPCAddress`, `AuthorizationServiceGRPCTimeout` default `5s`, `AuthorizationServiceTLSEnabled` default `false`, `ServiceTokenURL`, `ServiceClientID`, `ServiceClientSecret`, `ServiceTokenScopes`).

## 2. Standalone gRPC Permission Publisher

- [ ] 2.1 In `internal/permissions/publisher.go` and `internal/permissions/interfaces.go`, extract shared `PermissionUpdateEnvelope` construction (defaulting `service` to `"tenant-service"`, attaching OpenTelemetry trace ID as `correlation_id`, and splitting operations into chunks of at most `MaxOperationsPerEnvelope = 100`) and update `Publisher` doc comments to cover both Kafka and gRPC publishing modes.
- [ ] 2.2 In `internal/permissions/grpc_publisher.go`, implement `Credentials`, `TokenSource` (with explicit `*http.Client{Timeout: 5 * time.Second}` on `oauth2.HTTPClient`), custom `credentials.PerRPCCredentials` (attaching the OAuth2 access token to the `authorization` metadata header, returning `tlsEnabled` from `RequireTransportSecurity()`, and mapping 400/401 `oauth2.RetrieveError` to `codes.Unauthenticated` and transient token errors to `codes.Unavailable`), `NewGRPCConn`, `DialOptions` (with `UNAVAILABLE` retry service config for `PermissionIngestionService/IngestPermissionUpdate`), and `GRPCPublisher` (with a concurrency-safe bounded worker queue, `permission_events_total` metrics, and graceful drain on `Close()`).
- [ ] 2.3 In `internal/permissions/grpc_publisher_test.go` and `internal/permissions/publisher_test.go`, add unit and integration tests covering `TokenSource` caching and refresh, Bearer metadata attachment, `UNAVAILABLE` retries, token failure classification, 100-operation chunking, `permission_events_total` metrics, graceful drain on `Close()`, and concurrent/post-`Close()` `Publish()` calls without panicking.

## 3. Server Wiring

- [ ] 3.1 In `cmd/serve.go` and `cmd/serve_test.go`, wire `specs.PermissionsPublishingMode` (`noop`, `platform`, `standalone`) with validation of mode-specific required settings and comma/whitespace scope parsing for `ServiceTokenScopes`, ensure `grpcServer.GracefulStop()` (with deadline fallback to `grpcServer.Stop()`) completes before `publisher.Close()` during shutdown, and add unit tests for publisher mode selection and validation.

## 4. E2E Tests, Documentation, and Canonical Spec Purpose

- [ ] 4.1 In `tests/e2e/setup_test.go`, replace `KAFKA_ENABLED` with `PERMISSIONS_PUBLISHING_MODE: "noop"`.
- [ ] 4.2 In `start.sh` and `README.md`, replace `KAFKA_ENABLED` with `PERMISSIONS_PUBLISHING_MODE` and document the `standalone` mode gRPC and OAuth2 client-credentials environment variables.
- [ ] 4.3 In `openspec/specs/authorization-federation/spec.md`, update the `## Purpose` section so the capability rationale reflects configurable `noop`, `platform` (Kafka), and `standalone` (Authorization Service gRPC ingestion with OAuth2 client-credentials authentication) permission event publishing modes.

## 5. Verification

- [ ] 5.1 Run `openspec validate --all --strict` and confirm zero validation errors.
- [ ] 5.2 Run `make mocks`, `make build`, `go vet ./...`, and `make test-unit` (`go test ./... -short -race`) and confirm all checks pass.
- [ ] 5.3 Run `go vet ./...` in `tests/e2e` to confirm the E2E module compiles cleanly.

## Implementation Notes

- Pending implementation.
