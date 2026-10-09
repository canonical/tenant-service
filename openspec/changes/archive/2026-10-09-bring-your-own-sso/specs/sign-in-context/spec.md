## Purpose

Before a sign-in to a tenant is accepted, the login UI has to know what that tenant asks of this user: whether the user is a member, whether company sign-in is offered or required and through which connections, whether the tenant would admit an address that is not a member yet, and whether MFA is required. The SSO service asks the same before it lets a company sign-in through. `GetSignInContext` answers all of it in one call, for one tenant and one address or account.

Key decisions:

- The rules (effective enforcement, domains, what admits an address) are applied here. Returning the stored policy and letting each caller apply them was turned down: two services would each implement the rules, and could differ.
- One answer for both moments of a sign-in: an address before it, an account once there is a session.
- gRPC only. The answer says whether an address has an account and whether it is a member of a tenant, which no public route may reveal.
- A read with no transaction: it waits on Kratos between its reads and writes nothing.

**Non-goals:** deciding whether a sign-in is accepted (the caller decides, from the answer); listing the tenants of an address (that is `ListSignInTenants`); returning anything about a connection beyond its id.

## ADDED Requirements

### Requirement: GetSignInContext answers what a sign-in to one tenant needs
The system SHALL answer `GetSignInContext`, called by the login UI and by the SSO service with a `tenant_id` and an `email` or an `identity_id`, with the fields below. When `identity_id` is given, the account's own address is used and `email` is ignored.

- `account_exists`: an `identity_id` was given, or the address has an account.
- `member`: the account is a member of the tenant and the tenant is enabled.
- `enforcement`: the tenant's effective enforcement.
- `connection_ids`: the connections of the tenant's active bindings, when the tenant lists no domains or lists the domain of the address; empty otherwise.
- `invitation_admits` and `auto_join_admits`: for an address that is not a member, whether a pending invitation or auto-join admits it, by the rules of "What admits an address to a tenant" in `tenant-invitations`. Both are false for a member.
- `mfa_requirement`: the tenant's MFA requirement.

#### Scenario: A member of a tenant that offers company sign-in
- **WHEN** the address belongs to an account that is a member of an enabled tenant with `optional` enforcement, one active binding and the MFA requirement `required`
- **THEN** the answer is `account_exists` and `member` true, `enforcement` `OPTIONAL`, that connection id, `mfa_requirement` `REQUIRED`, and both admission flags false

#### Scenario: An address with no account that auto-join admits
- **WHEN** the address has no account, and the tenant is `required`, has auto-join on and lists the address's domain
- **THEN** the answer is `account_exists` and `member` false, `auto_join_admits` true, and the tenant's active connection ids

#### Scenario: An address with a pending invitation
- **WHEN** the address is not a member and has an unexpired invitation to the tenant
- **THEN** `invitation_admits` is true, whether or not the address has an account

#### Scenario: An address outside the tenant's domains
- **WHEN** the tenant lists domains and the address's domain is not one of them
- **THEN** `connection_ids` is empty, and `enforcement` is still the tenant's effective enforcement

#### Scenario: By account
- **WHEN** the call carries an `identity_id` and an `email` of another account
- **THEN** the answer is computed for the address of the `identity_id`'s account

#### Scenario: A disabled tenant
- **WHEN** the tenant is disabled
- **THEN** `member` and both admission flags are false, even for an account with a membership

### Requirement: Refusals of GetSignInContext
The system SHALL refuse a `GetSignInContext` call with `INVALID_ARGUMENT` when `tenant_id` is not a UUID, when neither `email` nor `identity_id` is given, when `identity_id` is given and is not a UUID, or when only `email` is given and is not an address. It SHALL answer `NOT_FOUND` for an unknown tenant and for an `identity_id` Kratos does not know. An address with no account is not an error.

#### Scenario: Unknown tenant
- **WHEN** no tenant has the `tenant_id`
- **THEN** the call fails with `NOT_FOUND`

#### Scenario: Unknown account
- **WHEN** Kratos has no identity with the `identity_id`
- **THEN** the call fails with `NOT_FOUND`

### Requirement: GetSignInContext is not reachable over HTTP
The system SHALL serve `GetSignInContext`, an RPC of `TenantSignInService`, over gRPC only, with no HTTP route, and authenticate the caller's token as for every other RPC.

#### Scenario: No HTTP route
- **WHEN** the service's HTTP routes are listed
- **THEN** none leads to `GetSignInContext`
