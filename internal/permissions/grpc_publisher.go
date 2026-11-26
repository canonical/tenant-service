// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package permissions

import (
	"context"
	"errors"
	"sync"
	"time"

	v1 "github.com/canonical/authorization-service/api/v1"
	"github.com/canonical/tenant-service/internal/grpcutil"
	"github.com/canonical/tenant-service/internal/logging"
	"github.com/canonical/tenant-service/internal/monitoring"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
)

const (
	defaultGRPCQueueSize    = 1024
	defaultGRPCWorkers      = 5
	defaultGRPCRPCTimeout   = 5 * time.Second
	defaultGRPCCloseTimeout = 10 * time.Second
)

const grpcRetryServiceConfig = `{
	"methodConfig": [{
		"name": [{"service": "authorization.service.api.v1.PermissionIngestionService", "method": "IngestPermissionUpdate"}],
		"waitForReady": true,
		"retryPolicy": {
			"maxAttempts": 3,
			"initialBackoff": "0.1s",
			"maxBackoff": "1s",
			"backoffMultiplier": 2,
			"retryableStatusCodes": ["UNAVAILABLE", "INTERNAL"]
		}
	}]
}`

type queuedEnvelope struct {
	ctx      context.Context
	tenantID string
	envelope *v1.PermissionUpdateEnvelope
}

// GRPCPublisherOption configures optional GRPCPublisher parameters (primarily for tests).
type GRPCPublisherOption func(*GRPCPublisher)

func withQueueSize(size int) GRPCPublisherOption {
	return func(p *GRPCPublisher) {
		if size > 0 {
			p.queueSize = size
		}
	}
}

func withWorkers(workers int) GRPCPublisherOption {
	return func(p *GRPCPublisher) {
		if workers > 0 {
			p.workers = workers
		}
	}
}

func withCloseTimeout(d time.Duration) GRPCPublisherOption {
	return func(p *GRPCPublisher) {
		if d > 0 {
			p.closeTimeout = d
		}
	}
}

// GRPCPublisher publishes permission mutation events asynchronously to the
// Authorization Service PermissionIngestionService over gRPC, sharding envelopes
// by tenant ID across a pool of background workers to preserve per-tenant FIFO
// ordering while parallelizing delivery and shutdown draining.
type GRPCPublisher struct {
	client       v1.PermissionIngestionServiceClient
	conn         *grpc.ClientConn
	service      string
	logger       logging.LoggerInterface
	recorder     eventRecorder
	rpcTimeout   time.Duration
	closeTimeout time.Duration
	workers      int
	queueSize    int
	queues       []chan queuedEnvelope
	baseCtx      context.Context
	cancel       context.CancelFunc

	mu        sync.RWMutex
	closed    bool
	wg        sync.WaitGroup
	closeOnce sync.Once
	closeErr  error
}

var _ Publisher = (*GRPCPublisher)(nil)

// NewGRPCPublisher initializes a GRPCPublisher connected to address with OAuth2 client-credentials auth.
func NewGRPCPublisher(
	address string,
	creds grpcutil.Credentials,
	tlsEnabled bool,
	rpcTimeout time.Duration,
	workers int,
	service string,
	logger logging.LoggerInterface,
	monitor monitoring.MonitorInterface,
	extraOpts ...grpc.DialOption,
) (*GRPCPublisher, error) {
	if workers < 0 {
		return nil, errors.New("gRPC worker count cannot be negative")
	}

	baseCtx, cancel := context.WithCancel(context.Background())
	conn, err := grpcutil.Dial(baseCtx, address, creds, tlsEnabled, grpcRetryServiceConfig, extraOpts...)
	if err != nil {
		cancel()
		return nil, err
	}

	return newGRPCPublisher(
		baseCtx,
		cancel,
		v1.NewPermissionIngestionServiceClient(conn),
		conn,
		rpcTimeout,
		workers,
		service,
		logger,
		monitor,
	), nil
}

// NewGRPCPublisherWithClient initializes a GRPCPublisher with a custom gRPC client (useful for testing).
func NewGRPCPublisherWithClient(
	client v1.PermissionIngestionServiceClient,
	rpcTimeout time.Duration,
	service string,
	logger logging.LoggerInterface,
	monitor monitoring.MonitorInterface,
	opts ...GRPCPublisherOption,
) *GRPCPublisher {
	baseCtx, cancel := context.WithCancel(context.Background())
	return newGRPCPublisher(baseCtx, cancel, client, nil, rpcTimeout, defaultGRPCWorkers, service, logger, monitor, opts...)
}

func newGRPCPublisher(
	baseCtx context.Context,
	cancel context.CancelFunc,
	client v1.PermissionIngestionServiceClient,
	conn *grpc.ClientConn,
	rpcTimeout time.Duration,
	workers int,
	service string,
	logger logging.LoggerInterface,
	monitor monitoring.MonitorInterface,
	opts ...GRPCPublisherOption,
) *GRPCPublisher {
	if service == "" {
		service = defaultServiceName
	}
	if rpcTimeout <= 0 {
		rpcTimeout = defaultGRPCRPCTimeout
	}
	if workers <= 0 {
		workers = defaultGRPCWorkers
	}
	if logger == nil {
		logger = logging.NewNoopLogger()
	}
	if monitor == nil {
		monitor = monitoring.NewNoopMonitor(service, logger)
	}

	p := &GRPCPublisher{
		client:       client,
		conn:         conn,
		service:      service,
		logger:       logger,
		recorder:     eventRecorder{monitor: monitor, logger: logger},
		rpcTimeout:   rpcTimeout,
		closeTimeout: defaultGRPCCloseTimeout,
		workers:      workers,
		queueSize:    defaultGRPCQueueSize,
		baseCtx:      baseCtx,
		cancel:       cancel,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(p)
		}
	}
	p.queues = make([]chan queuedEnvelope, p.workers)
	for i := range p.queues {
		ch := make(chan queuedEnvelope, p.queueSize)
		p.queues[i] = ch
		p.wg.Add(1)
		go p.runWorker(ch)
	}

	return p
}

// shardIndex maps a tenantID deterministically to a worker shard in [0, shards)
// using 32-bit FNV-1a so all events for a tenant preserve FIFO ordering.
func shardIndex(tenantID string, shards int) int {
	if shards <= 1 {
		return 0
	}
	const (
		offset32 = 2166136261
		prime32  = 16777619
	)
	var h uint32 = offset32
	for i := range len(tenantID) {
		h ^= uint32(tenantID[i])
		h *= prime32
	}
	return int(h % uint32(shards))
}

// Publish splits ops into PermissionUpdateEnvelopes of at most MaxOperationsPerEnvelope
// operations and enqueues them non-blockingly onto the tenant's worker shard for
// asynchronous gRPC delivery.
func (p *GRPCPublisher) Publish(ctx context.Context, tenantID string, ops ...*v1.PermissionOperation) {
	if p == nil || p.client == nil || len(p.queues) == 0 || len(ops) == 0 {
		return
	}

	envelopes := buildEnvelopes(ctx, p.service, ops...)
	itemCtx := context.Background()
	if ctx != nil {
		itemCtx = trace.ContextWithSpanContext(context.Background(), trace.SpanContextFromContext(ctx))
	}

	p.mu.RLock()
	defer p.mu.RUnlock()

	if p.closed {
		p.logger.Errorw("failed to enqueue permission update: publisher is closed",
			"tenant_id", tenantID,
			"envelopes_count", len(envelopes),
		)
		p.recorder.record(resultFailure, stageWrite, len(envelopes))
		return
	}

	queue := p.queues[shardIndex(tenantID, len(p.queues))]
	for i, env := range envelopes {
		select {
		case queue <- queuedEnvelope{ctx: itemCtx, tenantID: tenantID, envelope: env}:
		default:
			remaining := len(envelopes) - i
			p.logger.Errorw("failed to enqueue permission update: queue full",
				"tenant_id", tenantID,
				"envelopes_count", remaining,
			)
			p.recorder.record(resultFailure, stageWrite, remaining)
			return
		}
	}
}

func (p *GRPCPublisher) runWorker(queue <-chan queuedEnvelope) {
	defer p.wg.Done()
	for item := range queue {
		p.sendEnvelope(item)
	}
}

func (p *GRPCPublisher) sendEnvelope(item queuedEnvelope) {
	if p.baseCtx != nil {
		if err := p.baseCtx.Err(); err != nil {
			p.logger.Errorw("failed to publish permission update to authorization service: shutdown deadline exceeded",
				"tenant_id", item.tenantID,
				"message_id", item.envelope.GetMessageId(),
				"error", err,
			)
			p.recorder.record(resultFailure, stageDelivery, 1)
			return
		}
	}

	rpcCtx, cancel := context.WithTimeout(item.ctx, p.rpcTimeout)
	defer cancel()
	if p.baseCtx != nil {
		stopAfter := context.AfterFunc(p.baseCtx, cancel)
		defer stopAfter()
	}

	if _, err := p.client.IngestPermissionUpdate(rpcCtx, item.envelope); err != nil {
		p.logger.Errorw("failed to publish permission update to authorization service",
			"tenant_id", item.tenantID,
			"message_id", item.envelope.GetMessageId(),
			"error", err,
		)
		p.recorder.record(resultFailure, stageDelivery, 1)
		return
	}

	p.recorder.record(resultSuccess, stageDelivery, 1)
}

// Close marks the publisher closed, drains queued envelopes across all workers up to a
// bounded deadline, cancels any remaining in-flight RPCs if the deadline expires, and
// closes the gRPC connection. Close is safe to call multiple times.
func (p *GRPCPublisher) Close() error {
	if p == nil {
		return nil
	}

	p.closeOnce.Do(func() {
		p.mu.Lock()
		p.closed = true
		for _, q := range p.queues {
			if q != nil {
				close(q)
			}
		}
		p.mu.Unlock()

		done := make(chan struct{})
		go func() {
			p.wg.Wait()
			close(done)
		}()

		closeTimeout := p.closeTimeout
		if closeTimeout <= 0 {
			closeTimeout = defaultGRPCCloseTimeout
		}
		timer := time.NewTimer(closeTimeout)
		defer timer.Stop()

		select {
		case <-done:
			if p.cancel != nil {
				p.cancel()
			}
		case <-timer.C:
			if p.logger != nil {
				p.logger.Warnw("timed out draining permission update queue; cancelling remaining RPCs",
					"timeout", closeTimeout,
				)
			}
			if p.cancel != nil {
				p.cancel()
			}
			<-done
		}

		if p.conn != nil {
			p.closeErr = p.conn.Close()
		}
	})

	return p.closeErr
}
