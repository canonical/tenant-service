# tenant-lookup Specification

## Purpose

`LookupTenants` tells which tenants an address or an account belongs to. With company sign-in an address can also enter tenants it does not belong to yet: tenants that invited it and tenants that auto-join its domain. A user can only pick those at sign-in if something lists them, also for an address that has no account. `ListSignInTenants` does: it is what the login UI asks when a user has entered an address.

Key decisions:

- The join candidates are listed by an RPC of their own, `ListSignInTenants`, in `TenantSignInService`: gRPC only, for an authenticated caller. A field on `LookupTenants` that asks for them was turned down: the lookup is unauthenticated and has an HTTP route, so anyone could ask which tenants invited an address or auto-join its domain, and a caller that cannot complete a join would show tenants the user cannot enter.
- `LookupTenants` lists memberships only. This change only fixes their order.
- An entry of `ListSignInTenants` carries two flags and nothing else about the tenant's SSO policy.
- The personal tenant comes first. The order is fixed here, not left to each caller.

**Non-goals:** authenticating or rate limiting `LookupTenants` (rate limiting belongs in front of the service); hiding from a caller of `ListSignInTenants` that a tenant invited an address or auto-joins a domain; returning enforcement, connections, domains or the MFA policy.

## Requirements
### Requirement: The lookup returns the enabled tenants of an address or an account
The system SHALL answer `LookupTenants` (`GET /api/v0/tenants/lookup`, unauthenticated over HTTP and gRPC) for exactly one of `email` and `identity_id` with the enabled tenants the account is a member of: its personal tenant first, the others by tenant id. It SHALL list memberships only: a tenant that invited the address, or that auto-joins its domain, is not in the answer. An address with no account has no memberships, which is not an error. The system SHALL refuse with `INVALID_ARGUMENT` a call with both or neither of the two fields, an `email` that is not an address, and an `identity_id` that is not a UUID.

#### Scenario: Memberships of an address
- **WHEN** the address belongs to an account with a personal tenant, two enabled tenants and one disabled tenant
- **THEN** the answer lists the personal tenant, then the two enabled tenants by id, and not the disabled one

#### Scenario: By account
- **WHEN** the call carries an `identity_id`
- **THEN** the answer lists that account's enabled tenants without asking Kratos

#### Scenario: An invited address
- **WHEN** the address has an unexpired invitation to an enabled tenant it is not a member of
- **THEN** the answer does not list that tenant

### Requirement: ListSignInTenants lists the tenants an address may sign in to
The system SHALL answer `ListSignInTenants`, called by the login UI with an `email`, with the tenants the address may sign in to, each with the flags `invited` and `auto_join_candidate`:

- first the enabled tenants the address's account is a member of, its personal tenant first and the others by tenant id, with both flags false;
- then the enabled tenants that have an unexpired invitation for the address that still admits it (`tenant-invitations`: not a `required` tenant whose domains do not list the address's domain), by tenant id, with `invited`;
- then the enabled tenants whose auto-join admits the address's domain (effective enforcement `required`, auto-join on, the domain listed), by tenant id, with `auto_join_candidate`.

A tenant the account is a member of is never a candidate. A tenant that both invited the address and auto-joins its domain is listed once, with both flags. Candidates do not depend on the address having an account, so a candidate entry does not tell whether it has one. The system SHALL refuse an `email` that is not an address with `INVALID_ARGUMENT`. The call reads only and runs without a transaction.

#### Scenario: Memberships, then invitations, then auto-join
- **WHEN** the address is a member of tenant A, has an unexpired invitation to tenant B, and its domain is auto-joined by tenant C
- **THEN** the answer lists A with both flags false, B with `invited`, and C with `auto_join_candidate`

#### Scenario: An address with no account
- **WHEN** the address has no account and has an unexpired invitation to an enabled tenant
- **THEN** the answer lists that tenant with `invited`

#### Scenario: Invited and auto-joined by the same tenant
- **WHEN** one tenant both invited the address and auto-joins its domain
- **THEN** the tenant is listed once, with `invited` and `auto_join_candidate` both true

#### Scenario: Expired invitation, disabled tenant
- **WHEN** the address's only invitations are an expired one and one to a disabled tenant
- **THEN** neither tenant is listed

#### Scenario: Invitation the tenant's domains no longer allow
- **WHEN** the address has an unexpired invitation to a `required` tenant whose domains do not list the address's domain
- **THEN** the tenant is not listed

### Requirement: ListSignInTenants is not reachable over HTTP
The system SHALL serve `ListSignInTenants` over gRPC only, with no HTTP route, and authenticate the caller's token as for every other RPC but `LookupTenants`.

#### Scenario: No HTTP route
- **WHEN** the service's HTTP routes are listed
- **THEN** none leads to `ListSignInTenants`

#### Scenario: No token
- **WHEN** `ListSignInTenants` is called without a token while authentication is enabled
- **THEN** the call fails with `UNAUTHENTICATED`

