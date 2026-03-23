# 8. Tenant-Aware Login, Tenant Lookup, and Token Enrichment

Date: 2026-03-23

## Status

Accepted

## Context

Flows 2, 3, and 6 in `docs/SEQUENCE_DIAGRAMS.md` require three new capabilities:

1. **Login hook** — when a user authenticates, Kratos must validate that they are an active member
   of the tenant they selected (if any). This is implemented as a Kratos `after` login webhook at
   `POST /api/v0/webhooks/login`.

2. **Tenant lookup** — the Login UI must discover which active tenants a user belongs to before
   they authenticate. This is implemented as an unauthenticated HTTP endpoint
   `GET /api/v0/tenants/lookup?email=<email>`.

3. **Token enrichment** — after a successful login and consent, the Hydra token hook must inject a
   single `tenant_id` string claim into `id_token` and `access_token` (not a list). This replaces
   the existing placeholder behaviour that injects a `tenants` array.

This ADR also corrects a fundamental misunderstanding in the Flow 1 sequence diagram.

---

## Decision

### Correction: Kratos after-registration webhook behaviour

The comment in `docs/SEQUENCE_DIAGRAMS.md` for Flow 1 states:

> "Identity committed to Kratos DB only after 200 OK"

This is **incorrect**. Ory Kratos `after` registration webhooks fire *after* the identity has
already been persisted. A non-200 response with `response.ignore: false` causes Kratos to display
an error to the user, but it does **not** roll back the identity creation.

**Consequence**: orphaned identities — identities that exist in Kratos with no corresponding row
in `memberships` — are possible. This happens when the registration webhook returns a server error
(e.g. a transient DB or OpenFGA failure).

The Flow 1 sequence diagram has been updated to reflect reality. The aspirational
"blocking pre-registration hook" behaviour is not achievable with the current Kratos version.
If Ory introduces a true pre-registration blocking lifecycle event in a future release, this
decision should be revisited.

### Orphaned identity reconciliation — lazy strategy

**Problem**: A user whose registration webhook failed can authenticate (Kratos holds their
credentials) but has no tenant to log in to.

**Options considered**:

| Option | Description | Trade-off |
|---|---|---|
| A — Eager reconciliation job | A cron or queue-based job scans for Kratos identities without memberships and back-fills them | Added operational complexity; delay window; requires Kratos admin API access at scale |
HandleLoginHook| B — Lazy reconciliation at login | The login hook detects an identity with no memberships and re-runs the registration logic inline | Zero operational overhead; executes on-demand; email is already present in the webhook payload. However, exposes a synchronous attack vector on the login hook. |
| C — Do nothing (chosen) | Login succeeds with no tenants (or admin intervention required) | User must be manually activated or a background job handles it. This avoids blocking and complex logic inside the login webhook. |

**Chosen: Option C — Do nothing.**

When the login hook receives a request with an empty `tenant_id`, the hook simply allows the login to proceed. No lazy reconciliation is performed. An admin must set up the tenant separately, or the user logs in and the UI handles the empty-tenant state.

### Login hook design

- **Endpoint**: `POST /api/v0/webhooks/login` — registered in the **unprotected** zone of the
  chi router (alongside the existing registration and token webhooks). Kratos calls it with no
  Bearer token.
- **Request payload** (via Jsonnet template):
  ```json
  { "identity_id": "...", "email": "...", "tenant_id": "..." }
  ```
  `tenant_id` is the value the Login UI placed in `transient_payload.tenant_id` during the Kratos
  login flow submission. It will be empty if the user has no active tenants to select.
- **Behaviour**:
  - `tenant_id` empty → `200 OK` (no-op; allows pending or orphaned users to log in).
  - `tenant_id` present + user is active member of that tenant → `200 OK`.
  - `tenant_id` present + user is **not** an active member → `403 Forbidden` (Kratos blocks
    login and surfaces this error to the user).
- **Response**: an empty JSON object `{}` on success. Kratos requires a JSON response body.

### Tenant lookup endpoint design

- **Endpoint**: `GET /api/v0/tenants/lookup?email=<email>` — registered in the **unprotected**
  zone of the chi router. The Login UI calls this before the user authenticates.
- **Authentication**: none at present. The Login UI does not yet have a service-account token at
  this point in the flow. See security trade-offs below.
- **Behaviour**:
  - Resolve `email` → Kratos identity ID via Kratos Admin API.
  - If identity not found, return an empty list (no enumeration leak in the response to the
    unauthenticated caller; however, response timing may still be used to infer existence — tracked
    in TODO).
  - Query `memberships JOIN tenants WHERE enabled = true` for that identity.
  - Return `[{id, name}]` for enabled tenants only.

### Tenant selection mechanism

The tenant selection state is driven entirely by the Login UI Go backend through the
**InterceptLogin plugin** pattern. The `TenantResolverInterface` encapsulates all tenant-specific
decision logic, keeping the main `handleCreateFlow` handler multi-tenancy-agnostic.

#### Architecture: InterceptLogin plugin

The resolver implements **one entry point** called by `handleCreateFlow`:

```go
InterceptLogin(ctx, session, cookie, loginChallenge) → (LoginInterception, error)
```

`LoginInterception` is a value type with three boolean flags and the (possibly updated) cookie:

| Field | Meaning |
|---|---|
| `DeferMFAChecks` | Skip MFA/WebAuthn enforcement for now (user hasn't first-factor-auth'd for this challenge) |
| `SelectTenant` | Redirect user to `/ui/select_tenant` (2+ tenants, none selected yet) |
| `AcceptLogin` | Accept the Hydra login immediately (tenant resolved or not required) |
| `Cookie` | Updated `FlowStateCookie` with tenant selection persisted |

Two implementations exist:

- **`NoOpTenantResolver`** (multi-tenancy disabled): returns all-zero fields. The handler falls
  through to its normal `MustReAuthenticate` logic unchanged.
- **`CookieTenantResolver`** (multi-tenancy enabled): evaluates the cookie's
  `LoginChallengeHash`, looks up tenants via the tenant service, and returns the appropriate
  interception.

The plugin approach fixes a subtle bug in earlier designs: when `max_age=0` forces
re-authentication, the `NoOpTenantResolver` returns all-false (no intervention), so the handler
always reaches `MustReAuthenticate`. With the old per-method approach, the NoOp's
`IsAuthenticatedForChallenge` returning `true` could inadvertently bypass `MustReAuthenticate`.

#### CookieTenantResolver decision flow

1. **Cookie has no matching `LoginChallengeHash`** → `{DeferMFAChecks: true}`. The user has a
   Kratos session from a previous login but has not authenticated for *this* OAuth2 challenge.
   MFA is deferred; the handler runs `MustReAuthenticate` which creates a refresh login flow.
2. **Cookie matches + tenants not yet resolved** → the resolver calls
   `GET /api/v0/tenants/lookup?email=...`:
   - 0 tenants → auto-set `TenantID = "__none__"` → `{AcceptLogin: true}`
   - 1 tenant → auto-store tenant ID in cookie → `{AcceptLogin: true}`
   - 2+ tenants → `{SelectTenant: true}` (redirect to selection page)
3. **Cookie matches + TenantID already set** → `{AcceptLogin: true}`

#### State storage

Selected tenant is stored in the encrypted `FlowStateCookie` (`login_ui_state` cookie) with a
5-minute TTL, keyed by `LoginChallengeHash = hash(login_challenge)`. This binds the selection to
a specific OAuth2 challenge, preventing replay across different login attempts.

After the user selects a tenant on the `/ui/select_tenant` page, the frontend calls
`POST /api/v0/auth/tenant` which persists the choice via `StoreTenant`. On the next
`handleCreateFlow` invocation, `InterceptLogin` sees the stored tenant and returns `AcceptLogin`.

After credential submission, `handleUpdateFlow` also checks `NeedsTenantSelection` — for the
first-time login path where no session existed before authentication. If the user belongs to 2+
tenants, it returns a custom `tenant_selection_required` error that redirects the frontend to the
selection page.

#### Interaction with MFA and max_age

- **MFA**: When `DeferMFAChecks` is false (user is authenticated for this challenge), the handler
  enforces MFA/WebAuthn *before* checking `SelectTenant`. This ensures MFA always runs before
  tenant selection for security.
- **max_age=0**: Forces `MustReAuthenticate`, creating a new login flow. The fresh challenge has
  no `LoginChallengeHash` in the cookie, so `InterceptLogin` returns `DeferMFAChecks` on the next
  pass. After re-authentication, the full pipeline runs: MFA → tenant selection → accept.

- **Security trade-offs**:

  | Risk | Severity | Mitigation |
  |---|---|---|
  | Email-to-tenant enumeration by unauthenticated callers | Medium | Future: rate limiting per IP/email; future: require Login UI OAuth2 service-account token |
  | Response timing reveals whether email is known to Kratos | Low | Constant-time response not yet implemented; tracked in TODO |
  | Tenant name disclosure to unauthenticated callers | Low | Tenant names are not considered secrets |

  Mitigations are tracked in `TODO.md` under "Lookup endpoint hardening". This design follows the
  precedent set by ADR 0004 (logical API separation) — the lookup endpoint is a public-facing
  helper, similar to a "forgot password" email-check endpoint.

### Token enrichment design — human users

Two options were evaluated for injecting `tenant_id` into OAuth2 tokens:

**Option A — Login UI at consent step + Hydra token hook validation (chosen for human users)**:
1. During the Kratos login flow, the Login UI withholds the `login_challenge` from Kratos (see
   Login UI ADR 0001). This forces Kratos to redirect back to the Login UI after authentication,
   instead of auto-accepting the Hydra login request internally.
2. On the return redirect, the Login UI calls `AcceptLoginRequest` on Hydra, passing the selected
   `tenant_id` in the `context` field. Hydra propagates this context to the consent step.
3. During the Hydra consent step, the Login UI reads `tenant_id` from the consent request's
   `context` (set in step 2 above) and passes it as part of the `session.id_token` and
   `session.access_token` fields in `AcceptConsentRequest`.
4. Hydra propagates this proposed session payload to its token hook (`POST /api/v0/webhooks/token`).
5. The tenant-service webhook (token hook) acts as the final arbiter: it parses the proposed `tenant_id`
   from the request, validates that the user is still an active member of the selected tenant, and if so,
   explicitly confirms and injects the `tenant_id` into the final token's session claims, dictating exactly
   what goes into the resulting token.

**Option B — Token hook reads tenant_id from OAuth2 request parameters (future, machine users)**:
- Machine users (CLI tools, CI pipelines) perform OAuth2 `client_credentials` or
  `authorization_code` flows without a Kratos login session.
- No consent step exists for machine users; `tenant_id` must be communicated differently
  (e.g. a custom OAuth2 scope `tenant:<id>`, or a custom request parameter).
- This requires Hydra configuration changes to pass the parameter through to the token hook.
- **Tracked in `TODO.md` — not implemented in this iteration.**

**Choice rationale**: Option A reuses existing Hydra consent infrastructure and is aligned with
the sequence diagram already documented in `SEQUENCE_DIAGRAMS.md`. It requires the Login UI to
implement the consent step, which is documented in the Login UI prompt (`docs/login-ui-prompt.md`).

### Token hook behaviour (revised)

### `_none` Sentinel Invariant

The Login UI uses a `_none` sentinel value (`cookies.NoTenantAvailable`) to represent the state
where a user has no active tenants. This sentinel is **never forwarded** to the Hydra consent
session by the Login UI — both `InjectTenantPayload` (which skips injection when `tenantID` is
empty or `_none`) and `handleTenantSelection` (which rejects `_none` from the client) enforce
this guarantee.

If `_none` were to leak through to the token hook as a literal `_tenant_id` session value, the
`GetActiveMemberByTenantAndUserID` membership check would fail — no tenant with the ID `_none`
exists in the database — resulting in a `403 Forbidden` response. This is the correct
**fail-closed** behaviour.

By design, the Tenant Service does not need to special-case the `_none` sentinel. The existing
membership validation is sufficient to reject it.

The `HandleTokenHook` service method is updated as follows:

- Extract `tenantID` from `req.Session.Extra["tenant_id"]`.
- If `tenantID` is empty → respond with `200 OK` and **no** `tenant_id` claim. The user logged in
  without an active tenant context (e.g. went through recovery, or has no active tenants yet).
- If `tenantID` is non-empty → call `GetActiveMemberByTenantAndUserID(ctx, tenantID, userID)`.
  - `ErrNotFound` → return `ErrNotMember`; handler maps to `403 Forbidden`.
  - Success → inject `tenant_id` into both `id_token` and `access_token` claim maps.

The old `tenants` (array) claim is removed entirely. Consumers of the token must update to use
the singular `tenant_id` string claim.

---

## Consequences

### Positive
- Flow 2 (Tenant-Aware Login) and Flow 6 (Tenant Switching) are fully implementable.
- Token claims are now explicit and typed (`tenant_id` string, not `tenants` array).
- The Login UI has a well-defined contract to implement against.
- The webhook integration is robust and doesn't do complex reconciliation inline.

### Negative
- The tenant lookup endpoint is unauthenticated — email–tenant relationship is discoverable by
  unauthenticated callers. Acceptable for the current iteration; hardening is tracked.
- Orphaned identities are not automatically healed. They must be handled out-of-band by admins.
- Machine user token enrichment (Option B) is deferred. Until it is implemented, machine users
  will not receive `tenant_id` claims in their OAuth2 tokens.
- The existing `tenants` (array) claim is removed. Any existing token consumers relying on it
  will break.
