## MODIFIED Requirements

### Requirement: Permission Event Publishing on Member Provisioning and Invites
The system SHALL asynchronously publish a `PermissionUpdateEnvelope` event keyed by tenant ID containing a `WRITE` operation with relation `can_view` on the tenant resource when it creates a membership: when a user is provisioned into a tenant, when an invitation creates the account of a new address together with its membership, and when a user joins a tenant by signing in to it (`JoinTenant`). An invitation that creates no membership SHALL publish nothing: a pending invitation publishes when it is accepted, and inviting a member publishes nothing.

#### Scenario: Provisioning user into tenant publishes view permission
- **WHEN** a caller provisions a user into a tenant
- **THEN** the system creates the membership record in storage and asynchronously publishes an envelope with op `WRITE`, subject `user:<user_id>`, relation `can_view`, and object `tenant:<tenant_id>` keyed by `tenant_id` to Kafka

#### Scenario: Inviting a new address publishes view permission
- **WHEN** a caller invites an address with no account to a tenant that does not require company sign-in
- **THEN** the system creates the account and the membership, returns an invitation link, and publishes an envelope with op `WRITE` and relation `can_view` for that account

#### Scenario: A pending invitation publishes when it is accepted
- **WHEN** a caller invites an address whose account is not a member of the tenant, or a new address to a tenant that requires company sign-in
- **THEN** the system stores a pending invitation, returns no invitation link and publishes nothing
- **AND** when the user signs in to the tenant and `JoinTenant` creates the membership, the system publishes an envelope with op `WRITE` and relation `can_view` for that user

#### Scenario: Inviting an existing member is idempotent
- **WHEN** a caller invites a user who is already a member of the tenant
- **THEN** the system keeps the existing membership, publishes nothing and returns no invitation link
