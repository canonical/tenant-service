// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package permissions

//go:generate mockgen -build_flags=--mod=mod -package permissions -destination ./mock_publisher.go -source=./interfaces.go

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

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
		Relation: RelationCanDelete,
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
		Relation: RelationCanDelete,
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
	assert.Equal(t, RelationCanDelete, env.Operations[0].Relation)
	assert.Equal(t, "tenant:tenant-123", env.Operations[0].Object)
}

func TestKafkaPublisher_Publish_Async(t *testing.T) {
	mockWriter := &mockKafkaWriter{}
	publisher := NewKafkaPublisherWithWriter(mockWriter, "tenant-service", nil)

	op := &v1.PermissionOperation{
		Op:       v1.PermissionOp_PERMISSION_OP_DELETE,
		Subject:  "user:u2",
		Relation: RelationCanView,
		Object:   "tenant:tenant-456",
	}

	ctx := context.Background()
	publisher.Publish(ctx, "tenant-456", op)

	msgs := mockWriter.getMessages()
	require.Len(t, msgs, 1)
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
		Relation: RelationCanEdit,
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

func TestKafkaPublisher_Publish_WriteError(t *testing.T) {
	mockWriter := &mockKafkaWriter{
		writeErr: errors.New("write failure"),
	}

	publisher := NewKafkaPublisherWithWriter(mockWriter, "tenant-service", nil)

	ctx := context.Background()
	// Should not panic on write error
	publisher.Publish(ctx, "tenant-err", &v1.PermissionOperation{
		Op:       v1.PermissionOp_PERMISSION_OP_WRITE,
		Subject:  "user:u1",
		Relation: RelationCanDelete,
		Object:   "tenant:tenant-err",
	})

	assert.Equal(t, 1, mockWriter.getAttempts())
}

func TestKafkaPublisher_Close(t *testing.T) {
	mockWriter := &mockKafkaWriter{}
	publisher := NewKafkaPublisherWithWriter(mockWriter, "tenant-service", nil)

	assert.NoError(t, publisher.Close())
	assert.True(t, mockWriter.closed)
}

func TestNewKafkaPublisher(t *testing.T) {
	tests := []struct {
		name        string
		brokers     []string
		topic       string
		service     string
		expectErr   bool
		errContains string
	}{
		{
			name:        "empty brokers",
			brokers:     nil,
			topic:       "permissions",
			service:     "tenant-service",
			expectErr:   true,
			errContains: "brokers list cannot be empty",
		},
		{
			name:        "broker with whitespace only",
			brokers:     []string{"   "},
			topic:       "permissions",
			service:     "tenant-service",
			expectErr:   true,
			errContains: "broker address cannot be empty",
		},
		{
			name:        "empty topic",
			brokers:     []string{"localhost:9092"},
			topic:       "",
			service:     "tenant-service",
			expectErr:   true,
			errContains: "topic cannot be empty",
		},
		{
			name:      "valid configuration with defaults",
			brokers:   []string{"localhost:9092"},
			topic:     "permissions",
			service:   "",
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pub, err := NewKafkaPublisher(tt.brokers, tt.topic, tt.service, nil)
			if tt.expectErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
				assert.Nil(t, pub)
			} else {
				require.NoError(t, err)
				require.NotNil(t, pub)
				assert.Equal(t, "tenant-service", pub.service)
				assert.NotNil(t, pub.logger)
				assert.NotNil(t, pub.writer)
			}
		})
	}
}

func TestNewKafkaPublisher_WriterConfig(t *testing.T) {
	pub, err := NewKafkaPublisher([]string{"localhost:9092"}, "test-topic", "test-service", nil)
	require.NoError(t, err)
	require.NotNil(t, pub)

	kw, ok := pub.writer.(*kafka.Writer)
	require.True(t, ok)
	assert.True(t, kw.Async)
	assert.Equal(t, 10, kw.MaxAttempts)
	assert.IsType(t, &kafka.Hash{}, kw.Balancer)
	assert.NotNil(t, kw.Completion)

	// Verify completion hook runs without panicking
	assert.NotPanics(t, func() {
		kw.Completion([]kafka.Message{{Key: []byte("test")}}, errors.New("test error"))
		kw.Completion([]kafka.Message{{Key: []byte("test")}}, nil)
	})
}

func TestTenantPermissionOp(t *testing.T) {
	tests := []struct {
		name     string
		op       v1.PermissionOp
		relation string
	}{
		{name: "write can_view", op: v1.PermissionOp_PERMISSION_OP_WRITE, relation: RelationCanView},
		{name: "write can_edit", op: v1.PermissionOp_PERMISSION_OP_WRITE, relation: RelationCanEdit},
		{name: "write can_delete", op: v1.PermissionOp_PERMISSION_OP_WRITE, relation: RelationCanDelete},
		{name: "delete can_view", op: v1.PermissionOp_PERMISSION_OP_DELETE, relation: RelationCanView},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := TenantPermissionOp(tt.op, "user-123", tt.relation, "tenant-abc")
			require.NotNil(t, got)
			assert.Equal(t, tt.op, got.Op)
			assert.Equal(t, "user:user-123", got.Subject)
			assert.Equal(t, tt.relation, got.Relation)
			assert.Equal(t, "tenant:tenant-abc", got.Object)
		})
	}
}

func TestTenantRelationConstants(t *testing.T) {
	assert.Equal(t, "can_view", RelationCanView)
	assert.Equal(t, "can_edit", RelationCanEdit)
	assert.Equal(t, "can_delete", RelationCanDelete)
	assert.Equal(t, 100, MaxOperationsPerEnvelope)
}

func TestRevokeTenantPermissionOps(t *testing.T) {
	ops := RevokeTenantPermissionOps("user-456", "tenant-abc")
	require.Len(t, ops, 3)

	expectedRelations := []string{RelationCanView, RelationCanEdit, RelationCanDelete}
	for i, op := range ops {
		assert.Equal(t, v1.PermissionOp_PERMISSION_OP_DELETE, op.Op)
		assert.Equal(t, "user:user-456", op.Subject)
		assert.Equal(t, expectedRelations[i], op.Relation)
		assert.Equal(t, "tenant:tenant-abc", op.Object)
	}
}

func buildOps(n int, tenantID string) []*v1.PermissionOperation {
	ops := make([]*v1.PermissionOperation, 0, n)
	for i := range n {
		ops = append(ops, TenantPermissionOp(
			v1.PermissionOp_PERMISSION_OP_DELETE,
			fmt.Sprintf("user-%d", i),
			RelationCanView,
			tenantID,
		))
	}
	return ops
}

func assertChunkedMessages(t *testing.T, msgs []kafka.Message, tenantID string, ops []*v1.PermissionOperation, expectedSizes []int) {
	t.Helper()

	require.Len(t, msgs, len(expectedSizes))

	messageIDs := make(map[string]struct{}, len(msgs))
	idempotencyKeys := make(map[string]struct{}, len(msgs))
	offset := 0
	for i, msg := range msgs {
		assert.Equal(t, []byte(tenantID), msg.Key)

		var env v1.PermissionUpdateEnvelope
		require.NoError(t, proto.Unmarshal(msg.Value, &env))

		assert.Equal(t, "1.0", env.Version)
		assert.Equal(t, "tenant-service", env.Service)
		require.NotEmpty(t, env.MessageId)
		require.NotEmpty(t, env.IdempotencyKey)
		messageIDs[env.MessageId] = struct{}{}
		idempotencyKeys[env.IdempotencyKey] = struct{}{}

		require.Len(t, env.Operations, expectedSizes[i])
		for j, op := range env.Operations {
			expected := ops[offset+j]
			assert.Equal(t, expected.Op, op.Op)
			assert.Equal(t, expected.Subject, op.Subject)
			assert.Equal(t, expected.Relation, op.Relation)
			assert.Equal(t, expected.Object, op.Object)
		}
		offset += len(env.Operations)
	}

	assert.Equal(t, len(ops), offset)
	assert.Len(t, messageIDs, len(msgs), "message IDs must be distinct")
	assert.Len(t, idempotencyKeys, len(msgs), "idempotency keys must be distinct")
}

func TestKafkaPublisher_Chunking(t *testing.T) {
	tests := []struct {
		name          string
		numOps        int
		expectedSizes []int
	}{
		{name: "exactly max ops", numOps: 100, expectedSizes: []int{100}},
		{name: "one over max ops", numOps: 101, expectedSizes: []int{100, 1}},
		{name: "250 ops", numOps: 250, expectedSizes: []int{100, 100, 50}},
	}

	for _, tt := range tests {
		t.Run("PublishSync/"+tt.name, func(t *testing.T) {
			mockWriter := &mockKafkaWriter{}
			publisher := NewKafkaPublisherWithWriter(mockWriter, "tenant-service", nil)

			ops := buildOps(tt.numOps, "tenant-big")
			require.NoError(t, publisher.PublishSync(context.Background(), "tenant-big", ops...))

			assert.Equal(t, 1, mockWriter.getAttempts(), "all envelopes must be written in a single WriteMessages call")
			assertChunkedMessages(t, mockWriter.getMessages(), "tenant-big", ops, tt.expectedSizes)
		})

		t.Run("Publish/"+tt.name, func(t *testing.T) {
			mockWriter := &mockKafkaWriter{}
			publisher := NewKafkaPublisherWithWriter(mockWriter, "tenant-service", nil)

			ops := buildOps(tt.numOps, "tenant-big")
			publisher.Publish(context.Background(), "tenant-big", ops...)

			assert.Equal(t, 1, mockWriter.getAttempts(), "all envelopes must be written in a single WriteMessages call")
			assertChunkedMessages(t, mockWriter.getMessages(), "tenant-big", ops, tt.expectedSizes)
		})
	}
}

func TestKafkaPublisher_NoOps(t *testing.T) {
	mockWriter := &mockKafkaWriter{}
	publisher := NewKafkaPublisherWithWriter(mockWriter, "tenant-service", nil)

	publisher.Publish(context.Background(), "tenant-empty")
	require.NoError(t, publisher.PublishSync(context.Background(), "tenant-empty"))

	assert.Equal(t, 0, mockWriter.getAttempts())
	assert.Empty(t, mockWriter.getMessages())
}

func TestKafkaPublisher_PublishSync_WriteError(t *testing.T) {
	mockWriter := &mockKafkaWriter{writeErr: errors.New("write failure")}
	publisher := NewKafkaPublisherWithWriter(mockWriter, "tenant-service", nil)

	err := publisher.PublishSync(context.Background(), "tenant-err", buildOps(250, "tenant-err")...)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "write failure")
	assert.Equal(t, 1, mockWriter.getAttempts())
}
