package config

import (
	"time"
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

	DSN string `envconfig:"DSN" required:"true"`

	DBMaxConns        int32         `envconfig:"db_max_conns" default:"25"`
	DBMinConns        int32         `envconfig:"db_min_conns" default:"2"`
	DBMaxConnLifetime time.Duration `envconfig:"db_max_conn_lifetime" default:"1h"`
	DBMaxConnIdleTime time.Duration `envconfig:"db_max_conn_idle_time" default:"30m"`

	AuthorizationEnabled bool   `envconfig:"authorization_enabled" default:"false"`
	OpenfgaApiScheme     string `envconfig:"openfga_api_scheme" default:""`
	OpenfgaApiHost       string `envconfig:"openfga_api_host"`
	OpenfgaApiToken      string `envconfig:"openfga_api_token"`
	OpenfgaStoreId       string `envconfig:"openfga_store_id"`
	OpenfgaModelId       string `envconfig:"openfga_authorization_model_id" default:""`
}
