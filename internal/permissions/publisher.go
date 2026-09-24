// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package permissions

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	v1 "github.com/canonical/authorization-service/api/v1"
	"github.com/canonical/tenant-service/internal/logging"
	"github.com/google/uuid"
	"github.com/segmentio/kafka-go"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// KafkaWriter abstracts kafka.Writer for testing.
type KafkaWriter interface {
	WriteMessages(ctx context.Context, msgs ...kafka.Message) error
	Close() error
}

// KafkaPublisher publishes permission mutation events to Kafka asynchronously.
type KafkaPublisher struct {
	writer         KafkaWriter
	service        string
	logger         logging.LoggerInterface
	publishTimeout time.Duration
}

// NewKafkaPublisher initializes a new KafkaPublisher with the specified brokers and topic.
func NewKafkaPublisher(
	brokers []string,
	topic string,
	service string,
	logger logging.LoggerInterface,
) (*KafkaPublisher, error) {
	if len(brokers) == 0 {
		return nil, errors.New("kafka brokers list cannot be empty")
	}

	for _, b := range brokers {
		if strings.TrimSpace(b) == "" {
			return nil, errors.New("kafka broker address cannot be empty")
		}
	}

	if strings.TrimSpace(topic) == "" {
		return nil, errors.New("kafka topic cannot be empty")
	}

	if service == "" {
		service = "tenant-service"
	}

	if logger == nil {
		logger = logging.NewNoopLogger()
	}

	writer := &kafka.Writer{
		Addr:                   kafka.TCP(brokers...),
		Topic:                  topic,
		Balancer:               &kafka.Hash{},
		RequiredAcks:           kafka.RequireAll,
		MaxAttempts:            10,
		Async:                  true,
		AllowAutoTopicCreation: false,
		Completion: func(messages []kafka.Message, err error) {
			if err != nil {
				logger.Errorw("failed to publish permission update to kafka",
					"error", err,
					"messages_count", len(messages),
				)
			}
		},
	}

	return &KafkaPublisher{
		writer:         writer,
		service:        service,
		logger:         logger,
		publishTimeout: 5 * time.Second,
	}, nil
}

// NewKafkaPublisherWithWriter initializes a KafkaPublisher with a custom KafkaWriter (useful for testing).
func NewKafkaPublisherWithWriter(
	writer KafkaWriter,
	service string,
	logger logging.LoggerInterface,
) *KafkaPublisher {
	if service == "" {
		service = "tenant-service"
	}

	if logger == nil {
		logger = logging.NewNoopLogger()
	}

	return &KafkaPublisher{
		writer:         writer,
		service:        service,
		logger:         logger,
		publishTimeout: 5 * time.Second,
	}
}

// Publish builds one or more PermissionUpdateEnvelopes, keys the messages by tenantID for
// FIFO ordering, and publishes asynchronously via the underlying Kafka writer.
// Operations are split into envelopes of at most MaxOperationsPerEnvelope operations.
func (p *KafkaPublisher) Publish(ctx context.Context, tenantID string, ops ...*v1.PermissionOperation) {
	if p == nil || p.writer == nil || len(ops) == 0 {
		return
	}

	msgs, err := p.buildMessages(ctx, tenantID, ops...)
	if err != nil {
		p.logger.Errorw("failed to marshal permission update envelope",
			"tenant_id", tenantID,
			"error", err,
		)
		return
	}

	writeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), p.publishTimeout)
	defer cancel()

	if err := p.writer.WriteMessages(writeCtx, msgs...); err != nil {
		p.logger.Errorw("failed to write kafka message",
			"tenant_id", tenantID,
			"messages_count", len(msgs),
			"error", err,
		)
	}
}

// PublishSync publishes the permission operations synchronously (primarily for testing and migration scripts).
func (p *KafkaPublisher) PublishSync(ctx context.Context, tenantID string, ops ...*v1.PermissionOperation) error {
	if p == nil || p.writer == nil || len(ops) == 0 {
		return nil
	}

	msgs, err := p.buildMessages(ctx, tenantID, ops...)
	if err != nil {
		return fmt.Errorf("build permission message: %w", err)
	}

	writeCtx, cancel := context.WithTimeout(ctx, p.publishTimeout)
	defer cancel()

	if err := p.writer.WriteMessages(writeCtx, msgs...); err != nil {
		return fmt.Errorf("write kafka message: %w", err)
	}

	return nil
}

// buildMessages splits ops into chunks of at most MaxOperationsPerEnvelope and builds
// one Kafka message per chunk. All messages share the tenantID key, so they land on the
// same partition and are consumed in order.
func (p *KafkaPublisher) buildMessages(
	ctx context.Context,
	tenantID string,
	ops ...*v1.PermissionOperation,
) ([]kafka.Message, error) {
	msgs := make([]kafka.Message, 0, (len(ops)+MaxOperationsPerEnvelope-1)/MaxOperationsPerEnvelope)
	for chunk := range slices.Chunk(ops, MaxOperationsPerEnvelope) {
		msg, err := p.buildMessage(ctx, tenantID, chunk...)
		if err != nil {
			return nil, err
		}
		msgs = append(msgs, msg)
	}
	return msgs, nil
}

func (p *KafkaPublisher) buildMessage(
	ctx context.Context,
	tenantID string,
	ops ...*v1.PermissionOperation,
) (kafka.Message, error) {
	envelope := &v1.PermissionUpdateEnvelope{
		Version:        "1.0",
		Service:        p.service,
		MessageId:      uuid.NewString(),
		IdempotencyKey: uuid.NewString(),
		EventTime:      timestamppb.Now(),
		Operations:     ops,
	}

	span := trace.SpanFromContext(ctx)
	if span.SpanContext().IsValid() {
		traceID := span.SpanContext().TraceID().String()
		envelope.CorrelationId = &traceID
	}

	data, err := proto.Marshal(envelope)
	if err != nil {
		return kafka.Message{}, fmt.Errorf("marshal envelope: %w", err)
	}

	return kafka.Message{
		Key:   []byte(tenantID),
		Value: data,
	}, nil
}

// Close closes the underlying Kafka writer, which flushes any pending messages.
func (p *KafkaPublisher) Close() error {
	if p == nil || p.writer == nil {
		return nil
	}
	return p.writer.Close()
}

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
