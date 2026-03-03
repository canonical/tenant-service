# ADR 0005: Business Metrics Design

## Status

Accepted

## Context

The tenant service exposes a Prometheus metrics endpoint. An existing HTTP middleware (`monitoring.NewMiddleware`) already records `http_response_time_seconds` for every request, partitioned by path, method, and HTTP status code. This gives operators full visibility into request rates, latencies, and error rates at the transport level.

A requirement existed to augment these metrics with business-level counters — e.g. `tenant_created_total` — to capture operational activity.

The key question was: where should such counters live, and what should they track?

**Option A — Per-handler success/error counters.**
Add `IncrementCounter` calls in every handler method with `status=success` or `status=error`. This mirrors what the HTTP middleware already provides and adds no new information.

**Option B — Targeted sub-operation counters in the service layer.**
Add counters only at points in the business logic that are not already observable at the HTTP layer: specifically, events that distinguish *how* an operation completed, not just *whether* it succeeded.

## Decision

We adopt **Option B**: targeted sub-operation counters at the service layer.

A single `business_operations_total` Prometheus counter vector is registered with variable labels `operation` and `role`, and a constant label `service`. Counters are incremented only at two meaningful differentiation points:

1. **`InviteMember`** — records every successful invitation as `operation=invitation_sent`, labelled with the invited member's `role` (`owner`, `member`, `admin`). This covers both new invites and re-sends to already-pending members, since the HTTP response code is `200` in both cases and the fleet-level invite rate is the meaningful signal.

2. **`ProvisionUser`** — records each successful provisioning event (`operation=user_provisioned`) with the provisioned user's `role`. This allows operators to track the composition of provisioned users over time.

Handler-level counters were explicitly **not** added, because they would duplicate the information already present in `http_response_time_seconds`.

High-cardinality labels such as `tenant_id` were also rejected, as they would create an unbounded number of Prometheus time series and degrade scrape performance.

## Consequences

- `MonitorInterface` gains an `IncrementCounter(map[string]string) error` method, implemented in both the Prometheus and noop backends.
- The `Service` struct in `pkg/tenant` receives a `monitoring.MonitorInterface` dependency, injected via `NewService`.
- `Handler` retains a `monitoring.MonitorInterface` field (for potential future use) but does not call `IncrementCounter` itself.
- Two new alert/dashboard dimensions are available: `invitation_sent` by role (fleet-level invite rate), and `user_provisioned` by role (useful for capacity planning).
- Per-tenant observability is intentionally handled via structured logging (with `tenant_id` log fields) rather than Prometheus labels, to avoid unbounded label cardinality.
- Adding new business events in the future follows the same pattern: increment in the service layer at the point of distinction, using meaningful `operation` and `role` label values.
