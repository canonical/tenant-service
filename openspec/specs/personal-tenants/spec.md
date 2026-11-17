# personal-tenants Specification

## Purpose

Every sign-in ends in a tenant. A user who registers alone, or who was removed from every tenant, needs a tenant to sign in to. This tenant is the **personal tenant**. It belongs to one account, and that account is its owner and only member. Before this change, the registration webhook created a disabled tenant for each registration. No sign-in was able to use that tenant until a platform admin enabled it.

Key decisions:

- One call creates a personal tenant: `CreatePersonalTenant`. The registration webhook is the same call from another caller. Only the source of the account's address differs. Kratos sends the address with the webhook. The RPC has only the id, so it reads the address from Kratos. Kratos at registration and the login UI at the sign-in of an account with no tenant can run at the same time. The outcome is the same, whichever runs first.
- The call is an RPC of its own, and the system serves it over gRPC only. We turned down a field on `CreateTenant`. `CreateTenant` has an HTTP route for platform admins, takes a name and publishes no permission event. A service asks for a personal tenant, the tenant names itself, and the system publishes the permission event of its owner.
- The call refuses an account that already has a place to sign in. This is an enabled tenant, or a pending invitation to one. We turned down leaving this to the callers, because the webhook knows nothing that it can decide with. A personal tenant for every account removes the refusal. We did not choose it, but it stays possible.
- The system creates a personal tenant enabled. We turned down keeping it disabled until an activation step. There is no such step. The lookup skips a disabled tenant and the sign-in refuses it, so its owner has nowhere to go.
- A personal tenant stays personal. It takes no other member and has no SSO policy.

**Non-goals:**

- A personal tenant for every account.
- Conversion of the disabled tenants that earlier registrations created.
- An activation step for self-registered tenants.

## Requirements
### Requirement: Creating a personal tenant
The system SHALL create the personal tenant of an account with `CreatePersonalTenant`. `CreatePersonalTenant` is an RPC of `TenantSignInService`. The system SHALL serve it over gRPC only, with no HTTP route. The login UI calls it with the `identity_id` of an account that signs in and has no tenant. The system SHALL do these steps in this order:

1. Refuse an `identity_id` that is not a UUID with `INVALID_ARGUMENT`.
2. Read the account from Kratos, and answer `NOT_FOUND` if Kratos does not know it.
3. Return the personal tenant of the account if it has one, and write nothing.
4. Refuse with `FAILED_PRECONDITION` and reason `HAS_TENANT` in 2 cases. In the first case, the address of the account has an unexpired invitation to an enabled tenant that still admits it. In the second case, the account is a member of an enabled tenant.
5. Otherwise create the tenant, enabled, named `<address>'s Org` and marked with the id of the account. Make the account its member. Then publish the permission event of the owner, which is a `WRITE` of `can_delete` on the tenant for the account, keyed by the tenant id. Log the admin action `create_personal_tenant`.

An account has at most one personal tenant, also when 2 calls run at the same time. One call creates the tenant. The other call returns the same tenant and publishes nothing. `CreateTenant` creates no personal tenant. It requires a name, takes no account and publishes no permission event.

#### Scenario: An account with no tenant
- **WHEN** the call names an account with no tenant and no pending invitation
- **THEN** the system creates an enabled tenant named `<address>'s Org`
- **AND** the account is the only member of the tenant
- **AND** the system publishes `can_delete` for the account

#### Scenario: A repeat
- **WHEN** the call is repeated for an account that has its personal tenant
- **THEN** the call returns that tenant and the system writes and publishes nothing

#### Scenario: An account that is a member of a tenant
- **WHEN** the account is a member of an enabled tenant and has no personal tenant
- **THEN** the call fails with `FAILED_PRECONDITION` and reason `HAS_TENANT`
- **AND** the system creates nothing

#### Scenario: An account with a pending invitation
- **WHEN** the address of the account has an unexpired invitation to an enabled tenant
- **THEN** the call fails with `HAS_TENANT`
- **AND** the invitation stays pending

#### Scenario: Only disabled tenants
- **WHEN** every tenant that the account is a member of is disabled
- **THEN** the system creates the personal tenant

#### Scenario: Two calls at once
- **WHEN** two calls for the same account run concurrently
- **THEN** one tenant exists afterwards and both calls return it
- **AND** the system published `can_delete` once

### Requirement: Registration creates the personal tenant through the same call
The system SHALL handle the registration webhook (`POST /api/v0/webhooks/registration`, called by Kratos after a self-registration). It SHALL make the personal-tenant call above, from its third step on, for the account id and the address of the request. It SHALL NOT ask Kratos for the account, because the webhook does not depend on the admin API of Kratos.

The webhook SHALL answer 200 in these cases:

- The system created the tenant.
- The tenant existed already.
- The call answered `HAS_TENANT`. In this case the account gets no tenant and no membership.

When the webhook created a tenant, the system also logs the admin action `self_registration` with the account as the actor. A request without an account id or an address answers 500. Any other failure also answers 500.

#### Scenario: A user registers
- **WHEN** Kratos calls the webhook for a new account with no pending invitation
- **THEN** the account has an enabled personal tenant and `can_delete` on it

#### Scenario: Kratos cannot be reached
- **WHEN** Kratos calls the webhook while the admin API of Kratos does not answer
- **THEN** the system creates the personal tenant all the same

#### Scenario: A user registers with an invited address
- **WHEN** the registered address has an unexpired invitation to an enabled tenant
- **THEN** the webhook answers 200
- **AND** the system creates no tenant and no membership
- **AND** the invitation stays pending

### Requirement: A personal tenant takes no other member and no SSO policy
The system SHALL refuse these calls for a personal tenant with `FAILED_PRECONDITION` and reason `PERSONAL_TENANT`:

- `InviteMember`
- `ProvisionUser`
- `RemoveTenantUser`, because the removal locks its owner out of the tenant for good
- `GetTenantSSOPolicy`
- `PutTenantSSOPolicy`
- `SetTenantSSODomains`
- `RemoveTenantSSOBinding`

A personal tenant admits nobody. `JoinTenant` answers `NOT_ADMITTED`. `GetSignInContext` answers `OFF`, gives no connection ids, and sets both admission flags to false. `UpdateTenant`, `DeleteTenant` and the MFA policy RPCs make no exception for a personal tenant.

#### Scenario: Inviting to a personal tenant
- **WHEN** `InviteMember` names a personal tenant
- **THEN** it fails with `FAILED_PRECONDITION` and reason `PERSONAL_TENANT`
- **AND** the system creates no account and no invitation

#### Scenario: Removing the owner
- **WHEN** `RemoveTenantUser` names a personal tenant and its owner
- **THEN** it fails with `PERSONAL_TENANT` and the membership stays

### Requirement: The personal tenant is listed first
The system SHALL list the personal tenant of an account first, and its other tenants by id after it. This applies to `ListMyTenants`, `ListUserTenants`, `LookupTenants` and `ListSignInTenants`. For an account with no personal tenant, the system lists its tenants by id, as before.

#### Scenario: A user with a personal tenant and two organisations
- **WHEN** the system lists the tenants of the user, and one organisation is older than the personal tenant
- **THEN** the personal tenant is first, then the organisations by id

