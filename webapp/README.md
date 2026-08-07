<!--
 Copyright (c) 2026 WSO2 LLC. (https://www.wso2.com).

 WSO2 LLC. licenses this file to you under the Apache License,
 Version 2.0 (the "License"); you may not use this file except
 in compliance with the License.
 You may obtain a copy of the License at

 http://www.apache.org/licenses/LICENSE-2.0

 Unless required by applicable law or agreed to in writing,
 software distributed under the License is distributed on an
 "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 KIND, either express or implied.  See the License for the
 specific language governing permissions and limitations
 under the License.
-->

# Rally Ops — organizer web app

The organizer portal for WSO2 Motor Rally 2027. React 19 SPA talking to the Go
backend in [`../backend`](../backend) over REST, and (in later increments) over
WebSocket for the live monitor and leaderboard.

Conventions mirror
[`cs-tools/apps/customer-portal/webapp`](https://github.com/wso2-open-operations/cs-tools):
runtime `window.config`, Asgardeo auth, TanStack Query over a `fetch` client,
`react-router` 7, `@wso2/oxygen-ui`, feature-based `src/features/*`, ESLint flat
config and **no prettier**.

## What is built

| Screen | Route | Status |
| ------ | ----- | ------ |
| A1 Events dashboard | `/events` | ✅ |
| A2 Event setup | `/events/new`, `/events/:eventId/setup` | ✅ |
| A3 Routes & geofences | `/routes` | placeholder |
| A4 Task library | `/tasks` | placeholder |
| A5 Vehicles & crews | `/vehicles` | placeholder |
| A6 Live monitor | `/monitor` | placeholder |
| A7 Leaderboard | `/leaderboard` | placeholder |
| A8 Debrief | `/debrief` | placeholder |

The sidebar lists all seven features so navigation matches the product shape;
the unbuilt ones render a "not built yet" page rather than a dead 404.

## Prerequisites

- **Node.js 20.19+** (or 22.12+). Vite 7 warns below 20.19; the build still
  runs on 20.17 but is not supported there.
- **pnpm** 10+ — `npm install -g pnpm@10`. The web app uses pnpm; the micro app
  uses npm. They are not interchangeable.
- The backend running locally (`cd ../backend && make docker-db && make run`).

## Getting started

```bash
pnpm install
cp public/config.js.example public/config.js   # then fill in the values
pnpm dev                                        # http://localhost:3000
```

`public/config.js` is gitignored — it is per-environment runtime configuration,
not build input. Every key is documented in `public/config.js.example`; the app
throws on startup if a required one is missing rather than guessing a default.

## Commands

```bash
pnpm dev                                  # dev server on :3000
pnpm build                                # tsc -b && vite build → dist/
pnpm test                                 # vitest, watch mode
pnpm exec vitest run                      # vitest, single run
pnpm exec vitest run src/config           # one directory
pnpm lint                                 # eslint
```

## Deployment

The same `dist/` is promoted across environments; only `config.js` is swapped.
The Choreo gateway owns TLS, CORS and organizer token validation, so app-level
CORS is a dev-only concern.

## Notes

- **jsdom is pinned to 26.x.** jsdom 27 needs `require(esm)`, which lands in
  Node 20.19. Raise it once the team's Node floor moves.
- **`@types` is deliberately not a path alias** — it would shadow the
  DefinitelyTyped scope. Domain types are imported as `@/types/event`.
- Map tiles are OpenStreetMap via `react-leaflet`, with no API key, per the
  design spec.
