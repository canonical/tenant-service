## Purpose

A tenant can require MFA for every sign-in to it: a second factor (an authenticator app or a recovery code) after the first. The tenant's MFA policy is that one choice, the tenant's MFA requirement: `none` or `required`. The API documents its meaning: a sign-in through a company or a public identity provider needs a second factor only at `required`; every other first factor always needs one.

This service stores the value and reports it in `GetSignInContext`. The login UI applies it at sign-in.

Key decisions:

- One column of `tenants`, written by its own RPC, which tenant admins call through the gateway. Writing it through the SSO service, as the SSO policy is, was turned down: no rule about it involves a connection.
- Last write wins. The write locks the tenant's row in the statement that changes it, so the value it logs as replaced is exact when two writes race.
- The change history is the log: the admin-action record of each write carries the value before and after. These records are written at log level `info`, so a deployment that keeps the history runs with `LOG_LEVEL` `info` or `debug`.

**Non-goals:** a security key as the required second factor (the value is `none` or `required`; nothing names a method); a policy per user or per role; a maximum age of the second factor; asking the user for it, which the login UI does.

## ADDED Requirements

### Requirement: A tenant has one MFA policy value
The system SHALL store an MFA requirement for every tenant (`tenants.mfa_requirement`) as `none` or `required`, with `none` for a tenant that never set it, and SHALL report it as `mfa_requirement` in `GetSignInContext`. The database SHALL refuse any other value. The policy applies to any tenant, a personal tenant included.

#### Scenario: A tenant that never set a policy
- **WHEN** the MFA policy of a new tenant is read
- **THEN** the answer is `MFA_REQUIREMENT_NONE`

#### Scenario: Reported at sign-in
- **WHEN** a tenant's MFA requirement is `required` and `GetSignInContext` is called for it
- **THEN** the answer carries `mfa_requirement` `REQUIRED`, whatever the tenant's SSO policy is

### Requirement: Reading and writing the MFA policy
The system SHALL serve `GetTenantMFAPolicy` (`GET /api/v0/tenants/{tenant_id}/mfa-policy`) and `PutTenantMFAPolicy` (`PUT /api/v0/tenants/{tenant_id}/mfa-policy`) to tenant admins and platform admins through the gateway. Both SHALL refuse a `tenant_id` that is not a UUID with `INVALID_ARGUMENT` and answer `NOT_FOUND` for an unknown tenant. `PutTenantMFAPolicy` SHALL refuse a `requirement` other than `NONE` and `REQUIRED` with `INVALID_ARGUMENT`, replace the stored value (last write wins), return the new policy, and log the admin action `put_tenant_mfa_policy` with the caller as the actor, the tenant id as the resource, and the values `before` and `after`. An admin-action record is written at log level `info`: at the default `LOG_LEVEL`, `error`, none is written. A write that waits for the tenant's row longer than the lock timeout fails with `ABORTED`.

#### Scenario: MFA made required
- **WHEN** a tenant admin puts `MFA_REQUIREMENT_REQUIRED` for a tenant that stores `none`
- **THEN** the tenant stores `required`, the response carries it, and the admin-action record carries `none` before and `required` after

#### Scenario: Unspecified value
- **WHEN** the request carries no `requirement`
- **THEN** it fails with `INVALID_ARGUMENT` and nothing is written

#### Scenario: Two writes at once
- **WHEN** two admins write different values for one tenant at the same time
- **THEN** the later write is stored, and each write's record carries the value it actually replaced
