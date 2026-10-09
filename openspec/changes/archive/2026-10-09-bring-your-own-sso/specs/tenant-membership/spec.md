## Purpose

Two operations change a tenant's members directly, without an invitation: a platform admin provisions a user into a tenant, and a tenant admin removes a member. Provisioning existed; this change makes it fit personal tenants and pending invitations and gives its refusals a status code a caller can act on. Removal is new: until now a membership ended only when its tenant was deleted.

Key decisions:

- Provisioning stays the direct way in: it makes the address a member at once, creating the account if needed, and a pending invitation of that address is spent by it.
- Removal revokes every relation the member may hold on the tenant: `can_view`, `can_edit` and `can_delete`. Revoking `can_view` alone, the one relation this service grants to a member, was turned down: a relation granted through the Authorization Service would outlive the membership, and this service does not know which were granted.
- Removal ends the membership and nothing else: the account, its sessions and its other tenants are untouched.

**Non-goals:** ending the removed user's sessions or tokens (the token webhook refuses the tenant the next time a token is issued); keeping a removed user out of a tenant whose auto-join admits their address; removing the owner of a personal tenant; membership roles.

## ADDED Requirements

### Requirement: ProvisionUser makes an address a member at once
The system SHALL handle `ProvisionUser` (`POST /api/v0/tenants/{tenant_id}/users`, called by platform admins through the gateway) by validating the request (`tenant_id` a UUID, `email` an address; otherwise `INVALID_ARGUMENT`), asking Kratos for the account of the address, and then, in this order:

1. Answer `NOT_FOUND` for an unknown tenant.
2. Refuse a personal tenant with `FAILED_PRECONDITION`, reason `PERSONAL_TENANT`.
3. Answer `ALREADY_EXISTS` when the account is a member already, also when a concurrent call made it one first.
4. Create the account in Kratos if the address has none.
5. Create the membership, delete the address's invitation to the tenant if it has one, publish `can_view` for the account, and log the admin action `provision_user` with the resource `<tenant id>:<address>`.

No account is created for a tenant that does not exist or is a personal tenant.

#### Scenario: A new address
- **WHEN** a platform admin provisions an address with no account into an existing tenant
- **THEN** the account is created, it is a member, and `can_view` is published

#### Scenario: An address with a pending invitation
- **WHEN** the address has a pending invitation to the tenant
- **THEN** it becomes a member and the invitation is deleted

#### Scenario: A member already
- **WHEN** the address belongs to a member of the tenant
- **THEN** the call fails with `ALREADY_EXISTS` and nothing is published

#### Scenario: Unknown tenant
- **WHEN** no tenant has the `tenant_id` and the address has no account
- **THEN** the call fails with `NOT_FOUND` and no account is created

### Requirement: RemoveTenantUser ends a membership
The system SHALL remove a member from a tenant with `RemoveTenantUser` (`DELETE /api/v0/tenants/{tenant_id}/users/{user_id}`, called by tenant admins through the gateway). It SHALL, in this order: refuse a `tenant_id` or `user_id` that is not a UUID with `INVALID_ARGUMENT`; answer `NOT_FOUND` for an unknown tenant; refuse a personal tenant with `FAILED_PRECONDITION`, reason `PERSONAL_TENANT`; answer `NOT_FOUND` when the user is not a member; and otherwise delete the membership, publish the deletion of `can_view`, `can_edit` and `can_delete` on the tenant for the user, and log the admin action `remove_tenant_user` with the resource `<tenant id>:<user id>`. The removal does not ask Kratos and changes nothing but the membership.

#### Scenario: A member is removed
- **WHEN** a tenant admin removes a member of the tenant
- **THEN** the membership is deleted and the deletion of all three relations is published for that user

#### Scenario: Not a member
- **WHEN** the user is not a member of the tenant
- **THEN** the call fails with `NOT_FOUND` and nothing is published

#### Scenario: A removed user at an auto-join tenant
- **WHEN** the removed user's address is in the domains of a tenant with auto-join on, and `JoinTenant` is called for it again
- **THEN** the user becomes a member again
