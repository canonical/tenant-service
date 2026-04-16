#!/usr/bin/env bash
# Copyright 2026 Canonical Ltd.
# SPDX-License-Identifier: AGPL-3.0
#
# Run Playwright browser tests for the Identity Platform login flows.
#
# Two phases:
#   Phase 1 — Multi-tenancy ON:  runs specs/tenant-login.spec.ts
#   Phase 2 — Multi-tenancy OFF: runs specs/login.spec.ts
#
# Between phases the login-ui container is restarted with different config.
# The rest of the stack (Kratos, Hydra, Postgres, OpenFGA, tenant-service)
# stays running throughout. MFA is always enabled.
#
# Usage:
#   ./run-all-tests.sh                         # Run all tests
#   ./run-all-tests.sh --multi-tenancy-only    # Phase 1 only
#   ./run-all-tests.sh --no-multi-tenancy-only # Phase 2 only
#   ./run-all-tests.sh --no-build              # Skip building tenant-service
#
# Environment:
#   LOGIN_UI_IMAGE  Override the login-ui container image
#                   (default: ghcr.io/canonical/identity-platform-login-ui:v0.25.0)
#
# Prerequisites: docker, Go 1.24+, Node.js, yq, fga CLI

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
cd "$REPO_ROOT"

# --- Configuration ---
LOGIN_UI_IMAGE="${LOGIN_UI_IMAGE:-ghcr.io/canonical/identity-platform-login-ui:v0.25.0}"
HYDRA_IMAGE="ghcr.io/canonical/hydra:2.3.0-canonical"
OIDC_CONTAINER_NAME="oidc_client_test"
DSN="postgres://tenants:tenants@127.0.0.1:5432/tenants"
OPENFGA_API_TOKEN="42"

# --- Parse arguments ---
SKIP_BUILD=false
RUN_MULTI_TENANCY=true
RUN_NO_MULTI_TENANCY=true
for arg in "$@"; do
  case "$arg" in
    --no-build)              SKIP_BUILD=true ;;
    --multi-tenancy-only)    RUN_NO_MULTI_TENANCY=false ;;
    --no-multi-tenancy-only) RUN_MULTI_TENANCY=false ;;
    --help|-h)   head -28 "$0" | tail -24; exit 0 ;;
    *)           echo "Unknown argument: $arg"; exit 1 ;;
  esac
done

# --- Colours / helpers ---
RED='\033[0;31m'; GREEN='\033[0;32m'; NC='\033[0m'
log()   { echo -e "${GREEN}[+]${NC} $*"; }
fail()  { echo -e "${RED}[✗]${NC} $*"; }
phase() { echo -e "\n${GREEN}══════════════════════════════════════${NC}"; echo -e "${GREEN}  $*${NC}"; echo -e "${GREEN}══════════════════════════════════════${NC}\n"; }

# --- Cleanup (runs on EXIT) ---
cleanup() {
  log "Cleaning up..."
  docker stop "$OIDC_CONTAINER_NAME" 2>/dev/null || true
  docker rm "$OIDC_CONTAINER_NAME" 2>/dev/null || true
  if [ -n "${APP_PID:-}" ] && kill -0 "$APP_PID" 2>/dev/null; then
    kill "$APP_PID" 2>/dev/null || true
    wait "$APP_PID" 2>/dev/null || true
  fi
  docker compose -f "$REPO_ROOT/docker-compose.dev.yml" down --remove-orphans 2>/dev/null || true
  rm -f "${COMPOSE_OVERRIDE:-}"
}
trap cleanup EXIT

# --- Build tenant-service ---
if $SKIP_BUILD; then
  log "Skipping build (--no-build). Using existing ./app binary."
else
  log "Building tenant-service..."
  make build
fi

# --- Helper: (re)start login-ui with specific config ---
# Args: MULTI_TENANCY_ENABLED MFA_ENABLED
restart_login_ui() {
  local multi_tenancy="$1"
  local mfa="$2"

  log "Restarting login-ui (MULTI_TENANCY=$multi_tenancy, MFA=$mfa)..."

  COMPOSE_OVERRIDE=$(mktemp /tmp/docker-compose.test.XXXXXX.yml)
  cat > "$COMPOSE_OVERRIDE" <<EOF
services:
  identity-platform-login-ui:
    image: ${LOGIN_UI_IMAGE}
    environment:
      - KRATOS_PUBLIC_URL=http://kratos:4433
      - KRATOS_ADMIN_URL=http://kratos:4434
      - HYDRA_ADMIN_URL=http://hydra:4445
      - BASE_URL=http://localhost/
      - COOKIES_ENCRYPTION_KEY=WrfOcYmVBwyduEbKYTUhO4X7XVaOQ1wF
      - PORT=4455
      - LOG_LEVEL=DEBUG
      - TRACING_ENABLED=FALSE
      - MFA_ENABLED=${mfa}
      - OPENFGA_API_SCHEME=http
      - OPENFGA_API_HOST=openfga:8080
      - IDENTIFIER_FIRST_ENABLED=TRUE
      - MULTI_TENANCY_ENABLED=${multi_tenancy}
      - TENANTS_SERVICE_URL=http://host.docker.internal:8000
EOF

  docker compose \
    -f "$REPO_ROOT/docker-compose.dev.yml" \
    -f "$COMPOSE_OVERRIDE" \
    up -d --no-deps --force-recreate identity-platform-login-ui 2>/dev/null

  # Wait for login-ui to be ready
  for i in $(seq 1 30); do
    if curl -sf "http://localhost:4455/api/v0/status" > /dev/null 2>&1; then break; fi
    if [ "$i" -eq 30 ]; then fail "login-ui not ready"; exit 1; fi
    sleep 2
  done
  log "login-ui ready."
}

# --- Docker compose: start infrastructure ---
log "Starting docker compose infrastructure..."
docker compose \
  -f "$REPO_ROOT/docker-compose.dev.yml" \
  up --force-recreate --remove-orphans -d 2>/dev/null

# --- Wait for core services ---
log "Waiting for services..."
for url in \
  "http://localhost:4433/health/ready" \
  "http://localhost:4444/health/ready" \
  "http://localhost:8080/healthz" \
  "http://localhost:5556/dex/.well-known/openid-configuration"; do
  for i in $(seq 1 30); do
    if curl -sf "$url" > /dev/null 2>&1; then break; fi
    if [ "$i" -eq 30 ]; then fail "Service at $url not ready"; exit 1; fi
    sleep 2
  done
done
log "Core services ready."

# --- Hydra clients ---
HYDRA_CONTAINER_ID=$(docker ps -aqf "name=hydra-1" | head -1)

log "Creating Hydra OIDC client..."
CLIENT_RESULT=$(docker exec "$HYDRA_CONTAINER_ID" \
  hydra create client \
    --endpoint http://127.0.0.1:4445 \
    --name "OIDC App" \
    --grant-type authorization_code,refresh_token,urn:ietf:params:oauth:grant-type:device_code \
    --response-type code \
    --format json \
    --scope openid,profile,offline_access,email \
    --redirect-uri http://127.0.0.1:4446/callback)
CLIENT_ID=$(echo "$CLIENT_RESULT" | yq -p json '.client_id // .[0].client_id')
CLIENT_SECRET=$(echo "$CLIENT_RESULT" | yq -p json '.client_secret // .[0].client_secret')

log "Creating Hydra auth client..."
AUTH_CLIENT_RESULT=$(docker exec "$HYDRA_CONTAINER_ID" \
  hydra create client \
    --endpoint http://127.0.0.1:4445 \
    --name "Tenant Service Auth Client" \
    --grant-type client_credentials \
    --scope tenant-service \
    --format json)
AUTH_CLIENT_ID=$(echo "$AUTH_CLIENT_RESULT" | yq -p json '.client_id // .[0].client_id')
AUTH_CLIENT_SECRET=$(echo "$AUTH_CLIENT_RESULT" | yq -p json '.client_secret // .[0].client_secret')

# --- OIDC consumer ---
log "Starting OIDC consumer on :4446..."
docker stop "$OIDC_CONTAINER_NAME" 2>/dev/null || true
docker rm "$OIDC_CONTAINER_NAME" 2>/dev/null || true
docker run --network="host" -d --name="$OIDC_CONTAINER_NAME" "$HYDRA_IMAGE" \
  exec hydra perform authorization-code \
  --endpoint http://localhost:4444 \
  --client-id "$CLIENT_ID" \
  --client-secret "$CLIENT_SECRET" \
  --scope openid,profile,email,offline_access \
  --no-open --no-shutdown --format json

for i in $(seq 1 15); do
  if curl -sf "http://127.0.0.1:4446/" > /dev/null 2>&1; then break; fi
  sleep 1
done

# --- OpenFGA ---
log "Setting up OpenFGA..."
OPENFGA_STORE_ID=$(fga store create --name tenant-service --api-token "$OPENFGA_API_TOKEN" | yq .store.id)
OPENFGA_AUTHORIZATION_MODEL_ID=$(./app create-fga-model \
  --fga-api-url http://127.0.0.1:8080 \
  --fga-api-token "$OPENFGA_API_TOKEN" \
  --fga-store-id "$OPENFGA_STORE_ID" \
  --format json | yq .model_id)

# --- DB migrations ---
log "Running database migrations..."
./app migrate --dsn "$DSN" up

# --- Tenant-service ---
TENANT_LOG="$SCRIPT_DIR/tenant-service.log"
log "Starting tenant-service... (logs: $TENANT_LOG)"
PORT="8000" \
TRACING_ENABLED="false" \
LOG_LEVEL="error" \
KRATOS_ADMIN_URL="http://127.0.0.1:4434" \
AUTHENTICATION_ISSUER="http://localhost:4444" \
AUTHENTICATION_JWKS_URL="http://localhost:4444/.well-known/jwks.json" \
AUTHENTICATION_ENABLED="true" \
AUTHENTICATION_ALLOWED_SUBJECTS="$AUTH_CLIENT_ID" \
AUTHENTICATION_REQUIRED_SCOPE="tenant-service" \
OPENFGA_API_SCHEME="http" \
OPENFGA_API_HOST="127.0.0.1:8080" \
OPENFGA_API_TOKEN="$OPENFGA_API_TOKEN" \
OPENFGA_STORE_ID="$OPENFGA_STORE_ID" \
OPENFGA_AUTHORIZATION_MODEL_ID="$OPENFGA_AUTHORIZATION_MODEL_ID" \
AUTHORIZATION_ENABLED="true" \
WEBHOOKS_API_TOKEN="secret_api_key" \
DSN="$DSN" \
./app serve > "$TENANT_LOG" 2>&1 &
APP_PID=$!

for i in $(seq 1 15); do
  if curl -sf "http://localhost:8000/api/v0/status" > /dev/null 2>&1; then break; fi
  sleep 1
done
log "Tenant-service ready (PID=$APP_PID)."

# --- Install Playwright deps ---
cd "$SCRIPT_DIR"
npm install --no-audit --no-fund --silent

# --- Track overall result ---
OVERALL_EXIT=0

# ═══════════════════════════════════════════════════════════════════════════
# Phase 1: Multi-tenancy ON
# ═══════════════════════════════════════════════════════════════════════════
if $RUN_MULTI_TENANCY; then
  phase "Phase 1: Multi-tenancy ON (MFA enabled)"
  cd "$REPO_ROOT"
  restart_login_ui "TRUE" "TRUE"
  cd "$SCRIPT_DIR"

  if AUTH_CLIENT_ID="$AUTH_CLIENT_ID" \
     AUTH_CLIENT_SECRET="$AUTH_CLIENT_SECRET" \
     npx playwright test specs/tenant-login.spec.ts specs/oidc-tenant-login.spec.ts; then
    log "Phase 1 PASSED"
  else
    fail "Phase 1 FAILED"
    OVERALL_EXIT=1
  fi
fi

# ═══════════════════════════════════════════════════════════════════════════
# Phase 2: Multi-tenancy OFF
# ═══════════════════════════════════════════════════════════════════════════
if $RUN_NO_MULTI_TENANCY; then
  phase "Phase 2: Multi-tenancy OFF (MFA enabled)"
  cd "$REPO_ROOT"
  restart_login_ui "FALSE" "TRUE"
  cd "$SCRIPT_DIR"

  if npx playwright test specs/login.spec.ts specs/oidc-login.spec.ts; then
    log "Phase 2 PASSED"
  else
    fail "Phase 2 FAILED"
    OVERALL_EXIT=1
  fi
fi

# --- Summary ---
echo
if [ "$OVERALL_EXIT" -eq 0 ]; then
  log "All phases PASSED"
else
  fail "Some phases FAILED"
fi
exit "$OVERALL_EXIT"
