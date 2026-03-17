# ADR-0008: Support for Client (Machine) Users

## Status

Accepted

## Context

The tenant service currently only supports human users backed by Kratos identities.
Each user authenticates via a browser-based flow (authorization code / device code),
and the resulting access token carries a `tenants` claim populated by the Hydra
token hook.

Machine-to-machine (M2M) API access, however, uses OAuth2 `client_credentials`
grants. These clients authenticate with a `client_id` and `client_secret` and
receive a JWT directly from Hydra, without any Kratos identity. Today, such
clients have **no tenant context**: the token hook cannot resolve tenants for
them because there is no membership row linking a client ID to a tenant.

We need a way to:

1. Associate an OAuth2 `client_credentials` client with exactly one tenant.
2. Ensure the Hydra token hook injects the tenant into the client's access token
   — using the same mechanism as human users.
3. Manage client lifecycle (create, list, delete) through the tenant service API.
4. Authorize client operations using the existing OpenFGA model.

## Decision

### 1. Treat clients as first-class tenant members

OAuth2 clients are modeled as members of a tenant, reusing the same `memberships`
table that tracks human users. This enables the token hook to resolve tenants for
both human and machine subjects uniformly.

### 2. Extend the `memberships` table with an `identity_type` discriminator

- Rename the column `kratos_identity_id` → `identity_id` to reflect its broader
  purpose.
- Add a `identity_type` column (`VARCHAR(10) NOT NULL DEFAULT 'user'`) with
  valid values `'user'` (Kratos identity) and `'client'` (Hydra OAuth2 client).
- The unique constraint becomes `UNIQUE(tenant_id, identity_id)`.
- Existing rows default to `identity_type = 'user'`.

### 3. Store tenant association in both the database and Hydra client metadata

| Aspect           | Database (authoritative)     | Hydra metadata (supplementary) |
|------------------|------------------------------|-------------------------------|
| Token hook       | ~1 ms SQL lookup             | Not used                      |
| List clients     | Simple SQL query             | Not used                      |
| Auditability     | Requires DB access           | Self-describing in Hydra      |
| Consistency      | Can drift if Hydra client    | Always reflects Hydra state   |
|                  | is deleted externally        |                               |
| Failure modes    | Single write                 | Must succeed alongside DB     |

On client creation, we write to both the database and set
`metadata: { "tenant_id": "<id>" }` on the Hydra client. The database is the
source of truth for lookups; the metadata is supplementary for debugging and
auditing via Hydra's admin API.

### 4. Each client belongs to exactly one tenant

A `client_credentials` client is created within the scope of a single tenant.
This is simpler than the multi-tenant model for human users and matches the
typical M2M use case where a service account belongs to one organization.

### 5. Expose separate `/clients` CRUD endpoints

New gRPC/HTTP endpoints:

| RPC                   | HTTP Method | Path                                          |
|-----------------------|-------------|-----------------------------------------------|
| `CreateTenantClient`  | `POST`      | `/api/v0/tenants/{tenant_id}/clients`         |
| `ListTenantClients`   | `GET`       | `/api/v0/tenants/{tenant_id}/clients`         |
| `DeleteTenantClient`  | `DELETE`    | `/api/v0/tenants/{tenant_id}/clients/{client_id}` |

- `CreateTenantClient` generates a UUID for the client, writes it to the local
  database, then creates the OAuth2 client in Hydra with that ID. Hydra is called
  last so that a failure automatically rolls back the database transaction
  (via the transaction middleware), leaving no orphaned state. It returns the
  `client_id` and `client_secret` (secret is only available at creation time).
- `ListTenantClients` queries the local database for memberships with
  `identity_type = 'client'`.
- `DeleteTenantClient` removes the membership row first, then deletes the client
  from Hydra. A Hydra failure rolls back the transaction, keeping both systems
  consistent.

These endpoints are separate from the `/users` endpoints because the create and
delete flows are fundamentally different (Hydra admin API vs. Kratos identity
management).

### 6. Use contextual OpenFGA tuples (no stored tuples for clients)

Unlike human users, client memberships are **not** written as stored tuples in
OpenFGA. Instead, the service passes contextual tuples at authorization-check
time (e.g., `user:<client_id>` as `member` of `tenant:<tenant_id>`), derived
from the database.

This avoids an additional external write during create/delete and keeps the
database as the single source of truth for client-tenant associations. The
existing `Check(ctx, user, relation, object, tuples ...openfga.Tuple)` API
already supports contextual tuples.

### 7. Token hook requires no code changes

The existing `HandleTokenHook` method:

1. Extracts `subject` from the token request (for `client_credentials`, this is
   the `client_id`).
2. Calls `storage.ListActiveTenantsByUserID(ctx, subject)`.
3. Injects matching tenant IDs into the token claims.

Since the membership row stores `identity_id = client_id`, the existing query
matches and returns the client's tenant. No changes are required.

### 8. Create a Hydra admin client wrapper

Following the same pattern as `internal/kratos/client.go`, create
`internal/hydra/client.go` wrapping the Hydra admin SDK with tracing, monitoring,
and logging. The wrapper exposes:

- `CreateOAuth2Client(ctx, clientID, metadata) (clientSecret, error)`
- `DeleteOAuth2Client(ctx, clientID) error`

The caller generates and supplies the client ID (a UUID), allowing the service
to write the database record first and call Hydra last. This ordering leverages
the HTTP transaction middleware: if the Hydra call fails, the enclosing database
transaction is rolled back automatically, eliminating the need for manual
rollback logic.

## Consequences

### Positive

- **Uniform tenant resolution**: The token hook works identically for human and
  machine users. No branching logic needed.
- **Minimal authorization model changes**: No new FGA types or stored tuples.
  Contextual tuples at check-time keep the database as the single source of truth.
- **Auditability**: Hydra client metadata carries `tenant_id` for external
  inspection.
- **Clean API separation**: `/clients` endpoints handle the distinct lifecycle
  of OAuth2 clients without polluting the `/users` API.
- **No rollback complexity**: By writing to the database first and calling Hydra
  last, the transaction middleware handles failures automatically. There is no
  manual rollback logic.

### Negative

- **External deletion drift**: If a Hydra client is deleted directly via Hydra's
  admin API (bypassing the tenant service), the membership row becomes orphaned.
  This is acceptable — the token hook simply won't find a matching client in
  Hydra, so no token will be issued. Orphaned rows can be cleaned up via a
  future reconciliation job.
- **Column rename**: Renaming `kratos_identity_id` → `identity_id` requires
  updating all Go code references. This is a one-time migration cost.

### Data Model Changes

**New migration** (`002_client_users.sql`):

```sql
ALTER TABLE memberships RENAME COLUMN kratos_identity_id TO identity_id;
ALTER TABLE memberships ADD COLUMN identity_type VARCHAR(10) NOT NULL DEFAULT 'user';
ALTER TABLE memberships ADD CONSTRAINT chk_identity_type CHECK (identity_type IN ('user', 'client'));
-- identity_id is no longer necessarily a UUID (Hydra client IDs are UUIDs, but
-- we relax the type to be safe)
ALTER TABLE memberships ALTER COLUMN identity_id TYPE TEXT;
```

**New configuration**:

- `HYDRA_ADMIN_URL`: URL of the Hydra admin API (e.g., `http://localhost:4445`).
