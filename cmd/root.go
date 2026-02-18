// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0

package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

var (
	userID       string
	grpcEndpoint string
	httpEndpoint string
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "app",
	Short: "Tenant Service",
	Long:  `Tenant Service CLI for managing tenants and users.`,
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&grpcEndpoint, "grpc-endpoint", "localhost:50051", "gRPC server endpoint")
	rootCmd.PersistentFlags().StringVar(&httpEndpoint, "http-endpoint", "", "HTTP server endpoint (e.g. http://localhost:8000)")
	rootCmd.PersistentFlags().StringVar(&userID, "user-id", "", "User ID for impersonation")
}
