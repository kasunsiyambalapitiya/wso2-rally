<!--
Copyright (c) 2026 WSO2 LLC. (https://www.wso2.com).
Licensed under the Apache License, Version 2.0.
-->

# Rally Backend

The game engine for **WSO2 Motor Rally 2027**: a chi REST API plus a WebSocket
hub, backed by MySQL. It serves two very different callers from one service —
organizers running the rally from a browser, and one phone per car running it
from the passenger seat.

Design spec: [`docs/specs/2026-07-24-wso2-motor-rally-design.md`](../docs/specs/2026-07-24-wso2-motor-rally-design.md).

## Prerequisites

- Go 1.23 or newer
- Docker (for the local MySQL used by the DB-backed tests)

## Running it

```bash
cp config.example.env config.env   # then edit, at minimum DB_DSN and TEAM_TOKEN_SECRET
set -a && source config.env && set +a

make docker-db     # start MySQL 8 on :3306
make run           # migrations run on boot, then the server listens on :8080

curl localhost:8080/health
```

`make migrate-up` applies the schema and exits, for a deploy step that wants
migrations separate from the rollout.

## Testing

```bash
make test              # unit tests; DB-backed tests skip themselves
make docker-db
make test-integration  # everything, including the full happy-path walkthrough
make lint              # gofmt + go vet
```

Tests that need a database read `TEST_DB_DSN` and call `t.Skip` when it is
unset, so `go test ./...` stays green on a machine without Docker. **A green
`make test` therefore does not mean the SQL was exercised** — run
`make test-integration` before trusting a schema or repository change. That
target passes `-count=1`, because a cached `ok` from a run without a database
would otherwise report success for tests that skipped.

DB-backed tests hold a MySQL advisory lock (`wso2_rally_test`) for their
duration, so they run one at a time. `go test ./...` runs package binaries in
parallel and they all share one database; without the lock, one package's
truncation wipes rows another has just seeded. Per-database isolation would be
faster, but the compose-provisioned user cannot `CREATE DATABASE`.

`make docker-db` picks `docker compose` or `nerdctl compose`, whichever the
machine has — Rancher Desktop in containerd mode has no dockerd for
`docker compose` to talk to.

## The two identities

| Caller | Credential | How it is checked |
|---|---|---|
| Organizer | Asgardeo id token | JWKS signature validation when `TOKEN_VALIDATOR_ENABLED=true`; claims decoded without verification otherwise |
| Crew | Team JWT minted by `POST /sessions/bind` | HMAC signature, `iss=rally-team`, expiry |

Both arrive as `Authorization: Bearer <token>`. The auth middleware tries the
team token first — it is cheap and carries our own issuer — and hands anything
else to the organizer validator. Requests then pass a role gate:
`RequireOrganizer`, `RequireTeam`, or `RequireAdmin`.

> **Decode-only mode is for local development only.** With
> `TOKEN_VALIDATOR_ENABLED=false` the service trusts organizer claims without
> checking a signature, and logs a warning at startup saying so. Every deployed
> environment must set it to `true` and supply `JWKS_ENDPOINT`; `config.Load`
> refuses to start if one is set without the other.

`POST /sessions/bind` is the only unauthenticated write: binding a vehicle is
what authenticates a crew, and it is guarded instead by the one-active-phone
rule below.

## How a rally runs

1. An organizer creates an event, draws the start and finish geofences, orders
   the waypoints of each route, attaches tasks to them, and provisions vehicles
   and crews (by hand or by CSV).
2. Publishing the event opens it to crews. Both geofences must be placed first,
   or the start could never lock and arrival could never be detected.
3. A crew picks their vehicle on one phone. `POST /sessions/bind` mints their
   team token. A second phone binding the same vehicle gets a `409` — the
   **one-active-phone** rule, enforced by a unique index on
   `(vehicle_id, active_flag)` rather than by a read-then-write check.
4. The phone streams position to `POST /sessions/me/location`. **The server
   decides what that position means**: it evaluates every waypoint boundary,
   returns which tasks unlocked, and raises rest-lock, trivia, and arrival
   events.
5. Submissions go to `POST /sessions/me/tasks/{taskId}/submit`. The
   `taskengine` validates and scores them against the task's config; the phone
   never decides whether it was right, and cannot earn more than the task is
   worth.
6. Arriving inside the finish geofence auto-finishes the session, locks the
   score, and issues the crew's voucher.

## Layout

```
cmd/server/          wiring: config → store → services → router → listener
internal/
  config/            env loading and validation
  httpx/             JSON, the {"message": ...} error shape, paging
  apperr/            the shared error categories every domain maps onto
  authz/             team tokens, organizer JWTs, Identity, role checks
  middleware/        request id, recovery, security headers, logging, auth
  store/             MySQL pool, id generation, transactions, migrations
  geo/               haversine and point-in-radius
  realtime/          the WebSocket hub
  taskengine/        one validator and scorer per task type
  events/ routes/ tasks/ vehicles/ alerts/ sessions/ scoring/ debrief/
api/                 openapi.yaml (REST), asyncapi.yaml (WebSocket)
.choreo/             deployment descriptor
```

Every domain package follows the same shape, mirroring the Ballerina module
split the rest of the platform uses:

| File | Holds |
|---|---|
| `<name>.go` | domain types, enums, sentinel errors |
| `service.go` | the rules, plus the `Repo` interface it depends on |
| `repo.go` | the SQL behind that interface |
| `dto.go` | wire shapes and the mappers to and from the domain |
| `handler.go` | HTTP routes |

Services depend on `Repo` interfaces, so their rules are tested against an
in-memory fake and the SQL is tested separately against a real MySQL.

## Conventions

- Every non-2xx body is exactly `{"message": "..."}`. Internal detail is
  logged with the request id, never returned.
- No silent fallbacks: a database or parse error is handled or returned, never
  swallowed into a zero value.
- Entity ids are 32-character lowercase hex from `store.NewID`, generated with
  `crypto/rand` because they appear in URLs and tokens.
- Lists are `POST /<resource>/search` with `{offset, limit, filters}`; the
  limit defaults to 20 and caps at 100.
- `GET /health` is unauthenticated, for Choreo's probe.
- Apache-2.0 header on every source file.

## Things worth knowing before you change them

- **`trigger` is a MySQL reserved word.** Every query touching `task.trigger`
  backticks it.
- **Task answers are stripped for crews.** `GET /tasks/{id}` is read by both
  identities; `tasks.RedactForCrew` removes the scoring keys before the
  definition reaches a phone. **Any new config key that decides a score must be
  added to `secretConfigKeys`**, or it ships to the car with the question.
- **Broadcasts are best-effort.** A subscriber that stops reading loses
  messages rather than blocking the crew whose submission produced them; the
  count is exposed by `Hub.Dropped`.
- **The leaderboard is composed at the wiring layer.** `sessions` publishes a
  score change and `cmd/server` turns that into a refreshed leaderboard, so the
  in-car runtime does not depend on `scoring`.
