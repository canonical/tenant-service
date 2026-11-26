## Why

With `canonical/authorization-service#116`, the `authorization-service` listener exposes a `PermissionIngestionService/IngestPermissionUpdate` gRPC endpoint authenticated via Ory Hydra JWTs for standalone and hybrid deployments. To support deployments without Kafka, `tenant-service` needs a configurable `standalone` permission publishing mode that dispatches `PermissionUpdateEnvelope` messages over gRPC alongside the existing `platform` (Kafka) and `noop` modes.

## What Changes

- **BREAKING**: Replace `KAFKA_ENABLED` with `PERMISSIONS_PUBLISHING_MODE` supporting `noop` (default), `platform` (Kafka), and `standalone` (gRPC) in `internal/config` and `cmd`.
- Add `standalone` configuration parameters in `internal/config` for the target `authorization-service` gRPC listener (`AUTHORIZATION_SERVICE_GRPC_ADDRESS`, `AUTHORIZATION_SERVICE_GRPC_TIMEOUT`, `AUTHORIZATION_SERVICE_GRPC_WORKERS`, `AUTHORIZATION_SERVICE_TLS_ENABLED`) and Ory Hydra OAuth2 client credentials (`SERVICE_TOKEN_URL`, `SERVICE_CLIENT_ID`, `SERVICE_CLIENT_SECRET`, `SERVICE_TOKEN_SCOPES`).
- Implement an asynchronous `GRPCPublisher` in `internal/permissions` conforming to `permissions.Publisher`, backed by `v1.PermissionIngestionServiceClient` and an OAuth2 client-credentials token source that attaches and refreshes `authorization: Bearer <token>` metadata on outgoing gRPC calls.

## Non-goals

- Supporting simultaneous dual publishing (publishing to both Kafka and gRPC from `tenant-service` at the same time).
- Modifying the `PermissionUpdateEnvelope` schema, chunking size (`MaxOperationsPerEnvelope = 100`), or business events that trigger permission updates.
- Performing synchronous blocking gRPC calls on request critical paths.

## Capabilities

### New Capabilities
<!-- None -->

### Modified Capabilities
- `authorization-federation`: Support configurable `noop`, `platform` (Kafka), and `standalone` (gRPC with OAuth2 client-credentials authentication) permission event publishing modes.

## Impact

- Affected packages: `internal/config`, `internal/permissions`, `cmd`, `tests/e2e`.
- Dependencies: bumps `github.com/canonical/authorization-service` in `go.mod` to include `v1.PermissionIngestionServiceClient`.
- Configuration & scripts: updates `start.sh`, `README.md`, and `tests/e2e` environment configuration from `KAFKA_ENABLED` to `PERMISSIONS_PUBLISHING_MODE`.
