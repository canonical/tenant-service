# Architectural Decisions

## 1. Multiple Owners
We decided to support multiple owners per tenant to allow shared administrative responsibilities without account sharing.
- **API Impact**: `UpdateTenant` accepts a list of `owner_ids`.
- **Behavior**: The update operation is a "sync" (replace all existing owners with the provided list).

## 2. Tenant Activation
Tenants are created enabled, through the Admin API and at self-service registration alike.
- **Schema**: Added `enabled BOOLEAN DEFAULT FALSE` to `tenants` table.
- **Admin API**: `ActivateTenant` explicitly sets this flag to `true`.
- **Self-Service**: The registration webhook (`pkg/webhooks`) creates the tenant enabled, with no activation step.

## 3. Package Structure
We separated Admin API logic into `pkg/admin` to enforce network isolation boundaries in the future.
- **Implementation**: Uses a `CompositeHandler` in `internal/server` to merge `pkg/tenant` (public) and `pkg/admin` (private) into a single gRPC service implementation as required by the current Proto definition.
