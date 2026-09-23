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
- Changing database schema or SQL storage operations.
- Modifying client-facing gRPC or REST protobuf schemas.

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
  2. Set `SkipIssuerCheck: true` when manual JWKS (`AUTHENTICATION_JWKS_URL`) is configured.
  3. Permit validly signed tokens when no explicit `allowedSubjects` or `requiredScope` are specified, so any authenticated user identity (`sub`) is accepted and injected into the request context.
- **Alternative Considered**: Requiring hardcoded subject lists or scopes—rejected because STS user tokens represent arbitrary identities and route authorization is enforced upstream.

### Decision 7: Bypass `authorization-service` for `GET /api/v0/me/tenants` via Istio Gateway Rules
- **Rationale**: The endpoint `GET /api/v0/me/tenants` returns the calling user's tenant memberships and only inspects the caller's own memberships (`authentication.GetUserID(ctx)`). Rather than maintaining an artificial `account:me` OpenFGA resource and publishing static wildcard tuples (`user:* -> can_view -> account:me`) on startup, Istio Gateway rules bypass external authorization (`notPaths: ["/api/v0/me/tenants"]`) and route requests directly to `tenant-service`, where the built-in JWT authentication middleware validates the token and extracts the user identity.
- **Alternatives Considered**:
  - OpenFGA `type account` with static `user:* -> can_view -> account:me` tuple—rejected due to unnecessary schema complexity, startup synchronization requirements, and ext_authz latency on self-inspection queries.
  - Per-user tuple publishing on registration/provisioning—rejected due to write amplification, redundancy, and chicken-and-egg issues for new identities.
### Decision 8: Replace Roles in Tenant Authorization with Direct Fine-Grained Permissions and Cascading Privileges
- **Rationale**: Rather than using role relations (`owner`, `member`) on `type tenant` or attempting to bind tenant ownership to a global `role:tenant-owner` (which introduces multi-tenancy context collision issues and requires bridging tuples), roles are completely eliminated from tenant service authorization in favor of direct fine-grained permissions on `tenant:<tenant_id>`:
  - `owner` maps to `can_delete`
  - `admin` maps to `can_edit`
  - `member` maps to `can_view`
  The authorization service model defines cascading privileges where `can_delete` automatically confers `can_edit`, and `can_edit` automatically confers `can_view` (`can_delete -> can_edit -> can_view`).
- **Alternatives Considered**:
  - Global `role:tenant-owner#assignee` with `tenant_match` condition—rejected because OpenFGA tuple keys `(user, relation, object)` collide when a user owns multiple tenants, `PermissionOperation` in `messages.proto` lacks condition context parameters, and extra bridging tuples would be required.
  - Retaining role relations (`owner`, `member`) on `type tenant`—rejected to eliminate role abstractions from authorization and align directly with fine-grained endpoint rules (`can_view`, `can_edit`, `can_delete`).

## Risks / Trade-offs

- **[Risk] Kafka publish failure in fire-and-forget mode** → **Mitigation**: Publisher performs background retries with exponential backoff using a detached context (`context.WithoutCancel`). If publishing ultimately fails, structured error logs and telemetry metrics are emitted for operational alerting and auditing.
- **[Risk] Local test environment overhead** → **Mitigation**: `NoopPublisher` and mock publisher interfaces allow all existing test suites to run without requiring a running Kafka broker.

## Migration Plan

1. Remove `OPENFGA_*` configuration from deployment manifests and replace with `KAFKA_BROKERS`, `KAFKA_ENABLED=true`, `KAFKA_PERMISSIONS_TOPIC=tenant-service.permissions`.
2. Deploy updated `tenant-service` alongside `authorization-service`.
