#!/usr/bin/env bash
# Build, launch, and curl-drive the dzbazar Go API against the real dev DB.
# Run from server/. Requires .env already populated (DB_URI, JWT_SECRET, etc).
set -euo pipefail
cd "$(dirname "$0")/../../.." # server/

PIDFILE=/c/tmp/server.pid
LOGFILE=/c/tmp/server-run.log
PORT="$(grep '^PORT=' .env | cut -d= -f2)"
PORT="${PORT:-8080}"

echo "==> building"
go build -o ./tmp/main.exe .

echo "==> launching (APP_ENV=development override: real .env has APP_ENV=production," \
     "which flips cookies to Secure=true and breaks plain-http curl testing)"
APP_ENV=development ./tmp/main.exe > "$LOGFILE" 2>&1 &

# $! is the bash job PID, not the real Windows PID of main.exe on Git-Bash/MSYS —
# taskkill on it silently fails ("process not found"). Resolve the real PID
# from the listening socket instead, retrying until the port is bound.
REAL_PID=""
for _ in $(seq 1 30); do
  REAL_PID="$(netstat -ano | grep ":$PORT " | grep LISTENING | head -1 | awk '{print $NF}' || true)"
  [ -n "$REAL_PID" ] && break
  sleep 1
done
echo "$REAL_PID" > "$PIDFILE"
echo "listening pid: $REAL_PID"

echo "==> waiting for the port (routes are auth-gated, so poll for *any* HTTP response, not 2xx)"
timeout 30 bash -c "until curl -s -o /dev/null -w '%{http_code}' http://localhost:$PORT/v1/plans | grep -q 200; do sleep 1; done"

echo "==> driving: public plan catalog (proves DB round-trip with real seeded data)"
curl -s http://localhost:$PORT/v1/plans | head -c 400; echo

echo "==> driving: auth-gated route without a cookie (expect 401, proves middleware runs)"
curl -s -o /dev/null -w "orders (no cookie) -> %{http_code}\n" \
  "http://localhost:$PORT/v1/shops/00000000-0000-0000-0000-000000000000/orders"

echo "==> tail of server log"
tail -n 20 "$LOGFILE"

echo
echo "Server is left running (pid $REAL_PID, log $LOGFILE)."
echo "Stop it with: taskkill //F //PID \$(cat $PIDFILE)"
