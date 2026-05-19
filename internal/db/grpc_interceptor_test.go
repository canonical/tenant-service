// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package db

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/mock/gomock"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

//go:generate mockgen -build_flags=--mod=mod -package db -destination ./mock_db_test.go -source=./interfaces.go
//go:generate mockgen -build_flags=--mod=mod -package db -destination ./mock_logger_test.go -source=../logging/interfaces.go

func TestTransactionUnaryInterceptor(t *testing.T) {
	type result struct {
		resp interface{}
		err  error
	}

	tests := []struct {
		name            string
		fullMethod      string
		readOnlyMethods map[string]bool
		setupMock       func(*MockDBClientInterface, *MockLoggerInterface)
		handlerResp     interface{}
		handlerErr      error
		expected        result
	}{
		{
			name:            "read-only method skips transaction",
			fullMethod:      "/test.Service/ListItems",
			readOnlyMethods: map[string]bool{"/test.Service/ListItems": true},
			setupMock:       func(m *MockDBClientInterface, l *MockLoggerInterface) {},
			handlerResp:     "list-response",
			handlerErr:      nil,
			expected:        result{resp: "list-response", err: nil},
		},
		{
			name:            "mutating method commits on success",
			fullMethod:      "/test.Service/CreateItem",
			readOnlyMethods: map[string]bool{"/test.Service/ListItems": true},
			setupMock: func(m *MockDBClientInterface, l *MockLoggerInterface) {
				m.EXPECT().WithTx(gomock.Any(), gomock.Any()).DoAndReturn(
					func(ctx context.Context, fn func(context.Context) error) error {
						return fn(ctx)
					},
				)
			},
			handlerResp: "create-response",
			handlerErr:  nil,
			expected:    result{resp: "create-response", err: nil},
		},
		{
			name:            "mutating method rolls back on handler error",
			fullMethod:      "/test.Service/CreateItem",
			readOnlyMethods: map[string]bool{},
			setupMock: func(m *MockDBClientInterface, l *MockLoggerInterface) {
				m.EXPECT().WithTx(gomock.Any(), gomock.Any()).DoAndReturn(
					func(ctx context.Context, fn func(context.Context) error) error {
						return fn(ctx)
					},
				)
			},
			handlerResp: nil,
			handlerErr:  status.Error(codes.InvalidArgument, "bad request"),
			expected:    result{resp: nil, err: status.Error(codes.InvalidArgument, "bad request")},
		},
		{
			name:            "handler error is returned without partial response",
			fullMethod:      "/test.Service/DeleteItem",
			readOnlyMethods: map[string]bool{},
			setupMock: func(m *MockDBClientInterface, l *MockLoggerInterface) {
				m.EXPECT().WithTx(gomock.Any(), gomock.Any()).DoAndReturn(
					func(ctx context.Context, fn func(context.Context) error) error {
						return fn(ctx)
					},
				)
			},
			handlerResp: "partial",
			handlerErr:  status.Error(codes.NotFound, "not found"),
			expected:    result{resp: nil, err: status.Error(codes.NotFound, "not found")},
		},
		{
			name:            "commit failure logs error and returns Internal",
			fullMethod:      "/test.Service/CreateItem",
			readOnlyMethods: map[string]bool{},
			setupMock: func(m *MockDBClientInterface, l *MockLoggerInterface) {
				m.EXPECT().WithTx(gomock.Any(), gomock.Any()).DoAndReturn(
					func(ctx context.Context, fn func(context.Context) error) error {
						if err := fn(ctx); err != nil {
							return err
						}
						return errors.New("commit failed")
					},
				)
				l.EXPECT().Errorf(gomock.Any(), gomock.Any(), gomock.Any())
			},
			handlerResp: "response",
			handlerErr:  nil,
			expected:    result{resp: nil, err: status.Error(codes.Internal, "transaction failed: commit failed")},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)

			mockDB := NewMockDBClientInterface(ctrl)
			mockLogger := NewMockLoggerInterface(ctrl)

			tt.setupMock(mockDB, mockLogger)

			interceptor := TransactionUnaryInterceptor(mockDB, tt.readOnlyMethods, mockLogger)

			handler := func(ctx context.Context, req interface{}) (interface{}, error) {
				return tt.handlerResp, tt.handlerErr
			}

			info := &grpc.UnaryServerInfo{FullMethod: tt.fullMethod}
			resp, err := interceptor(context.Background(), "request", info, handler)

			if resp != tt.expected.resp {
				t.Errorf("resp = %v, want %v", resp, tt.expected.resp)
			}
			if tt.expected.err == nil && err != nil {
				t.Errorf("err = %v, want nil", err)
			}
			if tt.expected.err != nil {
				if err == nil {
					t.Errorf("err = nil, want %v", tt.expected.err)
				} else if status.Code(err) != status.Code(tt.expected.err) {
					t.Errorf("err code = %v, want %v", status.Code(err), status.Code(tt.expected.err))
				}
			}
		})
	}
}
