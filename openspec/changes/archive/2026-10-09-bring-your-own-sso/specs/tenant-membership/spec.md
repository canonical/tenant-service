## Purpose

Two operations change the members of a tenant directly, without an invitation. A platform admin provisions a user into a tenant, and a tenant admin removes a member. Provisioning existed before. This change makes it fit personal tenants and pending invitations. It also gives its refusals a status code that a caller can act on. Removal is new. Until now, a membership ended only when the system deleted its tenant.

Key decisions:

- Provisioning stays the direct way in. It makes the address a member at once, and it creates the account if needed. It also spends a pending invitation of that address.
- Removal revokes every relation that the member can hold on the tenant: `can_view`, `can_edit` and `can_delete`. This service grants only `can_view` to a member. We turned down a removal that revokes `can_view` alone. In that case, a relation granted through the Authorization Service stays after the membership ends. This service does not know which relations were granted.
- Removal ends the membership and nothing else. It does not change the account, its sessions or its other tenants.

**Non-goals:**

- Ending the sessions or tokens of the removed user. The token webhook refuses the tenant the next time a token is issued.
- Keeping a removed user out of a tenant whose auto-join admits their address.
- Removal of a personal tenant's owner.
- Membership roles.

## ADDED Requirements

### Requirement: ProvisionUser makes an address a member at once
The system SHALL handle `ProvisionUser` (`POST /api/v0/tenants/{tenant_id}/users`, called by platform admins through the gateway). First, it SHALL validate the request. `tenant_id` must be a UUID and `email` must be an address. Otherwise the system SHALL answer `INVALID_ARGUMENT`. Then it SHALL ask Kratos for the account of the address. Then the system SHALL do these steps in this order:

1. Answer `NOT_FOUND` for an unknown tenant.
2. Refuse a personal tenant with `FAILED_PRECONDITION` and reason `PERSONAL_TENANT`.
3. Answer `ALREADY_EXISTS` when the account is a member already. This includes the case where a concurrent call made it a member first.
4. Create the account in Kratos if the address has none.
5. Create the membership, and delete the invitation of the address to the tenant if it has one. Then publish `can_view` for the account, and log the admin action `provision_user` with the resource `<tenant id>:<address>`.

The system creates no account for a tenant that does not exist or is a personal tenant.

#### Scenario: A new address
- **WHEN** a platform admin provisions an address with no account into an existing tenant
- **THEN** the system creates the account and the account is a member
- **AND** the system publishes `can_view`

#### Scenario: An address with a pending invitation
- **WHEN** the address has a pending invitation to the tenant
- **THEN** the address becomes a member and the system deletes the invitation

#### Scenario: A member already
- **WHEN** the address belongs to a member of the tenant
- **THEN** the call fails with `ALREADY_EXISTS` and the system publishes nothing

#### Scenario: Unknown tenant
- **WHEN** no tenant has the `tenant_id` and the address has no account
- **THEN** the call fails with `NOT_FOUND` and the system creates no account

### Requirement: RemoveTenantUser ends a membership
The system SHALL remove a member from a tenant with `RemoveTenantUser` (`DELETE /api/v0/tenants/{tenant_id}/users/{user_id}`, called by tenant admins through the gateway). The system SHALL do these steps in this order:

1. Refuse a `tenant_id` or a `user_id` that is not a UUID with `INVALID_ARGUMENT`.
2. Answer `NOT_FOUND` for an unknown tenant.
3. Refuse a personal tenant with `FAILED_PRECONDITION` and reason `PERSONAL_TENANT`.
4. Answer `NOT_FOUND` when the user is not a member.
5. Otherwise delete the membership, and publish the deletion of `can_view`, `can_edit` and `can_delete` on the tenant for the user. Then log the admin action `remove_tenant_user` with the resource `<tenant id>:<user id>`.

The removal does not ask Kratos. It changes only the membership.

#### Scenario: A member is removed
- **WHEN** a tenant admin removes a member of the tenant
- **THEN** the system deletes the membership
- **AND** the system publishes the deletion of all 3 relations for that user

#### Scenario: Not a member
- **WHEN** the user is not a member of the tenant
- **THEN** the call fails with `NOT_FOUND` and the system publishes nothing

#### Scenario: A removed user at an auto-join tenant
- **WHEN** the address of the removed user is in the domains of a tenant with auto-join on
- **AND** `JoinTenant` is called for it again
- **THEN** the user becomes a member again
