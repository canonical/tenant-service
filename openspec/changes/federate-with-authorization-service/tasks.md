## 1. Dependencies and Configuration

- [x] 1.1 Update `go.mod` to add `github.com/segmentio/kafka-go` and `github.com/canonical/authorization-service/api/v1`, and remove `github.com/openfga/go-sdk` and `github.com/openfga/language/pkg/go`.
- [x] 1.2 Update `internal/config/specs.go` to replace `OPENFGA_*` options with `KAFKA_*` configuration (`Enabled`, `Brokers`, `PermissionsTopic`, `ClientID`).

## 2. Authentication and STS Token Verification

- [x] 2.1 Update `pkg/authentication/provider.go` to support `RS256` and `ES256` signing algorithms and set `SkipIssuerCheck: true` when a manual JWKS URL is provided.
- [x] 2.2 Update `pkg/authentication/verifier.go` to permit validly signed tokens when neither allowed subjects nor required scopes are configured.
- [x] 2.3 Update unit tests in `pkg/authentication/verifier_test.go` and `pkg/authentication/middleware_test.go` to cover STS token verification and algorithm support.

## 3. Permissions Event Publisher

- [x] 3.1 Create `internal/permissions/interfaces.go` and `internal/permissions/publisher.go` implementing asynchronous Kafka event publishing for `PermissionUpdateEnvelope` messages keyed by tenant ID and providing a `NoopPublisher`.
- [x] 3.2 Create `internal/permissions/publisher_test.go` and generate GoMock mocks for `internal/permissions/interfaces.go`.

## 4. Service Layer Migration

- [x] 4.1 Refactor `pkg/tenant/service.go` to replace OpenFGA calls with asynchronous `permissions.Publisher` event publishing and remove internal authorization check queries.
- [x] 4.2 Refactor `pkg/webhooks/service.go` to publish owner permission events asynchronously via `permissions.Publisher` on self-registration.
- [x] 4.3 Update unit tests in `pkg/tenant/service_test.go`, `pkg/tenant/handlers_test.go`, and `pkg/webhooks/service_test.go` with mock publisher expectations.

## 5. Legacy OpenFGA Cleanup

- [x] 5.1 Delete `internal/openfga/` package files and tests.
- [x] 5.2 Delete `internal/authorization/` package files, tests, and OpenFGA DSL model files.
- [x] 5.3 Delete `cmd/createFgaModel.go` CLI command.
- [x] 5.4 Update `cmd/serve.go` dependency injection to initialize and wire `permissions.Publisher` into `tenant` and `webhooks` services.

## 6. Verification

- [x] 6.1 Run test suite `go test ./...` in `tenant-service` to verify all unit and handler tests pass.
- [x] 6.2 Validate `openspec validate` to confirm all planning artifacts are consistent.
