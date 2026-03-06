# ADR 0007: Structured Logging Enhancements

Date: 2026-03-03

## Status

Accepted

## Context

Logging across the service had three structural gaps that made production
diagnosis difficult and left the existing `SecurityLogger` investment unused
in business logic:

**Gap 1 — No structured fields; all calls use printf-style interpolation.**
Every error and warning log in `pkg/tenant/service.go`, `pkg/tenant/handlers.go`,
`pkg/webhooks/service.go`, and `pkg/webhooks/handlers.go` was:

```go
s.logger.Errorf("failed to add member to storage: %v", err)
```

This embeds context—tenant ID, user ID, email, role—into the message string
rather than emitting it as discrete key-value fields. Log aggregation tools
(Loki, CloudWatch Insights, Elastic) cannot filter or correlate on values that
are concatenated into a message. Reconstructing the flow for a tenant requires
grep-style text search rather than structured queries.

**Gap 2 — No `tenant_id` or `user_id` log fields.**
None of the service methods attached a `tenant_id` or `user_id` field to any
log entry. This was already noted in `TODO.md` ("Missing `tenant_id` log fields
in service methods"). Without these fields, log-based debugging of
multi-tenant issues requires trace-backend correlation rather than log-only
queries.

**Gap 3 — `SecurityLogger` unused in business logic.**
`SecurityLogger.SystemStartup()` and `SystemShutdown()` were called in
`cmd/serve.go`, but no business-logic call sites existed for `AdminAction`,
`AuthzFailure*`, or any of the write-operation audit methods. The
`SecurityLogger` was designed for OWASP-aligned audit logging of sensitive
state changes and auth events; leaving it unwired meant audit traces were
absent from the `type=security` log stream for every tenant mutation.

**Gap 4 — No success-path logs on mutating operations.**
Mutating service methods (`CreateTenant`, `UpdateTenant`, `DeleteTenant`,
`InviteMember`, `ProvisionUser`, `UpdateTenantUser`) had no `Info`-level log
on the success path. Errors were logged, but a successful operation left no
trace in the service log stream.

## Decision

### Decision A — Add `w`-suffix methods to `LoggerInterface`

Add `Errorw`, `Infow`, `Warnw`, `Debugw` to `LoggerInterface` in
`internal/logging/interfaces.go`. These map to `zap.SugaredLogger`'s
key-value pair methods (the `w` suffix is the idiomatic zap convention for
structured, named-field logging). All generated mocks are regenerated via
`make mocks`.

The `f`-suffix methods (`Errorf`, `Infof`, etc.) are retained for cases where
a formatted string without discrete fields is genuinely appropriate (e.g.
startup debug messages that include no identifiers).

### Decision B — Adopt structured key-value logging throughout `pkg/tenant` and `pkg/webhooks`

All log calls in the service and handler layers are migrated from:

```go
s.logger.Errorf("failed to add member to storage: %v", err)
```

to:

```go
s.logger.Errorw("failed to add member to storage",
    "tenant_id", tenantID,
    "error", err,
)
```

Standard field names:

| Field key    | Type   | Populated in  | Notes |
|--------------|--------|---------------|-------|
| `tenant_id`  | string | service layer | Primary grouping key for multi-tenant queries |
| `user_id`    | string | service layer | Caller or target identity |
| `email`      | string | service layer | Only when directly relevant (invite, provision) |
| `role`       | string | service layer | When mutating role assignments |
| `error`      | error  | all layers    | Always the raw error, never a wrapped public message |

Handler-layer logs retain only fields derivable from the transport request
object (`tenant_id`, `user_id`, `email`, `role` from request fields). Success
logs are emitted only in the service layer to avoid duplication.

### Decision C — Add `Debug`-level method-entry logs in the service layer

Each service method emits a `Debugw` log immediately after the span starts,
recording the method's key input parameters:

```go
s.logger.Debugw("inviting member",
    "tenant_id", tenantID,
    "email",     email,
    "role",      role,
)
```

This provides a log-only execution audit trail for deployments that do not
have access to a trace backend. The `Debug` level means these entries are
suppressed in production unless the log level is lowered, incurring no overhead
at `Info` and above.

### Decision D — Wire `SecurityLogger.AdminAction` for every successful mutation

For each mutating service method, a `SecurityLogger.AdminAction` call is added
on the success path, after all storage and authorization operations complete:

```go
s.logger.Security().AdminAction(actor, action, apiPath, resource)
```

Fields:
- `actor`: the caller's identity ID, extracted from context via
  `authentication.GetUserID(ctx)`. If no identity is present in context (e.g.
  admin service-to-service calls without a user subject), the empty string `""`
  is used—the mutation proceeds and the audit log records a blank actor rather
  than failing.
- `action`: a dot-namespaced string describing the operation
  (e.g. `"create_tenant"`, `"invite_member"`, `"update_tenant_user"`).
- `apiPath`: the fully qualified service method name
  (e.g. `"tenant.Service.CreateTenant"`).
- `resource`: the resource being acted upon
  (tenant ID, or `tenantID:userID` for membership operations).

This populates the `type=security` log stream with business-level audit events,
complementing the existing `SystemStartup`/`SystemShutdown` calls in
`cmd/serve.go`.

The method-to-resource mapping is:

| Service method    | action                | resource                |
|-------------------|-----------------------|-------------------------|
| `CreateTenant`    | `create_tenant`       | `tenant.ID`             |
| `UpdateTenant`    | `update_tenant`       | `tenant.ID`             |
| `DeleteTenant`    | `delete_tenant`       | `id` arg                |
| `InviteMember`    | `invite_member`       | `tenantID:email`        |
| `ProvisionUser`   | `provision_user`      | `tenantID:email`        |
| `UpdateTenantUser`| `update_tenant_user`  | `tenantID:userID`       |

`HandleRegistration` in `pkg/webhooks` is treated as a self-service event
(user registers and auto-creates their own tenant). It uses:
`AdminAction(identityID, "self_registration", "webhooks.Service.HandleRegistration", tenant.ID)`.

### Decision E — No changes to `LogContextMiddleware` or `LogEntry`

The HTTP middleware layer (`LogContextMiddleware`, `LogEntry`) already emits
request-level context (method, path, remote addr, status, elapsed duration) at
`Debug` level, and injects request fields into context for `WithContext` use.
No changes are needed there.

## Consequences

- **Log verbosity**: `Debug`-level method-entry logs add one log line per
  service method invocation at `debug` level. At `info` and above (standard
  production config), they are suppressed. The `Infow` success logs add one
  line per successful mutation (previously zero).
- **Audit trail**: every successful tenant mutation now appears in the
  `type=security` log stream, enabling audit queries without requiring a trace
  backend.
- **Log aggregation**: all structured fields are discrete JSON keys in the
  output, enabling queries like `{type="service"} | json | tenant_id="<id>"` in
  Loki without regex parsing.
- **Interface extension**: adding four methods to `LoggerInterface` required
  regenerating all 9 mock files. Existing tests continue to compile without
  changes because mock methods are unused unless explicitly `EXPECT()`-ed.
- **No new dependencies**: `zap.SugaredLogger` already provides `Infow` etc.;
  `authentication.GetUserID` is already used in the handler layer and is safe
  to call from the service layer.
