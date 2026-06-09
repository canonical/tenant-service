## Context

The tenant-service was bootstrapped with locally vendored gRPC/protobuf-generated Go files (`v0/tenant.pb.go`, `v0/tenant.pb.gw.go`, `v0/tenant_grpc.pb.go`) and OpenAPI specs (`openapi/openapi.swagger.json`, `openapi/openapi.yaml`). These files were generated from proto definitions originally co-located in this repository (`api/proto/v0/`).

The canonical source of truth for all API contracts in the Identity Platform is `github.com/canonical/identity-platform-api`. That repository already contains the tenant service proto definitions and generates the same Go artifacts under `v0/tenant/`. Maintaining local copies violates the established API ownership convention and introduces risk of the two copies diverging.

The branch `IAM-1998` in `identity-platform-api` adds the tenant service definitions and will be merged to `main`. Until then, the `go.mod` will pin to a pseudo-version for that commit.

**Constraints:**
- The Go package name in the upstream library is `tenantv0` (same as local), sourced from `github.com/canonical/identity-platform-api/v0/tenant`.
- The local package was imported as `v0 "github.com/canonical/tenant-service/v0"`. After migration the alias stays `v0` but the import path changes.
- The e2e test module (`tests/e2e/`) has its own `go.mod` and also imports the local `v0` package via a `replace` directive pointing at the parent module.

## Goals / Non-Goals

**Goals:**
- Remove locally generated gRPC/protobuf files from the repository.
- Point `go.mod` (main module and e2e test module) at the upstream `identity-platform-api` library.
- Update all Go import paths referencing `github.com/canonical/tenant-service/v0` to `github.com/canonical/identity-platform-api/v0/tenant`.
- Update the e2e tests to use the upstream gRPC client types.
- Remove build tooling (`buf.gen.yaml`, `buf.yaml`) that was only used to regenerate the local files.
- Keep a locally generated OpenAPI HTTP client (`client/http/client.gen.go`) by fetching the upstream OpenAPI spec from GitHub at generation time, so the eventual upstream migration is a source swap, not a code rewrite.
- Ensure `make test` and `make build` pass after migration.

**Non-Goals:**
- Changing the tenant service API contract or proto definitions.
- Upgrading gRPC/protobuf library versions beyond what the upstream module requires.

## Decisions

### Decision: Use branch pseudo-version, switch to main once merged

**Choice**: Pin `go.mod` to the `IAM-1998` branch at its current commit pseudo-version, then update to `main` (or a tagged release) once the PR is merged.

**Rationale**: The user explicitly requested this workflow. It avoids blocking the task on the merge and keeps the change self-contained. The `go get` command with a specific commit hash generates the correct pseudo-version.

**Alternative considered**: Fork or vendor the branch directly. Rejected — vendoring is already done via `go mod vendor` and updating `go.mod` is the standard Go workflow.

### Decision: Replace `v0/` directory wholesale (not incrementally)

**Choice**: Delete all three files in `v0/` and the `api/proto/v0/` directory in a single step, updating all imports at the same time.

**Rationale**: The files are purely generated code with no hand-written additions. A partial migration would leave the build broken mid-way. A single atomic change is safer.

### Decision: Keep import alias `v0` in all files

**Choice**: Retain the `v0` alias when updating the import path (e.g., `v0 "github.com/canonical/identity-platform-api/v0/tenant"`).

**Rationale**: Minimises diff noise; no logic changes required alongside the import path change. The upstream package name is `tenantv0` which matches the local one, so the alias keeps call-sites identical.

### Decision: Keep a locally generated HTTP client from upstream OpenAPI

**Choice**: Generate `client/http/client.gen.go` locally with `oapi-codegen` by downloading `openapi/openapi.yaml` from `identity-platform-api` on GitHub inside the Make target; keep `cmd/client_http.go` and e2e HTTP tests using that generated package.

**Rationale**: The upstream `identity-platform-api` library does not currently ship an HTTP client. Keeping a local generated client avoids handwritten REST plumbing while preserving a clean migration path: once upstream publishes the HTTP client, switching can be done by replacing the import source with minimal call-site changes. Fetching the spec from GitHub means we do not need to commit OpenAPI documents in this repository.

**Alternative considered**: Rewrite the HTTP path as a plain `net/http` client. Rejected — it increases local maintenance and makes the future source swap harder.

### Decision: Remove convert/ module and local OpenAPI files

**Choice**: Remove `convert/convert.go` and its `go.mod`/`go.sum`.

**Rationale**: The `convert/` tool existed solely to convert the local OpenAPI v2 swagger file to OpenAPI v3 YAML. We now fetch the v3 source from upstream during generation, so `convert/` and both local OpenAPI files are no longer needed.

### Decision: Update e2e module to consume upstream v0 types

**Choice**: Replace the `replace github.com/canonical/tenant-service => ../..` indirect dependency for `v0` types with a direct dependency on `github.com/canonical/identity-platform-api`.

**Rationale**: The e2e module only uses the `v0` types from the parent module. After migration, those types live in the upstream library. The `replace` directive for `github.com/canonical/tenant-service` can remain (it is still needed for other shared types used by e2e tests).

## Risks / Trade-offs

- **Branch not yet merged** → pseudo-version must be updated once `IAM-1998` is merged to `main`. Risk is low since this is tracked by the user.
- **Vendor directory** → After updating `go.mod`, `go mod vendor` must be re-run to refresh the vendor tree. The upstream library's transitive dependencies may add or change vendored files.
- **Build tags / generated file checks** → Any CI step that checks for `buf generate` output or validates the `v0/` directory must be removed or updated.

## Migration Plan

1. Update `go.mod` to require `github.com/canonical/identity-platform-api` at the `IAM-1998` branch pseudo-version (or use `go get github.com/canonical/identity-platform-api@IAM-1998`).
2. Run `go mod tidy && go mod vendor` to update the vendor tree.
3. Update all Go source import paths: `github.com/canonical/tenant-service/v0` → `github.com/canonical/identity-platform-api/v0/tenant`.
4. Update the e2e `go.mod` similarly.
5. Delete `v0/`, `api/proto/v0/`, `openapi/openapi.swagger.json`, `openapi/openapi.yaml`, `convert/`, `buf.gen.yaml`, `buf.yaml`.
6. Add/maintain a temporary local generation command for `client/http/client.gen.go` that fetches upstream `openapi/openapi.yaml` from GitHub.
7. Run `make test` to verify unit tests pass.
8. Run e2e tests against a local stack.

**Rollback**: Revert the commit. The `v0/` directory is fully recoverable from `buf generate` using the existing proto definitions in `api/proto/v0/` before deletion.

## Open Questions

- Once `IAM-1998` is merged to `main`, should a tagged release of `identity-platform-api` be cut, or is a `main` pseudo-version sufficient? (User to decide after merge.)
- Should `openapi/` be removed entirely or kept for documentation purposes? (Leaning toward removal since the upstream library is the source of truth.)
