// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package cmd

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"buf.build/go/protovalidate"
	v0 "github.com/canonical/identity-platform-api/v0/tenant"
	"github.com/canonical/tenant-service/internal/config"
	"github.com/canonical/tenant-service/internal/db"
	"github.com/canonical/tenant-service/internal/grpcutil"
	"github.com/canonical/tenant-service/internal/kratos"
	"github.com/canonical/tenant-service/internal/logging"
	"github.com/canonical/tenant-service/internal/monitoring/prometheus"
	"github.com/canonical/tenant-service/internal/permissions"
	"github.com/canonical/tenant-service/internal/storage"
	"github.com/canonical/tenant-service/internal/tracing"
	"github.com/canonical/tenant-service/pkg/authentication"
	"github.com/canonical/tenant-service/pkg/tenant"
	"github.com/canonical/tenant-service/pkg/web"
	grpc_prometheus "github.com/grpc-ecosystem/go-grpc-prometheus"
	"github.com/kelseyhightower/envconfig"
	"github.com/spf13/cobra"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "serve starts the web server",
	Long:  `Launch the web application, list of environment variables is available in the readme`,
	Run: func(cmd *cobra.Command, args []string) {
		main()
	},
}

func init() {
	rootCmd.AddCommand(serveCmd)
}

func serve() error {
	specs := new(config.EnvSpec)
	if err := envconfig.Process("", specs); err != nil {
		panic(fmt.Errorf("issues with environment sourcing: %s", err))
	}

	logger := logging.NewLogger(specs.LogLevel)
	logger.Debugf("env vars: port=%d, grpc_port=%d, log_level=%s, debug=%v, tracing_enabled=%v, permissions_publishing_mode=%s, authentication_enabled=%v",
		specs.Port, specs.GRPCPort, specs.LogLevel, specs.Debug, specs.TracingEnabled, specs.PermissionsPublishingMode, specs.AuthenticationEnabled)
	defer logger.Sync()

	monitor := prometheus.NewMonitor("tenant-service", logger)
	tracer := tracing.NewTracer(tracing.NewConfig(specs.TracingEnabled, specs.OtelGRPCEndpoint, specs.OtelHTTPEndpoint, logger))

	dbConfig := db.Config{
		DSN:             specs.DSN,
		MaxConns:        specs.DBMaxConns,
		MinConns:        specs.DBMinConns,
		MaxConnLifetime: specs.DBMaxConnLifetime,
		MaxConnIdleTime: specs.DBMaxConnIdleTime,
		TracingEnabled:  specs.TracingEnabled,
	}
	dbClient, err := db.NewDBClient(dbConfig, tracer, monitor, logger)
	if err != nil {
		return fmt.Errorf("failed to create database client: %v", err)
	}
	defer dbClient.Close()
	s := storage.NewStorage(dbClient, tracer, monitor, logger)

	publisher, err := permissions.NewPublisher(specs.PermissionsConfig(), logger, monitor)
	if err != nil {
		return err
	}
	defer func() {
		if err := publisher.Close(); err != nil {
			logger.Warnw("failed to close permissions publisher", "error", err)
		}
	}()

	var jwtVerifier authentication.TokenVerifierInterface
	if specs.AuthenticationEnabled {
		// Parse allowed subjects from comma-separated string
		var allowedSubjects []string
		if specs.AuthenticationAllowedSubjects != "" {
			subjects := strings.Split(specs.AuthenticationAllowedSubjects, ",")
			for _, s := range subjects {
				trimmed := strings.TrimSpace(s)
				if trimmed != "" {
					allowedSubjects = append(allowedSubjects, trimmed)
				}
			}
		}

		var err error
		jwtVerifier, err = authentication.NewJWTAuthenticator(
			context.Background(),
			specs.AuthenticationIssuer,
			specs.AuthenticationJwksURL,
			allowedSubjects,
			specs.AuthenticationRequiredScope,
			tracer,
			monitor,
			logger,
		)
		if err != nil {
			return fmt.Errorf("failed to setup JWT authenticator: %v", err)
		}
	} else {
		logger.Warn("WARNING: JWT authentication is DISABLED — this should only be used in development/debug mode")
		jwtVerifier = authentication.NewNoopVerifier()
	}

	kratosClient := kratos.NewClient(
		specs.KratosAdminURL,
		tracer,
		monitor,
		logger,
	)

	tenantService := tenant.NewService(
		s,
		publisher,
		kratosClient,
		specs.InvitationLifetime,
		tracer,
		monitor,
		logger,
	)

	authMiddleware := authentication.NewMiddleware(jwtVerifier, tracer, monitor, logger)

	validator, err := protovalidate.New()
	if err != nil {
		return fmt.Errorf("failed to create request validator: %w", err)
	}

	tenantHandler := tenant.NewHandler(tenantService, validator, tracer, monitor, logger)

	// Start gRPC server
	lis, err := net.Listen("tcp", fmt.Sprintf("0.0.0.0:%v", specs.GRPCPort))
	if err != nil {
		logger.Fatalf("failed to listen on grpc port: %v", err)
	}

	readOnlyMethods, err := grpcutil.ReadOnlyMethods(v0.TenantService_ServiceDesc)
	if err != nil {
		logger.Fatalf("failed to resolve read-only gRPC methods: %v", err)
	}

	grpc_prometheus.EnableHandlingTimeHistogram()

	grpcServer := grpc.NewServer(
		grpc.StatsHandler(otelgrpc.NewServerHandler()),
		grpc.ChainUnaryInterceptor(
			logging.LoggingUnaryInterceptor(logger),
			grpc_prometheus.UnaryServerInterceptor,
			authMiddleware.GRPCInterceptorExcluding(
				"/identity.platform.api.tenant.TenantService/LookupTenants",
			),
			db.TransactionUnaryInterceptor(dbClient, readOnlyMethods, logger),
		),
		grpc.ChainStreamInterceptor(
			logging.LoggingStreamInterceptor(logger),
			grpc_prometheus.StreamServerInterceptor,
			authMiddleware.GRPCStreamInterceptor,
		),
	)
	v0.RegisterTenantServiceServer(grpcServer, tenantHandler)
	grpc_prometheus.Register(grpcServer)

	go func() {
		logger.Infof("Starting gRPC server on port %v", specs.GRPCPort)
		if err := grpcServer.Serve(lis); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			logger.Fatalf("failed to serve gRPC: %v", err)
		}
	}()

	router := web.NewRouter(
		specs.WebhooksAPIToken,
		tenantHandler,
		authMiddleware,
		s,
		dbClient,
		publisher,
		tracer,
		monitor,
		logger,
	)
	logger.Infof("Starting HTTP server on port %v", specs.Port)

	srv := &http.Server{
		Addr:         fmt.Sprintf("0.0.0.0:%v", specs.Port),
		WriteTimeout: time.Second * 60,
		ReadTimeout:  time.Second * 15,
		IdleTimeout:  time.Second * 60,
		Handler:      router,
	}

	var serverError error
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	go func() {
		logger.Security().SystemStartup()
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverError = fmt.Errorf("server error: %w", err)
			c <- os.Interrupt
		}
	}()

	<-c

	// Create a deadline to wait for.
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	logger.Security().SystemShutdown()

	var shutdownErrMu sync.Mutex
	var shutdownWg sync.WaitGroup
	shutdownWg.Add(2)

	go func() {
		defer shutdownWg.Done()
		if err := srv.Shutdown(ctx); err != nil {
			shutdownErrMu.Lock()
			serverError = fmt.Errorf("server shutdown error: %w", err)
			shutdownErrMu.Unlock()
		}
	}()

	go func() {
		defer shutdownWg.Done()
		grpcStopped := make(chan struct{})
		go func() {
			grpcServer.GracefulStop()
			close(grpcStopped)
		}()
		select {
		case <-grpcStopped:
		case <-ctx.Done():
			logger.Warn("gRPC server graceful stop timed out, forcing stop")
			grpcServer.Stop()
			<-grpcStopped
		}
	}()

	shutdownWg.Wait()

	return serverError
}

func main() {
	if err := serve(); err != nil {
		fmt.Fprintf(os.Stderr, "Fatal error: %v\n", err)
		os.Exit(1)
	}
}
