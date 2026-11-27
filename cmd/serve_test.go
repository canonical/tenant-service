// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package cmd

import (
	"testing"
	"time"

	"github.com/canonical/tenant-service/internal/config"
	"github.com/canonical/tenant-service/internal/logging"
	"github.com/canonical/tenant-service/internal/monitoring"
	"github.com/canonical/tenant-service/internal/permissions"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewPermissionsPublisher(t *testing.T) {
	logger := logging.NewNoopLogger()
	monitor := monitoring.NewNoopMonitor("tenant-service", logger)

	tests := []struct {
		name        string
		specs       *config.EnvSpec
		wantType    any
		wantErr     bool
		errContains string
	}{
		{
			name:     "default empty mode selects noop",
			specs:    &config.EnvSpec{},
			wantType: &permissions.NoopPublisher{},
		},
		{
			name:     "explicit noop mode selects noop",
			specs:    &config.EnvSpec{PermissionsPublishingMode: "noop"},
			wantType: &permissions.NoopPublisher{},
		},
		{
			name: "platform mode selects kafka publisher",
			specs: &config.EnvSpec{
				PermissionsPublishingMode: "platform",
				KafkaBrokers:              []string{"localhost:9092"},
				KafkaPermissionsTopic:     "permissions.tenant-service",
			},
			wantType: &permissions.KafkaPublisher{},
		},
		{
			name: "platform mode missing brokers fails",
			specs: &config.EnvSpec{
				PermissionsPublishingMode: "platform",
				KafkaPermissionsTopic:     "permissions.tenant-service",
			},
			wantErr:     true,
			errContains: "invalid platform permissions configuration",
		},
		{
			name: "standalone mode selects grpc publisher",
			specs: &config.EnvSpec{
				PermissionsPublishingMode:       "standalone",
				AuthorizationServiceGRPCAddress: "localhost:9091",
				AuthorizationServiceGRPCTimeout: 5 * time.Second,
				AuthorizationServiceGRPCWorkers: 5,
				ServiceTokenURL:                 "http://localhost:4444/oauth2/token",
				ServiceClientID:                 "tenant-service",
				ServiceClientSecret:             "secret",
				ServiceTokenScopes:              "authorization:ingest, openid",
			},
			wantType: &permissions.GRPCPublisher{},
		},
		{
			name: "standalone mode negative workers fails",
			specs: &config.EnvSpec{
				PermissionsPublishingMode:       "standalone",
				AuthorizationServiceGRPCAddress: "localhost:9091",
				AuthorizationServiceGRPCTimeout: 5 * time.Second,
				AuthorizationServiceGRPCWorkers: -1,
				ServiceTokenURL:                 "http://localhost:4444/oauth2/token",
				ServiceClientID:                 "tenant-service",
				ServiceClientSecret:             "secret",
			},
			wantErr:     true,
			errContains: "invalid standalone permissions configuration",
		},
		{
			name: "standalone mode missing grpc address fails",
			specs: &config.EnvSpec{
				PermissionsPublishingMode: "standalone",
				ServiceTokenURL:           "http://localhost:4444/oauth2/token",
				ServiceClientID:           "tenant-service",
				ServiceClientSecret:       "secret",
			},
			wantErr:     true,
			errContains: "invalid standalone permissions configuration",
		},
		{
			name: "standalone mode missing oauth2 credentials fails",
			specs: &config.EnvSpec{
				PermissionsPublishingMode:       "standalone",
				AuthorizationServiceGRPCAddress: "localhost:9091",
				ServiceTokenURL:                 "http://localhost:4444/oauth2/token",
			},
			wantErr:     true,
			errContains: "invalid standalone permissions configuration",
		},
		{
			name: "invalid mode fails",
			specs: &config.EnvSpec{
				PermissionsPublishingMode: "unknown",
			},
			wantErr:     true,
			errContains: "invalid permissions publishing mode",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pub, err := permissions.NewPublisher(tt.specs.PermissionsConfig(), logger, monitor)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
				assert.Nil(t, pub)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, pub)
			assert.IsType(t, tt.wantType, pub)
			assert.NoError(t, pub.Close())
		})
	}
}

func TestParseScopes(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want []string
	}{
		{name: "empty", raw: "", want: []string{}},
		{name: "whitespace only", raw: "   \t\n ", want: []string{}},
		{name: "comma separated", raw: "a,b,c", want: []string{"a", "b", "c"}},
		{name: "space separated", raw: "a b c", want: []string{"a", "b", "c"}},
		{name: "mixed commas and whitespace", raw: " a , b\tc , , d ", want: []string{"a", "b", "c", "d"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, config.ParseScopes(tt.raw))
		})
	}
}
