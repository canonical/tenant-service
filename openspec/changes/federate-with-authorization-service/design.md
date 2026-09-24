## Context

See `proposal.md` for background and motivation. Currently, `tenant-service` directly queries OpenFGA for checks and directly writes ReBAC tuples upon tenant/member mutations. Under the federated architecture, Envoy Gateway + Authorization Service enforce route-level authorization upstream, while `tenant-service` publishes mutation events to the Kafka topic `tenant-service.permissions`.

## Goals / Non-Goals

**Goals:**
- Implement `internal/permissions` Kafka publisher emitting protobuf `PermissionUpdateEnvelope` messages.
- Replace OpenFGA SDK calls in `pkg/tenant/service.go` and `pkg/webhooks/service.go` with publisher invocations.
- Provide a `NoopPublisher` for local development and offline unit tests.
- Remove `internal/openfga`, `internal/authorization`, OpenFGA CLI commands, and OpenFGA dependencies from `go.mod`.
- Update configuration in `internal/config/specs.go` to support Kafka configuration (`KAFKA_ENABLED`, `KAFKA_BROKERS`, `KAFKA_PERMISSIONS_TOPIC`, `KAFKA_CLIENT_ID`).

**Non-Goals:**
- Transactional outbox table implementation (fire-and-forget asynchronous publishing with retries and structured error logging is used).
- Changing database schema or client-facing gRPC/REST schemas, other than removing membership roles (Decision 8).

## Decisions

### Decision 1: Use `github.com/canonical/authorization-service/api/v1` protobuf types
- **Rationale**: Reusing the published protobuf Go structs from `authorization-service` guarantees schema parity without manual `.proto` duplication.
- **Alternative Considered**: Copying the `.proto` file into `tenant-service`—rejected to avoid schema drift.

### Decision 2: Use `github.com/segmentio/kafka-go` for Kafka Producer
- **Rationale**: Matches the Kafka client library used in `authorization-service` and provides a lightweight, pure-Go implementation without CGo dependencies.
- **Alternative Considered**: `confluent-kafka-go`—rejected due to CGo/librdkafka build requirements.

### Decision 3: Keyed messages by tenant ID
- **Rationale**: Messages are keyed by tenant ID (e.g. `tenantID`). In Kafka, message keys ensure that all events for a given tenant are routed to the same partition, preserving strict per-tenant FIFO ordering regardless of partition count or consumer scaling in `authorization-service`.
- **Alternative Considered**: Unkeyed messages—rejected because multi-partition topics would risk out-of-order event consumption across partitions.

### Decision 4: NoopPublisher fallback
- **Rationale**: When `KAFKA_ENABLED=false` (e.g., during unit tests or lightweight standalone local runs), the service instantiates a `NoopPublisher` that logs operations without erroring.

### Decision 5: Asynchronous Fire-and-Forget Publishing
- **Rationale**: `tenant-service` uses request-lifecycle transaction management (`TransactionMiddleware` and `TransactionUnaryInterceptor`) that wraps each mutating request in `db.WithTx`. Performing synchronous blocking Kafka calls inside the handler would hold open database connections and table locks during Kafka network I/O and retries. Using asynchronous fire-and-forget publishing (with a detached background context) ensures that database transactions commit immediately without latency overhead.
- **Alternative Considered**: Synchronous blocking publish—rejected because holding database transactions open during remote network calls increases latency, starves connection pools, and couples DB transaction completion to Kafka availability.

### Decision 6: Support STS JWT Verification in `pkg/authentication`
- **Rationale**: Upstream Envoy Gateway and Authorization Service forward the caller's STS access token via `Authorization: Bearer <token>`. In `pkg/authentication/provider.go` and `verifier.go`:
  1. Add `SupportedSigningAlgs: []string{oidc.RS256, oidc.ES256}` to support STS signing keys.
  2. Enforce issuer validation (`SkipIssuerCheck: false`) when manual JWKS (`AUTHENTICATION_JWKS_URL`) is configured, ensuring tokens originate from the expected issuer.
  3. Permit validly signed tokens when no explicit `allowedSubjects` or `requiredScope` are specified, so any authenticated user identity (`sub`) is accepted and injected into the request context.
- **Alternative Considered**: Requiring hardcoded subject lists or scopes—rejected because STS user tokens represent arbitrary identities and route authorization is enforced upstream.

### Decision 7: Bypass `authorization-service` for `GET /api/v0/me/tenants` via Istio Gateway Rules
- **Rationale**: The endpoint `GET /api/v0/me/tenants` returns the calling user's tenant memberships and only inspects the caller's own memberships (`authentication.GetUserID(ctx)`). Rather than maintaining an artificial `account:me` OpenFGA resource and publishing static wildcard tuples (`user:* -> can_view -> account:me`) on startup, Istio Gateway rules bypass external authorization (`notPaths: ["/api/v0/me/tenants"]`) and route requests directly to `tenant-service`, where the built-in JWT authentication middleware validates the token and extracts the user identity.
- **Alternatives Considered**:
  - OpenFGA `type account` with static `user:* -> can_view -> account:me` tuple—rejected due to unnecessary schema complexity, startup synchronization requirements, and ext_authz latency on self-inspection queries.
  - Per-user tuple publishing on registration/provisioning—rejected due to write amplification, redundancy, and chicken-and-egg issues for new identities.
### Decision 8: Remove Membership Roles; Tenant Service Grants Baseline Permissions Only
- **Rationale**: The authorization service model grants fine-grained permissions directly on `tenant:<tenant_id>` with cascading privileges (`can_delete -> can_edit -> can_view`). Permission management belongs to the authorization service, so the tenant service no longer tracks or accepts a membership role:
  - Self-registration grants the registering user `can_delete` on their new tenant.
  - `InviteMember` and `ProvisionUser` always grant `can_view`.
  - `can_edit` and `can_delete` for any other user are granted only through the authorization service API. This includes the first owner of a tenant created by a platform admin via `CreateTenant`, which publishes no permission events.
  - `UpdateTenantUser` is removed, and the `role` fields are removed from the API (`identity-platform-api`) and from `memberships` (migration `003`).

  Accepting a caller-supplied role allowed privilege escalation: the gateway only checks the route-level permission (e.g. `can_edit`), so any caller with `can_edit` could invite or promote a user (including themselves) to `can_delete`. A stored role would also drift from OpenFGA as soon as permissions are granted through the authorization service API.
- **Alternatives Considered**:
  - Ceiling check in the tenant service (reject requested roles above the caller's own)—rejected because it duplicates authorization logic outside the authorization service and relies on the drift-prone stored role.
  - Mapping `admin` to `can_view`—rejected because users granted `can_edit` through the authorization service API could still escalate to `can_delete`.
  - Global `role:tenant-owner#assignee` with `tenant_match` condition—rejected because OpenFGA tuple keys `(user, relation, object)` collide when a user owns multiple tenants, `PermissionOperation` in `messages.proto` lacks condition context parameters, and extra bridging tuples would be required.

### Decision 9: Tenant Deletion Revokes Every Tenant Relation, Chunked per Envelope
- **Rationale**: Without a stored role, the tenant service cannot know which relation each member holds, and members may have been granted `can_edit`/`can_delete` through the authorization service API. On tenant deletion the service pages through all memberships and publishes `DELETE` operations for `can_view`, `can_edit`, and `can_delete` for every member. The authorization service applies deletes with "ignore missing" semantics, so revoking tuples that do not exist is safe.

  The authorization service applies each envelope as a single OpenFGA `Write`, which accepts at most 100 tuples by default. The publisher therefore splits operations into envelopes of at most 100 operations (`MaxOperationsPerEnvelope`), all keyed by tenant ID to preserve ordering.
- **Alternative Considered**: Wildcard deletes in the authorization service (e.g. delete all tuples on `tenant:<id>`)—preferable long-term because it does not depend on tenant-service storage, but it requires an authorization-service API change and is deferred.

## Risks / Trade-offs

- **[Risk] Kafka publish failure in fire-and-forget mode** → **Mitigation**: Publisher performs background retries with exponential backoff using a detached context (`context.WithoutCancel`). If publishing ultimately fails, structured error logs and telemetry metrics are emitted for operational alerting and auditing.
- **[Risk] Local test environment overhead** → **Mitigation**: `NoopPublisher` and mock publisher interfaces allow all existing test suites to run without requiring a running Kafka broker.
- **[Risk] Breaking API change for other `identity-platform-api` consumers (e.g. Admin UI)** → **Mitigation**: Coordinate the upstream change; removed proto fields are `reserved` so field numbers are never reused.
- **[Risk] Admin-created tenants have no owner until one is granted through the authorization service API** → **Mitigation**: Document the platform-admin flow; self-registered tenants are unaffected.
- **[Risk] `business_operations_total` loses its `role` label** → **Mitigation**: Update dashboards and alerts that group by `role`.
- **[Trade-off] Migration `003` down step cannot restore previous roles** → Existing rows are restored as `member`.

## Migration Plan

1. Land the `identity-platform-api` change removing membership roles and bump the dependency in `go.mod` and `tests/e2e/go.mod`.
2. Remove `OPENFGA_*` configuration from deployment manifests and replace with `KAFKA_BROKERS`, `KAFKA_ENABLED=true`, `KAFKA_PERMISSIONS_TOPIC=tenant-service.permissions`.
3. Deploy updated `tenant-service` alongside `authorization-service`; migration `003_drop_memberships_role.sql` runs on startup/migrate.
4. Grant owners of existing tenants `can_delete` (and `can_edit` where required) through the authorization service API.
