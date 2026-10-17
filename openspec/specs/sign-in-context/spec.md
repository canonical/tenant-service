# sign-in-context Specification

## Purpose

Before the login UI accepts a sign-in to a tenant, it must know what the tenant asks of the user. It must know these things:

- If the user is a member.
- If company sign-in is offered or required, and through which connections.
- If the tenant admits an address that is not a member yet.
- If MFA is required.

The SSO service asks the same before it lets a company sign-in through. `GetSignInContext` answers all of it in one call, for one tenant and one address or account.

Key decisions:

- This service applies the rules: effective enforcement, domains, and what admits an address. We turned down returning the stored policy and letting each caller apply the rules. In that case 2 services each implement the rules, and they can differ.
- One answer serves both moments of a sign-in: an address before the sign-in, and an account once there is a session.
- The call is gRPC only. The answer says if an address has an account and if it is a member of a tenant. A public route must not reveal this.
- The call is a read with no transaction. It waits on Kratos between its reads and writes nothing.

**Non-goals:**

- The decision if a sign-in is accepted. The caller decides this from the answer.
- A list of an address's tenants. `ListSignInTenants` does this.
- Anything about a connection other than its id.

## Requirements
### Requirement: GetSignInContext answers what a sign-in to one tenant needs
The system SHALL answer `GetSignInContext`, called by the login UI and by the SSO service with a `tenant_id` and an `email` or an `identity_id`. The answer SHALL have the fields below. When the call has an `identity_id`, the system uses the address of that account and ignores `email`.

- `account_exists`: the call has an `identity_id`, or the address has an account.
- `member`: the account is a member of the tenant and the tenant is enabled.
- `enforcement`: the effective enforcement of the tenant.
- `connection_ids`: the connections of the tenant's active bindings. The list has them when the tenant lists no domains or lists the domain of the address. Otherwise the list is empty.
- `invitation_admits` and `auto_join_admits`: for an address that is not a member, these show if a pending invitation or auto-join admits it. The rules are in "What admits an address to a tenant" in `tenant-invitations`. Both are false for a member.
- `mfa_requirement`: the MFA requirement of the tenant.

#### Scenario: A member of a tenant that offers company sign-in
- **WHEN** the address belongs to an account that is a member of an enabled tenant
- **AND** the tenant has `optional` enforcement, one active binding and the MFA requirement `required`
- **THEN** `account_exists` and `member` are true
- **AND** `enforcement` is `OPTIONAL` and `mfa_requirement` is `REQUIRED`
- **AND** the answer has the connection id of the binding and both admission flags are false

#### Scenario: An address with no account that auto-join admits
- **WHEN** the address has no account, and the tenant is `required`, has auto-join on and lists the domain of the address
- **THEN** `account_exists` and `member` are false
- **AND** `auto_join_admits` is true and the answer has the active connection ids of the tenant

#### Scenario: An address with a pending invitation
- **WHEN** the address is not a member and has an unexpired invitation to the tenant
- **THEN** `invitation_admits` is true, both when the address has an account and when it has none

#### Scenario: An address outside the tenant's domains
- **WHEN** the tenant lists domains and the domain of the address is not one of them
- **THEN** `connection_ids` is empty
- **AND** `enforcement` is still the effective enforcement of the tenant

#### Scenario: By account
- **WHEN** the call carries an `identity_id` and an `email` of another account
- **THEN** the system computes the answer for the address of the `identity_id`'s account

#### Scenario: A disabled tenant
- **WHEN** the tenant is disabled
- **THEN** `member` and both admission flags are false, also for an account with a membership

### Requirement: Refusals of GetSignInContext
The system SHALL refuse a `GetSignInContext` call with `INVALID_ARGUMENT` in these cases:

- `tenant_id` is not a UUID.
- The call has neither `email` nor `identity_id`.
- The call has an `identity_id` that is not a UUID.
- The call has only `email`, and it is not an address.

The system SHALL answer `NOT_FOUND` for an unknown tenant and for an `identity_id` that Kratos does not know. An address with no account is not an error.

#### Scenario: Unknown tenant
- **WHEN** no tenant has the `tenant_id`
- **THEN** the call fails with `NOT_FOUND`

#### Scenario: Unknown account
- **WHEN** Kratos has no identity with the `identity_id`
- **THEN** the call fails with `NOT_FOUND`

### Requirement: GetSignInContext is not reachable over HTTP
The system SHALL serve `GetSignInContext`, an RPC of `TenantSignInService`, over gRPC only, with no HTTP route. The RPCs of `TenantSignInService` are called from the internal network while a user signs in, before there is a token. So the system SHALL serve them without a token, as it serves `LookupTenants`. Every other RPC authenticates the token of the caller.

#### Scenario: No HTTP route
- **WHEN** the HTTP routes of the service are listed
- **THEN** no route leads to `GetSignInContext`

#### Scenario: No token
- **WHEN** a caller calls an RPC of `TenantSignInService` without a token while authentication is enabled
- **THEN** the service serves the call

