// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0

package cmd

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"

	v0 "github.com/canonical/tenant-service/v0"
	"github.com/spf13/cobra"
)

var tenantCmd = &cobra.Command{
	Use:   "tenant",
	Short: "Manage tenants",
}

var createTenantCmd = &cobra.Command{
	Use:   "create [name]",
	Short: "Create a new tenant",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		conn, client, err := getClient()
		if err != nil {
			return err
		}
		defer conn()

		ctx := getAuthenticatedContext(context.Background())
		resp, err := client.CreateTenant(ctx, &v0.CreateTenantRequest{
			Name: args[0],
		})
		if err != nil {
			return fmt.Errorf("failed to create tenant: %w", err)
		}

		fmt.Printf("Tenant created: %s (ID: %s)\n", resp.Name, resp.Id)
		return nil
	},
}

var deleteTenantCmd = &cobra.Command{
	Use:   "delete [id]",
	Short: "Delete a tenant",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		conn, client, err := getClient()
		if err != nil {
			return err
		}
		defer conn()

		ctx := getAuthenticatedContext(context.Background())
		_, err = client.DeleteTenant(ctx, &v0.DeleteTenantRequest{
			TenantId: args[0],
		})
		if err != nil {
			return fmt.Errorf("failed to delete tenant: %w", err)
		}

		fmt.Printf("Tenant deleted: %s\n", args[0])
		return nil
	},
}

var listTenantsCmd = &cobra.Command{
	Use:   "list",
	Short: "List tenants for the authenticated user",
	RunE: func(cmd *cobra.Command, args []string) error {
		conn, client, err := getClient()
		if err != nil {
			return err
		}
		defer conn()

		ctx := getAuthenticatedContext(context.Background())
		resp, err := client.ListTenants(ctx, &v0.ListTenantsRequest{})
		if err != nil {
			return fmt.Errorf("failed to list tenants: %w", err)
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
		fmt.Fprintln(w, "ID\tNAME\tENABLED\tCREATED_AT")
		for _, t := range resp.Tenants {
			fmt.Fprintf(w, "%s\t%s\t%v\t%s\n", t.Id, t.Name, t.Enabled, t.CreatedAt)
		}
		w.Flush()
		return nil
	},
}

var activateTenantCmd = &cobra.Command{
	Use:   "activate [id]",
	Short: "Activate a tenant",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		conn, client, err := getClient()
		if err != nil {
			return err
		}
		defer conn()

		ctx := getAuthenticatedContext(context.Background())
		_, err = client.ActivateTenant(ctx, &v0.ActivateTenantRequest{
			TenantId: args[0],
		})
		if err != nil {
			return fmt.Errorf("failed to activate tenant: %w", err)
		}

		fmt.Printf("Tenant activated: %s\n", args[0])
		return nil
	},
}

var deactivateTenantCmd = &cobra.Command{
	Use:   "deactivate [id]",
	Short: "Deactivate a tenant",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		conn, client, err := getClient()
		if err != nil {
			return err
		}
		defer conn()

		ctx := getAuthenticatedContext(context.Background())
		_, err = client.DeactivateTenant(ctx, &v0.DeactivateTenantRequest{
			TenantId: args[0],
		})
		if err != nil {
			return fmt.Errorf("failed to deactivate tenant: %w", err)
		}

		fmt.Printf("Tenant deactivated: %s\n", args[0])
		return nil
	},
}

var owners []string

var updateTenantCmd = &cobra.Command{
	Use:   "update [id] [name]",
	Short: "Update a tenant name and optionally its owners",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		conn, client, err := getClient()
		if err != nil {
			return err
		}
		defer conn()

		ctx := getAuthenticatedContext(context.Background())
		_, err = client.UpdateTenant(ctx, &v0.UpdateTenantRequest{
			TenantId: args[0],
			Name:     args[1],
			OwnerIds: owners,
		})
		if err != nil {
			return fmt.Errorf("failed to update tenant: %w", err)
		}

		fmt.Printf("Tenant updated: %s\n", args[0])
		return nil
	},
}

func init() {
	rootCmd.AddCommand(tenantCmd)
	tenantCmd.AddCommand(createTenantCmd)
	tenantCmd.AddCommand(deleteTenantCmd)
	tenantCmd.AddCommand(listTenantsCmd)
	tenantCmd.AddCommand(activateTenantCmd)
	tenantCmd.AddCommand(deactivateTenantCmd)
	tenantCmd.AddCommand(updateTenantCmd)

	updateTenantCmd.Flags().StringSliceVar(&owners, "owners", []string{}, "Comma-separated list of owner IDs")
}
