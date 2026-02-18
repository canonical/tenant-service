// Copyright 2025 Canonical Ltd
// SPDX-License-Identifier: AGPL-3.0

package web

import (
	"context"
	"net/http"

	"github.com/canonical/tenant-service/internal/authorization"
	"github.com/canonical/tenant-service/internal/db"
	"github.com/canonical/tenant-service/internal/http/types"
	"github.com/canonical/tenant-service/internal/identity"
	"github.com/canonical/tenant-service/internal/logging"
	"github.com/canonical/tenant-service/internal/monitoring"
	"github.com/canonical/tenant-service/internal/storage"
	"github.com/canonical/tenant-service/internal/tracing"
	"github.com/canonical/tenant-service/pkg/metrics"
	"github.com/canonical/tenant-service/pkg/status"
	"github.com/canonical/tenant-service/pkg/webhooks"
	v0 "github.com/canonical/tenant-service/v0"
	chi "github.com/go-chi/chi/v5"
	middleware "github.com/go-chi/chi/v5/middleware"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"google.golang.org/protobuf/encoding/protojson"
)

func NewRouter(
	tenantHandler v0.TenantServiceServer,
	identityMiddleware *identity.Middleware,
	s storage.StorageInterface,
	dbClient db.DBClientInterface,
	authz authorization.AuthorizerInterface,
	tracer tracing.TracingInterface,
	monitor monitoring.MonitorInterface,
	logger logging.LoggerInterface,
) http.Handler {
	router := chi.NewMux()

	middlewares := make(chi.Middlewares, 0)
	middlewares = append(
		middlewares,
		middleware.RequestID,
		monitoring.NewMiddleware(monitor, logger).ResponseTime(),
		middlewareCORS([]string{"*"}),
		middleware.RequestLogger(logging.NewLogFormatter(logger)),
	)

	if dbClient != nil {
		middlewares = append(middlewares, db.TransactionMiddleware(dbClient, logger))
	}

	gRPCGatewayMux := runtime.NewServeMux(
		runtime.WithForwardResponseRewriter(types.ForwardErrorResponseRewriter),
		runtime.WithDisablePathLengthFallback(),
		// Use proto field names (snake_case) in JSON output instead of lowerCamelCase.
		runtime.WithMarshalerOption(runtime.MIMEWildcard, &runtime.JSONPb{
			MarshalOptions: protojson.MarshalOptions{
				UseProtoNames:   true,
				EmitUnpopulated: true,
			},
		}),
	)
	_ = v0.RegisterTenantServiceHandlerServer(context.Background(), gRPCGatewayMux, tenantHandler)

	router.Use(middlewares...)

	metrics.NewAPI(logger).RegisterEndpoints(router)
	status.NewAPI(tracer, monitor, logger).RegisterEndpoints(router)
	webhooks.NewAPI(webhooks.NewService(s, authz, tracer, monitor, logger), logger).RegisterEndpoints(router)

	// Protected routes
	authRouter := chi.NewRouter()
	authRouter.Use(identityMiddleware.HTTPMiddleware)
	authRouter.Mount("/", gRPCGatewayMux)

	router.Mount("/", authRouter)

	return tracing.NewMiddleware(monitor, logger).OpenTelemetry(router)
}
