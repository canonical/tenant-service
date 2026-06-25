// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package logging

import (
	"context"
	"testing"

	"go.uber.org/mock/gomock"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// mockServerStream is a minimal grpc.ServerStream for testing.
type mockServerStream struct {
	grpc.ServerStream
}

func (m *mockServerStream) Context() context.Context { return context.Background() }

func TestLoggingStreamInterceptor(t *testing.T) {
	tests := []struct {
		name       string
		fullMethod string
		handlerErr error
		wantCode   codes.Code
		wantLevel  string // "debug" or "error"
	}{
		{
			name:       "successful stream logs at Debug",
			fullMethod: "/test.Service/StreamItems",
			handlerErr: nil,
			wantCode:   codes.OK,
			wantLevel:  "debug",
		},
		{
			name:       "client error logs at Debug",
			fullMethod: "/test.Service/StreamItems",
			handlerErr: status.Error(codes.NotFound, "not found"),
			wantCode:   codes.NotFound,
			wantLevel:  "debug",
		},
		{
			name:       "Internal error logs at Error",
			fullMethod: "/test.Service/StreamItems",
			handlerErr: status.Error(codes.Internal, "db failure"),
			wantCode:   codes.Internal,
			wantLevel:  "error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			mockLogger := NewMockLoggerInterface(ctrl)

			switch tt.wantLevel {
			case "error":
				mockLogger.EXPECT().Errorf(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(1)
			case "debug":
				mockLogger.EXPECT().Debugf(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(1)
			}

			interceptor := LoggingStreamInterceptor(mockLogger)

			handler := func(srv interface{}, ss grpc.ServerStream) error {
				return tt.handlerErr
			}

			ss := &mockServerStream{}
			info := &grpc.StreamServerInfo{FullMethod: tt.fullMethod}
			err := interceptor(nil, ss, info, handler)

			if tt.handlerErr == nil && err != nil {
				t.Errorf("err = %v, want nil", err)
			}
			if tt.handlerErr != nil && status.Code(err) != tt.wantCode {
				t.Errorf("err code = %v, want %v", status.Code(err), tt.wantCode)
			}
		})
	}
}
