# 9. Consolidated List Filters for Tenant Listing Endpoints

Date: 2026-05-01

## Status

Accepted

## Context

The storage layer had three overlapping methods for fetching tenants by user ID
(`ListTenantsByUserID`, `ListActiveTenantsByUserID`, `listTenantsByUserID`), making
it impossible to expose query filters to the API without duplicating logic. Admin UIs
and the Login UI require the ability to filter tenant lists by enabled status,
membership role, and email. Three key design decisions were made during this work.

## Decision

### 1. Filter fields are flat on request messages (not wrapped in a sub-message)

Filter criteria (`enabled`, `role`, `email`, `identity_id`) are placed directly on
each request message rather than in separate `TenantFilter` / `TenantUserFilter`
wrapper messages. Deferred filters (`ids`, `name`) were omitted (YAGNI).

**Rationale:**
- Standard REST ergonomics: `?enabled=true` instead of `?filter.enabled=true`.
- Eliminates a layer of proto indirection that served no purpose for the current
  set of filter fields.
- The domain is bounded: there are 3–4 filterable properties per resource.
- Typed fields give compile-time safety and auto-generated documentation.
- `buf.validate` constraints (e.g. `string.uuid`, `string.email`) are trivially applied
  to individual fields without a custom parser.
- Adding a new filter is a purely additive, non-breaking proto change: add a field
  to the request message, a field to `ListOptions`, and a WHERE clause in storage.

### 2. Endpoints kept separate (no merge)

All five list endpoints (`ListMyTenants`, `ListTenants`, `ListUserTenants`,
`ListTenantUsers`, `LookupTenants`) remain as separate RPCs rather than being merged.

**Rationale:**
- REST best practice: sub-resource URLs encode hierarchy (e.g. `/users/{id}/tenants`).
- gRPC best practice (AIP-132): separate RPCs for different parent resource scopes.
- Security: each RPC has a single, unambiguous authorization check with no
  data-dependent branching. Merging endpoints would require runtime inspection of the
  request to determine which auth policy to apply, increasing the risk of authorization
  bypass.

### 3. Email filter resolved in service layer, not denormalized into the DB

The `email` filter on `ListTenantUsers` is resolved to a Kratos `identity_id` in the
service layer before being passed to storage, rather than being stored as a column in
the `memberships` table.

**Rationale:**
- Storing email in `memberships` would cause schema drift when a user changes their
  email address in Kratos, requiring a synchronization mechanism.
- Kratos is the single source of truth for identity data; denormalizing it introduces
  consistency risk.
- The additional Kratos lookup on `ListTenantUsers` with an email filter is acceptable:
  it is an admin-only operation and not on a hot path.
- If the email is unknown in Kratos, an empty result is returned silently; no error is
  surfaced to the caller.

## Consequences

### Positive
- The storage layer is unified: `ListActiveTenantsByUserID` and `listTenantsByUserID`
  are deleted; all callers use the single `ListTenantsByUserID` with `WithEnabled(true)`.
- Filter logic is additive: new filter fields require only (a) a field on the request
  message, (b) a `ListOptions` field + constructor, and (c) a WHERE clause in storage.
- Handler code is simple: filter fields are read directly from the (flat) request struct
  with no intermediate helper functions.
- HTTP query parameters are idiomatic REST: `?enabled=true`, `?role=owner`.

### Negative
- `ListTenantUsers` with an `email` filter incurs an extra Kratos round-trip.
- `ids` and `name` filters were deferred; they can be added later as purely additive
  proto/storage/handler changes.
