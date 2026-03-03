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

- [ ] Add `page_token` + `page_size` fields to `ListTenants`, `ListTenantUsers`,
      `ListMyTenants`, `ListUserTenants` request protos
- [ ] Implement cursor-based pagination in `internal/storage/storage.go` using UUIDv7 ordering
- [ ] Document max page size

### [#14](https://github.com/canonical/tenant-service/issues/14) — Handle client users

- [ ] Determine how client (machine/service) users differ from human users in the data model
- [ ] Add `user_type` field or separate membership path as appropriate
- [ ] Update FGA model if required

---

## 🟢 Low — Improvements & Cleanup

### Observability

- [ ] Fix span status in service methods — call `span.RecordError(err)` and
      `span.SetStatus(codes.Error, ...)` on all error paths (currently spans never record errors)
- [ ] Add OpenTelemetry gRPC unary interceptor in `cmd/serve.go`
- [ ] [#10](https://github.com/canonical/tenant-service/issues/10) Implement business metrics
      (e.g. `tenant_created_total`, `invite_sent_total`) in the service layer using `MonitorInterface`

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
