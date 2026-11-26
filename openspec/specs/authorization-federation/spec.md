# authorization-federation Specification

## Purpose

Defines the contract for federating `tenant-service` with the Canonical Identity Platform Authorization Service, delegating access checks to upstream gateway infrastructure and publishing permission lifecycle events via configurable `noop`, `platform` (Kafka), and `standalone` (Authorization Service gRPC ingestion with OAuth2 client-credentials authentication) modes.
## Requirements
### Requirement: Ingress Authorization Delegation
The system SHALL execute incoming tenant and member operations without executing in-process OpenFGA authorization checks, relying on upstream API Gateway and Authorization Service enforcement.

#### Scenario: Authorized request execution
- **WHEN** a valid authenticated request reaches the service handler
- **THEN** the system executes the requested operation directly against storage without performing in-process authorization queries

### Requirement: Permission Event Publishing on Self-Registration
The system SHALL asynchronously publish a `PermissionUpdateEnvelope` event for the tenant containing a `WRITE` operation for the tenant resource with relation `can_delete` via the configured permission publisher (`platform` Kafka topic `permissions.tenant-service` keyed by tenant ID, or `standalone` Authorization Service gRPC ingestion endpoint) when a tenant is created through self-registration. Tenants created by a platform admin via `CreateTenant` SHALL NOT publish permission events; their owners are granted through the Authorization Service API.

#### Scenario: Self-registration creates tenant and publishes owner permission
- **WHEN** a user completes self-registration triggering tenant creation
- **THEN** the system saves the tenant and membership to storage and asynchronously publishes an envelope with op `WRITE`, subject `user:<identity_id>`, relation `can_delete`, and object `tenant:<tenant_id>` via the configured permission publisher

#### Scenario: Admin tenant creation publishes no permission events
- **WHEN** a platform admin creates a tenant via `CreateTenant`
- **THEN** the system saves the tenant to storage and publishes no permission events

### Requirement: Permission Event Publishing on Member Provisioning and Invites
The system SHALL asynchronously publish a `PermissionUpdateEnvelope` event for the tenant containing a `WRITE` operation with relation `can_view` on the tenant resource via the configured permission publisher when it creates a membership. This happens in these cases:
- A user is provisioned into a tenant.
- An invitation creates the account of a new address together with its membership.
- A user joins a tenant when the user signs in to it (`JoinTenant`).

An invitation that creates no membership SHALL publish nothing. A pending invitation publishes when the user accepts it. An invitation of a member publishes nothing.

#### Scenario: Provisioning user into tenant publishes view permission
- **WHEN** a caller provisions a user into a tenant
- **THEN** the system creates the membership record in storage and asynchronously publishes an envelope with op `WRITE`, subject `user:<user_id>`, relation `can_view`, and object `tenant:<tenant_id>` via the configured permission publisher

#### Scenario: Inviting a new address publishes view permission
- **WHEN** a caller invites an address with no account to a tenant that does not require company sign-in
- **THEN** the system creates the account and the membership and returns an invitation link
- **AND** the system publishes an envelope with op `WRITE` and relation `can_view` for that account

#### Scenario: A pending invitation publishes when it is accepted
- **WHEN** a caller invites an address whose account is not a member of the tenant, or a new address to a tenant that requires company sign-in
- **THEN** the system stores a pending invitation, returns no invitation link and publishes nothing
- **AND** when the user signs in to the tenant and `JoinTenant` creates the membership, the system publishes an envelope
- **AND** that envelope has op `WRITE` and relation `can_view` for that user

#### Scenario: Inviting an existing member is idempotent
- **WHEN** a caller invites a user who is already a member of the tenant
- **THEN** the system keeps the existing membership, publishes nothing and returns no invitation link

### Requirement: Elevated Permissions Are Not Assigned by the Tenant Service
The system SHALL NOT accept a membership role or any other caller-supplied permission level, and SHALL NOT expose an operation to change a member's permissions. Permissions above `can_view` for existing members SHALL be managed through the Authorization Service API.

#### Scenario: Invite and provision requests carry no role
- **WHEN** a caller invites or provisions a user into a tenant
- **THEN** the request contains only the tenant ID and email, and the only permission published is `can_view`

### Requirement: Permission Event Publishing on Tenant Deletion
The system SHALL asynchronously publish `PermissionUpdateEnvelope` events for the tenant containing `DELETE` operations for relations `can_view`, `can_edit`, and `can_delete` on the tenant resource for every member of the tenant via the configured permission publisher when a tenant is deleted. The system SHALL include all members regardless of page size and SHALL split operations into envelopes of at most 100 operations.

#### Scenario: Tenant deletion emits delete permission ops for members
- **WHEN** a tenant is deleted
- **THEN** the system lists every membership (across all pages) before removing the tenant and memberships from storage, and asynchronously publishes `DELETE` operations for `can_view`, `can_edit`, and `can_delete` for each member via the configured permission publisher

#### Scenario: Large tenant deletion is split into multiple envelopes
- **WHEN** a tenant is deleted and the revocation requires more than 100 operations
- **THEN** the system publishes multiple envelopes in order, each containing at most 100 operations

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

### Requirement: Permission Publishing Mode Configuration
The system SHALL support configuring the permission event publishing mode via `PERMISSIONS_PUBLISHING_MODE` with values `noop` (default), `platform`, and `standalone`, and SHALL fail to start if an unrecognized mode or incomplete mode-specific configuration is provided.

#### Scenario: Default noop mode requires no external broker or listener
- **WHEN** the service starts with `PERMISSIONS_PUBLISHING_MODE` unset or set to `noop`
- **THEN** the system initializes a no-op permission publisher without requiring Kafka or Authorization Service gRPC configuration

#### Scenario: Platform mode publishes permission updates to Kafka
- **WHEN** the service starts with `PERMISSIONS_PUBLISHING_MODE=platform` and valid `KAFKA_BROKERS` and `KAFKA_PERMISSIONS_TOPIC`
- **THEN** the system initializes the Kafka permission publisher and dispatches `PermissionUpdateEnvelope` events keyed by tenant ID to the configured Kafka topic

#### Scenario: Standalone mode publishes permission updates over gRPC
- **WHEN** the service starts with `PERMISSIONS_PUBLISHING_MODE=standalone` and valid `AUTHORIZATION_SERVICE_GRPC_ADDRESS`, `SERVICE_TOKEN_URL`, `SERVICE_CLIENT_ID`, and `SERVICE_CLIENT_SECRET`
- **THEN** the system initializes the gRPC permission publisher without requiring Kafka configuration and dispatches `PermissionUpdateEnvelope` events to `PermissionIngestionService/IngestPermissionUpdate`

#### Scenario: Invalid mode or missing standalone configuration rejects startup
- **WHEN** the service starts with an unrecognized `PERMISSIONS_PUBLISHING_MODE` or with `PERMISSIONS_PUBLISHING_MODE=standalone` missing required gRPC address or OAuth2 client-credentials configuration
- **THEN** the system fails startup with a configuration error

### Requirement: Standalone gRPC Permission Publishing with OAuth2 Bearer Authentication
In `standalone` mode, the system SHALL asynchronously send `PermissionUpdateEnvelope` messages to the Authorization Service `PermissionIngestionService/IngestPermissionUpdate` gRPC endpoint, authenticating every request with an OAuth2 client-credentials bearer token in the `authorization` metadata header and automatically refreshing tokens before expiry.

#### Scenario: Outgoing gRPC permission update includes valid Bearer JWT
- **WHEN** a permission update envelope is dispatched in `standalone` mode
- **THEN** the system obtains a bearer access token via the OAuth2 client-credentials flow, attaches it to the outgoing gRPC `authorization` request metadata, and invokes `IngestPermissionUpdate`

#### Scenario: Valid OAuth2 access token is reused across gRPC calls and refreshed when expired
- **WHEN** multiple permission update envelopes are dispatched in `standalone` mode while the cached access token remains valid
- **THEN** the system reuses the cached token without querying the token endpoint again, and fetches a fresh token automatically once the cached token expires

#### Scenario: Pending gRPC permission updates are flushed on shutdown
- **WHEN** the permission publisher is closed during service shutdown in `standalone` mode
- **THEN** the system drains and sends any queued permission update envelopes up to a bounded drain deadline before cancelling remaining RPCs and closing the underlying gRPC connection

