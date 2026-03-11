# TODO List

Sequence diagrams for all user flows are in [docs/SEQUENCE_DIAGRAMS.md](docs/SEQUENCE_DIAGRAMS.md).

---

## 🔴 Critical — Security / Correctness

### [#11](https://github.com/canonical/tenant-service/issues/11) — Add authorisation enforcement

Every write operation in `pkg/tenant/service.go` must call `s.authz.Check()` before proceeding.
The `Authorizer` and OpenFGA model are wired up but never consulted in the service layer.

- [ ] `InviteMember`: check caller has `owner` relation on `tenant:{id}` before inviting
- [ ] `UpdateTenant`: check caller has `can_edit` on `tenant:{id}`
- [ ] `DeleteTenant`: check caller has `can_delete` on `tenant:{id}`
- [ ] `UpdateTenantUser`: check caller has `can_edit` on `tenant:{id}`
- [ ] `ListTenantUsers`: check caller has `can_view` on `tenant:{id}`
- [ ] `CreateTenant`: decide and enforce who is allowed to create tenants (admin-only vs. self-service)
- [ ] Add `SecurityLogger` audit calls for all state-changing operations (currently wired but unused)

### Token hook — inject single `tenant_id` instead of a list

The current `HandleTokenHook` queries all active tenants for the user and injects them as a
`tenants: [...]` list. The correct behaviour is to read the `tenant_id` that was selected during
login from the session (set at the Hydra consent step in Flow 2), validate that the user is still
an active member of that tenant, and inject a single `tenant_id` claim.

- [ ] Read `tenant_id` from `req.Session.Extra["tenant_id"]` in `pkg/webhooks/service.go`
- [ ] Replace the `ListActiveTenantsByUserID` call with a single membership existence check
      (`GetMembership(ctx, tenantID, userID)` — requires the storage method from the login hook work)
- [ ] Change claim key from `tenants` (array) to `tenant_id` (string) in both `IDToken` and `AccessToken`
- [ ] Return `403 Forbidden` if the user is not an active member of the requested tenant
- [ ] Add unit tests

### [#15](https://github.com/canonical/tenant-service/issues/15) — Implement Kratos login hook

Implement the `POST /api/v0/webhooks/login` endpoint so that Kratos can validate during login
that the user is a member of the tenant they are trying to access.
Required for **Tenant-Aware Login** (Flow 2) and **Tenant Switching** (Flow 6) to work end-to-end.

- [ ] Add `login` handler to `pkg/webhooks/handlers.go` — register `POST /webhooks/login`
- [ ] Add `HandleLoginHook(ctx, identityID, tenantID string) error` to `pkg/webhooks/service.go`
- [ ] Query `memberships` to verify `(tenantID, identityID)` exists
- [ ] Return `200 OK` if valid, `403 Forbidden` if user is not a member
- [ ] Add `StorageInterface.GetMembership(ctx, tenantID, identityID)` method
- [ ] Add `storage.GetMembership` SQL implementation
- [ ] Add unit tests for the new handler and service method

### [#15](https://github.com/canonical/tenant-service/issues/15) — Implement tenant lookup endpoint

Implement `GET /api/v0/tenants/lookup?email=...` so the Login UI can discover a user's tenants
by email during the identifier-first login step.
Required for **Tenant-Aware Login** (Flow 2) and **Tenant Switching** (Flow 6).

- [ ] Add `LookupTenantsByEmail` gRPC method to `api/proto/v0/tenant.proto` with HTTP binding
      `GET /api/v0/tenants/lookup` and query param `email`
- [ ] Regenerate protobuf (`buf generate`)
- [ ] Implement `Handler.LookupTenantsByEmail` in `pkg/tenant/handlers.go`
- [ ] Implement `Service.LookupTenantsByEmail(ctx, email string)` in `pkg/tenant/service.go`
      — calls Kratos Admin to resolve email → identityID, then calls storage
- [ ] Add unit tests

---

## 🟡 Medium — Functionality Gaps

### `ProvisionUser` — add recovery link generation

The spec ([ID054](../ID054%20-%20Tenant%20API.md)) states that `ProvisionUser` should generate a
Kratos recovery link so the provisioned user receives an invitation email. This is currently missing.

- [ ] Call `s.kratos.CreateRecoveryLink(ctx, identityID, s.invitationLifetime)` at the end of
      `pkg/tenant/service.go` `ProvisionUser`
- [ ] Return the link/code in `ProvisionUserResponse` (requires proto update)
- [ ] Add unit test covering the recovery link generation step

### [#12](https://github.com/canonical/tenant-service/issues/12) — Add pagination

- [x] Add `page_token` + `page_size` fields to `ListTenants`, `ListTenantUsers`,
      `ListMyTenants`, `ListUserTenants` request protos
- [x] Implement cursor-based pagination in `internal/storage/storage.go` using UUIDv7 ordering
- [x] Document max page size (100, enforced via `types.ListOptions.ResolvePageSize()`)

#### Follow-up: pagination × OpenFGA authorization

Currently `ListMyTenants` and `ListTenantsByUserID` enforce visibility via a DB `JOIN memberships`
(the membership table mirrors OpenFGA tuples), so no per-row OpenFGA check is required.
`ListTenants` and `ListTenantUsers` need only a single gate `Check` at entry (not per-row).

When more sophisticated per-row authz is required (e.g. attribute-based visibility, or decoupling
the membership table from OpenFGA as the source of truth), the OpenFGA docs ("Search with
Permissions") and Zanzibar paper give clear guidance on the trade-offs. There are three patterns;
the right choice depends on how many objects the user can access and the total object count.

---

**Option A — Search then BatchCheck (recommended for paginated APIs)**

Paginate the DB normally, then call `/batch-check` on the returned page to filter out any rows
the caller is not permitted to see. Repeat if needed.

```
// 1. Fetch a page from DB (cursor-based, as today).
tenants, nextToken, err := storage.ListTenants(ctx, opts)

// 2. BatchCheck every tenant ID against the caller.
checks := make([]openfga.CheckRequest, len(tenants))
for i, t := range tenants {
    checks[i] = openfga.CheckRequest{User: caller, Relation: "can_view", Object: "tenant:" + t.ID}
}
results, err := authz.BatchCheck(ctx, checks) // internal/authorization

// 3. Filter denied rows.
allowed := tenants[:0]
for i, r := range results {
    if r.Allowed { allowed = append(allowed, tenants[i]) }
}
```

- Correct page sizes and cursor integrity: paging happens entirely in the DB layer.
- BatchCheck batches N checks in a single gRPC call to OpenFGA — much lower latency than N
  individual Check calls. The OpenFGA server also runs checks concurrently internally.
- Scales to arbitrarily large data sets because only one page's worth of IDs is sent per request.
- Con: If many rows on a given page are denied (sparse permissions), the effective page may be
  smaller than requested. Callers should treat a non-empty `next_page_token` as the signal to
  continue, not the page length.
- **This is the pattern OpenFGA explicitly recommends for paginated listing** (see "Search with
  Permissions — Option 1", and the BatchCheck "When to use" guidance).

---

**Option B — ListObjects-first (only for small, non-paginated sets)**

Call `ListObjects` once to get all permitted IDs, then restrict the DB query with
`WHERE id = ANY($ids)`.

```
ids, err := authz.ListObjects(ctx, caller, "can_view", "tenant")
tenants, nextToken, err := storage.ListTenants(ctx, types.ListOptions{
    PageToken:  opts.PageToken,
    PageSize:   opts.PageSize,
    AllowedIDs: ids, // new field — generates WHERE id = ANY($1)
})
```

- Exact, deterministic page sizes.
- **Con — not suitable for paginated APIs at scale:** `ListObjects` returns *all* authorized IDs
  in a single (deadline-bounded, default 3 s) call with a default cap of 1000 results. You must
  receive the complete list before you can begin sorting/filtering in the DB. This breaks
  pagination UX and wastes memory.
- The OpenFGA docs state explicitly: "As the number of objects increases, this solution becomes
  impractical because you would need to paginate over multiple pages [of ListObjects] to get the
  entire list before being able to search and sort. A partial list is not enough because you won't
  be able to sort using it." (Search with Permissions — Option 3, scenario C/D)
- Only appropriate when: total objects the user can access is low (~≤1000) **and** the use-case
  does not require server-side sort/filter on metadata (names, dates, etc.).
- If used, cache the `ListObjects` result with a short TTL (keyed on `(userID, relation, type)`)
  to amortise cost across pages of the same listing session.

---

**Option C — Local index from changes endpoint (for Google Drive–scale scenarios)**

Consume the `GET /changes` (ReadChanges) endpoint to build a local authorisation index, intersect
that with DB results, and call `/check` for borderline cases only. Described in OpenFGA "Search
with Permissions — Option 2". Not appropriate for this service at current scale.

---

**Recommendation:** implement **Option A (BatchCheck)**. It composes cleanly with the existing
cursor-based pagination, scales without bounds, and is the officially recommended pattern.

BatchCheck is available in the OpenFGA Go SDK (`>=v0.8.0` server-side).
Always pass `authorizationModelId` in BatchCheck calls (avoids an extra DB round-trip on the
OpenFGA side — documented in "Best Practices of Managing Tuples and Invoking APIs").

**Consistency note (from Zanzibar paper / OpenFGA consistency docs):**
Zanzibar introduced *zookies* — opaque consistency tokens returned by writes and supplied to
subsequent reads to guarantee "new-enemy" safety (a permission grant is never missed). OpenFGA
does not yet implement zookies. In the interim, use `HIGHER_CONSISTENCY` mode on the BatchCheck
call for `ListTenantUsers` (role visibility after an `UpdateTenantUser`). A practical heuristic:
if `updated_at` on the tenant/membership row is within the OpenFGA cache TTL window, use
`HIGHER_CONSISTENCY`; otherwise `MINIMIZE_LATENCY` is safe and cheap.

- [ ] Add `BatchCheck(ctx, []CheckRequest) ([]CheckResult, error)` to `AuthorizerInterface`
      and implement it in `internal/authorization/authorization.go` using the OpenFGA SDK
- [ ] In service `ListTenants` / `ListTenantUsers`: after fetching each page from storage, call
      `authz.BatchCheck` and filter the results before returning
- [ ] Always pass `authorizationModelId` to all OpenFGA API calls
- [ ] Use `HIGHER_CONSISTENCY` when `membership.updated_at` is within the cache TTL window
- [ ] Consider removing the `JOIN memberships` authz shortcut from `listTenantsByUserID`
      once OpenFGA is the sole source of truth for membership

### [#14](https://github.com/canonical/tenant-service/issues/14) — Handle client users

- [x] Determine how client (machine/service) users differ from human users in the data model
- [x] Add `user_type` field or separate membership path as appropriate
- [x] Update FGA model if required
- [ ] Expand `CreateTenantClientRequest` to mirror Hydra's supported fields (e.g., `client_name`, `scope`, `audience`, `grant_types`) so the endpoint isn't artificially limited just to ID/Secret.
- [ ] Add support/documentation for Client Secret Rotation. (Note: Instead of building native rotation endpoints, consider proxying/documenting the native Hydra Admin `PUT /clients/{id}` endpoint to accomplish this).

### Orphaned Identities on Tenant Cascades

- [ ] When a tenant is deleted, the database foreign keys clean up the `memberships` table, but the underlying identities (Kratos user accounts bound specifically to that tenant, and Hydra OAuth2 clients) remain orphaned in their respective systems. A mechanism needs to be introduced to explicitly garbage-collect/delete Kratos users and Hydra clients associated exclusively with the deleted tenant.

---

## 🟢 Low — Improvements & Cleanup

### Observability

- [x] Fix span status in service methods — call `span.RecordError(err)` and
      `span.SetStatus(codes.Error, ...)` on all error paths (currently spans never record errors)
- [x] Add OpenTelemetry gRPC unary interceptor in `cmd/serve.go`
- [ ] [#10](https://github.com/canonical/tenant-service/issues/10) Implement business metrics
      (e.g. `tenant_created_total`, `invite_sent_total`) in the service layer using `MonitorInterface`
- [ ] Enhance structured logging with `tenant_id` context — add `tenant_id` to log fields in
      service methods so per-tenant activity is observable via logs rather than Prometheus
      (Prometheus `tenant_id` label is intentionally omitted due to unbounded cardinality)

### Technical debt

- [ ] Remove unused `invites` table logic from `internal/storage/storage.go`
      (invitation flow is fully handled via Kratos recovery codes)
- [ ] Remove the `Add invites table migration` placeholder — migration is no longer needed
- [ ] Standardise role values: replace `string` with a typed constant/enum across
      `pkg/tenant`, `pkg/webhooks`, and `internal/storage` to prevent typo bugs

### Testing

- [ ] Implement unit tests for `pkg/tenant` (handler + service layers)
- [ ] Initialise E2E test suite
- [ ] Implement browser-based E2E tests for Hydra/Kratos hooks

### Infrastructure

- [ ] [#16](https://github.com/canonical/tenant-service/issues/16) Onboard repo to Tiobe
- [ ] [#13](https://github.com/canonical/tenant-service/issues/13) Move protobuf definition
      to `identity-platform-api`
