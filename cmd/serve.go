// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0

package cmd

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/canonical/tenant-service/internal/authorization"
	"github.com/canonical/tenant-service/internal/config"
	"github.com/canonical/tenant-service/internal/db"
	"github.com/canonical/tenant-service/internal/identity"
	"github.com/canonical/tenant-service/internal/kratos"
	"github.com/canonical/tenant-service/internal/logging"
	"github.com/canonical/tenant-service/internal/monitoring/prometheus"
	"github.com/canonical/tenant-service/internal/openfga"
	"github.com/canonical/tenant-service/internal/storage"
	"github.com/canonical/tenant-service/internal/tracing"
	"github.com/canonical/tenant-service/pkg/tenant"
	"github.com/canonical/tenant-service/pkg/web"
	v0 "github.com/canonical/tenant-service/v0"
	"github.com/kelseyhightower/envconfig"
	"github.com/spf13/cobra"
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
	logger.Debugf("env vars: %v", specs)
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

	var authorizer *authorization.Authorizer
	if specs.AuthorizationEnabled {
		ofga := openfga.NewClient(
			openfga.NewConfig(
				specs.OpenfgaApiScheme,
				specs.OpenfgaApiHost,
				specs.OpenfgaStoreId,
				specs.OpenfgaApiToken,
				specs.OpenfgaModelId,
				specs.Debug,
				tracer,
				monitor,
				logger,
			),
		)
		authorizer = authorization.NewAuthorizer(
			ofga,
			tracer,
			monitor,
			logger,
		)
		logger.Info("Authorization is enabled")
		if authorizer.ValidateModel(context.Background()) != nil {
			panic("Invalid authorization model provided")
		}
	} else {
		authorizer = authorization.NewAuthorizer(
			openfga.NewNoopClient(tracer, monitor, logger),
			tracer,
			monitor,
			logger,
		)
		logger.Info("Using noop authorizer")
	}

	kratosClient := kratos.NewClient(
		specs.KratosAdminURL,
		tracer,
		monitor,
		logger,
	)

	tenantService := tenant.NewService(
		s,
		authorizer,
		kratosClient,
		specs.InvitationLifetime,
		tracer,
		monitor,
		logger,
	)

	identityMiddleware := identity.NewMiddleware(tracer, monitor, logger)
	tenantHandler := tenant.NewHandler(tenantService, tracer, monitor, logger)

	// Start gRPC server
	lis, err := net.Listen("tcp", fmt.Sprintf("0.0.0.0:%v", specs.GRPCPort))
	if err != nil {
		logger.Fatalf("failed to listen on grpc port: %v", err)
	}

	grpcServer := grpc.NewServer(
		grpc.UnaryInterceptor(identityMiddleware.GRPCInterceptor),
	)
	v0.RegisterTenantServiceServer(grpcServer, tenantHandler)

	go func() {
		logger.Infof("Starting gRPC server on port %v", specs.GRPCPort)
		if err := grpcServer.Serve(lis); err != nil {
			logger.Fatalf("failed to serve gRPC: %v", err)
		}
	}()

	router := web.NewRouter(
		tenantHandler,
		identityMiddleware,
		s,
		dbClient,
		authorizer,
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
	if err := srv.Shutdown(ctx); err != nil {
		serverError = fmt.Errorf("server shutdown error: %w", err)
	}

	return serverError
}

func main() {
	if err := serve(); err != nil {
		fmt.Fprintf(os.Stderr, "Fatal error: %v\n", err)
		os.Exit(1)
	}
}
