## Purpose

Defines the contract for federating `tenant-service` with the Canonical Identity Platform Authorization Service, delegating access checks to upstream gateway infrastructure and publishing permission lifecycle events to Kafka.

## ADDED Requirements

### Requirement: Ingress Authorization Delegation
The system SHALL execute incoming tenant and member operations without executing in-process OpenFGA authorization checks, relying on upstream API Gateway and Authorization Service enforcement.

#### Scenario: Authorized request execution
- **WHEN** a valid authenticated request reaches the service handler
- **THEN** the system executes the requested operation directly against storage without performing in-process authorization queries

### Requirement: Permission Event Publishing on Self-Registration
The system SHALL asynchronously publish a `PermissionUpdateEnvelope` event keyed by tenant ID containing a `WRITE` operation for the tenant resource with relation `can_delete` on `tenant-service.permissions` topic when a tenant is created through self-registration. Tenants created by a platform admin via `CreateTenant` SHALL NOT publish permission events; their owners are granted through the Authorization Service API.

#### Scenario: Self-registration creates tenant and publishes owner permission
- **WHEN** a user completes self-registration triggering tenant creation
- **THEN** the system saves the tenant and membership to storage and asynchronously publishes an envelope with op `WRITE`, subject `user:<identity_id>`, relation `can_delete`, and object `tenant:<tenant_id>` keyed by `tenant_id` to Kafka

#### Scenario: Admin tenant creation publishes no permission events
- **WHEN** a platform admin creates a tenant via `CreateTenant`
- **THEN** the system saves the tenant to storage and publishes no permission events

### Requirement: Permission Event Publishing on Member Provisioning and Invites
The system SHALL asynchronously publish a `PermissionUpdateEnvelope` event keyed by tenant ID containing a `WRITE` operation with relation `can_view` on the tenant resource when a member is provisioned into or invited to a tenant.

#### Scenario: Provisioning user into tenant publishes view permission
- **WHEN** a caller provisions a user into a tenant
- **THEN** the system creates the membership record in storage and asynchronously publishes an envelope with op `WRITE`, subject `user:<user_id>`, relation `can_view`, and object `tenant:<tenant_id>` keyed by `tenant_id` to Kafka

#### Scenario: Inviting an existing member is idempotent
- **WHEN** a caller invites a user who is already a member of the tenant
- **THEN** the system keeps the existing membership, publishes an envelope with op `WRITE` and relation `can_view` for that user, and returns a new invitation link

### Requirement: Elevated Permissions Are Not Assigned by the Tenant Service
The system SHALL NOT accept a membership role or any other caller-supplied permission level, and SHALL NOT expose an operation to change a member's permissions. Permissions above `can_view` for existing members SHALL be managed through the Authorization Service API.

#### Scenario: Invite and provision requests carry no role
- **WHEN** a caller invites or provisions a user into a tenant
- **THEN** the request contains only the tenant ID and email, and the only permission published is `can_view`

### Requirement: Permission Event Publishing on Tenant Deletion
The system SHALL asynchronously publish `PermissionUpdateEnvelope` events keyed by tenant ID containing `DELETE` operations for relations `can_view`, `can_edit`, and `can_delete` on the tenant resource for every member of the tenant when a tenant is deleted. The system SHALL include all members regardless of page size and SHALL split operations into envelopes of at most 100 operations.

#### Scenario: Tenant deletion emits delete permission ops for members
- **WHEN** a tenant is deleted
- **THEN** the system lists every membership (across all pages) before removing the tenant and memberships from storage, and asynchronously publishes `DELETE` operations for `can_view`, `can_edit`, and `can_delete` for each member keyed by `tenant_id`

#### Scenario: Large tenant deletion is split into multiple envelopes
- **WHEN** a tenant is deleted and the revocation requires more than 100 operations
- **THEN** the system publishes multiple envelopes keyed by `tenant_id`, each containing at most 100 operations

### Requirement: STS Bearer Token Verification
The system SHALL verify incoming bearer JWT tokens issued by STS using keys from the configured JWKS endpoint, supporting both RS256 and ES256 signing algorithms.

#### Scenario: Valid STS token with ES256 signature is accepted
- **WHEN** an authenticated request arrives with a valid STS token signed with ES256
- **THEN** the system verifies the token signature against the JWKS endpoint and attaches the `sub` claim to the request context

#### Scenario: Token verification without static allowed subjects or scopes
- **WHEN** an authenticated request arrives with a validly signed token but no `AUTHENTICATION_ALLOWED_SUBJECTS` or `AUTHENTICATION_REQUIRED_SCOPE` is configured
- **THEN** the system accepts the token and extracts the `sub` identity without rejecting the request

#### Scenario: Token with mismatched issuer is rejected
- **WHEN** an authenticated request arrives with a token whose `iss` claim does not match the configured issuer
- **THEN** the system rejects the token verification with an error

### Requirement: Gateway Bypass and Direct JWT Authentication for Self-Inspection
The system SHALL configure Istio Gateway rules to bypass external authorization checks for `GET /api/v0/me/tenants` and SHALL validate caller identity directly via JWT authentication middleware.

#### Scenario: Self-inspection endpoint validates caller JWT directly
- **WHEN** a request arrives for `GET /api/v0/me/tenants` bypassing external authorization at the Istio Gateway
- **THEN** the system validates the caller's JWT bearer token, extracts the user ID from the `sub` claim, and returns tenants associated with that user ID

