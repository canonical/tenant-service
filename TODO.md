# TODO List
- [] Add Pagination back to List endpoints
- [] Handle client users
- [] Remove unused 'invites' table logic from storage layer (replaced by Kratos flow)
- [] Implement tenant choosing logic in login UI, this relies on identifier first login being implemented
- [] Implement Login Validation Webhook (`POST /webhooks/login`) to enforce access control during Kratos login flow
- [] Add `invites` table migration
- [] Implement Unit Tests for `pkg/tenant`
- [] Initialize E2E Test Suite
- [] Create Sequence Diagrams
- [] Fix Span Status in service methods (record errors on span)
- [] Add OpenTelemetry gRPC Interceptors in `cmd/serve.go`
- [] Implement Business Metrics (e.g. `tenant_created_total`) in service layer
- [] Implement browser-based E2E tests for Hydra/Kratos hooks


