## Purpose

The tenant-service exposes both an HTTP/JSON gateway (via grpc-gateway) and a pure gRPC interface.
The HTTP path applies middleware for transaction management, Prometheus metrics, and request logging.
Direct gRPC calls bypass all of these, creating a consistency and safety gap: mutating gRPC RPCs
are not wrapped in a database transaction, so multi-write operations are not atomic.

This spec defines automatic transaction wrapping for mutating gRPC unary RPCs using a server
interceptor, achieving parity with the HTTP `db.TransactionMiddleware`.

---

## ADDED Requirements

### Requirement: Mutating gRPC unary RPCs execute within a database transaction
The system SHALL wrap all mutating gRPC unary RPC handlers in a database transaction that commits on success and rolls back on error. Read-only RPCs SHALL bypass transaction wrapping entirely.

Streaming RPCs are explicitly excluded from automatic transaction wrapping. Because a stream's lifetime is unbounded and its message semantics vary by endpoint, transaction boundaries for streaming handlers SHALL be managed explicitly within the handler implementation.

#### Scenario: Successful mutating RPC triggers commit
- **WHEN** a gRPC client calls a mutating unary RPC (e.g., `CreateTenant`)
- **THEN** the handler executes within a database transaction
- **AND** the transaction commits after successful handler completion

#### Scenario: Failed mutating RPC triggers rollback
- **WHEN** a gRPC client calls a mutating unary RPC and the handler returns an error
- **THEN** the transaction rolls back
- **AND** the original handler error is returned to the client with a nil response body

#### Scenario: Commit failure is logged and surfaced as Internal
- **WHEN** a mutating RPC handler completes successfully but the transaction fails to commit
- **THEN** the error is logged at Error level
- **AND** the client receives a `codes.Internal` status with no response body

#### Scenario: Read-only RPC skips transaction
- **WHEN** a gRPC client calls a read-only RPC (e.g., `ListTenants`)
- **THEN** the handler executes without a database transaction
- **AND** no BEGIN/COMMIT overhead is incurred

### Requirement: Read-only methods are identified via HTTP annotations
The system SHALL identify read-only gRPC methods by inspecting the `google.api.http` annotation on the proto method descriptor. Methods with a `get:` HTTP rule SHALL be classified as read-only.

#### Scenario: Proto methods with GET annotation are read-only
- **WHEN** the server starts and builds the read-only method set
- **THEN** methods annotated with `get:` (e.g., `ListTenants`, `LookupTenants`) are included in the read-only set
- **AND** methods annotated with `post:`, `patch:`, or `delete:` are excluded

#### Scenario: Full method name format is used for lookup
- **WHEN** a gRPC call arrives at the interceptor
- **THEN** the interceptor checks `info.FullMethod` against the pre-built set
- **AND** `info.FullMethod` uses the format `/package.ServiceName/MethodName`

### Requirement: Startup fails fast if proto descriptor cannot be resolved
The system SHALL return an error from `ReadOnlyMethods` if the proto file cannot be found in the global registry or if the service descriptor metadata is not a string. The server SHALL treat this as a fatal startup error.

#### Scenario: Unknown proto file causes fatal startup error
- **WHEN** the server starts and `ReadOnlyMethods` cannot locate the proto file in the registry
- **THEN** an error is returned and the server exits with a fatal log entry
- **AND** the server does not begin accepting traffic
