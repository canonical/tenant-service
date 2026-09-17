## Why

Currently, `tenant-service` directly queries and mutates OpenFGA tuple stores in-process. To align with the Canonical Identity Platform architecture, external authorization checks are offloaded upstream to the Envoy API Gateway and `Authorization Service`, while `tenant-service` publishes permission mutations asynchronously via Kafka. This decouples `tenant-service` from OpenFGA infrastructure and standardizes centralized authorization.

## What Changes

- **Publish Permission Events**: Publish protobuf `PermissionUpdateEnvelope` messages to the Kafka topic `tenant-service.permissions` on tenant and membership mutations.
- **Remove In-Process Authorization Checks**: Rely on upstream Envoy/Authorization Service gateway enforcement and remove internal OpenFGA check calls.
- **Remove Direct OpenFGA Dependencies**: Drop `internal/openfga`, `internal/authorization`, `cmd/createFgaModel.go`, and OpenFGA SDK dependencies.
- **Support STS Token Verification**: Update `pkg/authentication` to support STS-issued bearer tokens with RS256/ES256 algorithms and permissive subject/scope validation for federated ingress.
- **Update Service Configuration**: Replace `OPENFGA_*` environment variables with `KAFKA_*` configuration in `internal/config/specs.go`.

## Non-Goals

- Modifying upstream `Authorization Service` route rules (maintained separately).
- Implementing transactional outbox tables in PostgreSQL (fire-and-forget asynchronous publishing with retry and error logging is used).
- Changing external gRPC/HTTP API contracts or client protobuf interfaces.

## Capabilities

### New Capabilities
- `authorization-federation`: Asynchronous permission update publishing via Kafka and reliance on upstream gateway authorization.

### Modified Capabilities
<!-- None -->

## Impact

- **Affected Packages**: `pkg/tenant`, `pkg/webhooks`, `pkg/authentication`, `internal/config`, `internal/authorization` (removed), `internal/openfga` (removed), `internal/permissions` (new).
- **Dependencies**: Remove `github.com/openfga/go-sdk` and `github.com/openfga/language/pkg/go`. Add `github.com/segmentio/kafka-go` and `github.com/canonical/authorization-service/api/v1`.
- **Infrastructure**: Requires Kafka broker connection (`KAFKA_BROKERS`, `KAFKA_PERMISSIONS_TOPIC`). Removes OpenFGA store connection requirements.
