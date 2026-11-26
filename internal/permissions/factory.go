// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package permissions

import (
	"context"
	"fmt"
	"slices"

	v1 "github.com/canonical/authorization-service/api/v1"
	"github.com/canonical/tenant-service/internal/config"
	"github.com/canonical/tenant-service/internal/grpcutil"
	"github.com/canonical/tenant-service/internal/logging"
	"github.com/canonical/tenant-service/internal/monitoring"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const defaultServiceName = "tenant-service"

// Labels for the permission_events_total metric. Each envelope is counted once:
// as a success when acknowledged, or as a failure at the stage where it was lost.
const (
	resultSuccess = "success"
	resultFailure = "failure"

	stageMarshal  = "marshal"
	stageWrite    = "write"
	stageDelivery = "delivery"
)

// Tenant relations defined by the authorization service model for tenant-service.
// Permissions cascade: can_delete implies can_edit, which implies can_view.
const (
	RelationCanView   = "can_view"
	RelationCanEdit   = "can_edit"
	RelationCanDelete = "can_delete"
)

// MaxOperationsPerEnvelope bounds the operations carried by a single envelope. The
// authorization service applies each envelope as one OpenFGA Write, which by default
// accepts at most 100 tuples.
const MaxOperationsPerEnvelope = 100

// tenantRelations lists every relation a user may hold directly on a tenant, whether
// granted by the tenant service or through the authorization service API.
var tenantRelations = []string{RelationCanView, RelationCanEdit, RelationCanDelete}

// NewPublisher constructs the configured permission Publisher for cfg.Mode
// ("noop", "platform", or "standalone").
func NewPublisher(
	cfg config.PermissionsConfig,
	logger logging.LoggerInterface,
	monitor monitoring.MonitorInterface,
) (Publisher, error) {
	if logger == nil {
		logger = logging.NewNoopLogger()
	}

	switch cfg.Mode {
	case "", "noop":
		logger.Info("Using noop permissions publisher")
		return NewNoopPublisher(), nil
	case "platform":
		publisher, err := NewKafkaPublisher(
			cfg.Kafka.Brokers,
			cfg.Kafka.Topic,
			defaultServiceName,
			logger,
			monitor,
		)
		if err != nil {
			return nil, fmt.Errorf("invalid platform permissions configuration: %w", err)
		}
		logger.Info("Kafka permissions publisher is enabled (platform mode)")
		return publisher, nil
	case "standalone":
		creds := grpcutil.Credentials{
			TokenURL:     cfg.AuthorizationService.TokenURL,
			ClientID:     cfg.AuthorizationService.ClientID,
			ClientSecret: cfg.AuthorizationService.ClientSecret,
			Scopes:       cfg.AuthorizationService.TokenScopes,
		}
		publisher, err := NewGRPCPublisher(
			cfg.AuthorizationService.GRPCAddress,
			creds,
			cfg.AuthorizationService.TLSEnabled,
			cfg.AuthorizationService.GRPCTimeout,
			cfg.AuthorizationService.GRPCWorkers,
			defaultServiceName,
			logger,
			monitor,
		)
		if err != nil {
			return nil, fmt.Errorf("invalid standalone permissions configuration: %w", err)
		}
		if !cfg.AuthorizationService.TLSEnabled {
			logger.Warn("Authorization Service gRPC TLS is disabled; OAuth2 bearer tokens will be sent in plaintext")
		}
		logger.Info("gRPC permissions publisher is enabled (standalone mode)")
		return publisher, nil
	default:
		return nil, fmt.Errorf("invalid permissions publishing mode %q: expected noop, platform, or standalone", cfg.Mode)
	}
}

type eventRecorder struct {
	monitor monitoring.MonitorInterface
	logger  logging.LoggerInterface
}

func (r eventRecorder) record(result, stage string, count int) {
	if count == 0 || r.monitor == nil {
		return
	}
	if err := r.monitor.AddPermissionEvents(map[string]string{"result": result, "stage": stage}, float64(count)); err != nil && r.logger != nil {
		r.logger.Warnw("failed to record permission events metric", "error", err)
	}
}

// envelopeCount returns the number of envelopes needed to carry ops operations.
func envelopeCount(ops int) int {
	return (ops + MaxOperationsPerEnvelope - 1) / MaxOperationsPerEnvelope
}

// buildEnvelopes splits ops into chunks of at most MaxOperationsPerEnvelope and
// builds one PermissionUpdateEnvelope per chunk, attaching the caller's
// OpenTelemetry trace ID as correlation_id.
func buildEnvelopes(
	ctx context.Context,
	service string,
	ops ...*v1.PermissionOperation,
) []*v1.PermissionUpdateEnvelope {
	if len(ops) == 0 {
		return nil
	}

	var traceID string
	if ctx != nil {
		span := trace.SpanFromContext(ctx)
		if span.SpanContext().IsValid() {
			traceID = span.SpanContext().TraceID().String()
		}
	}

	envelopes := make([]*v1.PermissionUpdateEnvelope, 0, envelopeCount(len(ops)))
	for chunk := range slices.Chunk(ops, MaxOperationsPerEnvelope) {
		envelope := &v1.PermissionUpdateEnvelope{
			Version:        "1.0",
			Service:        service,
			MessageId:      uuid.NewString(),
			IdempotencyKey: uuid.NewString(),
			EventTime:      timestamppb.Now(),
			Operations:     chunk,
		}
		if traceID != "" {
			correlationID := traceID
			envelope.CorrelationId = &correlationID
		}
		envelopes = append(envelopes, envelope)
	}
	return envelopes
}

// TenantPermissionOp builds a permission operation for a user on a tenant.
func TenantPermissionOp(op v1.PermissionOp, identityID, relation, tenantID string) *v1.PermissionOperation {
	return &v1.PermissionOperation{
		Op:       op,
		Subject:  "user:" + identityID,
		Relation: relation,
		Object:   "tenant:" + tenantID,
	}
}

// RevokeTenantPermissionOps builds DELETE operations for every relation a user may hold
// on a tenant. The authorization service ignores deletes for tuples that do not exist.
func RevokeTenantPermissionOps(identityID, tenantID string) []*v1.PermissionOperation {
	ops := make([]*v1.PermissionOperation, 0, len(tenantRelations))
	for _, relation := range tenantRelations {
		ops = append(ops, TenantPermissionOp(v1.PermissionOp_PERMISSION_OP_DELETE, identityID, relation, tenantID))
	}
	return ops
}
