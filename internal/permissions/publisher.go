// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package permissions

import (
	"context"
	"errors"
	"fmt"
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

// Publish builds a PermissionUpdateEnvelope, keys the message by tenantID for FIFO ordering,
// and publishes asynchronously via the underlying Kafka writer.
func (p *KafkaPublisher) Publish(ctx context.Context, tenantID string, ops ...*v1.PermissionOperation) {
	if p == nil || p.writer == nil || len(ops) == 0 {
		return
	}

	envelope, msg, err := p.buildMessage(ctx, tenantID, ops...)
	if err != nil {
		p.logger.Errorw("failed to marshal permission update envelope",
			"tenant_id", tenantID,
			"error", err,
		)
		return
	}

	writeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), p.publishTimeout)
	defer cancel()

	if err := p.writer.WriteMessages(writeCtx, msg); err != nil {
		p.logger.Errorw("failed to write kafka message",
			"tenant_id", tenantID,
			"message_id", envelope.MessageId,
			"error", err,
		)
	}
}

// PublishSync publishes the permission operations synchronously (primarily for testing and migration scripts).
func (p *KafkaPublisher) PublishSync(ctx context.Context, tenantID string, ops ...*v1.PermissionOperation) error {
	if p == nil || p.writer == nil || len(ops) == 0 {
		return nil
	}

	_, msg, err := p.buildMessage(ctx, tenantID, ops...)
	if err != nil {
		return fmt.Errorf("build permission message: %w", err)
	}

	writeCtx, cancel := context.WithTimeout(ctx, p.publishTimeout)
	defer cancel()

	if err := p.writer.WriteMessages(writeCtx, msg); err != nil {
		return fmt.Errorf("write kafka message: %w", err)
	}

	return nil
}

func (p *KafkaPublisher) buildMessage(
	ctx context.Context,
	tenantID string,
	ops ...*v1.PermissionOperation,
) (*v1.PermissionUpdateEnvelope, kafka.Message, error) {
	msgID := uuid.NewString()
	idempotencyKey := uuid.NewString()

	envelope := &v1.PermissionUpdateEnvelope{
		Version:        "1.0",
		Service:        p.service,
		MessageId:      msgID,
		IdempotencyKey: idempotencyKey,
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
		return nil, kafka.Message{}, fmt.Errorf("marshal envelope: %w", err)
	}

	msg := kafka.Message{
		Key:   []byte(tenantID),
		Value: data,
	}

	return envelope, msg, nil
}

// Close closes the underlying Kafka writer, which flushes any pending messages.
func (p *KafkaPublisher) Close() error {
	if p == nil || p.writer == nil {
		return nil
	}
	return p.writer.Close()
}

// PermissionOpForRole constructs a *v1.PermissionOperation mapped for the given role.
// In the authz model, roles are entirely replaced with fine-grained cascading permissions
// on the tenant resource directly:
// - "owner" maps to "can_delete" (cascades to can_edit and can_view)
// - "admin" maps to "can_edit" (cascades to can_view)
// - "member" maps to "can_view"
func PermissionOpForRole(op v1.PermissionOp, identityID, role, tenantID string) *v1.PermissionOperation {
	var relation string
	switch role {
	case "owner":
		relation = "can_delete"
	case "admin":
		relation = "can_edit"
	case "member":
		relation = "can_view"
	default:
		relation = role
	}
	return &v1.PermissionOperation{
		Op:       op,
		Subject:  "user:" + identityID,
		Relation: relation,
		Object:   "tenant:" + tenantID,
	}
}
