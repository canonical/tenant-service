// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package permissions

//go:generate mockgen -build_flags=--mod=mod -package permissions -destination ./mock_publisher.go -source=./interfaces.go

import (
	"context"

	v1 "github.com/canonical/authorization-service/api/v1"
)

// Publisher defines the contract for publishing permission lifecycle events to Kafka.
type Publisher interface {
	// Publish publishes permission operations asynchronously for the specified tenant.
	// Messages are keyed by tenantID to guarantee per-tenant FIFO ordering.
	Publish(ctx context.Context, tenantID string, ops ...*v1.PermissionOperation)

	// Close flushes any pending messages and releases underlying connections.
	Close() error
}

// NoopPublisher is a no-op implementation of Publisher used when Kafka is disabled or during testing.
type NoopPublisher struct{}

// NewNoopPublisher creates a new NoopPublisher.
func NewNoopPublisher() *NoopPublisher {
	return &NoopPublisher{}
}

// Publish implements Publisher.
func (n *NoopPublisher) Publish(ctx context.Context, tenantID string, ops ...*v1.PermissionOperation) {
}

// Close implements Publisher.
func (n *NoopPublisher) Close() error {
	return nil
}
