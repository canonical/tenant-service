# 2. Support Multiple Owners per Tenant

Date: 2026-02-18

## Status

Accepted

## Context

Tenants often need shared administrative responsibilities. Creating a single owner account encourages credential sharing, which is a security risk.

## Decision

We will support multiple owners per tenant. The creation and update operations will allow specifying a list of owner IDs. Updates to the owner list will be treated as a synchronization operation (replacing the existing set).

## Consequences

### Positive
- Removes the need for account sharing.
- Better auditability of administrative actions.

### Negative
- Increases complexity in the update logic (need to handle diffs or full replace).
- Requires careful authorization checks to prevent the last owner from being removed (if that's a requirement).
