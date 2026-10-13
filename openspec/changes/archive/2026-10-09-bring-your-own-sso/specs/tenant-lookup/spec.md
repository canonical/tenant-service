## Purpose

`LookupTenants` tells which tenants an address or an account belongs to. With company sign-in, an address can also enter tenants that it does not belong to yet. These are the tenants that invited the address and the tenants that auto-join its domain. A user can pick these tenants at sign-in only if something lists them. This is also true for an address that has no account. `ListSignInTenants` lists them. The login UI asks it when a user has entered an address.

Key decisions:

- An RPC of its own lists the join candidates: `ListSignInTenants`, in `TenantSignInService`. It is gRPC only, for the login UI on the internal network. We turned down a field on `LookupTenants` that asks for the candidates, for 2 reasons:
  - The lookup has an HTTP route. With such a field, anyone who reaches the API can ask which tenants invited an address or auto-join its domain.
  - With such a field, a caller that cannot complete a join shows tenants that the user cannot enter.
- `LookupTenants` lists memberships only. This change only fixes their order.
- An entry of `ListSignInTenants` carries 2 flags and nothing else about the SSO policy of the tenant.
- The personal tenant comes first. The order is fixed here and is not left to each caller.

**Non-goals:**

- Authentication or rate limiting of `LookupTenants`. Rate limiting belongs in front of the service.
- Hiding from a caller of `ListSignInTenants` that a tenant invited an address or auto-joins a domain.
- Enforcement, connections, domains or the MFA policy in the answer.

## ADDED Requirements

### Requirement: The lookup returns the enabled tenants of an address or an account
The system SHALL answer `LookupTenants` (`GET /api/v0/tenants/lookup`, unauthenticated over HTTP and gRPC) for exactly one of `email` and `identity_id`. The answer SHALL list the enabled tenants that the account is a member of. The answer SHALL list the personal tenant first and the other tenants by tenant id. The answer SHALL list memberships only. It does not list a tenant that invited the address or that auto-joins its domain. An address with no account has no memberships, which is not an error.

The system SHALL refuse these calls with `INVALID_ARGUMENT`:

- A call with both fields or with neither field.
- A call with an `email` that is not an address.
- A call with an `identity_id` that is not a UUID.

#### Scenario: Memberships of an address
- **WHEN** the address belongs to an account with a personal tenant, 2 enabled tenants and a disabled tenant
- **THEN** the answer lists the personal tenant, then the 2 enabled tenants by id
- **AND** the answer does not list the disabled tenant

#### Scenario: By account
- **WHEN** the call carries an `identity_id`
- **THEN** the answer lists the enabled tenants of that account without a call to Kratos

#### Scenario: An invited address
- **WHEN** the address has an unexpired invitation to an enabled tenant that it is not a member of
- **THEN** the answer does not list that tenant

### Requirement: ListSignInTenants lists the tenants an address may sign in to
The system SHALL answer `ListSignInTenants`, called by the login UI with an `email`. The answer SHALL list the tenants that the address can sign in to. Each entry SHALL have the flags `invited` and `auto_join_candidate`. The answer SHALL have these groups, in this order:

1. The enabled tenants that the address's account is a member of. The personal tenant is first and the others are by tenant id. Both flags are false.
2. The enabled tenants that have an unexpired invitation for the address that still admits it, by tenant id, with `invited`. A `required` tenant whose domains do not list the domain of the address does not admit it (see `tenant-invitations`).
3. The enabled tenants whose auto-join admits the domain of the address, by tenant id, with `auto_join_candidate`. For this, the effective enforcement is `required`, auto-join is on, and the tenant lists the domain.

A tenant that the account is a member of is never a candidate. A tenant that both invited the address and auto-joins its domain is in the answer once, with both flags. A candidate does not depend on an account for the address. So a candidate entry does not tell if the address has an account.

The system SHALL refuse an `email` that is not an address with `INVALID_ARGUMENT`. The call only reads and runs without a transaction.

#### Scenario: Memberships, then invitations, then auto-join
- **WHEN** the address is a member of tenant A and has an unexpired invitation to tenant B
- **AND** tenant C auto-joins the domain of the address
- **THEN** the answer lists A with both flags false
- **AND** the answer lists B with `invited`
- **AND** the answer lists C with `auto_join_candidate`

#### Scenario: An address with no account
- **WHEN** the address has no account and has an unexpired invitation to an enabled tenant
- **THEN** the answer lists that tenant with `invited`

#### Scenario: Invited and auto-joined by the same tenant
- **WHEN** one tenant both invited the address and auto-joins its domain
- **THEN** the answer lists the tenant once, with `invited` and `auto_join_candidate` both true

#### Scenario: Expired invitation, disabled tenant
- **WHEN** the only invitations of the address are an expired one and one to a disabled tenant
- **THEN** the answer lists neither tenant

#### Scenario: Invitation the tenant's domains no longer allow
- **WHEN** the address has an unexpired invitation to a `required` tenant whose domains do not list the domain of the address
- **THEN** the answer does not list the tenant

### Requirement: ListSignInTenants is not reachable over HTTP
The system SHALL serve `ListSignInTenants` over gRPC only, with no HTTP route. The login UI calls it before the user has a token. So the system SHALL serve it without a token, as it serves `LookupTenants`. The gRPC listener is reachable from the internal network only, and this confines the call.

#### Scenario: No HTTP route
- **WHEN** the HTTP routes of the service are listed
- **THEN** no route leads to `ListSignInTenants`

#### Scenario: No token
- **WHEN** a caller calls `ListSignInTenants` without a token while authentication is enabled
- **THEN** the service serves the call
