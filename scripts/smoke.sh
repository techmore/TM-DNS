#!/usr/bin/env bash
set -euo pipefail

HTTP_ADDR="${TMDNS_HTTP_URL:-http://127.0.0.1:8080}"
DNS_HOST="${TMDNS_DNS_HOST:-127.0.0.1}"
DNS_PORT="${TMDNS_DNS_PORT:-1053}"

echo "Checking API health"
curl -fsS "$HTTP_ADDR/api/health" >/tmp/tmdns-health.json

echo "Checking static DNS"
static_result="$(dig @"$DNS_HOST" -p "$DNS_PORT" router.test A +short)"
test "$static_result" = "192.168.1.1"

echo "Checking blocked DNS"
blocked_result="$(dig @"$DNS_HOST" -p "$DNS_PORT" blocked.test A +short)"
test "$blocked_result" = "0.0.0.0"

echo "Checking upstream DNS"
upstream_result="$(dig @"$DNS_HOST" -p "$DNS_PORT" example.com A +short)"
test -n "$upstream_result"

echo "Checking dashboard events"
curl -fsS "$HTTP_ADDR/api/dashboard" | grep -q '"blocked_today"'

echo "Smoke test passed"
