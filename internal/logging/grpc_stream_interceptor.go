// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package logging

import (
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/status"
)

// LoggingStreamInterceptor returns a gRPC stream server interceptor that logs
// each streaming RPC with the method name, status code, and duration.
// Server-side error codes are logged at Error level; all others at Debug level.
func LoggingStreamInterceptor(logger LoggerInterface) grpc.StreamServerInterceptor {
	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		startTime := time.Now()
		err := handler(srv, ss)
		elapsed := time.Since(startTime)

		code := status.Code(err)
		if isServerErrorCode(code) {
			logger.Errorf("gRPC stream %s %s in %s", info.FullMethod, code, elapsed)
		} else {
			logger.Debugf("gRPC stream %s %s in %s", info.FullMethod, code, elapsed)
		}

		return err
	}
}
