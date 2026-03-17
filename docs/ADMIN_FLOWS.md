# Admin Flows, Test Scenarios, and CLI Design Notes

## 1. Authorization test flows

These are the scenarios to exercise after deployment with `AUTHORIZATION_ENABLED=true`.

### 1.1 Global admin assignment

| # | Steps | Expected result |
|---|---|---|
| 1 | Grant user A global admin via `./app admin assign global-admin --email a@example.com` | User A is written as `admin` on `privileged:main` in FGA |
| 2 | User A calls `CreateTenant` | 200 — tenant created; `privileged:main` linked to the new tenant |
| 3 | User B (non-admin) calls `CreateTenant` | 403 Permission Denied |
| 4 | User A calls `ListTenants` | 200 — all tenants returned |
| 5 | User B calls `ListTenants` | 403 Permission Denied |
| 6 | User A calls `ListUserTenants` for User B | 200 — admin can see any user's tenants |
| 7 | User B calls `ListUserTenants` for User A | 403 Permission Denied |
| 8 | User B calls `ListUserTenants` for themselves | 200 — users can always see their own tenants |
| 9 | Revoke User A's admin rights; User A calls `CreateTenant` | 403 Permission Denied |

### 1.2 Global admin inherits tenant permissions

This tests that the `userset` relation through `privileged:main` works end-to-end.

| # | Steps | Expected result |
|---|---|---|
| 1 | Admin creates Tenant T, which links `privileged:main → member of T` | Tenant T exists in DB and FGA |
| 2 | Grant User B global admin | User B written as `admin` on `privileged:main` |
| 3 | User B calls `UpdateTenant(T)` | 200 — inherited `can_edit` via `privileged:main` |
| 4 | User B calls `DeleteTenant(T)` | 200 — inherited `can_delete` |
| 5 | User B calls `ListTenantUsers(T)` | 200 — inherited `can_view` |
| 6 | User B calls `UpdateTenantUser(T, ...)` | 200 — inherited `can_edit` |
| 7 | User C (non-admin, non-member of T) calls any of the above | 403 for all |

### 1.3 Tenant owner operations

| # | Steps | Expected result |
|---|---|---|
| 1 | Platform admin provisions User O as owner of Tenant T | FGA tuple: `user:O owner tenant:T` |
| 2 | User O calls `UpdateTenant(T)` | 200 |
| 3 | User O calls `InviteMember` to add User M to T | 200 |
| 4 | User O calls `ProvisionUser` to add User P to T | 200 |
| 5 | User O calls `ListTenantUsers(T)` | 200 |
| 6 | User O calls `UpdateTenantUser(T, M, role=member)` | 200 |
| 7 | User O calls `DeleteTenant(T)` | 200 |

### 1.4 Tenant member operations

Members have `can_view` and `can_create` but not `can_edit` or `can_delete`.

| # | Steps | Expected result |
|---|---|---|
| 1 | User M is a member of Tenant T | FGA tuple: `user:M member tenant:T` |
| 2 | User M calls `InviteMember` to T | 200 |
| 3 | User M calls `ProvisionUser` to T | 200 |
| 4 | User M calls `ListTenantUsers(T)` | 200 |
| 5 | User M calls `UpdateTenant(T)` | 403 |
| 6 | User M calls `DeleteTenant(T)` | 403 |
| 7 | User M calls `UpdateTenantUser(T, ...)` | 403 |

### 1.5 Ownership transfer

| # | Steps | Expected result |
|---|---|---|
| 1 | Tenant owner O calls `UpdateTenantUser(T, M, role=owner)` | User M is now `owner` of T; security audit log written |
| 2 | O calls `UpdateTenantUser(T, O, role=member)` | O demoted to member |
| 3 | O calls `DeleteTenant(T)` | 403 — O no longer has `can_delete` |
| 4 | M calls `DeleteTenant(T)` | 200 — M is now the owner |

---

## 2. CLI design

### 2.1 Real-world scenarios

**Scenario A — Platform bootstrap**
> An operator has just deployed the service and wants to promote the first platform admin.

The operator knows the admin's email address (e.g. from their IdP). They need to:
1. Look the user up in Kratos by email to get the Kratos identity UUID.
2. Write an `admin` tuple on `privileged:main` in FGA.

**Scenario B — Enterprise onboarding**
> A platform admin is setting up a new enterprise customer. They create a tenant and install the customer's primary contact as the tenant owner.

1. Admin creates a tenant: `./app tenant create "Acme Corp"`.
2. Admin provisions the owner: `./app tenant users provision <tenant-id> alice@acme.com owner`.
3. Alice can then invite the rest of the Acme team herself.

**Scenario C — Ownership transfer**
> A tenant owner is leaving the organisation. Another user inside the tenant needs to take over ownership.

The owner (or a platform admin on their behalf) calls `UpdateTenantUser` to promote a member to owner, then demotes the departing user.
This is an API-level operation today; a CLI convenience could be:  
`./app tenant users update <tenant-id> <email> --role owner`

**Scenario D — Admin offboarding**
> A platform admin is leaving. Their global admin rights must be revoked.

`./app admin revoke global-admin --email admin@example.com`

### 2.2 The user-ID problem

All authz tuples are keyed on Kratos identity UUIDs (e.g. `user:4b3c2a1d-...`). Operators work with email addresses. There are two sensible options:

**Option A — CLI resolves email → UUID (recommended)**  
Accept `--email` and call the Kratos Admin API (`GET /admin/identities?credentials_identifier=<email>`) to resolve the UUID before writing to FGA. This mirrors how `InviteMember` and `ProvisionUser` already work in the service layer. The CLI needs `--kratos-admin-url` configured alongside the FGA flags.

**Option B — Accept both**  
Accept `--email` or `--user-id`; if both are given, error. If only `--email` is given, require `--kratos-admin-url`. This is more flexible for scripting when the UUID is already known.

Option A is cleaner for interactive use; Option B is better for CI pipelines. A sensible default is Option B with `--email` as the primary ergonomic path.

### 2.3 Proposed command structure

```
./app admin
  assign
    global-admin  --email / --user-id  [--kratos-admin-url]
    tenant-owner  --tenant-id  --email / --user-id  [--kratos-admin-url]
    tenant-member --tenant-id  --email / --user-id  [--kratos-admin-url]
  revoke
    global-admin  --email / --user-id  [--kratos-admin-url]
    tenant-owner  --tenant-id  --email / --user-id  [--kratos-admin-url]
    tenant-member --tenant-id  --email / --user-id  [--kratos-admin-url]
```

All subcommands also accept the standard FGA flags `--fga-api-url`, `--fga-api-token`, `--fga-store-id`, `--fga-model-id`. These can be factored into a parent `PersistentFlags` block on the `admin` command so they don't have to be repeated.

The existing `./app tenant users ...` commands (invite, provision, update) remain as is — they address the tenant-scoped user management path where the _caller_ identity comes from the API session, not from a CLI flag.

---

## 3. Authentication pattern and the FGA tension

### 3.1 Current flow

```
Browser / gRPC client
    │  session cookie / gRPC metadata
    ▼
Traefik (ForwardAuth → Kratos /sessions/whoami)
    │  injects X-Kratos-Authenticated-Identity-Id: <uuid>
    ▼
identity.Middleware (reads header, stores uuid in context)
    │
    ▼
Service layer  →  FGA (checks uuid against tuples)
```

The Kratos identity UUID is the single authoritative identity token through the entire stack. There is no separate JWT subject allowlist in the service itself today.

### 3.2 The concern with OAuth2 / Hydra JWTs

The E2E test stack issues Hydra JWTs. A Hydra access token's `sub` claim is the Kratos identity UUID for user tokens, but it is the OAuth2 client ID for machine-to-machine (client credentials) tokens. If the service is ever exposed directly behind an OAuth2 JWT gateway (rather than Traefik + Kratos session auth), the identity the service sees could be a client ID, not a Kratos UUID — meaning FGA lookups would return nothing and every caller would get 403.

### 3.3 Recommendations

**1. Keep the identity extraction in one place.**  
`identity.Middleware` is already the single point where the caller ID is written into context. If JWT-based auth is added, extend that middleware to additionally parse a `Bearer` token and extract the `sub` claim, falling back to the header if absent. Do not add a second context key.

**2. Separate human users from machine clients at the FGA level.**  
Machine service accounts should have their own FGA object type or should be assigned roles explicitly, the same way human users are. A client credentials token should never implicitly inherit human user permissions.

**3. Add a `user:` type prefix guard.**  
The tuple format is `user:<uuid>`. If a machine client ID (non-UUID string) were ever injected into context and used in a FGA check, it would simply return "not allowed" rather than grant access. This is the safe failure mode — no changes needed.

**4. Consider a service-level JWT middleware for the gRPC port.**  
The HTTP port is protected by Traefik, but the gRPC port (`50051`) is exposed directly. A lightweight JWT verification middleware on the gRPC interceptor chain — validating `iss` and `aud` claims, extracting `sub` — would close the gap for direct gRPC callers.

**5. The CLI is out-of-band by design.**  
The `./app admin assign ...` commands call FGA directly using a service token (or no auth for local deployments). They bypass the Kratos session flow entirely, which is intentional — they are operator tools, not user-facing API calls. The security boundary for these commands is the FGA API token itself (`--fga-api-token`).
