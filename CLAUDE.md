# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Current state

| Component | State |
| --- | --- |
| `backend/` | **Built.** Every domain in the spec (`events`, `routes`, `tasks`, `vehicles`, `alerts`, `sessions`, `scoring`, `debrief`, `taskengine`, `geo`, `realtime`), the full schema, and both the organizer and team-token paths. Unit tests everywhere; DB-backed and integration tests behind `TEST_DB_DSN`. |
| `webapp/` | **A1–A6 built** (`/events`, event setup, `/routes`, `/tasks`, `/vehicles`, `/monitor`). A7–A8 render `ComingSoonPage` so the sidebar never dead-ends. |
| `microapp/` | **Not scaffolded.** Nothing exists yet; start from `docs/plans/2026-07-24-microapp.md`. |

`webapp/README.md` and `backend/README.md` are kept current and are the fastest way in — read the component
README before its plan.

```
docs/specs/2026-07-24-wso2-motor-rally-design.md   authoritative design spec (read first)
docs/plans/2026-07-24-milestones.md                build order across the 3 plans
docs/plans/2026-07-24-backend.md                   BE-1..BE-18  (Go)
docs/plans/2026-07-24-webapp.md                    WA-1..WA-12  (organizer React SPA)
docs/plans/2026-07-24-microapp.md                  MA-1..MA-12  (in-car React PWA)
docs/wireframes.html                               A1–A8 (organizer) + B1–B10 (in-car) screens
WSO2 Proposal - Popcornteams.com.pdf               original source proposal
```

**The plan checkboxes lag the code.** Most of what is built is still ticked `- [ ]`, so treat the plans as
the specification of *what* a task means, and the code plus the component READMEs as the record of what is
done. Check the tree before assuming a step is outstanding.

Reference implementation for all conventions: `wso2-open-operations/cs-tools/apps/customer-portal`
(`{backend,webapp,microapp}`). Mirror it, with the three documented deviations below.

## Working the plans

- Plans are task-by-task with `- [ ]` checkboxes and **TDD is mandatory**: write the failing test, run it,
  see it fail, implement, see it pass, commit. Plan steps embed the exact test code and commands.
- Follow the order in `2026-07-24-milestones.md`, not strictly 1→N within one plan. Milestone 1 = organizer
  slice (backend organizer API + whole web app). Milestone 2 = in-car slice (game runtime + micro app).
- `BE-4` creates the **entire** schema (all tables) in Milestone 1 — do not add per-milestone migrations for
  tables the spec already lists. That rule is about not splitting table creation across milestones; a
  *corrective* migration is still fine, and `0002` is one. `BE-5`/`BE-6` also build the team-token path in M1
  though it is only exercised in M2. `BE-11`, `BE-16`, `BE-17` and the integration tests are deliberately
  split across milestones.
- Use `superpowers:subagent-driven-development` or `superpowers:executing-plans` to execute a plan.

## Commands (the microapp ones are valid once it is scaffolded)

Backend (`backend/`, Go 1.23, Makefile):

```bash
make docker-db                                       # local MySQL 8 for dev + repo tests
make migrate-up                                      # go run ./cmd/server -migrate
make run                                             # go run ./cmd/server
make test                                            # go test ./...
go test ./internal/geo/ -run TestHaversine -v        # single test
go test -tags=integration ./... -run TestHappyPath -v # gated on TEST_DB_DSN
```

Web app (`webapp/`, **pnpm**, dev port 3000):

```bash
pnpm dev · pnpm build · pnpm test
pnpm vitest run src/config/apiConfig.test.ts         # single test
```

Micro app (`microapp/`, **npm**):

```bash
npm run dev · npm run build · npm test
npx vitest run src/config/endpoints.test.ts          # single test
```

Package managers are not interchangeable: webapp is pnpm, microapp is npm (matching customer-portal).
Repo-test and integration-test targets skip themselves when `TEST_DB_DSN` is unset.

## Architecture — the parts that span files

**Two identities, one backend.** Organizer requests carry Asgardeo id tokens (JWKS-validated when
`TOKEN_VALIDATOR_ENABLED`, decode-only locally); participant requests carry backend-minted team JWTs
(`iss=rally-team`, `sub=sessionId`) issued by `POST /sessions/bind`. Auth middleware routes by issuer.
A vehicle may hold only one `bound`/`active` session — a second bind is a 409 (the one-active-phone rule).

**The task engine is config-driven, not coded per task.** All 15 tasks are `task` rows with a `TaskType`,
a `trigger`, a `sensor`, and a JSON `config`. The backend has one validator + scorer per type
(`internal/taskengine`); the micro app has one screen shell (`TaskRenderer`) that renders a body by type.
Adding or retuning a task is data, never new code paths.

**Geofencing and scoring are server-side.** The micro app streams `POST /sessions/me/location`; the backend
does point-in-radius math (`internal/geo`) and returns newly-unlocked tasks plus lifecycle events
(rest-lock, precision-radius, timed-trivia, arrival). Submissions are validated against `task.config`
server-side; the client never decides correctness or points. Arrival geofence auto-finishes and locks a session.

**WebSocket `/ws` drives every live view.** Organizers subscribe `event:{id}` (`vehicle_position`,
`task_completed`, `score_delta`, `leaderboard`, `alert`); the micro app subscribes `session:{id}`
(`start_signal` at 09:00, `cipher_reveal`, `rest_lock`, `arrival`). Contract in `api/asyncapi.yaml`.

**Frontends share no code** with each other or the backend — HTTP/WS only, same as customer-portal.

**Wireframe screens map 1:1 to features:** webapp `src/features/{events,routes,tasks,vehicles,monitor,leaderboard,debrief}`
= A1–A8; microapp `src/pages/*` = B1–B10. Keep that mapping when adding screens.

## Deviations from customer-portal (intentional — don't "fix" them)

1. **Backend is Go, not Ballerina.** Mirror the *module structure*, not the language: `internal/<domain>/`
   with `handler.go` (≈ resources in `service.bal`) → `service.go` → `repo.go` (SQL, replaces `client.bal`)
   → `<domain>.go`/`types.go`/`constants.go`/`enums.go`.
2. **Micro app is a standalone PWA**, not embedded in the WSO2 Open Super App. No `microapp-bridge`, no
   `window.nativebridge`, no `ReactNativeWebView`. A `sensors/` module wraps Web APIs (Geolocation,
   DeviceMotion, camera/`BarcodeDetector`, QR, wakeLock) and `services/session.ts` replaces native
   `services/auth.ts`. Every other microapp convention is retained.
3. **The backend owns its own MySQL data** instead of proxying downstream WSO2 services, hence
   `handler → service → repository` rather than `handler → downstream client`.

Web Bluetooth is never used (unsupported on iOS Safari) — Task 8 proximity is QR or geofence.

**WebSocket auth is a subprotocol, not a header or a query parameter.** Both front ends connect with
`new WebSocket(url, ["rally-bearer", token])`: a browser can set no header on a handshake, and a token in
the query string would land in the backend's request log and the browser's history. `middleware.Auth` reads
the entry after the marker; `realtime.Hub` echoes the marker (never the token) back on accept, because
RFC 6455 lets a browser close a connection that agreed on no offered subprotocol. The micro app's
`session:{id}` subscription uses the same mechanism with its team token.

## Conventions that are easy to get wrong

Backend:
- Every non-2xx body is exactly `{"message": "<human sentence>"}` via `httpx`. Never leak internal error
  text; log it with `slog` plus the actor id and return a safe message.
- **No silent fallbacks.** Never swallow a DB/JWKS/parse error to return a zero value.
- All entity ids are 32-char lowercase hex `CHAR(32)`, generated by `store.NewID()`.
- Lists are `POST /<resource>/search` with `{offset, limit, filters}` — default 0/20, max limit 100.
- `GET /health` is unauthenticated and skipped by auth middleware.
- SQL columns `snake_case`; enum string values `snake_case`; Go identifiers `CamelCase`.
- **A timestamp you compute with needs `TIMESTAMP(3)`.** A bare `TIMESTAMP` has no fractional seconds and
  MySQL *rounds* on write, so a value can read back half a second in the future — which is how the
  anti-teleport check on `last_ping_at` came to accept every jump. Display and audit columns stay at
  second resolution.
- Keep a DTO (wire: json tags, string dates, nullable) ↔ model (domain: typed, `time.Time`) split,
  mirroring the microapp's `.dto.ts`/`.model.ts` discipline.

Web app:
- **Runtime** config: `index.html` loads `/config.js` → `window.config`; `public/config.js` is gitignored,
  commit `public/config.js.example`. Keys are `RALLY_*`; `envPrefix: ["RALLY_"]`.
- Every request sends **both** `Authorization: Bearer <idToken>` and `x-user-id-token: <idToken>`.
- Query hooks named `useGetX`/`useSearchX`/`useCreateX`/`useUpdateX`/`useDeleteX`; query keys come from a
  central `ApiQueryKeys` in `constants/apiConstants.ts`.
- Path aliases must be duplicated in *both* `vite.config.ts` and `tsconfig.app.json`.
- ESLint flat config only — **no prettier** here (matches customer-portal webapp). TanStack Query owns
  server state; React Context owns cross-cutting UI state. No zustand, no axios.

Micro app:
- **Build-time** config: `config/endpoints.ts` reads `import.meta.env.RALLY_BACKEND_URL` and *throws* if
  undefined; `WS_URL` is derived `http→ws`. `base: "./"` for arbitrary mount paths.
- axios client with interceptors, zustand session store, `HashRouter`, formik + yup, dayjs.
- Services export TanStack `queryOptions`/`mutationOptions` and hold the `toX(dto): Model` mappers.
- On 401: clear the session store and route to `#/` (re-bind). Never retry silently forever.
- Prettier **is** used here, `printWidth: 120`.
- Feature-detect and fall back for iOS Safari: `@zxing/browser` when `BarcodeDetector` is absent,
  gesture-triggered `DeviceMotionEvent.requestPermission()`, and manual code entry always available.

Both frontends + backend: Apache-2.0 header (year 2026, `Copyright (c) 2026 WSO2 LLC.`) on every source file.
Maps are `react-leaflet` + OpenStreetMap tiles (no API key). Deployment target is Choreo — the gateway owns
TLS, CORS, and organizer token validation, so app-level CORS is dev-only, and `.choreo/component.yaml`
declares the REST + WS endpoints.

## Scope guardrails (MVP)

In scope: the happy path end-to-end. Explicitly **out** of scope: offline resilience, hardened anti-cheat /
GPS-spoof prevention (the server validates submissions but trusts client GPS), load-proven 150-device scale,
native mobile app, real BLE beacons, i18n, video transcoding, payment. Two routes (Inland, Wetlands) and
~150 vehicles are configuration, never hard-coded.
