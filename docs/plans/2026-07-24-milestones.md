# WSO2 Motor Rally 2027 — Milestone Roadmap

**Date:** 2026-07-24 · **Spec:** `../specs/2026-07-24-wso2-motor-rally-design.md`
**Detailed plans:** `2026-07-24-backend.md` (BE), `2026-07-24-webapp.md` (WA), `2026-07-24-microapp.md` (MA)

This roadmap sequences the three task-level plans into two shippable milestones. It does **not** restate task steps — it says *what to build in what order* and *when each milestone is done*. Follow the referenced tasks (e.g. `BE-7` = backend plan Task 7) in the order listed here rather than strictly 1→N within a single plan.

## Strategy

1. **Milestone 1 — Organizer slice.** Build the backend that the web app needs, then the whole web app. Outcome: an organizer can set up and manage a rally end-to-end.
2. **Milestone 2 — In-car slice.** Build the backend game layer (sessions, geofencing, task engine, participant broadcasts), then the whole micro app. Outcome: crews run the rally and their activity flows live into the organizer's monitor + leaderboard.

**Why this order:** every web-app screen depends only on organizer-authored data (events, routes, tasks, vehicles, alerts, debrief) plus read-only run views. None of that depends on the participant runtime. The in-car experience, by contrast, depends on the full data model already existing. So the organizer slice is a clean, self-contained first delivery, and the in-car slice layers the live game on top.

**Backend build-once note:** `BE-4` creates the *entire* schema (all tables) in Milestone 1, so no migration rework is needed in Milestone 2. A few backend tasks are **split across milestones** — see the split table below.

---

## Milestone 1 — Organizer portal (backend + web app)

**Goal:** An organizer signs in with Asgardeo and can create an event, draw start/end geofences, order route waypoints and attach tasks, author the 15 task definitions, provision vehicles + crews (incl. CSV), raise/resolve vehicle alerts, and open the live-monitor + leaderboard + debrief screens.

### Phase 1A — Backend (organizer API)

Order:
1. `BE-1` Scaffold, config, health
2. `BE-2` httpx (JSON + `{message}` errors)
3. `BE-4` store + MySQL migrations + **full schema (all tables)**
4. `BE-5` authz (organizer JWT + roles; team-token mint built now, exercised in M2)
5. `BE-6` middleware (auth, security headers, logging)
6. `BE-7` events (dashboard/setup API)
7. `BE-8` routes + waypoints (reorder, attach tasks)
8. `BE-9` tasks (the 15 definitions)
9. `BE-10` vehicles + crew + CSV import/export
10. `BE-11` alerts — **organizer** raise/resolve/list (crew-source path lands in M2)
11. `BE-14` scoring + leaderboard + monitor snapshot (read endpoints; return empty until M2 produces sessions)
12. `BE-15` debrief
13. `BE-16a` realtime hub + `event:{id}` subscribe + **alert** broadcast (participant-driven broadcasts deferred to M2)
14. `BE-17a` contracts scaffold: `.choreo/component.yaml`, README, and `openapi.yaml` covering the organizer endpoints above
15. `BE-M1-int` organizer integration test: create event → publish → route + waypoint → task → attach → vehicle (+CSV round-trip) → raise alert → `GET /leaderboard` (empty) all green

### Phase 1B — Web app (organizer portal)

Order: `WA-1` scaffold → `WA-2` auth API client → `WA-3` layouts/guards/sidenav → `WA-4` UI context → `WA-5` events (A1/A2) → `WA-6` routes (A3) → `WA-7` tasks (A4) → `WA-8` vehicles (A5) → `WA-9` live monitor (A6) → `WA-10` leaderboard (A7) → `WA-11` debrief (A8) → `WA-12` finalize/build.

> `WA-1`–`WA-4` (scaffold + plumbing) can run in parallel with Phase 1A once `BE-7` exists to hit. `WA-9`/`WA-10` wire the WebSocket + read endpoints now; the *live participant data* they display begins flowing in Milestone 2 (they show organizer/alert events and empty/seed leaderboard until then).

### Milestone 1 exit criteria (demoable)

- Organizer signs in (Asgardeo) and lands on A1 with real event/vehicle/crew/task counts + the ⚠ Alerts card.
- Full CRUD works against the Go backend for events, routes/waypoints (reorder + attach tasks + per-waypoint radius), tasks, vehicles/crews (CSV import + export).
- Start/end geofences and waypoint radii are drawn on the map and persisted.
- An organizer-raised vehicle alert appears live on A6 over WebSocket.
- A6/A7/A8 render (leaderboard empty, monitor shows provisioned vehicles, debrief attaches a clip).
- `make test` (BE unit + organizer integration) and `pnpm test`/`pnpm build` (WA) all green.

---

## Milestone 2 — In-car experience (backend game layer + micro app)

**Goal:** A crew binds one phone to a vehicle + crew, is held at the start geofence, gets the 09:00 sync + cipher, completes the 15 tasks (with real sensors), reports issues, and finishes at Pearl Bay with vouchers — while their positions, completions, scores, and alerts stream live into the organizer's A6/A7 from Milestone 1.

### Phase 2A — Backend (participant / game runtime)

Order:
1. `BE-3` geo (haversine + point-in-radius)
2. `BE-12` sessions — bind (team token, one-active-phone), state, location ping + server-side geofence eval, crew alert (`source=crew`, reuses `BE-11`)
3. `BE-13` task engine — per-type validators/scorers + `POST /sessions/me/tasks/{id}/submit`
4. `BE-16b` wire participant broadcasts into the hub + `session:{id}` subscribe: `score_delta` + `leaderboard` (on submit), `vehicle_position` + `task_completed` (on ping/submit), `start_signal` + `cipher_reveal` + `rest_lock` + `arrival` (session lifecycle)
5. arrival/vouchers finish flow (part of `BE-12`) + `GET /sessions/me/vouchers`
6. `BE-17b` extend `openapi.yaml` + write `asyncapi.yaml` for the participant + WS surface
7. `BE-18` full happy-path integration test (bind → geofenced unlock → submit → score → leaderboard; second bind → 409)

### Phase 2B — Micro app (in-car PWA)

Order: `MA-1` scaffold PWA/env/theme → `MA-2` axios + team-token + session store/service → `MA-3` sensors module → `MA-4` B1 init/bind → `MA-5` B2 geofence lock → `MA-6` B3 countdown+cipher (WS) → `MA-7` B4 route + stats header + report button → `MA-8` B7 task shell + input tasks → `MA-9` sensor task screens (B5/B6/B8 + proximity/trivia) → `MA-10` B9 arrival+vouchers → `MA-11` B10 report issue → `MA-12` finalize/PWA/build.

> `MA-1`–`MA-3` (scaffold + client + sensors) can run in parallel with Phase 2A once `BE-12` bind exists.

### Milestone 2 exit criteria (demoable)

- Bind a vehicle + crew on B1 → team token issued; a second bind for the same vehicle is rejected (one active phone).
- B2 stays locked until inside the start geofence (real GPS); B3 reveals the cipher on the WS start signal.
- Each `TaskType` renders and submits; the backend validates + scores; the running score updates on B4's header.
- Sensor tasks work on device: camera barcode (with manual fallback), accelerometer telematics, QR proximity, timed trivia, rest-lock.
- Finishing at the Pearl Bay geofence auto-locks the score and issues vouchers (B9); B10 sends an alert with live location.
- The **same activity appears live on the organizer's A6 monitor and A7 leaderboard** from Milestone 1 (positions, completions, scores, alerts).
- `go test -tags=integration` happy-path green; `npm test`/`npm run build` (MA) green.

---

## Backend tasks split across milestones

| Backend task | M1 (organizer) | M2 (in-car) |
|---|---|---|
| `BE-11` alerts | organizer raise/resolve/list | crew-source path via sessions |
| `BE-16` realtime | hub + `event:` subscribe + `alert` broadcast | `session:` subscribe + participant broadcasts (position/score/leaderboard/lifecycle) |
| `BE-17` contracts | choreo + README + organizer OpenAPI | participant OpenAPI + AsyncAPI |
| integration | `BE-M1-int` organizer flow | `BE-18` full happy-path |

All other backend tasks belong wholly to the milestone listed in the phase order above. `BE-4` (full schema) and `BE-5`/`BE-6` (auth incl. team-token mint) are built in M1 even though parts are only exercised in M2 — this avoids migration/auth rework.

## Sequencing summary (one line)

`BE foundation+organizer (1A)` → `Web app (1B)` → **Milestone 1 ship** → `BE game layer (2A)` → `Micro app (2B)` → **Milestone 2 ship**. Front-end scaffolds may start in parallel with their milestone's backend phase as noted.
