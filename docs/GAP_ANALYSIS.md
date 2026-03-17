# Gap Analysis Report

This document details the critical gaps between the current implementation of the Tenant Service, the intended design (ID054), and production readiness requirements.

## 1. Critical Security Missing Implementation

The service currently "implements" authorization components but does not **enforce** them in business logic.

- **Missing Authorization Enforcement**:
  The `pkg/tenant/service.go` methods are vulnerable because they skip authorization checks.
  - `InviteMember`: No check that the caller has permission to invite users to the target tenant.
  - `UpdateTenant`: No check that the caller is an owner/admin of the tenant.
  - `DeleteTenant`: No check that the caller is an owner of the tenant.
  - `CreateTenant`: No restriction on who can create tenants (may be intended for self-service, but needs verification).
  - **Remediation**: Every public method in `Service` must start with `s.authz.Check(ctx, ...)` before performing any logic.

- **Audit Logging**:
  - The `internal/logging.SecurityLogger` exists but is not used in `pkg/tenant/service.go`.
  - **Risk**: No record of who granted permissions, who deleted tenants, or who invited users.
  - **Remediation**: Add `s.securityLogger.Info(...)` calls for all state-changing operations (Create, Update, Delete, Invite).

- **Authentication Consistency**:
  - `internal/identity` middleware extracts the Kratos ID, but handlers do not consistently validate that a user ID is present before calling the service.

## 2. Design vs Implementation Discrepancies

Comparing `ID054 - Tenant API.md` with the current codebase:

| Feature | Design (ID054) | Implementation | Status |
| :--- | :--- | :--- | :--- |
| **Token Exchange** | `GET /onboard` | **Not Needed** | Deprecated. Was intended for 2-step invitation, now handled via Kratos recovery flow timeouts. |
| **Activations** | `POST .../activate` | **Missing** | No logic to handle tenant activation/deactivation states. |
| **Webhooks** | `POST /webhooks/registration`, `POST /webhooks/login` | Partially Implemented | Registration hook is implemented. Login hook is MISSING. |
| **Data Model** | `slug`, `tier` | Only `name` | The Tenant proto and DB schema are missing fields defined in the design. |
| **Roles** | Enum: `owner`, `admin`, `member` | String | Implementation uses loose strings compared to the strict enum in design. |
| **Storage: Dead Code** | N/A | `internal/storage` ("invites") | The code for managing an `invites` table is obsolete for the Kratos-based invitation flow and should be removed. |

## 3. Recommended Improvements

### High Priority (Security)
1.  **Enforce Authorization**: Refactor `pkg/tenant/service.go` to inject `s.authz.Check()` at the start of every method.
2.  **Implement Audit Logging**: Add security logging for all write operations.

### Medium Priority (Functionality)
3.  **Complete Webhooks**: Implement the Kratos LOGIN webhook (`POST /webhooks/login`) to validate user access to tenants.

### Low Priority (Cleanup)
5.  **Remove Technical Debt**: Delete the unused `invites` logic from `internal/storage/storage.go` as we now rely fully on Kratos for invitation flows.
6.  **Align Data Model**: Add `slug` and `tier` attributes to the Tenant entity if they are still required.
5.  **Standardize Roles**: Convert role strings to specific types/enums to prevent typos.
