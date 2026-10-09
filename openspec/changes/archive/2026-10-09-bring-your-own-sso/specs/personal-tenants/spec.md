## Purpose

Every sign-in ends in a tenant. A user who registers alone, or who was removed from every tenant, needs one to sign in to: the **personal tenant**, a tenant that belongs to one account, with that account as its owner and only member. Before this change the registration webhook created a disabled tenant per registration, which no sign-in could use until a platform admin enabled it.

Key decisions:

- One call creates a personal tenant, `CreatePersonalTenant`, and the registration webhook is that same call under another caller. Only the source of the account's address differs: Kratos sends it with the webhook, and the RPC, which has the id alone, reads it from Kratos. Kratos at registration and the login UI at a sign-in of an account with no tenant can run at the same time; the outcome is the same whichever comes first.
- It is an RPC of its own, served over gRPC only. A field on `CreateTenant` was turned down: `CreateTenant` has an HTTP route for platform admins, takes a name and publishes no permission event; a personal tenant is asked for by a service, names itself and publishes its owner's.
- The call refuses an account that already has somewhere to sign in: an enabled tenant, or a pending invitation to one. Leaving that to the callers was turned down: the webhook knows nothing it could decide with. A personal tenant for every account would remove the refusal; it was not chosen and stays possible.
- A personal tenant is created enabled. Keeping it disabled until an activation step was turned down: there is no such step, the lookup skips a disabled tenant and sign-in refuses it, so its owner would have nowhere to go.
- A personal tenant stays personal: it takes no other member and has no SSO policy.

**Non-goals:** a personal tenant for every account; converting the disabled tenants that earlier registrations created; an activation step for self-registered tenants.

## ADDED Requirements

### Requirement: Creating a personal tenant
The system SHALL create an account's personal tenant with `CreatePersonalTenant`, an RPC of `TenantSignInService` served over gRPC only, with no HTTP route. The login UI calls it with the `identity_id` of an account that signs in and has no tenant. The system SHALL, in this order:

1. Refuse an `identity_id` that is not a UUID with `INVALID_ARGUMENT`.
2. Read the account from Kratos, and answer `NOT_FOUND` if Kratos does not know it.
3. Return the account's personal tenant if it has one, and write nothing.
4. Refuse with `FAILED_PRECONDITION`, reason `HAS_TENANT`, an account whose address has an unexpired invitation to an enabled tenant that still admits it, or that is a member of an enabled tenant.
5. Otherwise create the tenant, enabled, named `<address>'s Org` and marked with the account's id, make the account its member, publish the owner's permission event (a `WRITE` of `can_delete` on the tenant for the account, keyed by the tenant id), and log the admin action `create_personal_tenant`.

An account has at most one personal tenant, also when two calls run at the same time: one creates it, the other returns the same tenant and publishes nothing. `CreateTenant` creates no personal tenant: it requires a name, takes no account and publishes no permission event.

#### Scenario: An account with no tenant
- **WHEN** the call names an account with no tenant and no pending invitation
- **THEN** a tenant named `<address>'s Org` is created enabled, the account is its only member, and `can_delete` is published for the account

#### Scenario: A repeat
- **WHEN** the call is repeated for an account that has its personal tenant
- **THEN** that tenant is returned and nothing is written or published

#### Scenario: An account that is a member of a tenant
- **WHEN** the account is a member of an enabled tenant and has no personal tenant
- **THEN** the call fails with `FAILED_PRECONDITION`, reason `HAS_TENANT`, and nothing is created

#### Scenario: An account with a pending invitation
- **WHEN** the account's address has an unexpired invitation to an enabled tenant
- **THEN** the call fails with `HAS_TENANT`, and the invitation stays pending

#### Scenario: Only disabled tenants
- **WHEN** every tenant the account is a member of is disabled
- **THEN** the personal tenant is created

#### Scenario: Two calls at once
- **WHEN** two calls for the same account run concurrently
- **THEN** one tenant exists afterwards, both calls return it, and `can_delete` was published once

### Requirement: Registration creates the personal tenant through the same call
The system SHALL handle the registration webhook (`POST /api/v0/webhooks/registration`, called by Kratos after a self-registration) by making the personal-tenant call above, from its third step on, for the account id and the address of the request. It SHALL NOT ask Kratos for the account: the webhook does not depend on Kratos's admin API. It SHALL answer 200 when the tenant was created, when it existed already, and when the call answered `HAS_TENANT`; in the last case the account gets no tenant and no membership. A tenant the webhook created is also logged as the admin action `self_registration` with the account as the actor. A request without an account id or an address, and any other failure, answers 500.

#### Scenario: A user registers
- **WHEN** Kratos calls the webhook for a new account with no pending invitation
- **THEN** the account has an enabled personal tenant and `can_delete` on it

#### Scenario: Kratos cannot be reached
- **WHEN** the webhook is called while Kratos's admin API does not answer
- **THEN** the personal tenant is created all the same

#### Scenario: A user registers with an invited address
- **WHEN** the registered address has an unexpired invitation to an enabled tenant
- **THEN** the webhook answers 200, creates no tenant and no membership, and the invitation stays pending

### Requirement: A personal tenant takes no other member and no SSO policy
The system SHALL refuse with `FAILED_PRECONDITION`, reason `PERSONAL_TENANT`, for a personal tenant: `InviteMember`, `ProvisionUser`, `RemoveTenantUser` (its owner would be locked out of it for good), `GetTenantSSOPolicy`, `PutTenantSSOPolicy`, `SetTenantSSODomains` and `RemoveTenantSSOBinding`. A personal tenant admits nobody: `JoinTenant` answers `NOT_ADMITTED` and `GetSignInContext` answers `OFF`, no connection ids and both admission flags false. `UpdateTenant`, `DeleteTenant` and the MFA policy RPCs make no exception for a personal tenant.

#### Scenario: Inviting to a personal tenant
- **WHEN** `InviteMember` names a personal tenant
- **THEN** it fails with `FAILED_PRECONDITION`, reason `PERSONAL_TENANT`, and no account or invitation is created

#### Scenario: Removing the owner
- **WHEN** `RemoveTenantUser` names a personal tenant and its owner
- **THEN** it fails with `PERSONAL_TENANT` and the membership stays

### Requirement: The personal tenant is listed first
The system SHALL list an account's personal tenant first, and its other tenants by id after it, in `ListMyTenants`, `ListUserTenants`, `LookupTenants` and `ListSignInTenants`. An account with no personal tenant has its tenants listed by id, as before.

#### Scenario: A user with a personal tenant and two organisations
- **WHEN** the user's tenants are listed and one organisation is older than the personal tenant
- **THEN** the personal tenant is first, then the organisations by id
