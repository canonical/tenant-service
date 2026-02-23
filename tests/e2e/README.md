# E2E Tests

End-to-end tests for the Tenant Service.

## Running Tests

### Mode 1: Full Environment Setup (Default)

This mode spins up the entire stack (Postgres, OpenFGA, Kratos, Hydra) using Docker Compose, creates an OAuth2 client in Hydra for authentication, and runs tests with JWT authentication enabled:

```bash
cd tests/e2e
go test -v .
```

Or from the project root:

```bash
make test-e2e
```

**What happens in this mode:**
- Docker Compose starts all dependencies (Postgres, OpenFGA, Kratos, Hydra)
- Migrations are run against the database
- OpenFGA store and authorization model are created
- OAuth2 client is created in Hydra for authentication
- Service starts with authentication enabled
- Tests automatically use the generated client credentials to obtain JWT tokens

### Running Specific Tests

To run only HTTP tests:
```bash
cd tests/e2e
go test -v -run TestTenantLifecycle
```

To run only gRPC tests:
```bash
cd tests/e2e
go test -v -run TestGRPC
```

### Mode 2: Against Existing Deployment

If you already have the tenant-service running (e.g., via `make dev`), you can run tests against it without setting up a new environment:

**Option A: Using a JWT Token Directly**
```bash
cd tests/e2e
E2E_USE_EXISTING_DEPLOYMENT=true \
HTTP_BASE_URL=http://localhost:8000 \
JWT_TOKEN=<your-jwt-token> \
go test -v .
```

**Option B: Using Client Credentials (exchanges for token automatically)**
```bash
cd tests/e2e
E2E_USE_EXISTING_DEPLOYMENT=true \
HTTP_BASE_URL=http://localhost:8000 \
CLIENT_ID=<client-id> \
CLIENT_SECRET=<client-secret> \
go test -v .
```

**Required Environment Variables for Existing Deployment:**
- `E2E_USE_EXISTING_DEPLOYMENT=true` - Enables existing deployment mode

**Authentication (choose one):**
- `JWT_TOKEN` - A valid JWT token for authentication
- `CLIENT_ID` + `CLIENT_SECRET` - OAuth2 client credentials (will exchange for token from Hydra)

**Optional Environment Variables:**
- `HTTP_BASE_URL` - The base URL of the running HTTP service (default: `http://localhost:8888`, no `/api/v0` prefix)
- `GRPC_ADDRESS` - gRPC server address (default: `localhost:50051`)
  - **Note:** The service's gRPC port is configured during environment setup. If your deployment uses a different port, set this variable accordingly.
- `E2E_STARTUP_TIMEOUT` - Timeout duration for waiting on services to start (default: `30s`, format: `60s`, `2m`, etc.)
  - Useful in CI environments or slower machines where services take longer to start
- `OPENFGA_STORE_ID` - OpenFGA store ID (if different from default)
- `OPENFGA_MODEL_ID` - OpenFGA model ID (if different from default)

### Example with Full Configuration

Using JWT token:
```bash
E2E_USE_EXISTING_DEPLOYMENT=true \
BASE_URL=http://localhost:8000 \
JWT_TOKEN=eyJhbGciOiJSUzI1NiIsImtpZCI6IjEyMyIsInR5cCI6IkpXVCJ9... \
go test -v .
```

Using client credentials:
```bash
E2E_USE_EXISTING_DEPLOYMENT=true \
BASE_URL=http://localhost:8000 \
CLIENT_ID=my-client \
CLIENT_SECRET=my-secret \
go test -v .
```

## Test Structure

- **`setup_test.go`**: Environment setup and teardown logic
- **`client_abstraction_test.go`**: TenantClient interface and HTTP/gRPC implementations
- **`tenant_lifecycle_test.go`**: Unified lifecycle tests (Create, List, Update, Delete) that run against both HTTP and gRPC
- **`e2e_test.go`**: HTTP-specific tests (authentication)
- **`grpc_test.go`**: gRPC-specific tests (authentication)

## Troubleshooting

### Tests fail with "timeout waiting for..." or "server not ready"

**Cause:** Services are taking longer than expected to start (common in CI or resource-constrained environments).

**Solutions:**
- Increase the startup timeout:
  ```bash
  E2E_STARTUP_TIMEOUT=60s go test -v .
  ```
- Check Docker Compose logs to identify slow services:
  ```bash
  docker-compose -f docker-compose.dev.yml logs
  ```
- Ensure Docker has sufficient resources (CPU, memory)

### Authentication failures

**Symptoms:**
- "401 Unauthorized" errors
- "authentication credentials not available"
- "failed to get JWT token"

**Solutions:**
1. **Verify Hydra is running:**
   ```bash
   curl http://localhost:4444/.well-known/openid-configuration
   ```
   Should return JSON configuration.

2. **Check client credentials are correct:**
   - If using `CLIENT_ID`/`CLIENT_SECRET`, ensure they match a valid OAuth2 client in Hydra
   - If using `JWT_TOKEN`, verify it hasn't expired:
     ```bash
     # Decode JWT (payload is between the dots)
     echo "<your-token>" | cut -d'.' -f2 | base64 -d | jq .
     ```
     Check the `exp` (expiration) claim.

3. **Token caching issues:**
   - Tokens are cached for performance
   - If you change credentials, restart the test process

### Port conflicts

**Symptoms:**
- "address already in use"
- Services fail to start in docker-compose

**Solutions:**
1. Check which ports are required:
   - HTTP API: `8000`
   - Hydra Public: `4444`
   - Hydra Admin: `4445`
   - OpenFGA: `8080`
   - PostgreSQL: `5432`
   - gRPC: `50051`

2. Identify conflicting processes:
   ```bash
   # Linux/macOS
   lsof -i :8000
   
   # Or use netstat
   netstat -tuln | grep 8000
   ```

3. Stop conflicting services or modify `docker-compose.dev.yml` to use different ports

### gRPC connection failures

**Symptoms:**
- "failed to connect to gRPC server"
- "context deadline exceeded" when creating gRPC client

**Solutions:**
1. **Verify gRPC port configuration:**
   - Default is `localhost:50051`
   - Check your service's actual gRPC port and set `GRPC_ADDRESS` if different:
     ```bash
     GRPC_ADDRESS=localhost:9090 go test -v .
     ```

2. **Check if service is listening:**
   ```bash
   netstat -tuln | grep 50051
   ```

3. **Firewall/network issues:**
   - Ensure localhost connections are allowed
   - Try with explicit IP: `GRPC_ADDRESS=127.0.0.1:50051`

### Tests are flaky or fail intermittently

**Possible causes:**
1. **Resource timing issues:** Increase `E2E_STARTUP_TIMEOUT`
2. **Database state:** Tests should clean up properly, but manual cleanup may be needed:
   ```bash
   docker-compose -f docker-compose.dev.yml down -v
   ```
3. **Token expiration during test run:** Tests with long timeouts may experience token expiry (tokens are cached with auto-refresh)

### Docker Compose cleanup issues

**Symptoms:**
- "network already exists"
- "container name already in use"
- Resources from previous test runs persist

**Solution:**
```bash
# Full cleanup (removes volumes, networks, containers)
docker-compose -f docker-compose.dev.yml down -v --remove-orphans

# If that doesn't work, nuclear option:
docker system prune -a --volumes
# WARNING: This removes ALL unused Docker resources
```

### Getting detailed logs

**Enable verbose logging:**
```bash
# Run tests with maximum verbosity
go test -v -race ./... 2>&1 | tee test-output.log
```

**Check service logs:**
- The test binary outputs service logs to stdout/stderr during setup
- Look for errors in the OpenFGA, Postgres, Hydra, or Kratos startup logs
