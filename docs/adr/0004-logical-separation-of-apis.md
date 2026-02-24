# 4. Separation of Public and Admin Logic

Date: 2026-02-18

## Status

Accepted

## Context

The service exposes both public-facing functionality (for users) and sensitive administrative functionality (for internal ops). We need to ensure that these concerns are separated to prevent accidental exposure of admin capabilities.

## Decision

We will logically separate Admin and Public API logic.
- **Network Level**: Admin endpoints (e.g., `pkg/admin` or admin-specific handlers) will be exposed on a separate route or protected by stricter network policies/auth middleware.
- **Code Level**: Handlers for admin operations will be kept distinct from standard user operations (currently implemented via `handlers_admin.go` in `pkg/tenant`).

## Consequences

### Positive
- improved security posture by reducing the attack surface of the public API.
- Clearer distinction between user capabilities and system administration.

### Negative
- May require some code duplication or shared helper functions.
- Requires careful configuration of the router and middleware to ensure boundaries are enforced.
