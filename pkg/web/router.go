// Copyright 2025 Canonical Ltd
// SPDX-License-Identifier: AGPL-3.0

package web

import (
	"net/http"

	"github.com/canonical/tenant-service/internal/authorization"
	"github.com/canonical/tenant-service/internal/db"
	"github.com/canonical/tenant-service/internal/http/types"
	"github.com/canonical/tenant-service/internal/logging"
	"github.com/canonical/tenant-service/internal/monitoring"
	"github.com/canonical/tenant-service/internal/storage"
	"github.com/canonical/tenant-service/internal/tracing"
	"github.com/canonical/tenant-service/pkg/metrics"
	"github.com/canonical/tenant-service/pkg/status"
	"github.com/canonical/tenant-service/pkg/webhooks"
	chi "github.com/go-chi/chi/v5"
	middleware "github.com/go-chi/chi/v5/middleware"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"google.golang.org/protobuf/encoding/protojson"
)

func NewRouter(
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
				UseProtoNames: true,
			},
		}),
	)

	router.Use(middlewares...)

	metrics.NewAPI(logger).RegisterEndpoints(router)
	status.NewAPI(tracer, monitor, logger).RegisterEndpoints(router)
	webhooks.NewAPI(webhooks.NewService(s, authz, tracer, monitor, logger)).RegisterEndpoints(router)

	router.Mount("/", gRPCGatewayMux)

	return tracing.NewMiddleware(monitor, logger).OpenTelemetry(router)
}
