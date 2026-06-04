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
- [x] 3.2 Run `go mod tidy` and `go mod vendor` (if vendored) inside `tests/e2e/`
- [x] 3.3 Update `tests/e2e/grpc_test.go`: change import `v0 "github.com/canonical/tenant-service/v0"` to `v0 "github.com/canonical/identity-platform-api/v0/tenant"`
- [x] 3.4 Update `tests/e2e/client_abstraction_test.go`: change import `v0 "github.com/canonical/tenant-service/v0"` to `v0 "github.com/canonical/identity-platform-api/v0/tenant"`

## 4. Remove local generated files

- [x] 4.1 Delete `v0/tenant.pb.go`, `v0/tenant.pb.gw.go`, `v0/tenant_grpc.pb.go` and the `v0/` directory
- [x] 4.2 Delete the `api/` directory containing `api/proto/v0/` proto source files
- [x] 4.3 Delete `openapi/openapi.swagger.json` and `openapi/openapi.yaml` (and the `openapi/` directory if empty)
- [x] 4.4 Delete `buf.gen.yaml` and `buf.yaml`

## 5. Update build tooling

- [x] 5.1 Remove or update the `generate` target in `Makefile` that calls `buf generate` (the target is no longer needed for local code generation; update comment or remove)
- [x] 5.2 Check `build.sh` and `skaffold.yaml` for any references to the `v0/` directory, `openapi/` directory, or `buf generate` and remove/update them

## 6. Verify the build

- [x] 6.1 Run `go build ./...` from the root and confirm it compiles without errors
- [x] 6.2 Run `make test` to confirm all unit tests pass
- [x] 6.3 Run `go vet ./...` to confirm there are no vet issues
- [x] 6.4 Confirm `tests/e2e/` compiles with `go build ./...` inside that directory
