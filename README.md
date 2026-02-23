# Tenant Service

The Tenant Service is the multi-tenancy orchestrator for the Identity Platform, providing authorization-aware tenant management.

## Getting Started

To start the development environment, run:

```bash
make dev
```

This command will:
1. Start the necessary dependencies (PostgreSQL, Kratos, Hydra, OpenFGA) using Docker Compose.
2. Build and run the service locally.
3. Start an OIDC client on `http://localhost:4446` to facilitate login flows.

## Kubernetes Development

To run the full stack in a MicroK8s cluster using Skaffold:

```bash
make dev-k8s
```

This command will:
1. Build the Rockcraft image locally.
2. Push the image to your configured registry (default `localhost:32000`).
3. Deploy dependencies (Postgres, OpenFGA) and the service to Kubernetes.
4. Run the setup jobs (DB migrations, OpenFGA config).
5. Port-forward the service and dependencies to localhost.

To clean up the Kubernetes resources:

```bash
make clean-k8s
```

## Configuration

The service is configured using environment variables.

| Variable | Description | Default | Required |
| :--- | :--- | :--- | :--- |
| `OTEL_GRPC_ENDPOINT` | OpenTelemetry gRPC Collector Endpoint | | No |
| `OTEL_HTTP_ENDPOINT` | OpenTelemetry HTTP Collector Endpoint | | No |
| `TRACING_ENABLED` | Enable OpenTelemetry Tracing | `true` | No |
| `KRATOS_ADMIN_URL` | Ory Kratos Admin API URL | | Yes |
| `INVITATION_LIFETIME` | Duration an invitation remains valid | `24h` | No |
| `LOG_LEVEL` | Logging Level | `error` | No |
| `DEBUG` | Enable Debug Mode | `false` | No |
| `PORT` | HTTP Server Port | `8080` | No |
| `GRPC_PORT` | gRPC Server Port | `50051` | No |
| `DSN` | PostgreSQL Connection String | | Yes |
| `DB_MAX_CONNS` | Maximum open DB connections | `25` | No |
| `DB_MIN_CONNS` | Minimum open DB connections | `2` | No |
| `DB_MAX_CONN_LIFETIME` | Maximum amount of time a connection may be reused | `1h` | No |
| `DB_MAX_CONN_IDLE_TIME` | Maximum amount of time a connection may be idle | `30m` | No |
| `AUTHORIZATION_ENABLED` | Enable OpenFGA authorization checks | `false` | No |
| `OPENFGA_API_SCHEME` | OpenFGA API Scheme (http/https) | | No |
| `OPENFGA_API_HOST` | OpenFGA API Host | | No |
| `OPENFGA_API_TOKEN` | OpenFGA API Token | | No |
| `OPENFGA_STORE_ID` | OpenFGA Store ID | | No |
| `OPENFGA_AUTHORIZATION_MODEL_ID` | OpenFGA Model ID | | No |

## Workflows

The Tenant Service supports several key workflows for managing tenants and users, as defined in ID054.

### 1. Self-Service Registration

This flow ensures that every new user is automatically assigned a Tenant, eliminating "orphaned" identities.

**How to run:**
1. Ensure the dev environment is running (`make dev`).
2. Visit `http://localhost:4446` in your browser.
3. Click the link to start the login flow.
4. You will be redirected to the Login UI. Click **"Sign Up"**.
5. Create a new account.
6. Upon success, you will be redirected back to the callback URL, where you can inspect the ID Token. The token should contain a `tenant_id` claim, indicating a tenant was auto-created.

### 2. User Invitation

Allows tenant owners to invite other users to their tenant.

**How to run:**
Use the CLI to simulate an invite. You need the Tenant ID from the previous step (or list them).

```bash
# List tenants to find your Tenant ID
./app tenant list

# Invite a user (email) to the tenant
./app tenant users invite <tenant-id> <email> <role>
# Example: ./app tenant users invite <uuid> bob@example.com member
```

### 3. Enterprise Onboarding

Manual provisioning flow for enterprise customers.

**How to run (Admin CLI):**

```bash
# 1. Create a new Tenant
./app tenant create "Acme Corp"
# Output: Tenant created: Acme Corp (ID: <uuid>)

# 2. Provision an Owner for the Tenant
./app tenant users provision <uuid> alice@acme.com owner
```

### 4. Tenant-Aware Login

Injects the tenant context into the login session.

**How to run:**
1. Visit `http://localhost:4446` to start a new login flow.
2. Log in with a user who belongs to multiple tenants (or created via Enterprise Onboarding).
3. The UI should prompt you to **select a tenant**.
4. After selection, the final ID Token issued will contain the specific `tenant_id` you selected.

### 5. Tenant Switching

Allows users to switch between tenants they belong to.

**How to run:**
1. In the "app" (simulated by `localhost:4446`), click "Log Out" (or clear cookies).
2. Start a new login flow (`http://localhost:4446`).
3. Log in again with the same user.
4. Select a *different* tenant from the selection screen.
5. The new token will reflect the switched tenant.

## E2E Tests

The E2E tests are located in `tests/e2e` and designed to run in isolation with full authentication enabled.

The tests cover both HTTP/REST and gRPC interfaces:
- **HTTP tests** (`e2e_test.go`): Test the REST API via the gRPC-gateway
- **gRPC tests** (`grpc_test.go`): Test the native gRPC interface directly

To run the E2E tests:

```bash
cd tests/e2e
go mod tidy
go test -v .
```

This will:
1. Spin up the full Docker Compose stack (Postgres, OpenFGA, Kratos, Hydra).
2. Build the `tenant-service` binary from the source.
3. Create an OAuth2 client in Hydra for authentication.
4. Run lifecycle tests with JWT authentication enabled.

### Running Against Existing Deployment

If you already have a running deployment (e.g., via `make dev`), you can run tests without setting up the environment:

**Option A: Using a JWT Token**
```bash
E2E_USE_EXISTING_DEPLOYMENT=true \
HTTP_BASE_URL=http://localhost:8000 \
JWT_TOKEN=<your-jwt-token> \
make test-e2e
```

**Option B: Using Client Credentials (exchanges for token automatically)**
```bash
E2E_USE_EXISTING_DEPLOYMENT=true \
CLIENT_ID=<client-id> \
CLIENT_SECRET=<client-secret> \
make test-e2e
```
