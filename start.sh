#!/bin/bash

# set -x
set -e

cleanup () {
  docker compose -f ./docker-compose.dev.yml down > /dev/null
  docker stop oidc_client > /dev/null
  exit
}

trap "cleanup" INT EXIT

# Start build in background
make build &
BUILD_PID=$!

SCRIPT_DIR=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )

# Start dependencies
echo "Starting docker compose services..."
docker compose -f ./docker-compose.dev.yml up --wait --force-recreate --build --remove-orphans -d 2>&1 | grep -E "(Creating|Starting|Waiting|Error|error)" || true
echo "Docker compose services started successfully"

# Start client app
HYDRA_CONTAINER_ID=$(docker ps -aqf "name=tenant-service-hydra-1")
HYDRA_IMAGE=ghcr.io/canonical/hydra:2.2.0-canonical

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

# Create a client credentials client for JWT authentication
AUTH_CLIENT_RESULT=$(docker exec "$HYDRA_CONTAINER_ID" \
  hydra create client \
  --endpoint http://127.0.0.1:4445 \
  --name "Tenant Service Auth Client" \
  --grant-type client_credentials \
  --format json)

AUTH_CLIENT_ID=$(echo "$AUTH_CLIENT_RESULT" | yq -p json '.client_id // .[0].client_id')
AUTH_CLIENT_SECRET=$(echo "$AUTH_CLIENT_RESULT" | yq -p json '.client_secret // .[0].client_secret')

echo "Waiting for build to complete..."
wait $BUILD_PID
echo "Build completed."

docker stop oidc_client > /dev/null 2>&1  || true
docker rm oidc_client > /dev/null 2>&1  || true
docker run --network="host" -d --name=oidc_client --rm $HYDRA_IMAGE \
  exec hydra perform authorization-code \
  --endpoint http://localhost:4444 \
  --client-id $CLIENT_ID \
  --client-secret $CLIENT_SECRET \
  --scope openid,profile,email,offline_access \
  --no-open --no-shutdown --format json


export PORT="8000"
export TRACING_ENABLED="false"
export LOG_LEVEL="debug"
export KRATOS_ADMIN_URL="http://127.0.0.1:4434"
export AUTHENTICATION_ISSUER="http://localhost:4444"
export AUTHENTICATION_JWKS_URL="http://localhost:4444/.well-known/jwks.json"
export AUTHENTICATION_ENABLED="true"
export AUTHENTICATION_ALLOWED_SUBJECTS="$AUTH_CLIENT_ID"
export AUTHENTICATION_REQUIRED_SCOPE="tenant-service"
export OPENFGA_API_SCHEME="http"
export OPENFGA_API_HOST="127.0.0.1:8080"
export OPENFGA_API_TOKEN="42"
export OPENFGA_STORE_ID=$(fga store create --name tenant-service --api-token $OPENFGA_API_TOKEN | yq .store.id)
export OPENFGA_AUTHORIZATION_MODEL_ID=$(./app create-fga-model --fga-api-url http://127.0.0.1:8080 --fga-api-token $OPENFGA_API_TOKEN --fga-store-id $OPENFGA_STORE_ID --format json | yq .model_id)
export AUTHORIZATION_ENABLED="true"
export DSN="postgres://tenants:tenants@127.0.0.1:5432/tenants"

echo "Running database migrations..."
./app migrate --dsn $DSN up

# Generate JWT token for convenience
# The app token command prints just the access token to stdout
AUTH_JWT=$(./app token \
  --client-id "$AUTH_CLIENT_ID" \
  --client-secret "$AUTH_CLIENT_SECRET" \
  --issuer-url "$AUTHENTICATION_ISSUER" \
  --scopes "$AUTHENTICATION_REQUIRED_SCOPE" || echo "Failed to generate token")

echo
echo "==============================================="
echo "App Client ID: $CLIENT_ID"
echo "Auth Client ID: $AUTH_CLIENT_ID"
echo "Auth Client Secret: $AUTH_CLIENT_SECRET"
echo "Store ID: $OPENFGA_STORE_ID"
echo "Model ID: $OPENFGA_AUTHORIZATION_MODEL_ID"
echo "==============================================="
echo "To get another API token, run:"
echo "curl -X POST http://localhost:4444/oauth2/token \\"
echo "  -u \"$AUTH_CLIENT_ID:$AUTH_CLIENT_SECRET\""
echo "==============================================="
echo

./app serve
