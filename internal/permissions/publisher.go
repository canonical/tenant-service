// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package permissions

import (
	"context"
	"fmt"
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
	maxRetries     int
	retryBackoff   time.Duration
}

// NewKafkaPublisher initializes a new KafkaPublisher with the specified brokers and topic.
func NewKafkaPublisher(
	brokers []string,
	topic string,
	service string,
	logger logging.LoggerInterface,
) *KafkaPublisher {
	if len(brokers) == 0 {
		return nil
	}

	if service == "" {
		service = "tenant-service"
	}

	writer := &kafka.Writer{
		Addr:     kafka.TCP(brokers...),
		Topic:    topic,
		Balancer: &kafka.LeastBytes{},
	}

	return &KafkaPublisher{
		writer:         writer,
		service:        service,
		logger:         logger,
		publishTimeout: 5 * time.Second,
		maxRetries:     3,
		retryBackoff:   100 * time.Millisecond,
	}
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

	return &KafkaPublisher{
		writer:         writer,
		service:        service,
		logger:         logger,
		publishTimeout: 5 * time.Second,
		maxRetries:     3,
		retryBackoff:   50 * time.Millisecond,
	}
}

// Publish builds a PermissionUpdateEnvelope, keys the message by tenantID for FIFO ordering,
// and publishes asynchronously using a detached context so callers never wait on Kafka I/O.
func (p *KafkaPublisher) Publish(ctx context.Context, tenantID string, ops ...*v1.PermissionOperation) {
	if p == nil || p.writer == nil || len(ops) == 0 {
		return
	}

	envelope, msg, err := p.buildMessage(ctx, tenantID, ops...)
	if err != nil {
		if p.logger != nil {
			p.logger.Errorw("failed to marshal permission update envelope",
				"tenant_id", tenantID,
				"error", err,
			)
		}
		return
	}

	// Detach context so request cancellation or database commit completion does not abort publishing
	detachedCtx := context.WithoutCancel(ctx)

	go func(targetCtx context.Context, message kafka.Message, msgID string, tID string, opCount int) {
		p.writeWithRetry(targetCtx, message, msgID, tID, opCount)
	}(detachedCtx, msg, envelope.MessageId, tenantID, len(ops))
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

func (p *KafkaPublisher) writeWithRetry(
	ctx context.Context,
	msg kafka.Message,
	msgID string,
	tenantID string,
	opCount int,
) {
	var err error
	backoff := p.retryBackoff

	for attempt := 1; attempt <= p.maxRetries; attempt++ {
		writeCtx, cancel := context.WithTimeout(ctx, p.publishTimeout)
		err = p.writer.WriteMessages(writeCtx, msg)
		cancel()

		if err == nil {
			return
		}

		if attempt < p.maxRetries {
			select {
			case <-ctx.Done():
				if p.logger != nil {
					p.logger.Errorw("context canceled during kafka write retries",
						"message_id", msgID,
						"tenant_id", tenantID,
						"attempt", attempt,
						"error", ctx.Err(),
					)
				}
				return
			case <-time.After(backoff):
				backoff *= 2
			}
		}
	}

	if p.logger != nil {
		p.logger.Errorw("failed to publish permission update to kafka after retries",
			"message_id", msgID,
			"tenant_id", tenantID,
			"operation_count", opCount,
			"attempts", p.maxRetries,
			"error", err,
		)
	}
}

// Close closes the underlying Kafka writer.
func (p *KafkaPublisher) Close() error {
	if p != nil && p.writer != nil {
		return p.writer.Close()
	}
	return nil
}
