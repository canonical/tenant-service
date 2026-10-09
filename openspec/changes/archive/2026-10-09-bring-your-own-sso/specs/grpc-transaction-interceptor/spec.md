## MODIFIED Requirements

### Requirement: Read-only methods are identified via HTTP annotations
The system SHALL identify read-only gRPC methods by inspecting the `google.api.http` annotation on the proto method descriptor. Methods with a `get:` HTTP rule SHALL be classified as read-only. A method with no HTTP annotation SHALL be classified as mutating unless the server names it as read-only. The server SHALL name `ListSignInTenants`, `GetSignInContext` and `GetTenantSSOPolicy`: the three only read and are gRPC only, and the first two wait on Kratos before or between their reads. The interceptor SHALL apply to every gRPC service the server registers: `TenantService`, `TenantSignInService` and `TenantSSOPolicyService`.

#### Scenario: Proto methods with GET annotation are read-only
- **WHEN** the server starts and builds the read-only method set
- **THEN** methods annotated with `get:` (e.g., `ListTenants`, `LookupTenants`) are included in the read-only set
- **AND** methods annotated with `post:`, `put:`, `patch:`, or `delete:` are excluded

#### Scenario: gRPC-only reads are named as read-only
- **WHEN** the server starts and builds the read-only method set
- **THEN** `ListSignInTenants`, `GetSignInContext` and `GetTenantSSOPolicy` are included in it
- **AND** the gRPC-only writes (`JoinTenant`, `CreatePersonalTenant`, `PutTenantSSOPolicy`, `SetTenantSSODomains`, `RemoveTenantSSOBinding`) are excluded and run in a transaction

#### Scenario: A gRPC-only method that is not named
- **WHEN** a method has no HTTP annotation and the server does not name it as read-only
- **THEN** its handler runs in a transaction

#### Scenario: Full method name format is used for lookup
- **WHEN** a gRPC call arrives at the interceptor
- **THEN** the interceptor checks `info.FullMethod` against the pre-built set
- **AND** `info.FullMethod` uses the format `/package.ServiceName/MethodName`
