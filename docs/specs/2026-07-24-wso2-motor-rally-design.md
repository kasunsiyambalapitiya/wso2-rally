# WSO2 Motor Rally 2027 — Design Spec

**Status:** Approved for planning · **Date:** 2026-07-24 · **Author:** kasunsi@wso2.com
**Wireframes:** https://claude.ai/code/artifact/0aa245be-2e09-4b7d-a678-76bb02d9074e
**Source proposal:** `WSO2 Proposal - Popcornteams.com.pdf` (Popcorn Teams Pvt Ltd)
**Reference application:** `wso2-open-operations/cs-tools/apps/customer-portal`

---

## 1. Overview

A team-building car rally. Vehicles are "data packets", crews are "nodes", the goal is to route
efficiently from a start line to an endpoint (Pearl Bay) in "system sync". **Every crew member runs
the in-car app on their own phone, and the whole car scores as one unit** — a zero-facilitator start,
15 modular tasks (GPS / sensor / puzzle based), live scoring, and a leaderboard projected at the finish.

One member is the **navigator** at any moment: their phone sits in the cradle, drives turn-by-turn
directions, and is the car's GPS — its position is what crosses a geofence. When it does, the unlocked
task appears on **all** the crew's phones, and **whoever answers first answers for the vehicle**. Any
member can take over navigation in one tap, so a dead battery or a driver swap costs nothing.

We build three components, following the `customer-portal` tech stack, standards, and patterns —
**except the backend is Go instead of Ballerina**:

| Component | Role | Stack |
|---|---|---|
| **Backend** | Game engine, data, REST + WebSocket | Go |
| **Web App** | Organizer portal + pavilion leaderboard | React + Oxygen UI + Asgardeo |
| **Micro App** | In-car participant app (one phone per crew member) | React, embedded in the WSO2 Open Super App |

### Goals (this build)

- **MVP prototype.** End-to-end happy path: the crew's phones join a vehicle → geofenced start →
  09:00 sync → the 15 tasks, raced across the car's phones → live scoring → leaderboard → arrival +
  vouchers.
- Mirror customer-portal conventions so the code is familiar to the team and Choreo-deployable.

### Non-goals (explicitly out of scope for the MVP)

- Offline resilience on weak mobile data (best-effort only).
- Hardened anti-cheat / GPS-spoofing prevention (server validates submissions, but trusts client GPS).
- Strong *roster* mapping. Identity itself is now the super app's (an Asgardeo-authenticated WSO2
  person), so impersonating a teammate is no longer the weak point. What is left is the join: the caller
  is matched to a `crew_member` row by email, so a mistyped roster email locks a real participant out
  rather than letting a stranger in. Acceptable — an organizer can fix a roster row in seconds.
- Load-proven scale to 150 concurrent devices (design allows it; not load-tested).
- Native mobile app, real BLE beacons, i18n, video transcoding, payment.

---

## 2. Locked decisions

| Decision | Choice | Consequence |
|---|---|---|
| Scope | MVP prototype | Happy-path first; hardening deferred |
| Micro-app platform | React static build **embedded in the [Open Super App](https://github.com/opensuperapp/opensuperapp)** | Distributed through the super app store, not installed; the host owns identity and the native scanner |
| Micro-app auth | **Super app identity + pick your vehicle** | The host has already signed the person in; `POST /sessions/join` trades its Asgardeo token plus a `vehicleId` for the rally team token. No name picker and no phone digits — those existed only because there was no identity provider |
| Phones per vehicle | **One per crew member, all in one session** | The car is the scoring unit; the one-active-phone rule moves down a level to one active phone per *member* |
| Navigator | **Runtime role, self-serve takeover** | Exactly one navigator per session (DB-enforced); their phone is the car's GPS, and any member can take it over in one tap |
| Task racing | **First submission wins for the vehicle** | Resolved atomically in SQL; a latecomer gets `409` naming the winner rather than overwriting the score |
| Turn-by-turn | **Deep link to the Google Maps app** (via `nativebridge.requestOpenUrl`) | Real navigation with no API key; the micro app keeps `react-leaflet` + OSM for the in-app course view |
| BLE (Task 8) | **QR / geofence checkpoint fallback** | Web Bluetooth is unsupported on iOS Safari; the QR scan uses the super app's native scanner, since a `file://` WebView cannot open a camera |
| Real-time | Backend **WebSocket** | Load-bearing, not decoration: the non-navigator phones learn about unlocks only over the socket |
| Task engine | **Config-driven** task definitions | One task-type registry; one shared micro-app screen shell |
| Vehicle problem state | New scope beyond proposal | `Vehicle.status` (ok / breakdown / device_issue) + organizer alert; surfaced on dashboard + live monitor |

### Deviations from customer-portal (documented on purpose)

1. **Backend language** is Go, not Ballerina — we mirror the *modular structure* and *conventions*,
   not the language.
2. **Backend owns its own data** (MySQL). customer-portal proxies downstream WSO2 services; our
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
  navigator  ───▶│      Go Backend           │───────▶│     MySQL     │
  phone (in super│  chi REST :8080           │        └───────────────┘
  location+motion│  WebSocket hub /ws        │
                 │  task-engine · scoring    │        ┌───────────────┐
                 │  geofence · realtime      │───────▶│ Object store  │ (debrief videos)
                 └───────────────────────────┘        └───────────────┘
                              ▲  REST  (Bearer team token)
                              │  WS    (task_unlocked, task_completed,
                              │         navigator_changed, start signal,
                              │         cipher, rest-lock)
  task phones ×3 ─────────────┘   one vehicle · one session · one score
```

- **Two identities, one backend.** Organizer tokens are Asgardeo id tokens (validated via JWKS on
  Choreo). Team tokens are backend-issued JWTs minted at `POST /sessions/join`. Auth middleware
  routes by issuer.
- **One session, many devices.** All of a vehicle's phones share a single `team_session`; the first
  member to join creates it and the rest find it. Only the navigator's phone reports location, so the
  car has exactly one authoritative position. Every other phone is a task terminal, kept in step by
  the WebSocket.
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
├── docs/specs/               this spec + future specs
├── docs/plans/               implementation + milestone plans
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
│   │   ├── store/                    database/sql (MySQL) pool, migrations, repository interfaces
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
└── microapp/                 React, embedded in the Open Super App (npm, mirrors customer-portal/microapp)
    ├── microapp.json                 super app store manifest (appId, clientId, versions[])
    ├── public/config.js.example      runtime window.config (BACKEND_BASE_URL, IS_MICROAPP, …)
    └── src/{components,config,context,pages,services,store,theme,types,utils}
```

Go module path (placeholder, adjust to the real repo): `github.com/wso2-open-operations/wso2-motor-rally/backend`.

---

## 5. Data model

Entities (MySQL tables; all ids are 32-char lowercase hex `CHAR(32)` to match customer-portal's `IdString`):

- **event** — `id, name, event_date, start_time, status(setup|active|complete), start_label,
  start_lat, start_lng, start_radius_m, end_label, end_lat, end_lng, end_radius_m, created_by, created_on`.
- **route** — `id, event_id, name(Inland|Wetlands), display_order`.
- **waypoint** — `id, route_id, display_order, label, lat, lng, boundary_radius_m`.
  Reorderable (`display_order`); each has 0..n attached tasks via **waypoint_task** (`waypoint_id, task_id, display_order`).
- **task** — `id, event_id, code(T1..T15), title, type(TaskType), trigger(geofence|sensor|choice|manual|timed),
  points, sensor(none|geolocation|devicemotion|camera|qr), config(JSON)`. `config` holds per-type
  parameters (cipher options, arithmetic operands, barcode payload, radius, timer seconds, gate spec, …).
- **vehicle** — `id, event_id, code(PKT-001), team_name, vehicle_type(SUV|Sedan|Van|…), contact_number,
  route_id, status(ok|breakdown|device_issue)`.
- **crew_member** (node) — `id, vehicle_id, name, email, phone_number, role(navigator|node), origin_country`.
  `email` is what a joining phone is matched on: the super app has already authenticated the person, so
  the roster only has to say which car that person is in. It is **NOT NULL** for the same reason
  `phone_number` used to be — a member without one could never join. `phone_number` stays, now purely so
  organizers can call a car that goes quiet. `role` is roster metadata (who is expected to drive); the
  *active* navigator lives on `session_device`.
- **team_session** — `id, event_id, vehicle_id, bound_at, started_at, finished_at,
  current_waypoint_id, total_score, status(bound|active|finished)`. One live session per vehicle,
  enforced by a unique index — which is what guarantees the whole crew lands in the **same** run.
- **session_device** — `id, session_id, crew_member_id, is_navigator, joined_at, last_seen_at`.
  One row per phone in the car. Two unique indexes carry the rules: `(session_id, crew_member_id)`
  gives one active phone per member, and `(session_id, navigator_flag)` — a generated column that is
  `1` for the navigator and `NULL` otherwise — lets MySQL guarantee **exactly one navigator** rather
  than trusting application code to keep count. Re-joining is an upsert, so a rebooted or borrowed
  phone just works.
- **task_submission** — `id, session_id, task_id, waypoint_id, status(pending|completed|skipped),
  payload(JSON), awarded_points, crew_member_id, submitted_at`. Unique on `(session_id, task_id)` —
  the submission belongs to the *vehicle*, and `crew_member_id` records which member won the race.
- **vehicle_alert** — `id, vehicle_id, type(breakdown|device_issue|other), note, source(organizer|crew),
  raised_by, lat, lng, raised_at, resolved_at`. Raised by an organizer (A5/A6) or by the crew from the micro app (B10).
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

**Triggering.** The **navigator's** phone streams location (`POST /sessions/me/location`); no other
phone may. The backend evaluates waypoint geofences server-side (`geo` package) and returns which tasks
are now unlocked, plus rest-lock / precision-radius / timed-trivia events. The same unlock fans out as
`task_unlocked` over the session's WebSocket topic, which is how the other phones — who never ping —
find out there is something to answer. Sensor and manual tasks unlock in the task body.

**Who may answer what.** The driver is driving, so their phone is a sensor, not an input device:

| Restriction | Types | Why |
|---|---|---|
| **Navigator's phone only** | `TELEMATICS`, `GEOFENCE_CROSS`, `PROXIMITY` | Read passively while the phone sits in the cradle. A passenger's pocket accelerometer would score the *car's* driving quality, and a second GPS would give the car two positions. |
| **Any phone in the car** | everything else, including `SCAN_BARCODE` | Needs hands and attention. The driver keeps both on the road. |

`GET /sessions/me/tasks` returns `navigatorOnly` per task so a phone can grey out what it may not touch,
and a wrong-phone submission is a `403` rather than a silently ignored score.

**Validation & scoring are server-side.** `POST /sessions/me/tasks/{taskId}/submit` sends the type's
payload; the backend validates against `task.config`, awards points, updates `team_session.total_score`,
and broadcasts a score/leaderboard delta over WebSocket.

**First submission wins for the vehicle.** Four phones can answer the same task at once, so the winner
is settled in one atomic statement rather than a read-then-write:

```sql
UPDATE task_submission SET status='completed', crew_member_id=?, awarded_points=?, submitted_at=NOW()
 WHERE session_id=? AND task_id=? AND status='pending'
```

Zero affected rows means someone else already won: the latecomer gets `409` naming the winner, and the
score is untouched. This replaces the old single-phone rule that a resubmission *corrected* the total —
with a race in play, letting the second answer overwrite the first would be a scoring bug. Every phone
also receives `task_completed` carrying `completedBy`, so the task closes on all four screens at once.

---

## 7. API surface

REST style mirrors customer-portal: resource paths, `POST /…/search` for lists, error body `{ "message": … }`,
`GET /health` unauthenticated.

**Organizer (Asgardeo token, role-gated):**
- `GET /users/me`
- Events: `POST /events` · `GET /events/{id}` · `PATCH /events/{id}` · `POST /events/search` · `POST /events/{id}/publish`
- Routes/waypoints: `POST /events/{id}/routes` · `GET /events/{id}/routes` · `GET /routes/{id}` · `PATCH /routes/{id}` ·
  `POST /routes/{id}/waypoints` · `PATCH /routes/{id}/waypoints/order` (reorder) · `PATCH /waypoints/{id}` ·
  `DELETE /waypoints/{id}` (renumbers the remainder) · `POST /waypoints/{id}/tasks` (attach task ids)
- Tasks: `POST /events/{id}/tasks` · `GET /tasks/{id}` · `PATCH /tasks/{id}` · `POST /events/{id}/tasks/search`
- Vehicles/crew: `POST /events/{id}/vehicles` · `GET /vehicles/{id}` · `PATCH /vehicles/{id}` ·
  `DELETE /vehicles/{id}` (refused once the vehicle has a session) ·
  `POST /events/{id}/vehicles/search` (`filters: {query, routeId}`) ·
  `POST /events/{id}/vehicles/import` (CSV) · `GET /events/{id}/vehicles/export` (CSV)
- Alerts: `POST /vehicles/{id}/alerts` · `PATCH /alerts/{id}` (resolve)
- Debrief: `POST /events/{id}/debrief-videos` · `POST /events/{id}/debrief-videos/search`
- Read-through for run views: `GET /events/{id}/monitor` (snapshot) · `GET /events/{id}/leaderboard`

**Participant (team token):**
- `POST /sessions/join` — `{ vehicleId }` with the **super app's Asgardeo token** in `Authorization` →
  `{ teamToken, session, device, crew }`. The one endpoint that takes an organizer-issued token and
  returns a team one; everything else under `/sessions/me/*` takes the team token. The crew member is
  resolved from the caller's email, so the phone answers "which car am I in", never "who am I". The
  first member to call it creates the session and becomes navigator; the rest join that same session.
  Named *join*, not *bind*, because it is no longer exclusive — "bind" carried the one-phone-per-vehicle
  rule that this design deliberately removes.
  `403` if the caller is not on that vehicle's roster, `409` if the event is not published, `404` for an
  unknown vehicle.
- `GET /sessions/me` — session state, assigned route, cipher (after 09:00), next waypoint, the full
  `crew` with who is navigating, and `you` (your own device + crew member)
- `POST /sessions/me/location` — **navigator only, else `403`** — `{ lat, lng, accuracy, ts? }` (`ts` is the
  ISO 8601 moment the fix was *taken*; omitted means "now", so a live ping is unchanged and a buffered flush
  from the super app is judged against real elapsed time) →
  `{ unlockedTasks, events:[geofence|rest|trivia|arrival] }`
- `POST /sessions/me/navigator` — take over navigation from whoever holds it → `{ crew }`, and
  broadcasts `navigator_changed`
- `GET /sessions/me/tasks` — task list + statuses, each with `navigatorOnly` and `completedBy` ·
  `GET /tasks/{id}` — definition to render
- `POST /sessions/me/tasks/{taskId}/submit` — type payload → validated + scored result.
  `403` if the task is navigator-only and you are not; `409` naming the winner if the car already answered it.
- `POST /sessions/me/alerts` — `{ type(breakdown|device_issue|other), note?, lat, lng }` → raises a vehicle alert (B10)
  and broadcasts `alert` to organizers (drives A1 ⚠ Alerts + A6 monitor)
- `POST /sessions/me/finish` — (also auto on arrival geofence) → locks score, issues vouchers
- `GET /sessions/me/vouchers`

**WebSocket `/ws`:**
- **Auth:** the token rides in the subprotocol list, `["rally-bearer", "<token>"]`, because a browser can
  set no header on a handshake and a query-string token would land in the request log, the browser history
  and every proxy. The server echoes back only the marker (RFC 6455 requires *an* agreed subprotocol). A
  non-browser client may still send `Authorization: Bearer`, which wins where both are present.
- Organizer subscribes `event:{id}` → receives `vehicle_position`, `task_completed`, `score_delta`,
  `leaderboard`, `alert` messages.
- Every phone in a car subscribes `session:{id}` → receives `task_unlocked` (the navigator crossed a
  geofence — this is the *only* way a non-navigator phone learns of it), `task_completed` with
  `completedBy` (close the task, a teammate got there first), `navigator_changed`, `score_delta`,
  `start_signal` (09:00 sync), `cipher_reveal`, `rest_lock`, `arrival`.
- Contract lives in `api/asyncapi.yaml`; port/path declared in `.choreo/component.yaml`.

---

## 8. Auth & authorization

- **Organizer.** Web app uses `@asgardeo/react` (as customer-portal `AppWithConfig.tsx`). Every request
  sends `Authorization: Bearer <idToken>` + `x-user-id-token`. Backend middleware validates via JWKS when
  `TOKEN_VALIDATOR_ENABLED` (Choreo), else decode-only (local). Claims → `UserInfo{Email,UserID,Groups}`
  in `context.Context`. `CheckRoles(required, groups)` gates organizer/admin actions. Group→role config from env.
- **Participant.** The micro app is embedded in the super app, so the phone already holds an Asgardeo
  access token minted for the rally's `clientId` — it asks for it over the native bridge
  (`nativebridge.requestToken()`), never through a browser redirect. `POST /sessions/join` takes that
  token plus a `vehicleId`, verifies the event is published and that the caller's email is on that
  vehicle's roster, then finds-or-creates the vehicle's session, upserts a `session_device` row, and mints
  a signed team JWT (`iss=rally-team`, `sub=sessionId`, plus `deviceId` and `crewMemberId`). The team
  token is kept in the super app's **secure store** — not `localStorage`, which would outlive the event on
  a shared phone — and sent as `Authorization: Bearer <teamToken>`. Team middleware validates signature +
  session status and resolves the device, so a handler can tell *which* phone is calling. Distinguished
  from organizer tokens by `iss`.
- **⚠ Consequence for the organizer surface.** Every participant now holds a valid Asgardeo token, which
  auth middleware classifies as an *organizer* identity. `RequireOrganizer` therefore has to gate on the
  organizer group, not merely on a decodable token — otherwise a crew member could read the whole
  monitor. Admin-only actions were already group-gated; the read surface was not.
- **One active phone per member, not per vehicle.** The old rule stopped a second phone from binding a
  vehicle. It now applies one level down: `session_device` is unique on `(session_id, crew_member_id)`,
  so Nimal cannot hold two phones, while all four of the crew can hold one each. Racing between a car's
  own phones is the intended behaviour, not a conflict to reject.
- **Exactly one navigator**, enforced by a unique index on a generated column rather than by counting in
  Go. `POST /sessions/me/navigator` moves the role in one transaction: demote the incumbent, promote the
  caller. Any member may take over — a navigator whose battery just died cannot hand anything over.

---

## 9. Geofencing & sensors (inside the super app's WebView)

- **Geolocation** (`navigator.geolocation.watchPosition`) drives start-grid validation (B2), precision radius
  (Task 5), rest lock (Task 12), geofence trivia (Task 14), and arrival auto-logoff (B9). Backend does the
  point-in-radius math (`geo` package) from reported coords. **Only the navigator's phone runs the watch**;
  the others leave the GPS alone, which also spares three batteries.
- **DeviceMotion** for eco-driving telematics (Task 4), again navigator-only — it is measuring the car, not
  a passenger. iOS requires `DeviceMotionEvent.requestPermission()` on a user gesture, so it is requested
  when a phone takes over navigation rather than at task start.
- **Camera → the super app's native scanner.** `nativebridge.requestQr()` returns the decoded string; the
  host takes over the screen for the scan. There is no web camera path: `getUserMedia` is blocked from the
  `file://` origin the micro app is served from, which also rules out `BarcodeDetector` and
  `@zxing/browser`. Manual code entry stays available (wireframe B5) and is now the only fallback.
- **BLE (Task 8)** → QR checkpoint (scan a QR placed at the beacon location) or a geofence checkpoint.
- **Maps** — `react-leaflet` + OpenStreetMap tiles (no API key), for organizer route editing/monitor and the
  micro-app route view.
- **Turn-by-turn** — the navigator's B4 screen carries a *Navigate with Google Maps* button that deep-links
  to the installed app: `https://www.google.com/maps/dir/?api=1&destination=<lat>,<lng>&travelmode=driving`.
  Real voice navigation with no API key, no billing, and no new dependency; the leaflet map stays for course
  context (geofence circles, waypoint order) that Google Maps cannot show.
- **Packaging** — no PWA. The micro app is a static build, zipped and registered in the super app store
  through `microapp.json`; the host downloads, extracts and loads it in a `WebView` from a `file://` path.
  Hence `base:"./"` and `HashRouter`, and no service worker. Keeping the screen awake is the host's job for
  a `fullscreen` micro app — `navigator.wakeLock` is not dependable in this WebView.

- **Location arrives over a new bridge topic**, not from `navigator.geolocation`. The super app grants the
  WebView no location today, so we are adding the capability there: native watches the position and pushes
  `{lat, lng, accuracy, ts, buffered?}` fixes into the micro app, buffering and flushing them across a spell
  in the background. It is a **subscription** (`requestLocationUpdates` → many `resolveLocationUpdate` →
  `requestStopLocationUpdates`) and it is gated on `microapp.json` declaring
  `requiredPermissions: ["location"]`. Chosen over WebView passthrough because a `file://` page is not a
  secure origin on iOS and because a suspended WebView runs no JS — and the rally's own route screen
  deep-links to Google Maps, so the driver leaves the app by design. Plan:
  `docs/plans/2026-08-10-superapp-location.md`. Consequence for this backend: `POST /sessions/me/location`
  takes an optional client `ts` so a flushed fix is judged against when it was taken rather than when it
  arrived (`BE-20`).

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
`queryOptions`/`mutationOptions`, runtime `window.config` (`./config.js`, including `IS_MICROAPP`),
`vite-tsconfig-paths`, prettier (`printWidth 120`). npm. `utils/bridge.ts` + `services/auth.ts` speak to
`window.nativebridge` exactly as customer-portal does; `services/session.ts` is the one rally addition,
trading the super app's token for the team token. `sensors/` is thin: a pluggable position source, motion,
and a `requestQr` wrapper.

**Both** send `Authorization: Bearer <token>`; on 401 refresh/redirect (organizer → Asgardeo sign-in;
participant → re-bind screen). Apache-2.0 header on every source file (year 2026).

---

## 11. Backend conventions (Go, mirroring the Ballerina module split)

- **Router:** `chi`. **WebSocket:** `coder/websocket`. **DB:** MySQL via `database/sql` +
  `go-sql-driver/mysql` (+ `golang-migrate` for schema migrations). **JWT:**
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

Micro app (in-car, embedded, mobile): **B1** Initialization (pick your vehicle — identity comes from the
super app) · **B2** Geofence lock ·
**B3** 09:00 sync + cipher reveal · **B4** Route / next leg · **B5** Camera barcode · **B6** Accelerometer
eco-drive · **B7** Input / multi-select shell · **B8** Rest-stop lock · **B9** Arrival + vouchers ·
**B10** Report vehicle issue (breakdown / device / other, with live location). All B-screens
show SCORE / DONE in the header (hidden on B1).

---

## 14. Testing

- **Backend:** Go `testing` + `httptest` for handlers; unit tests for `geo`, `taskengine` validators/scorers,
  `scoring`; repository tests against a throwaway MySQL (docker) test container. Table-driven tests
  per task type.
- **Web app / micro app:** `vitest` + `@testing-library/react` (as customer-portal). Test task-shell rendering
  by type, the bind flow, geofence-lock gating, and API hooks with a mocked client.
- **Manual E2E:** a scripted happy-path walkthrough (bind → start → one task per type → finish) with mocked
  geolocation, since real GPS/motion can't run in CI.

---

## 15. Open assumptions (flag if wrong before/while planning)

1. **MySQL** as the datastore (Choreo-provisioned or managed; docker for local). The repository interface
   keeps the store swappable. JSON columns hold per-task `config` and submission `payload`.
2. **Leaflet + OpenStreetMap** for maps (no API key). If a Google Maps key is available, we can switch.
3. **Content authoring** (cipher terms, arithmetic operands, crossword grid, trivia, barcode payloads) is
   entered by organizers via the Task library (A4) `config`; no bulk-authoring tooling in the MVP.
4. **Two routes** (Inland, Wetlands) and **~150 vehicles** are configuration, not hard-coded.
5. **Debrief videos** are attached by URL/upload to object storage; no in-app recording or transcoding.

---

## 16. Build sequence (high level — detailed plan follows via writing-plans)

1. **Backend foundation** — repo scaffold, config, `store` + migrations, `httpx`, `authorization` middleware,
   `GET /health`, `internal/geo`.
2. **Backend domain** — events → routes/waypoints → tasks → vehicles/crew → sessions/join → task-engine +
   scoring → alerts → debrief; `realtime` hub last.
3. **Web app** — scaffold (mirror webapp), Asgardeo, config, layouts/guards; then features A1–A5 (setup),
   then A6/A7 (WebSocket run views), then A8.
4. **Micro app** — scaffold (mirror microapp, including the bridge), `utils/bridge.ts` + `services/auth.ts`
   + `services/session.ts`, join (B1),
   geofence lock (B2), start/cipher (B3), route (B4), the task shell + the sensor task variants (B5–B8),
   arrival + vouchers (B9).
5. **Integration + happy-path E2E**, then `openapi.yaml` / `asyncapi.yaml` / `.choreo/component.yaml`.
