## Purpose

The tenant-service's HTTP path emits Prometheus response-time metrics for every request via
`monitoring.NewMiddleware(...).ResponseTime()`. Direct gRPC calls produce no equivalent metrics,
making gRPC traffic invisible to dashboards and alerts.

This spec defines standard Prometheus metrics for the gRPC server using `go-grpc-prometheus`,
achieving parity with the HTTP monitoring middleware without introducing custom instrumentation code.

---

## ADDED Requirements

### Requirement: gRPC server emits standard Prometheus metrics
The system SHALL emit standard Prometheus metrics for all gRPC unary and streaming server calls using `go-grpc-prometheus`. The metrics SHALL include call counts, handling duration histograms, and per-status-code counters with labels for service name, method name, call type, and gRPC status code.

#### Scenario: Metrics are exposed at the metrics endpoint
- **WHEN** the gRPC server handles any unary or streaming RPC call
- **THEN** `grpc_server_handling_seconds`, `grpc_server_started_total`, and `grpc_server_handled_total` metrics are recorded
- **AND** the metrics are exposed at the existing `/api/v0/metrics` endpoint alongside HTTP metrics

#### Scenario: Metrics have correct labels for unary calls
- **WHEN** a gRPC call to `CreateTenant` returns `OK`
- **THEN** the `grpc_server_handled_total` metric has `grpc_service="identity.platform.api.tenant.TenantService"`, `grpc_method="CreateTenant"`, `grpc_type="unary"`, `grpc_code="OK"`

#### Scenario: Metrics have correct labels for streaming calls
- **WHEN** a streaming RPC completes
- **THEN** the `grpc_server_handled_total` metric has `grpc_type` set to the appropriate streaming type (e.g., `server_stream`, `client_stream`, or `bidi_stream`)

### Requirement: gRPC metrics integrate with existing scraping
The system SHALL register gRPC Prometheus metrics with the existing Prometheus registry so that the existing HTTP metrics endpoint automatically serves both HTTP and gRPC metrics without additional endpoints or configuration changes.

#### Scenario: No new endpoints are needed
- **WHEN** the application starts
- **THEN** `grpc_prometheus.Register(grpcServer)` adds gRPC collectors to the default registry
- **AND** the existing `promhttp.Handler()` at `/api/v0/metrics` serves gRPC metrics automatically
