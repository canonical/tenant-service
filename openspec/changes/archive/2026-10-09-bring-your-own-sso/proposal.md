## Why

A tenant cannot make its members sign in through the company's own OIDC identity provider. Bring your own SSO (BYO-SSO) adds that. The SSO service holds the provider connections; this service stores each tenant's choices and answers what the login UI and the SSO service ask at sign-in.

## What Changes

- **SSO policy** per tenant: enforcement (off, optional, required), auto-join, bindings, domains. Written one part per RPC; the policy's rules are checked here.
- **Two gRPC-only services** for what only other services call: `TenantSignInService` and `TenantSSOPolicyService`. `TenantService` keeps what people call through the gateway.
- **`GetSignInContext`**: what a sign-in to one tenant needs.
- **`ListSignInTenants`**: the tenants an address may sign in to: its memberships, its invitations and the tenants it could auto-join. `LookupTenants` still lists memberships only.
- **Personal tenants**: `CreatePersonalTenant` and the registration webhook, one code path. `CreateTenant` is unchanged.
- **Pending invitations**, accepted only by signing in to the tenant (`JoinTenant`). **BREAKING**: `InviteMember` returns no recovery link for an existing account.
- **MFA policy** (an MFA requirement per tenant), **member removal** (`RemoveTenantUser`), database timeouts, migrations 004–007.

The service has no feature flag. With the login UI's feature off:

1. An invited existing account gets a pending invitation that nothing accepts.
2. Registration creates the personal tenant enabled, not disabled.
3. A personal tenant refuses invitations, provisioning and member removal.
4. A user's tenant lists start with the personal tenant.
5. A slow database answers `ABORTED` or `UNAVAILABLE` instead of waiting.

Deploy it with the login UI's feature on.

## Non-Goals

- Security keys as a tenant requirement: the MFA requirement is `none` or `required`.
- A connection bound by several tenants.
- Deleting expired invitations: they are ignored.
- Proof that a tenant owns its domains.
- Authorization: the gateway authorizes routes, not this service.
- Storing connections, calling identity providers, enforcing a policy at sign-in.

## Capabilities

### New Capabilities
- `tenant-sso-policy`: the policy, its rules, its four RPCs.
- `sign-in-context`: `GetSignInContext`.
- `tenant-lookup`: `ListSignInTenants`, the memberships and join candidates of an address.
- `personal-tenants`: creation, refusals, ordering.
- `tenant-invitations`: `InviteMember` outcomes, expiry, `JoinTenant`.
- `tenant-membership`: `ProvisionUser` changes, `RemoveTenantUser`.
- `tenant-mfa-policy`: the MFA requirement.
- `database-resilience`: timeouts, status codes, Kratos calls outside locks.

### Modified Capabilities
- `grpc-transaction-interceptor`: three gRPC-only reads run without a transaction.
- `authorization-federation`: `can_view` is published with the membership.

## Impact

- **Packages**: `pkg/tenant`, `pkg/webhooks`, `pkg/web`, `internal/storage`, `internal/types`, `internal/db`, `internal/kratos`, `internal/config`, `internal/testhelpers`, `cmd`, `migrations`, `client/http`.
- **API**: `identity-platform-api` `v0/tenant`: three new RPCs on `TenantService`, and eight in the two gRPC-only services.
- **Database**: four `tenants` columns; tables `tenant_sso_bindings`, `tenant_sso_domains`, `tenant_invitations`.
- **Configuration**: `INVITATION_LIFETIME` must be a positive duration.
