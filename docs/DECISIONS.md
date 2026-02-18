# Architectural Decisions

## 1. Multiple Owners
We decided to support multiple owners per tenant to allow shared administrative responsibilities without account sharing.
- **API Impact**: `UpdateTenant` accepts a list of `owner_ids`.
- **Behavior**: The update operation is a "sync" (replace all existing owners with the provided list).

## 2. Tenant Activation
Tenants are created with `enabled=false` by default (except when created via Admin API).
- **Schema**: Added `enabled BOOLEAN DEFAULT FALSE` to `tenants` table.
- **Admin API**: `ActivateTenant` explicitly sets this flag to `true`.
- **Self-Service**: Self-service registration flow (via `pkg/webhooks`) should likely set this to `true` or require activation step (TBD). For now, `pkg/admin` forces `true` on creation.

## 3. Package Structure
We separated Admin API logic into `pkg/admin` to enforce network isolation boundaries in the future.
- **Implementation**: Uses a `CompositeHandler` in `internal/server` to merge `pkg/tenant` (public) and `pkg/admin` (private) into a single gRPC service implementation as required by the current Proto definition.
