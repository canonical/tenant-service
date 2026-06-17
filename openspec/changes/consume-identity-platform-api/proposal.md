## Why

The tenant-service currently vendors its own gRPC/protobuf-generated Go files (`v0/`) and OpenAPI definitions (`openapi/`), duplicating the canonical API contract maintained in `github.com/canonical/identity-platform-api`. This creates drift risk and violates the project's API ownership convention that all generated artifacts must be consumed from that upstream library.

## What Changes

- Remove the local generated gRPC/protobuf files (`v0/tenant.pb.go`, `v0/tenant.pb.gw.go`, `v0/tenant_grpc.pb.go`) from the tenant-service repository.
- Remove the local proto definition directory (`api/proto/v0/`).
- Generate the HTTP client (`client/http/client.gen.go`) locally with `oapi-codegen`, fetching the OpenAPI v3 spec directly from `identity-platform-api` on GitHub during generation.
- Remove local OpenAPI files (`openapi/openapi.swagger.json`, `openapi/openapi.yaml`) and the `convert/` utility module.
- Remove `buf.gen.yaml` and `buf.yaml` (no longer needed for local code generation).
- Update `go.mod` to depend on `github.com/canonical/identity-platform-api` at the `IAM-1998` branch commit (to be switched to `main` once merged); keep `github.com/oapi-codegen/runtime` as a direct dependency for the generated HTTP client.
- Update all Go import paths from `github.com/canonical/tenant-service/v0` to `github.com/canonical/identity-platform-api/v0/tenant`.
- Keep `cmd/client_http.go` wired to the generated `client/http` package so a future upstream HTTP-client migration can be done by switching the import source.
- Update the e2e tests to import the gRPC client from the upstream library.

## Capabilities

### New Capabilities
<!-- No new user-facing capabilities; this is a dependency migration. -->

### Modified Capabilities
<!-- No spec-level behavior changes; all API contracts remain identical. -->

## Impact

- **Go files changed**: `pkg/tenant/handlers.go`, `pkg/tenant/handlers_test.go`, `pkg/web/router.go`, `cmd/serve.go`, `cmd/client.go`, `cmd/client_http.go`, `cmd/tenant.go`, `cmd/tenant_users.go`
- **E2E tests**: `tests/e2e/grpc_test.go`, `tests/e2e/client_abstraction_test.go`, `tests/e2e/go.mod`
- **Files removed**: `v0/`, `api/`, `convert/`, `buf.gen.yaml`, `buf.yaml`, `openapi/openapi.swagger.json`, `openapi/openapi.yaml`
- **Dependencies**: `go.mod` pinned to the `IAM-1998` branch pseudo-version; to be updated to a released version once the branch is merged to `main`
