## ADDED Requirements

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
