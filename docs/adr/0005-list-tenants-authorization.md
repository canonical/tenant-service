# ADR 0005 – ListTenants Authorization: Admin-Only Access

## Status

Accepted

## Context

The `ListTenants` RPC returns all tenants in the system. When adding fine-grained
authorization using OpenFGA, we needed a strategy for authorizing this endpoint.

Two approaches were considered:

1. **Per-tenant filtering with `ListObjects`**: Use the OpenFGA `ListObjects` call to
   return only the tenants the caller has access to, then intersect with a paginated
   DB query.

2. **Admin-only guard**: Restrict `ListTenants` to global administrators via a
   `CheckIsAdmin` call, returning all tenants for admins and `PermissionDenied` for
   everyone else.

## Decision

We chose **option 2: admin-only guard**.

The `ListObjects` approach is fundamentally incompatible with offset-based database
pagination because OpenFGA cannot guarantee stable ordering or coverage of a particular
page window in the database. Implementing a consistent paginated view over an external
authorization filter would require fetching a potentially unbounded set of tenant IDs
from OpenFGA, holding them in memory, and re-paginating them application-side. This
defeats the purpose of DB-level pagination and poses scalability concerns.

Given that the primary use-case for `ListTenants` is administrative (service management,
support tooling, internal dashboards), restricting it to global admins is an acceptable
security boundary. Non-admin users can call `ListUserTenants` to enumerate the tenants
they belong to.

## Consequences

- `ListTenants` calls `CheckIsAdmin(ctx, callerID)` before doing any storage lookup.
- Non-admin callers receive `PermissionDenied` regardless of which tenants they own
  or belong to. They should use `ListUserTenants` instead.
- If a future requirement arises for non-admin tenant listing with pagination, a
  dedicated RPC (e.g. `ListMyTenants`) should be introduced that loads tenant IDs from
  OpenFGA and does *not* attempt to layer DB pagination on top.
