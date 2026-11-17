## Purpose

A tenant can require MFA for every sign-in to it. MFA means a second factor after the first factor. The second factor is an authenticator app or a recovery code.

The MFA policy of a tenant is one choice, the MFA requirement of the tenant: `none` or `required`. The API documents the meaning. A sign-in through a company or a public identity provider needs a second factor only at `required`. Every other first factor always needs a second factor.

This service stores the value and reports it in `GetSignInContext`. The login UI applies it at sign-in.

Key decisions:

- The value is one column of `tenants`. Its own RPC writes it, and tenant admins call that RPC through the gateway. We turned down a write through the SSO service, as for the SSO policy, because no rule about the MFA policy involves a connection.
- Last write wins. The write locks the row of the tenant in the statement that changes it. So the value that the write logs as replaced is exact when two writes race.
- The log is the change history. The admin-action record of each write carries the value before and the value after. The service writes these records at log level `info`. So a deployment that keeps the history must run with `LOG_LEVEL` `info` or `debug`.

**Non-goals:**

- A security key as the required second factor. The value is `none` or `required`, and nothing names a method.
- A policy per user or per role.
- A maximum age of the second factor.
- Asking the user for the second factor. The login UI does this.

## ADDED Requirements

### Requirement: A tenant has one MFA policy value
The system SHALL store an MFA requirement for every tenant (`tenants.mfa_requirement`) as `none` or `required`. The value SHALL be `none` for a tenant that never set it. The system SHALL report the value as `mfa_requirement` in `GetSignInContext`. The database SHALL refuse any other value. The policy applies to every tenant, also to a personal tenant.

#### Scenario: A tenant that never set a policy
- **WHEN** a caller reads the MFA policy of a new tenant
- **THEN** the answer is `MFA_REQUIREMENT_NONE`

#### Scenario: Reported at sign-in
- **WHEN** the MFA requirement of a tenant is `required` and a caller calls `GetSignInContext` for it
- **THEN** the answer carries `mfa_requirement` `REQUIRED`
- **AND** this is true for every SSO policy of the tenant

### Requirement: Reading and writing the MFA policy
The system SHALL serve `GetTenantMFAPolicy` (`GET /api/v0/tenants/{tenant_id}/mfa-policy`) and `PutTenantMFAPolicy` (`PUT /api/v0/tenants/{tenant_id}/mfa-policy`) to tenant admins and platform admins through the gateway. Both RPCs SHALL refuse a `tenant_id` that is not a UUID with `INVALID_ARGUMENT`. Both RPCs SHALL answer `NOT_FOUND` for an unknown tenant.

`PutTenantMFAPolicy` SHALL refuse a `requirement` other than `NONE` and `REQUIRED` with `INVALID_ARGUMENT`. It SHALL replace the stored value (last write wins) and return the new policy. It SHALL log the admin action `put_tenant_mfa_policy`. The record SHALL name the caller as the actor, the tenant id as the resource, and the values `before` and `after`.

The service writes an admin-action record at log level `info`. At the default `LOG_LEVEL`, `error`, the service writes no record. A write that waits for the row of the tenant longer than the lock timeout fails with `ABORTED`.

#### Scenario: MFA made required
- **WHEN** a tenant admin puts `MFA_REQUIREMENT_REQUIRED` for a tenant that stores `none`
- **THEN** the tenant stores `required` and the response carries it
- **AND** the admin-action record carries `none` before and `required` after

#### Scenario: Unspecified value
- **WHEN** the request carries no `requirement`
- **THEN** the request fails with `INVALID_ARGUMENT`
- **AND** the service writes nothing

#### Scenario: Two writes at once
- **WHEN** two admins write different values for one tenant at the same time
- **THEN** the service stores the later write
- **AND** the record of each write carries the value that the write replaced
