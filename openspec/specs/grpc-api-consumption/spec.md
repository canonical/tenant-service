# grpc-api-consumption Specification

## Purpose

The tenant-service's gRPC, protobuf, and OpenAPI generated artifacts must be sourced exclusively from [`github.com/canonical/identity-platform-api`](https://github.com/canonical/identity-platform-api), the single canonical home for all Identity Platform API contracts.

Maintaining local copies of generated files alongside the upstream source creates silent divergence risk: the two copies can drift without any build-time signal. The upstream library already owns the proto definitions and generates the identical Go artifacts under `v0/tenant/`; the tenant-service consuming them as an ordinary Go dependency is the correct architecture.

**Goals:** remove the locally vendored `v0/`, `api/proto/v0/`, and `openapi/` directories; replace all import paths with the upstream module; keep the e2e test module aligned.

**Non-goals:** changing the API contract or proto definitions; upgrading gRPC/protobuf library versions beyond what the upstream module requires.
## Requirements
### Requirement: gRPC API sourced from upstream library
The service SHALL import gRPC, protobuf, and OpenAPI generated code exclusively from `github.com/canonical/identity-platform-api` and SHALL NOT maintain any locally generated copies of those files.

#### Scenario: gRPC server registration uses upstream types
- **WHEN** the gRPC server is registered
- **THEN** the `TenantServiceServer` interface and all message types MUST be sourced from `github.com/canonical/identity-platform-api/v0/tenant`

#### Scenario: e2e tests use upstream gRPC client
- **WHEN** e2e tests create a gRPC client
- **THEN** `NewTenantServiceClient` MUST be imported from `github.com/canonical/identity-platform-api/v0/tenant`

#### Scenario: Build succeeds without local generation toolchain
- **WHEN** the service is built and tested after migration
- **THEN** the codebase MUST compile and run using upstream generated artifacts without requiring local `buf generate` execution

### Requirement: API behavior parity during dependency migration
Migrating generated API artifacts to the upstream identity-platform-api dependency SHALL preserve existing tenant-service request and response behavior.

#### Scenario: Existing client requests remain valid
- **WHEN** a client sends a previously valid tenant-service request
- **THEN** the request MUST be accepted or rejected with the same semantic validation outcome as before migration

#### Scenario: Existing response contracts remain compatible
- **WHEN** tenant-service returns gRPC or HTTP/JSON responses for existing endpoints
- **THEN** response field semantics and error mappings MUST remain backward compatible for existing clients

