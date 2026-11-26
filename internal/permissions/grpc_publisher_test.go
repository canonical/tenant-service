// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package permissions

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	v1 "github.com/canonical/authorization-service/api/v1"
	"github.com/canonical/tenant-service/internal/grpcutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

type mockIngestionClient struct {
	mu       sync.Mutex
	calls    []*v1.PermissionUpdateEnvelope
	callFunc func(ctx context.Context, env *v1.PermissionUpdateEnvelope) error
}

func (m *mockIngestionClient) IngestPermissionUpdate(ctx context.Context, in *v1.PermissionUpdateEnvelope, _ ...grpc.CallOption) (*v1.IngestPermissionUpdateResponse, error) {
	if m.callFunc != nil {
		if err := m.callFunc(ctx, in); err != nil {
			return nil, err
		}
	}
	m.mu.Lock()
	m.calls = append(m.calls, in)
	m.mu.Unlock()
	return &v1.IngestPermissionUpdateResponse{}, nil
}

func (m *mockIngestionClient) getCalls() []*v1.PermissionUpdateEnvelope {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]*v1.PermissionUpdateEnvelope, len(m.calls))
	copy(out, m.calls)
	return out
}

type testIngestionServer struct {
	v1.UnimplementedPermissionIngestionServiceServer
	mu          sync.Mutex
	attempts    int
	failBefore  int
	failCode    codes.Code
	authHeaders []string
	envelopes   []*v1.PermissionUpdateEnvelope
}

func (s *testIngestionServer) IngestPermissionUpdate(ctx context.Context, req *v1.PermissionUpdateEnvelope) (*v1.IngestPermissionUpdateResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.attempts++
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		vals := md.Get("authorization")
		if len(vals) > 0 {
			s.authHeaders = append(s.authHeaders, vals[0])
		}
	}

	if s.attempts <= s.failBefore {
		code := s.failCode
		if code == codes.OK {
			code = codes.Unavailable
		}
		return nil, status.Error(code, "temporary outage")
	}

	s.envelopes = append(s.envelopes, req)
	return &v1.IngestPermissionUpdateResponse{}, nil
}

func TestGRPCPublisher_Integration_BearerAndRetries(t *testing.T) {
	for _, code := range []codes.Code{codes.Unavailable, codes.Internal} {
		t.Run(code.String(), func(t *testing.T) {
			tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{
					"access_token": "jwt-access-token",
					"token_type":   "Bearer",
					"expires_in":   3600,
				})
			}))
			defer tokenSrv.Close()

			lis := bufconn.Listen(1024 * 1024)
			ingestionSrv := &testIngestionServer{failBefore: 2, failCode: code}
			grpcSrv := grpc.NewServer()
			v1.RegisterPermissionIngestionServiceServer(grpcSrv, ingestionSrv)
			go func() {
				_ = grpcSrv.Serve(lis)
			}()
			defer grpcSrv.Stop()

			monitor := newRecordingMonitor()
			pub, err := NewGRPCPublisher(
				"passthrough:///bufnet",
				grpcutil.Credentials{
					TokenURL:     tokenSrv.URL,
					ClientID:     "tenant-service",
					ClientSecret: "secret",
					Scopes:       []string{"authorization:write"},
				},
				false,
				5*time.Second,
				0,
				"tenant-service",
				nil,
				monitor,
				grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
					return lis.DialContext(ctx)
				}),
			)
			require.NoError(t, err)

			pub.Publish(context.Background(), "tenant-1", TenantPermissionOp(
				v1.PermissionOp_PERMISSION_OP_WRITE, "u1", RelationCanDelete, "tenant-1",
			))
			require.NoError(t, pub.Close())

			ingestionSrv.mu.Lock()
			defer ingestionSrv.mu.Unlock()
			assert.Equal(t, 3, ingestionSrv.attempts)
			require.Len(t, ingestionSrv.envelopes, 1)
			assert.Equal(t, "tenant-service", ingestionSrv.envelopes[0].Service)
			assert.Equal(t, []string{
				"Bearer jwt-access-token",
				"Bearer jwt-access-token",
				"Bearer jwt-access-token",
			}, ingestionSrv.authHeaders)
			assert.Equal(t, map[string]float64{"success/delivery": 1}, monitor.snapshot())
		})
	}
}

func TestGRPCPublisher_ChunkingAndCorrelationID(t *testing.T) {
	prevTP := otel.GetTracerProvider()
	tp := sdktrace.NewTracerProvider()
	otel.SetTracerProvider(tp)
	defer otel.SetTracerProvider(prevTP)

	client := &mockIngestionClient{}
	monitor := newRecordingMonitor()
	pub := NewGRPCPublisherWithClient(client, 5*time.Second, "", nil, monitor)
	ctx, span := tp.Tracer("test").Start(context.Background(), "test-span")
	defer span.End()
	expectedTraceID := span.SpanContext().TraceID().String()

	ops := buildOps(250, "tenant-big")
	pub.Publish(ctx, "tenant-big", ops...)
	require.NoError(t, pub.Close())

	calls := client.getCalls()
	require.Len(t, calls, 3)
	assert.Len(t, calls[0].Operations, 100)
	assert.Len(t, calls[1].Operations, 100)
	assert.Len(t, calls[2].Operations, 50)

	for _, env := range calls {
		assert.Equal(t, "tenant-service", env.Service)
		require.NotNil(t, env.CorrelationId)
		assert.Equal(t, expectedTraceID, *env.CorrelationId)
	}
	assert.Equal(t, map[string]float64{"success/delivery": 3}, monitor.snapshot())
}

func TestGRPCPublisher_Metrics_DeliveryFailure(t *testing.T) {
	client := &mockIngestionClient{
		callFunc: func(context.Context, *v1.PermissionUpdateEnvelope) error {
			return errors.New("rpc failed")
		},
	}
	monitor := newRecordingMonitor()
	pub := NewGRPCPublisherWithClient(client, 5*time.Second, "tenant-service", nil, monitor)

	pub.Publish(context.Background(), "tenant-err", buildOps(150, "tenant-err")...)
	require.NoError(t, pub.Close())

	assert.Equal(t, map[string]float64{"failure/delivery": 2}, monitor.snapshot())
}

func TestGRPCPublisher_Metrics_QueueFullWriteFailure(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	client := &mockIngestionClient{
		callFunc: func(context.Context, *v1.PermissionUpdateEnvelope) error {
			select {
			case <-started:
			default:
				close(started)
			}
			<-release
			return nil
		},
	}

	monitor := newRecordingMonitor()
	pub := NewGRPCPublisherWithClient(client, 5*time.Second, "tenant-service", nil, monitor, withQueueSize(1))

	op := TenantPermissionOp(v1.PermissionOp_PERMISSION_OP_WRITE, "u1", RelationCanView, "t1")
	// 1st message picked up by worker and blocks.
	pub.Publish(context.Background(), "t1", op)
	<-started
	// 2nd message fills buffer of size 1.
	pub.Publish(context.Background(), "t1", op)
	// 3rd message times out on enqueue.
	pub.Publish(context.Background(), "t1", op)

	close(release)
	require.NoError(t, pub.Close())

	assert.Equal(t, map[string]float64{
		"failure/write":    1,
		"success/delivery": 2,
	}, monitor.snapshot())
}

func TestGRPCPublisher_GracefulDrainOnClose(t *testing.T) {
	gate := make(chan struct{})
	client := &mockIngestionClient{
		callFunc: func(context.Context, *v1.PermissionUpdateEnvelope) error {
			<-gate
			return nil
		},
	}
	pub := NewGRPCPublisherWithClient(client, 5*time.Second, "tenant-service", nil, nil)

	for range 10 {
		pub.Publish(context.Background(), "t1", TenantPermissionOp(
			v1.PermissionOp_PERMISSION_OP_WRITE, "u1", RelationCanView, "t1",
		))
	}

	closeDone := make(chan error, 1)
	go func() {
		closeDone <- pub.Close()
	}()

	close(gate)
	require.NoError(t, <-closeDone)
	assert.Len(t, client.getCalls(), 10)
}

func TestGRPCPublisher_ConcurrentAndPostClosePublish(t *testing.T) {
	client := &mockIngestionClient{}
	monitor := newRecordingMonitor()
	pub := NewGRPCPublisherWithClient(client, 5*time.Second, "tenant-service", nil, monitor)

	op := TenantPermissionOp(v1.PermissionOp_PERMISSION_OP_WRITE, "u1", RelationCanView, "t1")

	var wg sync.WaitGroup
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 20 {
				pub.Publish(context.Background(), "t1", op)
			}
		}()
	}

	time.Sleep(time.Millisecond)
	assert.NotPanics(t, func() {
		require.NoError(t, pub.Close())
	})
	wg.Wait()

	// Post-Close Publish must not panic and must record failure/write.
	before := monitor.snapshot()["failure/write"]
	assert.NotPanics(t, func() {
		pub.Publish(context.Background(), "t1", op)
	})
	assert.Equal(t, before+1, monitor.snapshot()["failure/write"])
}

func TestNewGRPCPublisher_Validation(t *testing.T) {
	validCreds := grpcutil.Credentials{
		TokenURL:     "http://localhost:4444/oauth2/token",
		ClientID:     "cid",
		ClientSecret: "csecret",
	}

	_, err := NewGRPCPublisher("", validCreds, false, 5*time.Second, 5, "tenant-service", nil, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "gRPC address")

	_, err = NewGRPCPublisher("localhost:50052", grpcutil.Credentials{ClientID: "cid", ClientSecret: "s"}, false, 5*time.Second, 5, "tenant-service", nil, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "token URL")

	_, err = NewGRPCPublisher("localhost:50052", validCreds, false, 5*time.Second, -1, "tenant-service", nil, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "worker count")
}

func TestGRPCPublisher_SinglePublishNonBlockingDrop(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	client := &mockIngestionClient{
		callFunc: func(context.Context, *v1.PermissionUpdateEnvelope) error {
			select {
			case <-started:
			default:
				close(started)
			}
			<-release
			return nil
		},
	}

	monitor := newRecordingMonitor()
	pub := NewGRPCPublisherWithClient(client, 5*time.Second, "tenant-service", nil, monitor, withQueueSize(1))

	op := TenantPermissionOp(v1.PermissionOp_PERMISSION_OP_WRITE, "u1", RelationCanView, "t1")
	pub.Publish(context.Background(), "t1", op)
	<-started
	pub.Publish(context.Background(), "t1", op) // fills buffer of size 1

	// Build 500 operations -> 5 envelopes. With non-blocking enqueue, all 5 drop immediately.
	ops := make([]*v1.PermissionOperation, 500)
	for i := range ops {
		ops[i] = op
	}
	pub.Publish(context.Background(), "t1", ops...)

	close(release)
	require.NoError(t, pub.Close())

	assert.Equal(t, map[string]float64{
		"failure/write":    5,
		"success/delivery": 2,
	}, monitor.snapshot())
}

func TestGRPCPublisher_CloseTimeoutCancelsInFlightAndDrainsFast(t *testing.T) {
	started := make(chan struct{})
	client := &mockIngestionClient{
		callFunc: func(ctx context.Context, _ *v1.PermissionUpdateEnvelope) error {
			select {
			case <-started:
			default:
				close(started)
			}
			<-ctx.Done()
			return ctx.Err()
		},
	}

	monitor := newRecordingMonitor()
	pub := NewGRPCPublisherWithClient(client, 5*time.Second, "tenant-service", nil, monitor, withCloseTimeout(40*time.Millisecond))

	op := TenantPermissionOp(v1.PermissionOp_PERMISSION_OP_WRITE, "u1", RelationCanView, "t1")
	pub.Publish(context.Background(), "t1", op)
	<-started
	pub.Publish(context.Background(), "t1", op)
	pub.Publish(context.Background(), "t1", op)

	require.NoError(t, pub.Close())
	assert.Equal(t, map[string]float64{
		"failure/delivery": 3,
	}, monitor.snapshot())
}

func TestGRPCPublisher_CloseIdempotent(t *testing.T) {
	pub := NewGRPCPublisherWithClient(&mockIngestionClient{}, 5*time.Second, "tenant-service", nil, nil)
	require.NoError(t, pub.Close())
	require.NoError(t, pub.Close())
}

func TestGRPCPublisher_Integration_TokenEndpoint5xxRetried(t *testing.T) {
	var tokenAttempts atomic.Int32
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if tokenAttempts.Add(1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "retried-jwt-token",
			"token_type":   "Bearer",
			"expires_in":   3600,
		})
	}))
	defer tokenSrv.Close()

	lis := bufconn.Listen(1024 * 1024)
	ingestionSrv := &testIngestionServer{}
	grpcSrv := grpc.NewServer()
	v1.RegisterPermissionIngestionServiceServer(grpcSrv, ingestionSrv)
	go func() {
		_ = grpcSrv.Serve(lis)
	}()
	defer grpcSrv.Stop()

	monitor := newRecordingMonitor()
	pub, err := NewGRPCPublisher(
		"passthrough:///bufnet",
		grpcutil.Credentials{
			TokenURL:     tokenSrv.URL,
			ClientID:     "cid",
			ClientSecret: "csecret",
		},
		false,
		5*time.Second,
		0,
		"tenant-service",
		nil,
		monitor,
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return lis.Dial()
		}),
	)
	require.NoError(t, err)

	pub.Publish(context.Background(), "t1", TenantPermissionOp(
		v1.PermissionOp_PERMISSION_OP_WRITE, "u1", RelationCanView, "t1",
	))
	require.NoError(t, pub.Close())

	assert.Equal(t, int32(2), tokenAttempts.Load())
	require.Len(t, ingestionSrv.envelopes, 1)
	assert.Equal(t, []string{"Bearer retried-jwt-token"}, ingestionSrv.authHeaders)
	assert.Equal(t, map[string]float64{
		"success/delivery": 1,
	}, monitor.snapshot())
}

func TestGRPCPublisher_TenantShardedWorkers_ParallelismAndPerTenantFIFO(t *testing.T) {
	const workers = 5

	// Pick one tenant ID landing on each of the 5 worker shards so we verify
	// all 5 default workers execute concurrently without head-of-line blocking.
	tenantsByShard := make([]string, workers)
	found := 0
	for i := 0; found < workers; i++ {
		candidate := "tenant-" + string(rune('a'+i/10)) + string(rune('0'+i%10))
		shard := shardIndex(candidate, workers)
		if tenantsByShard[shard] == "" {
			tenantsByShard[shard] = candidate
			found++
		}
	}

	allInFlight := make(chan struct{})
	release := make(chan struct{})
	var inFlight atomic.Int32

	client := &mockIngestionClient{
		callFunc: func(_ context.Context, _ *v1.PermissionUpdateEnvelope) error {
			if inFlight.Add(1) == workers {
				close(allInFlight)
			}
			<-release
			return nil
		},
	}

	pub := NewGRPCPublisherWithClient(client, 5*time.Second, "tenant-service", nil, nil)
	assert.Equal(t, defaultGRPCWorkers, pub.workers)

	customPub := NewGRPCPublisherWithClient(client, 5*time.Second, "tenant-service", nil, nil, withWorkers(3))
	assert.Equal(t, 3, customPub.workers)
	require.NoError(t, customPub.Close())

	// Enqueue 3 sequential operations per shard-distinct tenant; the first op on
	// each shard runs concurrently while the remaining 2 stay ordered behind it.
	relations := []string{RelationCanView, RelationCanEdit, RelationCanDelete}
	for _, tenantID := range tenantsByShard {
		for _, rel := range relations {
			pub.Publish(context.Background(), tenantID, TenantPermissionOp(
				v1.PermissionOp_PERMISSION_OP_WRITE, "u1", rel, tenantID,
			))
		}
	}

	select {
	case <-allInFlight:
	case <-time.After(2 * time.Second):
		t.Fatalf("expected all %d workers to run concurrently, only %d reached RPC", workers, inFlight.Load())
	}

	close(release)
	require.NoError(t, pub.Close())

	calls := client.getCalls()
	require.Len(t, calls, workers*len(relations))

	perTenantRelations := make(map[string][]string, workers)
	for _, env := range calls {
		require.Len(t, env.Operations, 1)
		op := env.Operations[0]
		perTenantRelations[op.Object] = append(perTenantRelations[op.Object], op.Relation)
	}
	for _, tenantID := range tenantsByShard {
		assert.Equal(t, relations, perTenantRelations["tenant:"+tenantID], "per-tenant FIFO order must hold for %s", tenantID)
	}
}

func TestGRPCPublisher_ZeroValueSafe(t *testing.T) {
	var zero GRPCPublisher
	assert.NotPanics(t, func() {
		zero.Publish(context.Background(), "t1", TenantPermissionOp(
			v1.PermissionOp_PERMISSION_OP_WRITE, "u1", RelationCanView, "t1",
		))
		require.NoError(t, zero.Close())
		require.NoError(t, zero.Close())
	})
}
