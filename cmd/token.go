// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0

package cmd

import (
	"context"
	"fmt"
	"log"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/spf13/cobra"
	"golang.org/x/oauth2/clientcredentials"
)

var (
	clientID     string
	clientSecret string
	tokenURL     string
	issuerURL    string
	scopes       []string
)

var tokenCmd = &cobra.Command{
	Use:   "token",
	Short: "Get an access token using Client Credentials flow",
	Run: func(cmd *cobra.Command, args []string) {
		ctx := context.Background()

		if tokenURL == "" {
			if issuerURL == "" {
				log.Fatal("Either --token-url or --issuer-url must be provided")
			}

			// Discovery endpoint
			provider, err := oidc.NewProvider(ctx, issuerURL)
			if err != nil {
				log.Fatalf("Failed to create OIDC provider from issuer: %v", err)
			}
			tokenURL = provider.Endpoint().TokenURL
		}

		config := &clientcredentials.Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			TokenURL:     tokenURL,
			Scopes:       scopes,
		}

		token, err := config.Token(ctx)
		if err != nil {
			log.Fatalf("Failed to get token: %v", err)
		}

		fmt.Println(token.AccessToken)
	},
}

func init() {
	rootCmd.AddCommand(tokenCmd)

	tokenCmd.Flags().StringVar(&clientID, "client-id", "", "Client ID")
	tokenCmd.Flags().StringVar(&clientSecret, "client-secret", "", "Client Secret")
	tokenCmd.Flags().StringVar(&tokenURL, "token-url", "", "Token URL")
	tokenCmd.Flags().StringVar(&issuerURL, "issuer-url", "", "Issuer URL (for OIDC discovery)")
	tokenCmd.Flags().StringSliceVar(&scopes, "scopes", []string{}, "Scopes (comma-separated)")

	_ = tokenCmd.MarkFlagRequired("client-id")
	_ = tokenCmd.MarkFlagRequired("client-secret")
}
