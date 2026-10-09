// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

// Package testhelpers provides shared testcontainers-based fixtures for
// integration tests. Helpers start containers, register teardown via
// t.Cleanup, and fail hard via t.Fatalf on any error. A container runtime
// (Docker or Podman socket) is a documented prerequisite for running
// integration tests; use `go test -short` for the unit-only suite.
package testhelpers

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

// Container image versions used across all integration tests. Bump here once.
const (
	postgresImage = "postgres:16-alpine"
)

const (
	pingTimeout   = 30 * time.Second
	pingRetryWait = time.Second
)

// pingPostgres pings the database until it accepts connections or the
// pingTimeout deadline is reached. Returns an error instead of failing the
// test so the caller decides how to surface the failure.
func pingPostgres(connStr string) error {
	cfg, err := pgx.ParseConfig(connStr)
	if err != nil {
		return fmt.Errorf("failed to parse connection string: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), pingTimeout)
	defer cancel()

	for {
		sqlDB := stdlib.OpenDB(*cfg)
		err := sqlDB.PingContext(ctx)
		sqlDB.Close()
		if err == nil {
			return nil
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("postgres at %s did not become ready within %s", cfg.Host, pingTimeout)
		case <-time.After(pingRetryWait):
		}
	}
}
