#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

go build ./cmd/tmdns

if screen -ls | grep -q "tmdns-dev"; then
  screen -S tmdns-dev -X quit || true
fi

for _ in {1..30}; do
  if ! lsof -nP -iTCP:8080 -iUDP:1053 -iTCP:1053 | grep -q tmdns; then
    break
  fi
  sleep 0.2
done

if lsof -nP -iTCP:8080 -iUDP:1053 -iTCP:1053 | grep -q tmdns; then
  echo "Existing tmdns process still owns a dev port; stop it with scripts/dev-stop.sh or kill it manually." >&2
  exit 1
fi

: > /tmp/tmdns-dev.log
screen -dmS tmdns-dev zsh -lc 'cd /Users/techmore/projects/TM-DNS && TMDNS_DNS_ADDR=${TMDNS_DNS_ADDR:-127.0.0.1:1053} TMDNS_HTTP_ADDR=${TMDNS_HTTP_ADDR:-127.0.0.1:8080} TMDNS_DB_PATH=${TMDNS_DB_PATH:-tm-dns-dev.db} TMDNS_LOG_LEVEL=${TMDNS_LOG_LEVEL:-debug} ./tmdns >> /tmp/tmdns-dev.log 2>&1'

for _ in {1..30}; do
  if curl -fsS http://127.0.0.1:8080/api/health >/dev/null 2>&1; then
    break
  fi
  sleep 0.2
done

curl -fsS http://127.0.0.1:8080/api/health >/dev/null

echo "TM-DNS dev service started"
echo "UI: http://127.0.0.1:8080"
echo "DNS: 127.0.0.1:1053"
echo "Logs: /tmp/tmdns-dev.log"
