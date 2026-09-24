## Purpose

Defines the contract for federating `tenant-service` with the Canonical Identity Platform Authorization Service, delegating access checks to upstream gateway infrastructure and publishing permission lifecycle events to Kafka.

## ADDED Requirements

### Requirement: Ingress Authorization Delegation
The system SHALL execute incoming tenant and member operations without executing in-process OpenFGA authorization checks, relying on upstream API Gateway and Authorization Service enforcement.

#### Scenario: Authorized request execution
- **WHEN** a valid authenticated request reaches the service handler
- **THEN** the system executes the requested operation directly against storage without performing in-process authorization queries

### Requirement: Permission Event Publishing on Tenant Creation
The system SHALL asynchronously publish a `PermissionUpdateEnvelope` event keyed by tenant ID containing a `WRITE` operation for the tenant resource with relation `can_delete` on `tenant-service.permissions` topic when a new tenant is created.

#### Scenario: Self-registration creates tenant and publishes owner permission
- **WHEN** a user completes self-registration triggering tenant creation
- **THEN** the system saves the tenant and membership to storage and asynchronously publishes an envelope with op `WRITE`, subject `user:<identity_id>`, relation `can_delete`, and object `tenant:<tenant_id>` keyed by `tenant_id` to Kafka

### Requirement: Permission Event Publishing on Member Provisioning and Invites
The system SHALL asynchronously publish a `PermissionUpdateEnvelope` event keyed by tenant ID containing a `WRITE` operation for the assigned role when a member is added or invited to a tenant, mapping roles to fine-grained permissions on the tenant resource (`owner` to `can_delete`, `admin` to `can_edit`, and `member` to `can_view`).

#### Scenario: Provisioning user into tenant publishes permission event
- **WHEN** an admin or owner provisions a user into a tenant with a given role
- **THEN** the system creates the membership record in storage and asynchronously publishes an envelope with op `WRITE`, subject `user:<user_id>`, mapped relation (`can_delete`, `can_edit`, or `can_view`), and mapped object keyed by `tenant_id` to Kafka

### Requirement: Permission Event Publishing on Role Update
The system SHALL asynchronously publish a `PermissionUpdateEnvelope` event keyed by tenant ID containing atomic `WRITE` for the new role's permission and `DELETE` for the prior role's permission when a member's role is updated, mapping roles to fine-grained permissions on the tenant resource (`owner` -> `can_delete`, `admin` -> `can_edit`, `member` -> `can_view`).

#### Scenario: Demoting owner to member emits write can_view and delete can_delete ops
- **WHEN** a tenant owner is demoted to member role
- **THEN** the system updates storage and asynchronously publishes an envelope containing op `WRITE` for `can_view` on `tenant:<tenant_id>` and op `DELETE` for `can_delete` on `tenant:<tenant_id>` for that user keyed by `tenant_id`

#### Scenario: Promoting member to owner emits write can_delete and delete can_view ops
- **WHEN** a tenant member is promoted to owner role
- **THEN** the system updates storage and asynchronously publishes an envelope containing op `WRITE` for `can_delete` on `tenant:<tenant_id>` and op `DELETE` for `can_view` on `tenant:<tenant_id>` for that user keyed by `tenant_id`


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

#### Scenario: Token with mismatched issuer is rejected
- **WHEN** an authenticated request arrives with a token whose `iss` claim does not match the configured issuer
- **THEN** the system rejects the token verification with an error

### Requirement: Gateway Bypass and Direct JWT Authentication for Self-Inspection
The system SHALL configure Istio Gateway rules to bypass external authorization checks for `GET /api/v0/me/tenants` and SHALL validate caller identity directly via JWT authentication middleware.

#### Scenario: Self-inspection endpoint validates caller JWT directly
- **WHEN** a request arrives for `GET /api/v0/me/tenants` bypassing external authorization at the Istio Gateway
- **THEN** the system validates the caller's JWT bearer token, extracts the user ID from the `sub` claim, and returns tenants associated with that user ID

