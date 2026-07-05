---
name: run-server
description: Build, run, and drive the dzbazar Go API against the real dev DB. Use when asked to start the server, build it, run a smoke test, hit an endpoint, or check that a backend change actually works end to end.
---

Go + Gin + GORM + Postgres API. No browser, no GUI — it's driven with
`curl`. Build it, launch it, then run
`.claude/skills/run-server/smoke.sh` (or hand-drive with `curl`) — that
script is the harness a future agent should reach for first.

All paths below are relative to `server/`.

## Prerequisites

Go 1.24+ (`go version` — already on PATH in this environment).
`.env` must already exist with `DB_URI` pointing at a reachable
Postgres (this project's dev DB is a live Railway instance — see
`CLAUDE.md`'s env var list). No install step needed beyond `go build`
pulling modules from `go.sum`.

## Build

```bash
go build -o ./tmp/main.exe .
```

## Run (agent path)

```bash
bash .claude/skills/run-server/smoke.sh
```

This builds, launches the binary in the background, resolves its real
PID off the listening socket, polls `/v1/plans` until it's up, then
drives two real requests: the public plan catalog (proves a full
DB round-trip against real seeded data) and an auth-gated order route
with no cookie (expects `401`, proves the auth middleware runs). It
leaves the server running and prints the stop command.

Manual equivalent, if you want to drive it yourself instead of the
script:

```bash
APP_ENV=development ./tmp/main.exe > /c/tmp/server-run.log 2>&1 &
sleep 2
PORT="$(grep '^PORT=' .env | cut -d= -f2)"
curl -s http://localhost:$PORT/v1/plans          # public, no auth needed
curl -s -X POST http://localhost:$PORT/v1/users/login \
  -H "Content-Type: application/json" \
  -d '{"email":"you@example.com","password":"..."}' \
  -c /c/tmp/server-cookies.txt                    # -> sets AccessToken/RefreshToken cookies
curl -s -b /c/tmp/server-cookies.txt \
  -H "X-Shop-ID: <a real shop uuid the user owns>" \
  http://localhost:$PORT/v1/shops/<shopId>/orders # now authenticated
```

Stop the server (see Gotchas for why `kill $!` doesn't work here):

```bash
netstat -ano | grep ":5000" | grep LISTENING   # find the real PID
taskkill //F //PID <pid>
```

## Run (human path)

```bash
air   # hot-reload dev server, per .air.toml — Ctrl-C to stop
```

## Test

No test suite in this project (`CLAUDE.md`: "There are no tests
currently"). The smoke script above is the closest thing to a check.

---

## Gotchas

- **`$!` after backgrounding `./tmp/main.exe` is not the real Windows
  PID** in this Git-Bash/MSYS environment — `taskkill //F //PID $!`
  reports "process not found" even though the server is up and
  serving. Resolve the actual PID from the listening socket instead:
  `netstat -ano | grep ":$PORT" | grep LISTENING | awk '{print $NF}'`.
- **`.env` ships with `APP_ENV=production`.** `middleware/requireAuth.go`
  and the login handler set cookies `Secure=true` when `APP_ENV` is
  `production`, which browsers/curl won't send back over plain
  `http://localhost`. Override it per-process — `APP_ENV=development
  ./tmp/main.exe` — without touching the committed `.env` (godotenv's
  `Load()` never overwrites a var that's already set in the process
  environment, so the override wins).
- **Almost every route is auth-gated** (`RequireRoles`/`RequireShopAccess`),
  so a naive `curl -sf` health-poll loop that waits for a `2xx` will
  spin for the full timeout against, e.g., `/v1/shops/:id/orders` (it
  correctly 401s). Poll a route that's genuinely public instead —
  `GET /v1/plans` (global pricing catalog, no auth) — or just check for
  any HTTP response.
- **Redis connect can time out** (`error while trying to connect to
  redis: dial tcp ...: i/o timeout`) without crashing the server —
  `initializers.InitRedis()` logs and continues rather than fataling.
  Don't mistake that log line for a failed launch; check the port
  instead.
- **Route prefix is `/v1/...`, not `/api/v1/...`** — `rest_tests/*.rest`
  in this repo has stale examples using `/api/v1/...` that 404. Every
  `routes/*.go` file registers under `/v1` directly (confirm with
  `grep -rn "Group(" routes/`).
