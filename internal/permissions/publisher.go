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
	"github.com/canonical/tenant-service/internal/monitoring"
	"github.com/segmentio/kafka-go"
	"google.golang.org/protobuf/proto"
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
	recorder       eventRecorder
	publishTimeout time.Duration
}

var _ Publisher = (*KafkaPublisher)(nil)

// NewKafkaPublisher initializes a new KafkaPublisher with the specified brokers and topic.
func NewKafkaPublisher(
	brokers []string,
	topic string,
	service string,
	logger logging.LoggerInterface,
	monitor monitoring.MonitorInterface,
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
		service = defaultServiceName
	}

	if logger == nil {
		logger = logging.NewNoopLogger()
	}

	if monitor == nil {
		monitor = monitoring.NewNoopMonitor(service, logger)
	}

	p := &KafkaPublisher{
		service:        service,
		logger:         logger,
		recorder:       eventRecorder{monitor: monitor, logger: logger},
		publishTimeout: 5 * time.Second,
	}

	p.writer = &kafka.Writer{
		Addr:                   kafka.TCP(brokers...),
		Topic:                  topic,
		Balancer:               &kafka.Hash{},
		RequiredAcks:           kafka.RequireAll,
		MaxAttempts:            10,
		Async:                  true,
		AllowAutoTopicCreation: false,
		Completion:             p.onCompletion,
	}

	return p, nil
}

// onCompletion is invoked by the async Kafka writer once a batch has been
// acknowledged by the broker or has failed after all retries.
func (p *KafkaPublisher) onCompletion(messages []kafka.Message, err error) {
	if err != nil {
		p.logger.Errorw("failed to publish permission update to kafka",
			"error", err,
			"messages_count", len(messages),
		)
		p.recorder.record(resultFailure, stageDelivery, len(messages))
		return
	}
	p.recorder.record(resultSuccess, stageDelivery, len(messages))
}

// NewKafkaPublisherWithWriter initializes a KafkaPublisher with a custom KafkaWriter (useful for testing).
func NewKafkaPublisherWithWriter(
	writer KafkaWriter,
	service string,
	logger logging.LoggerInterface,
	monitor monitoring.MonitorInterface,
) *KafkaPublisher {
	if service == "" {
		service = defaultServiceName
	}

	if logger == nil {
		logger = logging.NewNoopLogger()
	}

	if monitor == nil {
		monitor = monitoring.NewNoopMonitor(service, logger)
	}

	return &KafkaPublisher{
		writer:         writer,
		service:        service,
		logger:         logger,
		recorder:       eventRecorder{monitor: monitor, logger: logger},
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
		p.recorder.record(resultFailure, stageMarshal, envelopeCount(len(ops)))
		return
	}

	writeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), p.publishTimeout)
	defer cancel()

	// Delivery results are recorded by the writer's Completion callback; only
	// errors returned before messages are handed to the writer are recorded here.
	if err := p.writer.WriteMessages(writeCtx, msgs...); err != nil {
		p.logger.Errorw("failed to write kafka message",
			"tenant_id", tenantID,
			"messages_count", len(msgs),
			"error", err,
		)
		p.recorder.record(resultFailure, stageWrite, len(msgs))
	}
}

// buildMessages splits ops into chunks of at most MaxOperationsPerEnvelope and builds
// one Kafka message per chunk. All messages share the tenantID key, so they land on the
// same partition and are consumed in order.
func (p *KafkaPublisher) buildMessages(
	ctx context.Context,
	tenantID string,
	ops ...*v1.PermissionOperation,
) ([]kafka.Message, error) {
	envelopes := buildEnvelopes(ctx, p.service, ops...)
	msgs := make([]kafka.Message, 0, len(envelopes))
	for _, envelope := range envelopes {
		data, err := proto.Marshal(envelope)
		if err != nil {
			return nil, fmt.Errorf("marshal envelope: %w", err)
		}
		msgs = append(msgs, kafka.Message{
			Key:   []byte(tenantID),
			Value: data,
		})
	}
	return msgs, nil
}

// Close closes the underlying Kafka writer, which flushes any pending messages.
func (p *KafkaPublisher) Close() error {
	if p == nil || p.writer == nil {
		return nil
	}
	return p.writer.Close()
}
