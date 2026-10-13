## MODIFIED Requirements

### Requirement: Read-only methods are identified via HTTP annotations
The system SHALL identify read-only gRPC methods by inspecting the `google.api.http` annotation on the proto method descriptor. Methods with a `get:` HTTP rule SHALL be classified as read-only. A method with no HTTP annotation SHALL be classified as mutating, unless the server names it as read-only. The server SHALL name `ListSignInTenants`, `GetSignInContext` and `GetTenantSSOPolicy` as read-only. These 3 methods only read and are gRPC only. The first 2 methods wait for Kratos before their reads or between their reads. The interceptor SHALL apply to every gRPC service that the server registers: `TenantService`, `TenantSignInService` and `TenantSSOPolicyService`.

#### Scenario: Proto methods with GET annotation are read-only
- **WHEN** the server starts and builds the read-only method set
- **THEN** methods annotated with `get:` (e.g., `ListTenants`, `LookupTenants`) are included in the read-only set
- **AND** methods annotated with `post:`, `put:`, `patch:`, or `delete:` are excluded

#### Scenario: gRPC-only reads are named as read-only
- **WHEN** the server starts and builds the read-only method set
- **THEN** the read-only set includes `ListSignInTenants`, `GetSignInContext` and `GetTenantSSOPolicy`
- **AND** the read-only set excludes the gRPC-only writes: `JoinTenant`, `CreatePersonalTenant`, `PutTenantSSOPolicy`, `SetTenantSSODomains` and `RemoveTenantSSOBinding`
- **AND** the gRPC-only writes run in a transaction

#### Scenario: A gRPC-only method that is not named
- **WHEN** a method has no HTTP annotation and the server does not name it as read-only
- **THEN** the handler of the method runs in a transaction

#### Scenario: Full method name format is used for lookup
- **WHEN** a gRPC call arrives at the interceptor
- **THEN** the interceptor checks `info.FullMethod` against the pre-built set
- **AND** `info.FullMethod` uses the format `/package.ServiceName/MethodName`
