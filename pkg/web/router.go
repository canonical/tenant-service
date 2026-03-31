// Copyright 2025 Canonical Ltd
// SPDX-License-Identifier: AGPL-3.0

package web

import (
	"context"
	"net/http"

	"github.com/canonical/tenant-service/internal/authorization"
	"github.com/canonical/tenant-service/internal/db"
	"github.com/canonical/tenant-service/internal/http/types"
	"github.com/canonical/tenant-service/internal/logging"
	"github.com/canonical/tenant-service/internal/monitoring"
	"github.com/canonical/tenant-service/internal/storage"
	"github.com/canonical/tenant-service/internal/tracing"
	"github.com/canonical/tenant-service/pkg/authentication"
	"github.com/canonical/tenant-service/pkg/metrics"
	"github.com/canonical/tenant-service/pkg/status"
	"github.com/canonical/tenant-service/pkg/tenant"
	"github.com/canonical/tenant-service/pkg/webhooks"
	v0 "github.com/canonical/tenant-service/v0"
	chi "github.com/go-chi/chi/v5"
	middleware "github.com/go-chi/chi/v5/middleware"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"google.golang.org/protobuf/encoding/protojson"
)

func NewRouter(
	tenantHandler *tenant.Handler,
	authMiddleware *authentication.Middleware,
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

	// Unauthenticated tenant lookup — used by the Login UI before the user has a token.
	// See ADR 0008 for security trade-offs. Rate limiting should be enforced at the proxy/gateway layer.

	// Protected routes — lookup is excluded so the Login UI can call it without a Bearer token.
	// We create a separate authRouter to apply the authentication middleware exclusively
	// to the gRPC Gateway endpoints. We then mount this authRouter onto the main router.
	// This ensures that the webhook endpoints (registered above on the main router)
	// completely bypass the authentication middleware.
	authRouter := chi.NewRouter()
	authRouter.Use(authMiddleware.AuthenticateExcluding("/api/v0/tenants/lookup"))
	authRouter.Mount("/", gRPCGatewayMux)

	router.Mount("/", authRouter)

	return tracing.NewMiddleware(monitor, logger).OpenTelemetry(router)
}
