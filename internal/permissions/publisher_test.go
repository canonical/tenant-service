// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package permissions

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	v1 "github.com/canonical/authorization-service/api/v1"
	"github.com/segmentio/kafka-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"google.golang.org/protobuf/proto"
)

type mockKafkaWriter struct {
	mu           sync.Mutex
	messages     []kafka.Message
	writeErr     error
	writeErrFunc func(attempt int) error
	attempts     int
	closed       bool
}

func (m *mockKafkaWriter) WriteMessages(ctx context.Context, msgs ...kafka.Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.attempts++

	if m.writeErrFunc != nil {
		if err := m.writeErrFunc(m.attempts); err != nil {
			return err
		}
	} else if m.writeErr != nil {
		return m.writeErr
	}

	m.messages = append(m.messages, msgs...)
	return nil
}

func (m *mockKafkaWriter) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	return nil
}

func (m *mockKafkaWriter) getMessages() []kafka.Message {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]kafka.Message(nil), m.messages...)
}

func (m *mockKafkaWriter) getAttempts() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.attempts
}

func TestNoopPublisher(t *testing.T) {
	noop := NewNoopPublisher()
	require.NotNil(t, noop)

	// Should not panic or error
	noop.Publish(context.Background(), "tenant-123", &v1.PermissionOperation{
		Op:       v1.PermissionOp_PERMISSION_OP_WRITE,
		Subject:  "user:u1",
		Relation: "owner",
		Object:   "tenant:tenant-123",
	})

	assert.NoError(t, noop.Close())
}

func TestKafkaPublisher_PublishSync(t *testing.T) {
	mockWriter := &mockKafkaWriter{}
	publisher := NewKafkaPublisherWithWriter(mockWriter, "tenant-service", nil)
	require.NotNil(t, publisher)

	op := &v1.PermissionOperation{
		Op:       v1.PermissionOp_PERMISSION_OP_WRITE,
		Subject:  "user:u1",
		Relation: "owner",
		Object:   "tenant:tenant-123",
	}

	ctx := context.Background()
	err := publisher.PublishSync(ctx, "tenant-123", op)
	require.NoError(t, err)

	msgs := mockWriter.getMessages()
	require.Len(t, msgs, 1)

	// Verify message keying by tenant ID
	assert.Equal(t, []byte("tenant-123"), msgs[0].Key)

	// Verify protobuf deserialization
	var env v1.PermissionUpdateEnvelope
	err = proto.Unmarshal(msgs[0].Value, &env)
	require.NoError(t, err)

	assert.Equal(t, "1.0", env.Version)
	assert.Equal(t, "tenant-service", env.Service)
	assert.NotEmpty(t, env.MessageId)
	assert.NotEmpty(t, env.IdempotencyKey)
	assert.NotNil(t, env.EventTime)
	require.Len(t, env.Operations, 1)
	assert.Equal(t, v1.PermissionOp_PERMISSION_OP_WRITE, env.Operations[0].Op)
	assert.Equal(t, "user:u1", env.Operations[0].Subject)
	assert.Equal(t, "owner", env.Operations[0].Relation)
	assert.Equal(t, "tenant:tenant-123", env.Operations[0].Object)
}

func TestKafkaPublisher_Publish_Async(t *testing.T) {
	mockWriter := &mockKafkaWriter{}
	publisher := NewKafkaPublisherWithWriter(mockWriter, "tenant-service", nil)

	op := &v1.PermissionOperation{
		Op:       v1.PermissionOp_PERMISSION_OP_DELETE,
		Subject:  "user:u2",
		Relation: "member",
		Object:   "tenant:tenant-456",
	}

	ctx := context.Background()
	publisher.Publish(ctx, "tenant-456", op)

	// Wait briefly for asynchronous goroutine
	require.Eventually(t, func() bool {
		return len(mockWriter.getMessages()) == 1
	}, 1*time.Second, 10*time.Millisecond)

	msgs := mockWriter.getMessages()
	assert.Equal(t, []byte("tenant-456"), msgs[0].Key)

	var env v1.PermissionUpdateEnvelope
	err := proto.Unmarshal(msgs[0].Value, &env)
	require.NoError(t, err)
	assert.Equal(t, "tenant:tenant-456", env.Operations[0].Object)
}

func TestKafkaPublisher_WithCorrelationID(t *testing.T) {
	mockWriter := &mockKafkaWriter{}
	publisher := NewKafkaPublisherWithWriter(mockWriter, "tenant-service", nil)

	tp := sdktrace.NewTracerProvider()
	otel.SetTracerProvider(tp)
	tracer := tp.Tracer("test")

	ctx, span := tracer.Start(context.Background(), "test-span")
	defer span.End()

	expectedTraceID := span.SpanContext().TraceID().String()

	err := publisher.PublishSync(ctx, "tenant-789", &v1.PermissionOperation{
		Op:       v1.PermissionOp_PERMISSION_OP_WRITE,
		Subject:  "user:u1",
		Relation: "admin",
		Object:   "tenant:tenant-789",
	})
	require.NoError(t, err)

	msgs := mockWriter.getMessages()
	require.Len(t, msgs, 1)

	var env v1.PermissionUpdateEnvelope
	err = proto.Unmarshal(msgs[0].Value, &env)
	require.NoError(t, err)
	require.NotNil(t, env.CorrelationId)
	assert.Equal(t, expectedTraceID, *env.CorrelationId)
}

func TestKafkaPublisher_Retry(t *testing.T) {
	mockWriter := &mockKafkaWriter{
		writeErrFunc: func(attempt int) error {
			if attempt < 3 {
				return errors.New("temporary kafka error")
			}
			return nil
		},
	}

	publisher := NewKafkaPublisherWithWriter(mockWriter, "tenant-service", nil)

	ctx := context.Background()
	publisher.Publish(ctx, "tenant-retry", &v1.PermissionOperation{
		Op:       v1.PermissionOp_PERMISSION_OP_WRITE,
		Subject:  "user:u1",
		Relation: "owner",
		Object:   "tenant:tenant-retry",
	})

	require.Eventually(t, func() bool {
		return len(mockWriter.getMessages()) == 1
	}, 2*time.Second, 20*time.Millisecond)

	assert.Equal(t, 3, mockWriter.getAttempts())
}

func TestKafkaPublisher_Close(t *testing.T) {
	mockWriter := &mockKafkaWriter{}
	publisher := NewKafkaPublisherWithWriter(mockWriter, "tenant-service", nil)

	assert.NoError(t, publisher.Close())
	assert.True(t, mockWriter.closed)
}
