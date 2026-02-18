# 3. Tenant Activation Workflow

Date: 2026-02-18

## Status

Accepted

## Context

To prevent abuse and manage the onboarding process, we need control over when a tenant becomes active and usable.

## Decision

Tenants will be created with an `enabled=false` state by default.
- **Admin API**: Can explicitly set `enabled=true` during creation or updates.
- **Self-Service**: Will require an activation step (e.g., email verification or admin approval) before the tenant is enabled. Do now, the Admin API creates active tenants.

## Consequences

### Positive
-Prevents immediate abuse of the platform by automated scripts.
- Allows for a moderation or verification step in the user journey.

### Negative
- Adds friction to the onboarding process.
- Requires logic in all relevant endpoints to check the `enabled` status before proceeding.
