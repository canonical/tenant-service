// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package events

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/segmentio/kafka-go"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type PermissionPublisherInterface interface {
	PublishOperations(ctx context.Context, ops ...*PermissionOperation) error
	Close() error
}

type KafkaPublisher struct {
	writer  *kafka.Writer
	service string
}

func NewKafkaPublisher(brokers []string, topic string, service string) *KafkaPublisher {
	if len(brokers) == 0 {
		return nil
	}
	writer := &kafka.Writer{
		Addr:     kafka.TCP(brokers...),
		Topic:    topic,
		Balancer: &kafka.LeastBytes{},
	}
	return &KafkaPublisher{
		writer:  writer,
		service: service,
	}
}

func (p *KafkaPublisher) PublishOperations(ctx context.Context, ops ...*PermissionOperation) error {
	if p == nil || p.writer == nil || len(ops) == 0 {
		return nil
	}

	msgID := uuid.NewString()
	idempotencyKey := uuid.NewString()

	env := &PermissionUpdateEnvelope{
		Version:        "1.0",
		Service:        p.service,
		MessageId:      msgID,
		IdempotencyKey: idempotencyKey,
		EventTime:      timestamppb.Now(),
		Operations:     ops,
	}

	data, err := proto.Marshal(env)
	if err != nil {
		return fmt.Errorf("marshal permission update envelope: %w", err)
	}

	msg := kafka.Message{
		Key:   []byte(idempotencyKey),
		Value: data,
	}

	if err := p.writer.WriteMessages(ctx, msg); err != nil {
		return fmt.Errorf("publish permission update to kafka: %w", err)
	}

	return nil
}

func (p *KafkaPublisher) Close() error {
	if p != nil && p.writer != nil {
		return p.writer.Close()
	}
	return nil
}

type NoopPublisher struct{}

func (n *NoopPublisher) PublishOperations(ctx context.Context, ops ...*PermissionOperation) error {
	return nil
}

func (n *NoopPublisher) Close() error {
	return nil
}

func ParseBrokers(brokersStr string) []string {
	if strings.TrimSpace(brokersStr) == "" {
		return nil
	}
	parts := strings.Split(brokersStr, ",")
	var brokers []string
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			brokers = append(brokers, trimmed)
		}
	}
	return brokers
}
