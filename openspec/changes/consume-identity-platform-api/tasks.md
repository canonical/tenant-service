## 1. Update go.mod dependency

- [x] 1.1 Run `go get github.com/canonical/identity-platform-api@IAM-1998` (or the specific commit hash) to add/update the dependency in `go.mod` to pin the `IAM-1998` branch pseudo-version
- [x] 1.2 Run `go mod tidy` to clean up indirect dependencies
- [x] 1.3 Run `go mod vendor` to refresh the vendor tree with the new upstream library files

## 2. Update main module import paths

- [x] 2.1 Update `pkg/tenant/handlers.go`: change import `v0 "github.com/canonical/tenant-service/v0"` to `v0 "github.com/canonical/identity-platform-api/v0/tenant"`
- [x] 2.2 Update `pkg/tenant/handlers_test.go`: change import `v0 "github.com/canonical/tenant-service/v0"` to `v0 "github.com/canonical/identity-platform-api/v0/tenant"`
- [x] 2.3 Update `pkg/web/router.go`: change import `v0 "github.com/canonical/tenant-service/v0"` to `v0 "github.com/canonical/identity-platform-api/v0/tenant"`
- [x] 2.4 Update `cmd/serve.go`: change import `v0 "github.com/canonical/tenant-service/v0"` to `v0 "github.com/canonical/identity-platform-api/v0/tenant"`
- [x] 2.5 Update `cmd/client.go`: change import `v0 "github.com/canonical/tenant-service/v0"` to `v0 "github.com/canonical/identity-platform-api/v0/tenant"`
- [x] 2.6 Update `cmd/client_http.go`: change import `v0 "github.com/canonical/tenant-service/v0"` to `v0 "github.com/canonical/identity-platform-api/v0/tenant"`
- [x] 2.7 Update `cmd/tenant.go`: change import `v0 "github.com/canonical/tenant-service/v0"` to `v0 "github.com/canonical/identity-platform-api/v0/tenant"`
- [x] 2.8 Update `cmd/tenant_users.go`: change import `v0 "github.com/canonical/tenant-service/v0"` to `v0 "github.com/canonical/identity-platform-api/v0/tenant"`

## 3. Update e2e test module

- [x] 3.1 Update `tests/e2e/go.mod`: add `github.com/canonical/identity-platform-api` as a direct dependency at the same pseudo-version
- [x] 3.2 Run `go mod tidy` inside `tests/e2e/`
- [x] 3.3 Update `tests/e2e/grpc_test.go`: change import `v0 "github.com/canonical/tenant-service/v0"` to `v0 "github.com/canonical/identity-platform-api/v0/tenant"`
- [x] 3.4 Update `tests/e2e/client_abstraction_test.go`: change import `v0 "github.com/canonical/tenant-service/v0"` to `v0 "github.com/canonical/identity-platform-api/v0/tenant"`

## 4. Remove local generated files

- [x] 4.1 Delete `v0/tenant.pb.gw.go` and `v0/tenant_grpc.pb.go`
- [x] 4.2 Delete `v0/tenant.pb.go`
- [x] 4.3 Delete `api/proto/v0/tenant.proto`
- [x] 4.4 Remove `openapi/openapi.swagger.json` (legacy v2 source)
- [x] 4.5 Delete `buf.gen.yaml` and `buf.yaml`
- [x] 4.6 Delete `convert/convert.go`, `convert/go.mod`, `convert/go.sum`

## 5. Local OpenAPI HTTP client generation

- [x] 5.1 Generate `client/http/client.gen.go` locally using `oapi-codegen`
- [x] 5.2 Update `Makefile` (`generate-http-client`) to fetch upstream `openapi/openapi.yaml` from GitHub and generate only the tenant API tag (`TenantService`)
- [x] 5.3 Remove local OpenAPI files (`openapi/openapi.swagger.json`, `openapi/openapi.yaml`)

## 6. Update build tooling

- [x] 6.1 Remove or update the `generate` target in `Makefile` that calls `buf generate` (the target is no longer needed for local code generation; update comment or remove)
- [x] 6.2 Check `build.sh` and `skaffold.yaml` for any references to the `v0/` directory, `openapi/` directory, or `buf generate` and remove/update them
- [ ] 6.3 TODO: remove temporary `generate-http-client` make target once upstream publishes a consumable HTTP client library, and switch imports to that upstream package

## 7. Update cmd/client_http.go to use generated local client

`cmd/client_http.go` should remain a wrapper over `client/http/client.gen.go` so the future upstream migration is a source swap.

- [x] 7.1 Keep/import `httpclient "github.com/canonical/tenant-service/client/http"`
- [x] 7.2 Keep `httpTenantClient.client *httpclient.Client` and initialize via `httpclient.NewClient`
- [x] 7.3 Map protobuf request fields to generated query/body params correctly (including optional pointers)
- [x] 7.4 Support all methods including `UpdateTenantUser` and `LookupTenants`
- [x] 7.5 Keep `handleRequest` response decoding behavior

## 8. Clean up go.mod and vendor

- [x] 8.1 Run `go mod tidy` from repo root and keep `github.com/oapi-codegen/runtime` as a direct dependency for generated client support
- [x] 8.2 Run `go mod vendor` to refresh the vendor tree
- [x] 8.3 Ensure no nested `tests/e2e/vendor/` directory is present in the change

## 9. Verify the build

- [x] 9.1 Run `go build ./...` from the root and confirm it compiles without errors
- [x] 9.2 Run `go test ./...` from repo root
- [x] 9.3 Run `go vet ./...` to confirm there are no vet issues
- [x] 9.4 Confirm `tests/e2e/` compiles with `go test -c .` inside that directory
