// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package db

import (
	"context"

	"github.com/canonical/tenant-service/internal/logging"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// TransactionUnaryInterceptor returns a gRPC unary server interceptor that wraps
// mutating RPC handlers in a database transaction. Read-only methods (identified by
// the readOnlyMethods set) are called directly without transaction wrapping.
func TransactionUnaryInterceptor(db DBClientInterface, readOnlyMethods map[string]bool, logger logging.LoggerInterface) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		if readOnlyMethods[info.FullMethod] {
			return handler(ctx, req)
		}

		var resp interface{}
		var handlerErr error

		txErr := db.WithTx(ctx, func(txCtx context.Context) error {
			resp, handlerErr = handler(txCtx, req)
			if handlerErr != nil {
				return handlerErr
			}
			return nil
		})

		if handlerErr != nil {
			return nil, handlerErr
		}
		if txErr != nil {
			logger.Errorf("transaction failed for %s: %v", info.FullMethod, txErr)
			return nil, status.Errorf(codes.Internal, "transaction failed: %v", txErr)
		}
		return resp, nil
	}
}
