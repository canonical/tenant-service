// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package events

import (
	"context"
	"reflect"
	"testing"

	"google.golang.org/protobuf/proto"
)

func TestParseBrokers(t *testing.T) {
	brokers := ParseBrokers(" localhost:9092, kafka:9092 , ")
	expected := []string{"localhost:9092", "kafka:9092"}
	if !reflect.DeepEqual(brokers, expected) {
		t.Errorf("expected %v, got %v", expected, brokers)
	}

	emptyBrokers := ParseBrokers("")
	if emptyBrokers != nil {
		t.Errorf("expected nil, got %v", emptyBrokers)
	}
}

func TestNoopPublisher(t *testing.T) {
	publisher := &NoopPublisher{}
	err := publisher.PublishOperations(context.Background(), &PermissionOperation{
		Op:       PermissionOp_PERMISSION_OP_WRITE,
		Subject:  "user:u1",
		Relation: "owner",
		Object:   "tenant:t1",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := publisher.Close(); err != nil {
		t.Fatalf("unexpected error on close: %v", err)
	}
}

func TestEnvelopeSerialization(t *testing.T) {
	env := &PermissionUpdateEnvelope{
		Version:        "1.0",
		Service:        "tenant-service",
		MessageId:      "msg-1",
		IdempotencyKey: "idem-1",
		Operations: []*PermissionOperation{
			{
				Op:       PermissionOp_PERMISSION_OP_WRITE,
				Subject:  "user:u1",
				Relation: "owner",
				Object:   "tenant:t1",
			},
		},
	}

	data, err := proto.Marshal(env)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("expected non-empty byte slice")
	}

	var decoded PermissionUpdateEnvelope
	err = proto.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if decoded.GetVersion() != "1.0" {
		t.Errorf("expected version 1.0, got %s", decoded.GetVersion())
	}
	if decoded.GetService() != "tenant-service" {
		t.Errorf("expected service tenant-service, got %s", decoded.GetService())
	}
	if decoded.GetMessageId() != "msg-1" {
		t.Errorf("expected message_id msg-1, got %s", decoded.GetMessageId())
	}
	if decoded.GetIdempotencyKey() != "idem-1" {
		t.Errorf("expected idempotency_key idem-1, got %s", decoded.GetIdempotencyKey())
	}
	if len(decoded.GetOperations()) != 1 {
		t.Fatalf("expected 1 operation, got %d", len(decoded.GetOperations()))
	}
	op := decoded.GetOperations()[0]
	if op.GetOp() != PermissionOp_PERMISSION_OP_WRITE || op.GetSubject() != "user:u1" || op.GetRelation() != "owner" || op.GetObject() != "tenant:t1" {
		t.Errorf("unexpected operation contents: %+v", op)
	}
}
