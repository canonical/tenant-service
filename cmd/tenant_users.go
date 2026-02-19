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

var usersCmd = &cobra.Command{
	Use:   "users",
	Short: "Manage tenant users",
}

var listUsersCmd = &cobra.Command{
	Use:   "list [tenant-id]",
	Short: "List users for a tenant",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		conn, client, err := getClient()
		if err != nil {
			return err
		}
		defer conn()

		ctx := getAuthenticatedContext(context.Background())
		resp, err := client.ListTenantUsers(ctx, &v0.ListTenantUsersRequest{
			TenantId: args[0],
		})
		if err != nil {
			return fmt.Errorf("failed to list users: %w", err)
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
		fmt.Fprintln(w, "USER_ID\tEMAIL\tROLE")
		for _, u := range resp.Users {
			fmt.Fprintf(w, "%s\t%s\t%s\n", u.UserId, u.Email, u.Role)
		}
		w.Flush()
		return nil
	},
}

var inviteUserCmd = &cobra.Command{
	Use:   "invite [tenant-id] [email] [role]",
	Short: "Invite a user to a tenant",
	Args:  cobra.ExactArgs(3),
	RunE: func(cmd *cobra.Command, args []string) error {
		conn, client, err := getClient()
		if err != nil {
			return err
		}
		defer conn()

		ctx := getAuthenticatedContext(context.Background())
		resp, err := client.InviteMember(ctx, &v0.InviteMemberRequest{
			TenantId: args[0],
			Email:    args[1],
			Role:     args[2],
		})
		if err != nil {
			return fmt.Errorf("failed to invite user: %w", err)
		}

		fmt.Printf("User invited: %s\n", args[1])
		fmt.Printf("Status: %s\n", resp.Status)
		if resp.Link != "" {
			fmt.Printf("Link: %s\n", resp.Link)
		}
		if resp.Code != "" {
			fmt.Printf("Code: %s\n", resp.Code)
		}
		return nil
	},
}

var provisionUserCmd = &cobra.Command{
	Use:   "provision [tenant-id] [email] [role]",
	Short: "Provision a user to a tenant directly",
	Args:  cobra.ExactArgs(3),
	RunE: func(cmd *cobra.Command, args []string) error {
		conn, client, err := getClient()
		if err != nil {
			return err
		}
		defer conn()

		ctx := getAuthenticatedContext(context.Background())
		_, err = client.ProvisionUser(ctx, &v0.ProvisionUserRequest{
			TenantId: args[0],
			Email:    args[1],
			Role:     args[2],
		})
		if err != nil {
			return fmt.Errorf("failed to provision user: %w", err)
		}

		fmt.Printf("User provisioned: %s (Role: %s)\n", args[1], args[2])
		return nil
	},
}

var updateUserCmd = &cobra.Command{
	Use:   "update [tenant-id] [user-id] [role]",
	Short: "Update user role",
	Args:  cobra.ExactArgs(3),
	RunE: func(cmd *cobra.Command, args []string) error {
		conn, client, err := getClient()
		if err != nil {
			return err
		}
		defer conn()

		ctx := getAuthenticatedContext(context.Background())
		resp, err := client.UpdateTenantUser(ctx, &v0.UpdateTenantUserRequest{
			TenantId: args[0],
			UserId:   args[1],
			Role:     args[2],
		})
		if err != nil {
			return fmt.Errorf("failed to update user: %w", err)
		}

		fmt.Printf("User updated: %s\n", resp.User.Email)
		fmt.Printf("New Role: %s\n", resp.User.Role)
		return nil
	},
}

func init() {
	tenantCmd.AddCommand(usersCmd)
	usersCmd.AddCommand(listUsersCmd)
	usersCmd.AddCommand(inviteUserCmd)
	usersCmd.AddCommand(provisionUserCmd)
	usersCmd.AddCommand(updateUserCmd)
}
