## Why

A tenant cannot make its members sign in through the company's own OIDC identity provider. Bring your own SSO (BYO-SSO) adds that. The SSO service holds the connections to the identity providers. This service stores the choices of each tenant. It also answers what the login UI and the SSO service ask at sign-in.

## What Changes

- **SSO policy** per tenant: enforcement (off, optional, required), auto-join, bindings, domains. Each RPC writes one part of the policy. This service checks the rules of the policy.
- **2 gRPC-only services** for the RPCs that only other services call: `TenantSignInService` and `TenantSSOPolicyService`. `TenantService` keeps the RPCs that people call through the gateway.
- **`GetSignInContext`**: what a sign-in to one tenant needs.
- **`ListSignInTenants`**: the tenants that an address can sign in to. These are its memberships, its invitations and the tenants that it can auto-join. `LookupTenants` still lists memberships only.
- **Personal tenants**: `CreatePersonalTenant` and the registration webhook use one code path. `CreateTenant` does not change.
- **Pending invitations**: a user accepts a pending invitation only with a sign-in to the tenant (`JoinTenant`). **BREAKING**: `InviteMember` returns no recovery link for an existing account.
- **MFA policy** (an MFA requirement per tenant), **member removal** (`RemoveTenantUser`), database timeouts, migrations 004–007.

The service has no feature flag. These things change when the login UI's feature is off:

1. An invited existing account gets a pending invitation that nothing accepts.
2. Registration creates the personal tenant enabled, not disabled.
3. A personal tenant refuses invitations, provisioning and member removal.
4. A user's tenant lists start with the personal tenant.
5. When the database is slow, the service answers `ABORTED` or `UNAVAILABLE`. It does not wait.

Deploy it with the login UI's feature on.

## Non-Goals

- Security keys as a tenant requirement: the MFA requirement is `none` or `required`.
- A connection that several tenants bind.
- The deletion of expired invitations: the service ignores them.
- Proof that a tenant owns its domains.
- Authorization: the gateway authorizes routes. This service does not.
- The storage of connections, calls to identity providers, the enforcement of a policy at sign-in.

## Capabilities

### New Capabilities
- `tenant-sso-policy`: the policy, its rules, its 4 RPCs.
- `sign-in-context`: `GetSignInContext`.
- `tenant-lookup`: `ListSignInTenants`, the memberships and join candidates of an address.
- `personal-tenants`: creation, refusals, order.
- `tenant-invitations`: the outcomes of `InviteMember`, expiry, `JoinTenant`.
- `tenant-membership`: the changes to `ProvisionUser`, `RemoveTenantUser`.
- `tenant-mfa-policy`: the MFA requirement.
- `database-resilience`: timeouts, status codes, Kratos calls outside locks.

### Modified Capabilities
- `grpc-transaction-interceptor`: 3 gRPC-only reads run without a transaction.
- `authorization-federation`: the service publishes `can_view` with the membership.

## Impact

- **Packages**: `pkg/tenant`, `pkg/webhooks`, `pkg/web`, `internal/storage`, `internal/types`, `internal/db`, `internal/kratos`, `internal/config`, `internal/testhelpers`, `cmd`, `migrations`, `client/http`.
- **API**: `identity-platform-api` `v0/tenant`: 3 new RPCs on `TenantService`, and 8 in the 2 gRPC-only services.
- **Database**: 4 `tenants` columns, and the tables `tenant_sso_bindings`, `tenant_sso_domains`, `tenant_invitations`.
- **Configuration**: `INVITATION_LIFETIME` must be a positive duration.
