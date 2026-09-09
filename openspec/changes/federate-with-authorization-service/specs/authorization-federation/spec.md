## Purpose

Defines the contract for federating `tenant-service` with the Canonical Identity Platform Authorization Service, delegating access checks to upstream gateway infrastructure and publishing permission lifecycle events to Kafka.

## ADDED Requirements

### Requirement: Ingress Authorization Delegation
The system SHALL execute incoming tenant and member operations without executing in-process OpenFGA authorization checks, relying on upstream API Gateway and Authorization Service enforcement.

#### Scenario: Authorized request execution
- **WHEN** a valid authenticated request reaches the service handler
- **THEN** the system executes the requested operation directly against storage without performing in-process authorization queries

### Requirement: Permission Event Publishing on Tenant Creation
The system SHALL asynchronously publish a `PermissionUpdateEnvelope` event keyed by tenant ID containing a `WRITE` operation for `owner` relation on `tenant-service.permissions` topic when a new tenant is created.

#### Scenario: Self-registration creates tenant and publishes owner permission
- **WHEN** a user completes self-registration triggering tenant creation
- **THEN** the system saves the tenant and membership to storage and asynchronously publishes an envelope with op `WRITE`, subject `user:<identity_id>`, relation `owner`, and object `tenant:<tenant_id>` keyed by `tenant_id` to Kafka

### Requirement: Permission Event Publishing on Member Provisioning and Invites
The system SHALL asynchronously publish a `PermissionUpdateEnvelope` event keyed by tenant ID containing a `WRITE` operation for the assigned role (`owner` or `member`) when a member is added or invited to a tenant.

#### Scenario: Provisioning user into tenant publishes permission event
- **WHEN** an admin or owner provisions a user into a tenant with a given role
- **THEN** the system creates the membership record in storage and asynchronously publishes an envelope with op `WRITE`, subject `user:<user_id>`, relation `<role>`, and object `tenant:<tenant_id>` keyed by `tenant_id` to Kafka

### Requirement: Permission Event Publishing on Role Update
The system SHALL asynchronously publish a `PermissionUpdateEnvelope` event keyed by tenant ID containing atomic `WRITE` for the new role and `DELETE` for the prior role when a member's role is updated.

#### Scenario: Demoting owner to member emits write member and delete owner ops
- **WHEN** a tenant owner is demoted to member role
- **THEN** the system updates storage and asynchronously publishes an envelope containing op `WRITE` for `member` and op `DELETE` for `owner` for that user and tenant keyed by `tenant_id`

#### Scenario: Promoting member to owner emits write owner and delete member ops
- **WHEN** a tenant member is promoted to owner role
- **THEN** the system updates storage and asynchronously publishes an envelope containing op `WRITE` for `owner` and op `DELETE` for `member` for that user and tenant keyed by `tenant_id`


### Requirement: Permission Event Publishing on Tenant Deletion
The system SHALL asynchronously publish `PermissionUpdateEnvelope` events keyed by tenant ID containing `DELETE` operations for all members of the tenant when a tenant is deleted.

#### Scenario: Tenant deletion emits delete permission ops for members
- **WHEN** a tenant is deleted
- **THEN** the system removes the tenant and memberships from storage and asynchronously publishes delete permission events to Kafka for all tenant members keyed by `tenant_id`

### Requirement: STS Bearer Token Verification
The system SHALL verify incoming bearer JWT tokens issued by STS using keys from the configured JWKS endpoint, supporting both RS256 and ES256 signing algorithms.

#### Scenario: Valid STS token with ES256 signature is accepted
- **WHEN** an authenticated request arrives with a valid STS token signed with ES256
- **THEN** the system verifies the token signature against the JWKS endpoint and attaches the `sub` claim to the request context

#### Scenario: Token verification without static allowed subjects or scopes
- **WHEN** an authenticated request arrives with a validly signed token but no `AUTHENTICATION_ALLOWED_SUBJECTS` or `AUTHENTICATION_REQUIRED_SCOPE` is configured
- **THEN** the system accepts the token and extracts the `sub` identity without rejecting the request
