#!/usr/bin/env bash
set -euo pipefail

if screen -ls | grep -q "tmdns-dev"; then
  screen -S tmdns-dev -X quit || true
else
  echo "TM-DNS dev screen was not running"
fi

for _ in {1..30}; do
  if ! lsof -nP -iTCP:8080 -iUDP:1053 -iTCP:1053 | grep -q tmdns; then
    echo "TM-DNS dev service stopped"
    exit 0
  fi
  sleep 0.2
done

echo "Detached tmdns process still owns a dev port; terminating it"
pids="$(lsof -nP -iTCP:8080 -iUDP:1053 -iTCP:1053 -t | sort -u || true)"
if [ -n "$pids" ]; then
  kill $pids || true
fi

for _ in {1..30}; do
  if ! lsof -nP -iTCP:8080 -iUDP:1053 -iTCP:1053 | grep -q tmdns; then
    echo "TM-DNS dev service stopped"
    exit 0
  fi
  sleep 0.2
done

echo "TM-DNS dev process still owns a dev port after termination attempt" >&2
exit 1
