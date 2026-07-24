# WSO2 Motor Rally 2027 — Design Spec

**Status:** Approved for planning · **Date:** 2026-07-24 · **Author:** kasunsi@wso2.com
**Wireframes:** https://claude.ai/code/artifact/0aa245be-2e09-4b7d-a678-76bb02d9074e
**Source proposal:** `WSO2 Proposal - Popcornteams.com.pdf` (Popcorn Teams Pvt Ltd)
**Reference application:** `wso2-open-operations/cs-tools/apps/customer-portal`

---

## 1. Overview

A team-building car rally. Vehicles are "data packets", crews are "nodes", the goal is to route
efficiently from a start line to an endpoint (Pearl Bay) in "system sync". Each vehicle carries
**one active phone** running an in-car app that drives a zero-facilitator start, 15 modular tasks
(GPS / sensor / puzzle based), live scoring, and a leaderboard projected at the finish.

We build three components, following the `customer-portal` tech stack, standards, and patterns —
**except the backend is Go instead of Ballerina**:

| Component | Role | Stack |
|---|---|---|
| **Backend** | Game engine, data, REST + WebSocket | Go |
| **Web App** | Organizer portal + pavilion leaderboard | React + Oxygen UI + Asgardeo |
| **Micro App** | In-car participant PWA (one active phone) | React PWA + Web sensor APIs |

### Goals (this build)

- **MVP prototype.** End-to-end happy path: vehicle/crew bind → geofenced start → 09:00 sync →
  the 15 tasks → live scoring → leaderboard → arrival + vouchers.
- Mirror customer-portal conventions so the code is familiar to the team and Choreo-deployable.

### Non-goals (explicitly out of scope for the MVP)

- Offline resilience on weak mobile data (best-effort only).
- Hardened anti-cheat / GPS-spoofing prevention (server validates submissions, but trusts client GPS).
- Load-proven scale to 150 concurrent devices (design allows it; not load-tested).
- Native mobile app, real BLE beacons, i18n, video transcoding, payment.

---

## 2. Locked decisions

| Decision | Choice | Consequence |
|---|---|---|
| Scope | MVP prototype | Happy-path first; hardening deferred |
| Micro-app platform | React **PWA** + Web APIs | Geolocation, DeviceMotion, camera/barcode; installable |
| Micro-app auth | **Vehicle ID + crew dropdown** | Device binds a team session, holds a backend-issued team token; no per-user login |
| BLE (Task 8) | **QR / geofence checkpoint fallback** | Web Bluetooth is unsupported on iOS Safari |
| Real-time | Backend **WebSocket** | Live monitor, leaderboard, 09:00 sync, cipher reveal, rest-lock |
| Task engine | **Config-driven** task definitions | One task-type registry; one shared micro-app screen shell |
| Vehicle problem state | New scope beyond proposal | `Vehicle.status` (ok / breakdown / device_issue) + organizer alert; surfaced on dashboard + live monitor |

### Deviations from customer-portal (documented on purpose)

1. **Backend language** is Go, not Ballerina — we mirror the *modular structure* and *conventions*,
   not the language.
2. **Micro app is a standalone PWA**, not embedded in the WSO2 Open Super App. There is no
   `microapp-bridge` / `window.nativebridge`. Token comes from `POST /sessions/bind` instead of the
   native `getToken()`; sensors come from Web APIs instead of native bridges. Every other microapp
   convention (zustand, axios client, `HashRouter`, `.dto.ts`/`.model.ts` split, `services/` with
   TanStack `queryOptions`, build-time env, MUI + Oxygen) is retained.
3. **Backend owns its own data** (Postgres). customer-portal proxies downstream WSO2 services; our
   domain packages are `handler → service → repository` rather than `handler → downstream client`.

---

## 3. Architecture

```
                 ┌───────────────────────────┐
   Organizer ───▶│  Web App (React, Oxygen)  │──── Asgardeo login (id token)
   (browser)     │  window.config runtime    │
                 └────────────┬──────────────┘
                              │  REST  (Bearer id token + x-user-id-token)
                              │  WS    (monitor, leaderboard)
                 ┌────────────▼──────────────┐        ┌───────────────┐
   In-car    ───▶│      Go Backend           │───────▶│  PostgreSQL   │
   phone (PWA)   │  chi REST :8080           │        └───────────────┘
                 │  WebSocket hub /ws        │
                 │  task-engine · scoring    │        ┌───────────────┐
                 │  geofence · realtime      │───────▶│ Object store  │ (debrief videos)
                 └───────────────────────────┘        └───────────────┘
                              ▲  REST  (Bearer team token)
                              │  WS    (start signal, cipher, rest-lock)
   In-car phone (PWA) ────────┘
```

- **Two identities, one backend.** Organizer tokens are Asgardeo id tokens (validated via JWKS on
  Choreo). Team tokens are backend-issued JWTs minted at `POST /sessions/bind`. Auth middleware
  routes by issuer.
- **Frontends only talk HTTP/WS.** No shared code across the three components (same as customer-portal).
- **Choreo deployment.** The gateway handles TLS, CORS, and Asgardeo token validation for organizer
  routes; app-level CORS is dev-only. A `.choreo/component.yaml` declares the REST + WS endpoints.

---

## 4. Repository / project structure

Mirrors `apps/customer-portal/{backend,webapp,microapp}`, promoted to the repo root:

```
rally2026/
├── README.md                 Apache-2.0 header; setup for all three components
├── LICENSE                   Apache-2.0
├── docs/superpowers/specs/   this spec + future specs
├── backend/                  Go
│   ├── cmd/server/main.go            listener, router, middleware wiring  (≈ service.bal)
│   ├── internal/
│   │   ├── config/                   env config struct + load/validate
│   │   ├── httpx/                    error responses {message}, JSON helpers, request-id
│   │   ├── middleware/               auth (organizer JWT + team token), logging, CORS(dev)
│   │   ├── authorization/            JWT validate/decode (JWKS toggle), UserInfo in context, CheckRoles
│   │   ├── realtime/                 WebSocket hub, topics per event, broadcast
│   │   ├── geo/                      haversine, point-in-radius geofence math
│   │   ├── taskengine/               task-type registry + per-type validators + scoring
│   │   ├── store/                    pgx pool, migrations, repository interfaces
│   │   ├── events/                   handler.go · service.go · repo.go · types.go · constants.go
│   │   ├── routes/                   routes + waypoints (+ reorder, + attach task ids)
│   │   ├── tasks/                    task definitions (the 15)
│   │   ├── vehicles/                 vehicles + crew (nodes) + CSV import/export
│   │   ├── alerts/                   vehicle problem-state alerts
│   │   ├── sessions/                 bind, state, location ping, submit, finish, vouchers
│   │   ├── scoring/                  score aggregation + leaderboard view
│   │   └── debrief/                  video attachments
│   ├── migrations/                   SQL migrations (golang-migrate)
│   ├── api/openapi.yaml              REST contract (Choreo API management)
│   ├── api/asyncapi.yaml             WS contract
│   ├── .choreo/component.yaml        REST + WS endpoints
│   └── config.example.env / Config.example.toml
├── webapp/                   React organizer portal  (pnpm, mirrors customer-portal/webapp)
│   ├── public/config.js.example      runtime window.config (RALLY_* keys)
│   └── src/{api,components,config,constants,context,features,hooks,layouts,providers,types,utils}
└── microapp/                 React PWA  (npm, mirrors customer-portal/microapp minus native bridge)
    ├── .env.example                  build-time RALLY_* env
    └── src/{components,config,context,pages,services,store,theme,types,utils}
```

Go module path (placeholder, adjust to the real repo): `github.com/wso2-open-operations/wso2-motor-rally/backend`.

---

## 5. Data model

Entities (PostgreSQL tables; all ids are 32-char lowercase hex to match customer-portal's `IdString`):

- **event** — `id, name, event_date, start_time, status(setup|active|complete), start_label,
  start_lat, start_lng, start_radius_m, end_label, end_lat, end_lng, end_radius_m, created_by, created_on`.
- **route** — `id, event_id, name(Inland|Wetlands), display_order`.
- **waypoint** — `id, route_id, display_order, label, lat, lng, boundary_radius_m`.
  Reorderable (`display_order`); each has 0..n attached tasks via **waypoint_task** (`waypoint_id, task_id, display_order`).
- **task** — `id, event_id, code(T1..T15), title, type(TaskType), trigger(geofence|sensor|choice|manual|timed),
  points, sensor(none|geolocation|devicemotion|camera|qr), config(jsonb)`. `config` holds per-type
  parameters (cipher options, arithmetic operands, barcode payload, radius, timer seconds, gate spec, …).
- **vehicle** — `id, event_id, code(PKT-001), team_name, vehicle_type(SUV|Sedan|Van|…), contact_number,
  route_id, status(ok|breakdown|device_issue)`.
- **crew_member** (node) — `id, vehicle_id, name, role(navigator|node), origin_country`.
- **team_session** — `id, event_id, vehicle_id, bound_at, started_at, finished_at,
  current_waypoint_id, total_score, status(bound|active|finished)`. One active session per vehicle.
- **task_submission** — `id, session_id, task_id, waypoint_id, status(pending|completed|skipped),
  payload(jsonb), awarded_points, submitted_at`. Unique on `(session_id, task_id)`.
- **vehicle_alert** — `id, vehicle_id, type(breakdown|device_issue), note, raised_by, raised_at, resolved_at`.
- **debrief_video** — `id, event_id, vehicle_id, day, object_key, uploaded_at`.
- **voucher** — `id, session_id, entry_code, locker_id, lunch_passes`.

Derived (not stored): **leaderboard entry** = `rank, team, vehicle_code, total_score, finish_time`
(sum of `awarded_points`, tiebreak by earliest `finished_at`).

Backend keeps a **DTO ↔ model** discipline analogous to the microapp's `.dto.ts`/`.model.ts`:
wire structs (json tags, nullable, string dates) vs. domain structs (typed, `time.Time`).

---

## 6. Task engine (the 15 tasks)

Config-driven. Every task has a **TaskType**; the backend has one validator + scorer per type, and the
micro app has one screen shell that renders the body by type. Adding/retuning a task = data, not code.

| # | Task | TaskType | Trigger | Sensor | Scoring |
|---|---|---|---|---|---|
| 1 | Translation Cipher | `INPUT_SELECT` | geofence | — | exact match |
| 2 | Signpost Arithmetic | `INPUT_NUMBER` | geofence | — | exact match |
| 3 | Milestone Digit Scan | `INPUT_NUMBER` | geofence | — | exact match |
| 4 | Eco-Driving Telematics | `TELEMATICS` | sensor | DeviceMotion | scaled efficiency index |
| 5 | Precision Radius | `GEOFENCE_CROSS` | geofence | Geolocation | auto on crossing 15 m |
| 6 | Barcode Scan | `SCAN_BARCODE` | manual | Camera | payload match |
| 7 | Architectural Crossword | `GRID_FILL` | geofence | — | cells correct |
| 8 | BLE Proximity Ping | `PROXIMITY` | geofence/QR | QR (fallback) | checkpoint hit |
| 9 | Time-Locked Blind Count | `BLIND_TIMER` | manual | — | accuracy vs 45 s |
| 10 | Cultural Multi-Select | `MULTI_SELECT` | geofence | — | set match |
| 11 | Dynamic Route Select | `BRANCH` | choice | — | ± points by branch |
| 12 | Mandatory Static Rest | `REST_LOCK` | geofence | Geolocation | 0 (compliance lock) |
| 13 | Odometer Calibration | `INPUT_NUMBER` | manual | — | within tolerance |
| 14 | Geofence Trivia | `TIMED_TRIVIA` | geofence | Geolocation | answer before 30 s |
| 15 | Sequence Gate Match | `GATE_MATCH` | geofence | — | correct connector order |

**Triggering.** The micro app streams location (`POST /sessions/me/location`). The backend evaluates
waypoint geofences server-side (`geo` package) and returns which tasks are now unlocked, plus rest-lock /
precision-radius / timed-trivia events. Sensor and manual tasks unlock in the task body.

**Validation & scoring are server-side.** `POST /sessions/me/tasks/{taskId}/submit` sends the type's
payload; the backend validates against `task.config`, writes `task_submission`, awards points, updates
`team_session.total_score`, and broadcasts a score/leaderboard delta over WebSocket.

---

## 7. API surface

REST style mirrors customer-portal: resource paths, `POST /…/search` for lists, error body `{ "message": … }`,
`GET /health` unauthenticated.

**Organizer (Asgardeo token, role-gated):**
- `GET /users/me`
- Events: `POST /events` · `GET /events/{id}` · `PATCH /events/{id}` · `POST /events/search` · `POST /events/{id}/publish`
- Routes/waypoints: `POST /events/{id}/routes` · `PATCH /routes/{id}` · `POST /routes/{id}/waypoints` ·
  `PATCH /routes/{id}/waypoints/order` (reorder) · `PATCH /waypoints/{id}` · `POST /waypoints/{id}/tasks` (attach task ids)
- Tasks: `POST /events/{id}/tasks` · `GET /tasks/{id}` · `PATCH /tasks/{id}` · `POST /events/{id}/tasks/search`
- Vehicles/crew: `POST /events/{id}/vehicles` · `PATCH /vehicles/{id}` · `POST /events/{id}/vehicles/search` ·
  `POST /events/{id}/vehicles/import` (CSV) · `GET /events/{id}/vehicles/export` (CSV)
- Alerts: `POST /vehicles/{id}/alerts` · `PATCH /alerts/{id}` (resolve)
- Debrief: `POST /events/{id}/debrief-videos` · `POST /events/{id}/debrief-videos/search`
- Read-through for run views: `GET /events/{id}/monitor` (snapshot) · `GET /events/{id}/leaderboard`

**Participant (team token):**
- `POST /sessions/bind` — `{ vehicleId, crew:[...] }` → `{ teamToken, session }` (zero-facilitator start)
- `GET /sessions/me` — session state, assigned route, cipher (after 09:00), next waypoint
- `POST /sessions/me/location` — `{ lat, lng, accuracy, ts }` → `{ unlockedTasks, events:[geofence|rest|trivia|arrival] }`
- `GET /sessions/me/tasks` — task list + statuses · `GET /tasks/{id}` — definition to render
- `POST /sessions/me/tasks/{taskId}/submit` — type payload → validated + scored result
- `POST /sessions/me/finish` — (also auto on arrival geofence) → locks score, issues vouchers
- `GET /sessions/me/vouchers`

**WebSocket `/ws`:**
- Organizer subscribes `event:{id}` → receives `vehicle_position`, `task_completed`, `score_delta`,
  `leaderboard`, `alert` messages.
- Micro app subscribes `session:{id}` → receives `start_signal` (09:00 sync), `cipher_reveal`,
  `rest_lock`, `arrival`.
- Contract lives in `api/asyncapi.yaml`; port/path declared in `.choreo/component.yaml`.

---

## 8. Auth & authorization

- **Organizer.** Web app uses `@asgardeo/react` (as customer-portal `AppWithConfig.tsx`). Every request
  sends `Authorization: Bearer <idToken>` + `x-user-id-token`. Backend middleware validates via JWKS when
  `TOKEN_VALIDATOR_ENABLED` (Choreo), else decode-only (local). Claims → `UserInfo{Email,UserID,Groups}`
  in `context.Context`. `CheckRoles(required, groups)` gates organizer/admin actions. Group→role config from env.
- **Participant.** No Asgardeo. `POST /sessions/bind` verifies the vehicle exists and is unbound, then mints
  a signed team JWT (`iss=rally-team`, `sub=sessionId`, `vehicleId`) stored in the micro app's localStorage
  and sent as `Authorization: Bearer <teamToken>`. Team middleware validates signature + session status.
  Distinguished from organizer tokens by `iss`.
- **One active phone.** A vehicle has at most one `active`/`bound` session; a second bind is rejected (or
  supersedes with an organizer override) — enforces the one-active-phone policy.

---

## 9. Geofencing & sensors (PWA)

- **Geolocation** (`navigator.geolocation.watchPosition`) drives start-grid validation (B2), precision radius
  (Task 5), rest lock (Task 12), geofence trivia (Task 14), and arrival auto-logoff (B9). Backend does the
  point-in-radius math (`geo` package) from reported coords.
- **DeviceMotion** for eco-driving telematics (Task 4). iOS requires `DeviceMotionEvent.requestPermission()`
  on a user gesture — requested at task start.
- **Camera** via `getUserMedia` + `BarcodeDetector` (Chrome/Android). iOS Safari lacks `BarcodeDetector` →
  fall back to `@zxing/browser`, and manual code entry is always available (wireframe B5).
- **BLE (Task 8)** → QR checkpoint (scan a QR placed at the beacon location) or a geofence checkpoint.
- **Maps** — `react-leaflet` + OpenStreetMap tiles (no API key), for organizer route editing/monitor and the
  micro-app route view.
- **PWA** — `vite-plugin-pwa` adds a manifest + service worker for install + basic asset caching. Screen wake
  lock (`navigator.wakeLock`) keeps the in-car screen on.

---

## 10. Frontend conventions (both apps mirror customer-portal)

**Web app** — React 19 + React Compiler, Vite, TS strict, `@wso2/oxygen-ui`, `@tanstack/react-query`,
`react-router` 7 (`BrowserRouter`), `@asgardeo/react` + `@asgardeo/react-router`, native `fetch` via a
`useAuthApiClient` hook, `ApiError` class, context providers (error/success banners, loader), runtime
`window.config` (`RALLY_*` keys, `public/config.js`), feature-based `src/features/*`, path aliases in both
`vite.config.ts` and `tsconfig.app.json`, `slog`-style `Logger`, vitest + testing-library. pnpm. No prettier
(ESLint flat only), matching the webapp.

**Micro app** — React 19, Vite, TS, MUI + `@wso2/oxygen-ui`, `@tanstack/react-query`, `react-router-dom`
(`HashRouter`), **axios** client with request/response interceptors, **zustand** store, **formik + yup**,
`dayjs`, `.dto.ts`/`.model.ts` split with `toX(dto): Model` mappers in `services/`, services export TanStack
`queryOptions`/`mutationOptions`, build-time env (`RALLY_*`, `import.meta.env`), `vite-tsconfig-paths`, prettier
(`printWidth 120`). npm. **Replaces** `microapp-bridge` with a `sensors/` module (geolocation, motion, camera,
qr, wakelock) and `services/session.ts` (bind + team token) in place of the native `auth.ts`.

**Both** send `Authorization: Bearer <token>`; on 401 refresh/redirect (organizer → Asgardeo sign-in;
participant → re-bind screen). Apache-2.0 header on every source file (year 2026).

---

## 11. Backend conventions (Go, mirroring the Ballerina module split)

- **Router:** `chi`. **WebSocket:** `coder/websocket`. **DB:** `pgx` + `golang-migrate`. **JWT:**
  `golang-jwt/jwt v5` + JWKS via `MicahParks/keyfunc`. **Logging:** stdlib `log/slog` (DEBUG/INFO/WARN/ERROR,
  mirrors customer-portal `Logger`). **Config:** env-based `config` struct that validates required keys and
  errors clearly on missing (mirrors `apiConfig.ts`/`authConfig.ts` throwing).
- **Package shape per domain** mirrors `modules/<name>/`: `handler.go` (HTTP resources, typed responses,
  `{message}` errors) ≈ resources in `service.bal`; `service.go` (business logic) ; `repo.go` (SQL) replaces
  the downstream `client.bal`; `types.go` (structs + json tags + validation) ≈ `types.bal`; `constants.go`
  ≈ `constants.bal`; `enums.go` for typed string consts ≈ `enums.bal`.
- **Middleware = interceptors.** Auth middleware ≈ `JwtInterceptor`; a response middleware sets
  `X-Content-Type-Options: nosniff`, HSTS, CSP (≈ `ResponseInterceptor`); an error middleware maps
  binding/validation failures to `400 {message}` (≈ `ErrorInterceptor`). `GET /health` skips auth.
- **Errors:** central `httpx` helpers return `{ "message": … }`; error strings centralized in `constants.go`
  per package. No silent fallbacks — every downstream/db error is logged with the actor id and surfaced.

---

## 12. Scoring

- Points per task from `task.points`; `TELEMATICS` scaled from the efficiency index; `BRANCH` is ± by branch;
  `REST_LOCK` awards 0 (compliance). `BLIND_TIMER`, `GEOFENCE_CROSS`, `INPUT_NUMBER` (odometer) score by
  accuracy/tolerance.
- `team_session.total_score` is the sum of `task_submission.awarded_points`. Leaderboard rank orders by score
  desc, then earliest `finished_at`. Arrival geofence auto-finishes the session and **locks** further scoring.
- Every score change broadcasts a `score_delta` + recomputed `leaderboard` over WebSocket to organizer
  subscribers (drives A6 Live Monitor and A7 Leaderboard).

---

## 13. Wireframe → screen map

Web app (organizer, desktop): **A1** Events dashboard (edit active only; View for completed; ⚠ Alerts card) ·
**A2** Event setup (start/end location + boundary radii; 09:00 auto-start; start-grid geofence) ·
**A3** Routes & geofences (reorderable waypoints, per-waypoint boundary radius, attach task ids) ·
**A4** Task library (15 tasks, per-row Edit) · **A5** Vehicles & crews (contact, type; icon-only CSV
import/export) · **A6** Live monitor (WebSocket) · **A7** Leaderboard (pavilion present mode) · **A8** Video debrief.

Micro app (in-car PWA, mobile): **B1** Initialization (vehicle + crew dropdowns) · **B2** Geofence lock ·
**B3** 09:00 sync + cipher reveal · **B4** Route / next leg · **B5** Camera barcode · **B6** Accelerometer
eco-drive · **B7** Input / multi-select shell · **B8** Rest-stop lock · **B9** Arrival + vouchers. All B-screens
show SCORE / DONE in the header (hidden on B1).

---

## 14. Testing

- **Backend:** Go `testing` + `httptest` for handlers; unit tests for `geo`, `taskengine` validators/scorers,
  `scoring`; repository tests against a throwaway Postgres (docker) or `pgx` test container. Table-driven tests
  per task type.
- **Web app / micro app:** `vitest` + `@testing-library/react` (as customer-portal). Test task-shell rendering
  by type, the bind flow, geofence-lock gating, and API hooks with a mocked client.
- **Manual E2E:** a scripted happy-path walkthrough (bind → start → one task per type → finish) with mocked
  geolocation, since real GPS/motion can't run in CI.

---

## 15. Open assumptions (flag if wrong before/while planning)

1. **PostgreSQL** as the datastore (Choreo-provisioned; docker for local). Alternative for a lighter MVP is
   SQLite — repository interface keeps it swappable.
2. **Leaflet + OpenStreetMap** for maps (no API key). If a Google Maps key is available, we can switch.
3. **Content authoring** (cipher terms, arithmetic operands, crossword grid, trivia, barcode payloads) is
   entered by organizers via the Task library (A4) `config`; no bulk-authoring tooling in the MVP.
4. **Two routes** (Inland, Wetlands) and **~150 vehicles** are configuration, not hard-coded.
5. **Debrief videos** are attached by URL/upload to object storage; no in-app recording or transcoding.

---

## 16. Build sequence (high level — detailed plan follows via writing-plans)

1. **Backend foundation** — repo scaffold, config, `store` + migrations, `httpx`, `authorization` middleware,
   `GET /health`, `internal/geo`.
2. **Backend domain** — events → routes/waypoints → tasks → vehicles/crew → sessions/bind → task-engine +
   scoring → alerts → debrief; `realtime` hub last.
3. **Web app** — scaffold (mirror webapp), Asgardeo, config, layouts/guards; then features A1–A5 (setup),
   then A6/A7 (WebSocket run views), then A8.
4. **Micro app** — scaffold (mirror microapp minus bridge), `sensors/` + `services/session.ts`, bind (B1),
   geofence lock (B2), start/cipher (B3), route (B4), the task shell + the sensor task variants (B5–B8),
   arrival + vouchers (B9).
5. **Integration + happy-path E2E**, then `openapi.yaml` / `asyncapi.yaml` / `.choreo/component.yaml`.
