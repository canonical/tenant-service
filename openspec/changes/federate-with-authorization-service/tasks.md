## 1. Dependencies and Configuration

- [x] 1.1 Update `go.mod` to add `github.com/segmentio/kafka-go` and `github.com/canonical/authorization-service/api/v1`, and remove `github.com/openfga/go-sdk` and `github.com/openfga/language/pkg/go`.
- [x] 1.2 Update `internal/config/specs.go` to replace `OPENFGA_*` options with `KAFKA_*` configuration (`Enabled`, `Brokers`, `PermissionsTopic`, `ClientID`).

## 2. Authentication and STS Token Verification

- [x] 2.1 Update `pkg/authentication/provider.go` to support `RS256` and `ES256` signing algorithms and enforce issuer validation (`SkipIssuerCheck: false`).
- [x] 2.2 Update `pkg/authentication/verifier.go` to permit validly signed tokens when neither allowed subjects nor required scopes are configured.
- [x] 2.3 Update unit tests in `pkg/authentication/verifier_test.go` and `pkg/authentication/middleware_test.go` to cover STS token verification and algorithm support.

## 3. Permissions Event Publisher

- [x] 3.1 Create `internal/permissions/interfaces.go` and `internal/permissions/publisher.go` implementing asynchronous Kafka event publishing for `PermissionUpdateEnvelope` messages keyed by tenant ID and providing a `NoopPublisher`.
- [x] 3.2 Create `internal/permissions/publisher_test.go` and generate GoMock mocks for `internal/permissions/interfaces.go`.

## 4. Service Layer Migration

- [x] 4.1 Refactor `pkg/tenant/service.go` to replace OpenFGA calls with asynchronous `permissions.Publisher` event publishing and remove internal authorization check queries.
- [x] 4.2 Refactor `pkg/webhooks/service.go` to publish owner permission events asynchronously via `permissions.Publisher` on self-registration.
- [x] 4.3 Update unit tests in `pkg/tenant/service_test.go`, `pkg/tenant/handlers_test.go`, and `pkg/webhooks/service_test.go` with mock publisher expectations.
- [x] 4.4 Configure Istio Gateway rules (`k8s/istio.yaml`) to bypass `authorization-service` for `GET /api/v0/me/tenants` and rely on built-in JWT verification, removing static `account:me` emission from `cmd/serve.go`.
- [x] 4.5 Clean up static permission publishing tests in `cmd/serve_test.go`.

## 5. Legacy OpenFGA Cleanup

- [x] 5.1 Delete `internal/openfga/` package files and tests.
- [x] 5.2 Delete `internal/authorization/` package files, tests, and OpenFGA DSL model files.
- [x] 5.3 Delete `cmd/createFgaModel.go` CLI command.
- [x] 5.4 Update `cmd/serve.go` dependency injection to initialize and wire `permissions.Publisher` into `tenant` and `webhooks` services.

## 6. Verification

- [x] 6.1 Run test suite `go test ./...` in `tenant-service` to verify all unit and handler tests pass.
- [x] 6.2 Validate `openspec validate` to confirm all planning artifacts are consistent.

## 7. Remove Membership Roles

- [x] 7.1 In `identity-platform-api`, remove `role` from `InviteMemberRequest`, `ProvisionUserRequest`, `ListTenantUsersRequest` and `TenantUser` (reserving field numbers), remove the `UpdateTenantUser` RPC and messages, and regenerate Go and OpenAPI artifacts.
- [x] 7.2 Replace the local `replace` directives in `go.mod` and `tests/e2e/go.mod` with the released `identity-platform-api` version once the upstream change is merged.
- [x] 7.3 Add migration `migrations/003_drop_memberships_role.sql` dropping `memberships.role`.
- [x] 7.4 Remove role from `internal/types` and `internal/storage` (`AddMember` signature, `UpdateMember`, role filter).
- [x] 7.5 Update `pkg/tenant` service and handlers: invite/provision publish `can_view`, remove `UpdateTenantUser`, remove role from `ListTenantUsers`.
- [x] 7.6 Update `DeleteTenant` to page through all members and revoke `can_view`, `can_edit`, and `can_delete` per member; split publisher envelopes to at most 100 operations.
- [x] 7.7 Replace `PermissionOpForRole` with `TenantPermissionOp` / `RevokeTenantPermissionOps` and relation constants in `internal/permissions`.
- [x] 7.8 Update `pkg/webhooks` self-registration to add the member without a role and publish `can_delete`.
- [x] 7.9 Drop the `role` label from `business_operations_total`.
- [x] 7.10 Update CLI (`cmd/tenant_users.go`, `cmd/client_http.go`) and regenerate `client/http/client.gen.go`.
- [x] 7.11 Update unit, e2e, and browser tests.
- [x] 7.12 Add the `permission_events_total{result, stage}` counter to the publisher, recording marshal, write, and delivery outcomes per envelope, and remove the unused `PublishSync`.
