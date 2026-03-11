#!/usr/bin/env bash
# Copyright 2026 Canonical Ltd.
# SPDX-License-Identifier: AGPL-3.0
#
# test-pagination.sh — seed tenants and users then walk every page of each
#                      List endpoint to demonstrate cursor-based pagination.
#
# Usage:
#   AUTH_CLIENT_ID=<id> AUTH_CLIENT_SECRET=<secret> ./scripts/test-pagination.sh
#
# Overridable env vars:
#   BASE_URL          — tenant-service HTTP base (default: http://localhost:8000)
#   TOKEN_URL         — Hydra token endpoint   (default: http://localhost:4444/oauth2/token)
#   SCOPE             — OAuth2 scope           (default: tenant-service)
#   NUM_TENANTS       — tenants to create      (default: 25)
#   NUM_USERS         — users per demo tenant  (default: 12)
#   PAGE_SIZE_TENANTS — page size for tenants  (default: 5)
#   PAGE_SIZE_USERS   — page size for users    (default: 3)

set -euo pipefail

BASE_URL="${BASE_URL:-http://localhost:8000}"
TOKEN_URL="${TOKEN_URL:-http://localhost:4444/oauth2/token}"
SCOPE="${SCOPE:-}"   # leave empty unless the client was created with allowed scopes
NUM_TENANTS="${NUM_TENANTS:-25}"
NUM_USERS="${NUM_USERS:-12}"
PAGE_SIZE_TENANTS="${PAGE_SIZE_TENANTS:-5}"
PAGE_SIZE_USERS="${PAGE_SIZE_USERS:-3}"

# ── helpers ────────────────────────────────────────────────────────────────────

RED='\033[0;31m'; GREEN='\033[0;32m'; CYAN='\033[0;36m'
YELLOW='\033[1;33m'; BOLD='\033[1m'; RESET='\033[0m'

info()    { echo -e "${CYAN}[INFO]${RESET} $*"; }
success() { echo -e "${GREEN}[OK]${RESET}   $*"; }
warn()    { echo -e "${YELLOW}[WARN]${RESET} $*"; }
die()     { echo -e "${RED}[ERR]${RESET}  $*" >&2; exit 1; }
header()  { echo -e "\n${BOLD}${CYAN}══ $* ══${RESET}"; }

require() {
  for cmd in "$@"; do
    command -v "$cmd" &>/dev/null || die "'$cmd' is required but not installed."
  done
}

require curl jq

# ── auth ───────────────────────────────────────────────────────────────────────

if [[ -z "${AUTH_CLIENT_ID:-}" || -z "${AUTH_CLIENT_SECRET:-}" ]]; then
  die "AUTH_CLIENT_ID and AUTH_CLIENT_SECRET must be set.\n" \
      "  Example: AUTH_CLIENT_ID=abc AUTH_CLIENT_SECRET=xyz $0"
fi

header "Acquiring access token"
TOKEN_RESPONSE=$(curl -sf -X POST "$TOKEN_URL" \
  -u "${AUTH_CLIENT_ID}:${AUTH_CLIENT_SECRET}" \
  -d "grant_type=client_credentials${SCOPE:+&scope=${SCOPE}}")
TOKEN=$(echo "$TOKEN_RESPONSE" | jq -r '.access_token')
[[ "$TOKEN" != "null" && -n "$TOKEN" ]] || die "Failed to obtain token: $TOKEN_RESPONSE"
success "Token acquired"

api() {
  local method="$1"; shift
  local path="$1";   shift
  curl -sf -X "$method" "${BASE_URL}${path}" \
    -H "Authorization: Bearer ${TOKEN}" \
    -H "Content-Type: application/json" \
    "$@"
}

# ── seed tenants ───────────────────────────────────────────────────────────────

header "Creating ${NUM_TENANTS} tenants"
DEMO_TENANT_ID=""
for i in $(seq 1 "$NUM_TENANTS"); do
  NAME="pagination-demo-tenant-$(printf '%03d' "$i")-$$"
  RESP=$(api POST /api/v0/tenants -d "{\"name\":\"${NAME}\"}")
  ID=$(echo "$RESP" | jq -r '.tenant.id')
  [[ -n "$ID" && "$ID" != "null" ]] || die "CreateTenant failed: $RESP"
  # keep the first tenant for the users demo
  [[ -z "$DEMO_TENANT_ID" ]] && DEMO_TENANT_ID="$ID"
  printf "  created tenant %3d/%d  id=%s  name=%s\n" "$i" "$NUM_TENANTS" "$ID" "$NAME"
done
success "Created ${NUM_TENANTS} tenants. Demo tenant: ${DEMO_TENANT_ID}"

# ── seed users in one tenant ───────────────────────────────────────────────────

header "Provisioning ${NUM_USERS} users into tenant ${DEMO_TENANT_ID}"
for i in $(seq 1 "$NUM_USERS"); do
  EMAIL="pagination-user-$(printf '%02d' "$i")-$$@example.com"
  ROLE=$( [[ $i -eq 1 ]] && echo "owner" || echo "member" )
  RESP=$(api POST "/api/v0/tenants/${DEMO_TENANT_ID}/users" \
    -d "{\"email\":\"${EMAIL}\",\"role\":\"${ROLE}\"}")
  STATUS=$(echo "$RESP" | jq -r '.status // "ok"')
  printf "  provisioned %2d/%d  email=%s  role=%s  status=%s\n" \
    "$i" "$NUM_USERS" "$EMAIL" "$ROLE" "$STATUS"
done
success "Provisioned ${NUM_USERS} users"

# ── paginate ListTenants ───────────────────────────────────────────────────────

header "Paginating /api/v0/tenants  (pageSize=${PAGE_SIZE_TENANTS})"
page=0
total=0
page_token=""
while true; do
  page=$(( page + 1 ))
  url="/api/v0/tenants?page_size=${PAGE_SIZE_TENANTS}"
  [[ -n "$page_token" ]] && url="${url}&page_token=${page_token}"

  RESP=$(api GET "$url")
  count=$(echo "$RESP" | jq '.tenants | length')
  page_token=$(echo "$RESP" | jq -r '.next_page_token // ""')
  total=$(( total + count ))

  echo -e "  Page ${BOLD}${page}${RESET}: ${count} tenant(s) returned"
  echo "$RESP" | jq -r '.tenants[]? | "    · \(.id)  \(.name)"'

  if [[ -z "$page_token" ]]; then
    echo -e "  ${GREEN}(no next_page_token — last page)${RESET}"
    break
  fi
  echo -e "  ${CYAN}next_page_token: ${page_token}${RESET}"
done
echo
success "ListTenants: ${page} page(s), ${total} tenant(s) visible in this run"

# ── paginate ListTenantUsers ───────────────────────────────────────────────────

header "Paginating /api/v0/tenants/${DEMO_TENANT_ID}/users  (pageSize=${PAGE_SIZE_USERS})"
page=0
total=0
page_token=""
while true; do
  page=$(( page + 1 ))
  url="/api/v0/tenants/${DEMO_TENANT_ID}/users?page_size=${PAGE_SIZE_USERS}"
  [[ -n "$page_token" ]] && url="${url}&page_token=${page_token}"

  RESP=$(api GET "$url")
  count=$(echo "$RESP" | jq '.users | length')
  page_token=$(echo "$RESP" | jq -r '.next_page_token // ""')
  total=$(( total + count ))

  echo -e "  Page ${BOLD}${page}${RESET}: ${count} user(s) returned"
  echo "$RESP" | jq -r '.users[]? | "    · \(.user_id)  \(.email)  role=\(.role)"'

  if [[ -z "$page_token" ]]; then
    echo -e "  ${GREEN}(no next_page_token — last page)${RESET}"
    break
  fi
  echo -e "  ${CYAN}next_page_token: ${page_token}${RESET}"
done
echo
success "ListTenantUsers: ${page} page(s), ${total} user(s) in demo tenant"

# ── summary ───────────────────────────────────────────────────────────────────

echo
echo -e "${BOLD}${GREEN}All done.${RESET}"
echo -e "  Tenants created : ${NUM_TENANTS}"
echo -e "  Users created   : ${NUM_USERS}"
echo -e "  Page size used  : tenants=${PAGE_SIZE_TENANTS}, users=${PAGE_SIZE_USERS}"
echo
echo "To clean up the demo tenants, run:"
echo "  ${CYAN}api GET /api/v0/tenants | jq -r '.tenants[].id' |"
echo "    grep pagination-demo-tenant | xargs -I{} curl -sf -X DELETE ..."
echo -e "${RESET}"
