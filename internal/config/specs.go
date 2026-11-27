// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package config

import (
	"strings"
	"time"
	"unicode"
)

// EnvSpec is the basic environment configuration setup needed for the app to start
type EnvSpec struct {
	OtelGRPCEndpoint string `envconfig:"otel_grpc_endpoint"`
	OtelHTTPEndpoint string `envconfig:"otel_http_endpoint"`
	TracingEnabled   bool   `envconfig:"tracing_enabled" default:"true"`

	KratosAdminURL string `envconfig:"kratos_admin_url" required:"true"`

	InvitationLifetime string `envconfig:"invitation_lifetime" default:"24h"`

	LogLevel string `envconfig:"log_level" default:"error"`
	Debug    bool   `envconfig:"debug" default:"false"`

	Port     int `envconfig:"port" default:"8080"`
	GRPCPort int `envconfig:"grpc_port" default:"50051"`

	WebhooksAPIToken string `envconfig:"webhooks_api_token" default:""`

	DSN string `envconfig:"DSN" required:"true"`

	DBMaxConns        int32         `envconfig:"db_max_conns" default:"25"`
	DBMinConns        int32         `envconfig:"db_min_conns" default:"2"`
	DBMaxConnLifetime time.Duration `envconfig:"db_max_conn_lifetime" default:"1h"`
	DBMaxConnIdleTime time.Duration `envconfig:"db_max_conn_idle_time" default:"30m"`

	PermissionsPublishingMode string `envconfig:"permissions_publishing_mode" default:"noop"`

	KafkaBrokers          []string `envconfig:"kafka_brokers"`
	KafkaPermissionsTopic string   `envconfig:"kafka_permissions_topic" default:"permissions.tenant-service"`

	AuthorizationServiceGRPCAddress string        `envconfig:"authorization_service_grpc_address"`
	AuthorizationServiceGRPCTimeout time.Duration `envconfig:"authorization_service_grpc_timeout" default:"5s"`
	AuthorizationServiceGRPCWorkers int           `envconfig:"authorization_service_grpc_workers" default:"5"`
	AuthorizationServiceTLSEnabled  bool          `envconfig:"authorization_service_tls_enabled" default:"false"`
	ServiceTokenURL                 string        `envconfig:"service_token_url"`
	ServiceClientID                 string        `envconfig:"service_client_id"`
	ServiceClientSecret             string        `envconfig:"service_client_secret"`
	ServiceTokenScopes              string        `envconfig:"service_token_scopes"`

	AuthenticationEnabled         bool   `envconfig:"authentication_enabled" default:"true"`
	AuthenticationIssuer          string `envconfig:"authentication_issuer"`
	AuthenticationJwksURL         string `envconfig:"authentication_jwks_url"`
	AuthenticationAllowedSubjects string `envconfig:"authentication_allowed_subjects"`
	AuthenticationRequiredScope   string `envconfig:"authentication_required_scope"`
}

// PermissionsConfig groups the decoded permission-publishing configuration across
// noop, platform (Kafka), and standalone (Authorization Service gRPC) modes.
type PermissionsConfig struct {
	Mode                 string
	Kafka                KafkaConfig
	AuthorizationService AuthorizationServiceConfig
}

// KafkaConfig holds the Kafka permission-publishing configuration for platform mode.
type KafkaConfig struct {
	Brokers []string
	Topic   string
}

// AuthorizationServiceConfig holds the gRPC and OAuth2 client-credentials settings for standalone mode.
type AuthorizationServiceConfig struct {
	GRPCAddress  string
	GRPCTimeout  time.Duration
	GRPCWorkers  int
	TLSEnabled   bool
	TokenURL     string
	ClientID     string
	ClientSecret string
	TokenScopes  []string
}

// PermissionsConfig normalizes and groups permission-publishing settings from EnvSpec.
func (e *EnvSpec) PermissionsConfig() PermissionsConfig {
	return PermissionsConfig{
		Mode: strings.ToLower(strings.TrimSpace(e.PermissionsPublishingMode)),
		Kafka: KafkaConfig{
			Brokers: e.KafkaBrokers,
			Topic:   e.KafkaPermissionsTopic,
		},
		AuthorizationService: AuthorizationServiceConfig{
			GRPCAddress:  e.AuthorizationServiceGRPCAddress,
			GRPCTimeout:  e.AuthorizationServiceGRPCTimeout,
			GRPCWorkers:  e.AuthorizationServiceGRPCWorkers,
			TLSEnabled:   e.AuthorizationServiceTLSEnabled,
			TokenURL:     e.ServiceTokenURL,
			ClientID:     e.ServiceClientID,
			ClientSecret: e.ServiceClientSecret,
			TokenScopes:  ParseScopes(e.ServiceTokenScopes),
		},
	}
}

// ParseScopes splits a comma- or whitespace-separated OAuth2 scope string.
func ParseScopes(raw string) []string {
	return strings.FieldsFunc(raw, func(r rune) bool {
		return unicode.IsSpace(r) || r == ','
	})
}
