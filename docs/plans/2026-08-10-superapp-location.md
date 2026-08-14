# Location for Embedded Micro Apps — Super App Plan (Plan 4 of 4)

> **Repo:** this plan is executed in [`opensuperapp/opensuperapp`](https://github.com/opensuperapp/opensuperapp),
> not in `wso2-rally`. It lives here because the rally is what needs it and the rally cannot ship without it.
> The rally-side consumption is `MA-3` in `docs/plans/2026-07-24-microapp.md`.

**Goal:** give an embedded micro app a stream of position fixes that keeps arriving while the phone is in a
cradle with the screen off and the driver is looking at Google Maps — and do it generically, so it is a super
app capability rather than a rally special case.

**Why it blocks us.** WSO2 Motor Rally is a geofence engine. `POST /sessions/me/location` is what unlocks
tasks (B2 start lock, Task 5 precision radius, Task 12 rest lock, Task 14 trivia) and what auto-finishes a
run at Pearl Bay (B9). Today the super app grants the WebView no location at all:

| What | State | Consequence |
| --- | --- | --- |
| Android permissions (`app.config.ts`) | `CAMERA`, `RECORD_AUDIO`, `POST_NOTIFICATIONS` | no location permission is even declared |
| iOS `UIBackgroundModes` | `["remote-notification"]` | no background location |
| `WebView` props (`app/micro-app.tsx`) | no `geolocationEnabled` | `navigator.geolocation` is off on Android (the prop is Android-only and defaults false) |
| Bridge topics (`utils/bridge.ts`) | no location topic | no native fix stream |
| Lifecycle | the WebView suspends when the app backgrounds | JS stops running, so any web-API watch dies |

---

## 1. Evaluation

| # | Approach | Works from `file://`? | Survives backgrounding? | Coupling | Verdict |
| --- | --- | --- | --- | --- | --- |
| **A** | Turn on WebView passthrough: `geolocationEnabled` + OS permission, micro app uses `navigator.geolocation` | Android: yes. **iOS: no** — WKWebView gates geolocation on a secure origin and a `file://` page is not one. *Verify on device before trusting any claim here either way.* | **No.** The WebView's JS is suspended, so `watchPosition` stops firing | None | **Rejected as the primary.** Half the fleet is iPhones, and the failure is silent — a phone that reports nothing looks exactly like a parked car |
| **B** | A **`location` bridge topic**: native watches position and pushes fixes into the WebView | **Yes** — no web API and no origin check is involved | **Yes**, with background permission + a TaskManager task | Generic; matches the existing topic design | **Chosen** |
| **C** | Native posts fixes straight to the rally backend | Yes | Yes | **Bad** — the super app would need rally URLs and the rally's team token, and one micro app's API would leak into the host | Rejected |
| **D** | Native **buffers** fixes while backgrounded and flushes them when the WebView resumes | Yes | Yes, with a delay | None | **Chosen as B's background half** |

**Chosen: B + D.** Native owns the sensor and the lifecycle; the micro app stays a web app that receives
fixes and decides what they mean. A is still worth enabling as a **fallback** for micro apps that only need a
coarse one-shot fix and do not want the bridge, but nothing in the rally depends on it.

### The one new pattern

Every existing topic is one-shot: `requestX()` → the host calls `resolveX(data)` once. Location is a
**subscription**: one request, many callbacks, and an explicit stop. Keep the naming symmetrical with what is
there (`requestLocationUpdates` / `resolveLocationUpdate` / `rejectLocationUpdates` /
`requestStopLocationUpdates`) so it reads like the rest of the bridge, and document that
`resolveLocationUpdate` fires repeatedly — a reader who assumes one-shot will leak a subscription.

### Permission must be per micro app, not global

Granting the host `ACCESS_FINE_LOCATION` would otherwise silently give *every* micro app a location stream.
Add `requiredPermissions: ["location"]` to `microapp.json`, show it on the store page, and have
`micro-app.tsx` refuse `location_start` for an app that has not declared it. That keeps the blast radius of
this change to apps that asked for it.

---

## Task SA-1: Declare the permissions and the Expo plugin

**Files:** `frontend/app.config.ts`, `frontend/package.json`.

- [ ] `npx expo install expo-location expo-task-manager expo-keep-awake`
- [ ] `android.permissions`: add `ACCESS_FINE_LOCATION`, `ACCESS_COARSE_LOCATION`, `FOREGROUND_SERVICE`,
      `FOREGROUND_SERVICE_LOCATION`, and `ACCESS_BACKGROUND_LOCATION`.
- [ ] `ios.infoPlist.UIBackgroundModes`: add `"location"` alongside `"remote-notification"`.
- [ ] `plugins`: add
      ```ts
      ["expo-location", {
        locationAlwaysAndWhenInUsePermission:
          "Allow $(PRODUCT_NAME) to use your location so apps like Motor Rally can track your route.",
        isAndroidBackgroundLocationEnabled: true,
        isAndroidForegroundServiceEnabled: true,
      }]
      ```
- [ ] **Verify:** `npx expo prebuild --clean` and confirm the merged `AndroidManifest.xml` and
      `Info.plist` carry every entry. A missing background mode fails only at runtime, on a real device,
      hours into a rally.

> **Store review note:** background location triggers extra review on both stores and needs a written
> justification plus, on Android, a video of the in-app disclosure. Start that paperwork alongside SA-1, not
> after SA-5 — it is the longest lead time in this plan.

## Task SA-2: `location` topics on the bridge

**Files:** `frontend/utils/bridge.ts`.

- [ ] Add to `TOPIC`:
      ```ts
      LOCATION_START: "location_start",
      LOCATION_STOP: "location_stop",
      ```
- [ ] Add to `injectedJavaScript`, matching the surrounding style:
      ```js
      requestLocationUpdates: (options) => window.ReactNativeWebView.postMessage(JSON.stringify({ topic: "location_start", data: options })),
      requestStopLocationUpdates: () => window.ReactNativeWebView.postMessage(JSON.stringify({ topic: "location_stop" })),
      // Fires repeatedly until requestStopLocationUpdates — a subscription, not a one-shot.
      resolveLocationUpdate: (fix) => console.log("Location fix:", fix),
      rejectLocationUpdates: (err) => console.error("Location updates failed:", err),
      ```
- [ ] **Contract** (document it in `frontend/README.md`, Task SA-6):
      - `options`: `{ accuracy?: "high" | "balanced", distanceIntervalM?: number, timeIntervalMs?: number, background?: boolean }`
      - each fix: `{ lat: number, lng: number, accuracy: number, ts: string /* ISO 8601, when the fix was taken */, buffered?: boolean }`
      - `rejectLocationUpdates(err)` reasons: `"permission_denied"`, `"services_disabled"`,
        `"not_declared"` (the manifest did not request location), `"unavailable"`.

**`ts` is not optional decoration.** A buffered fix flushed after a two-minute gap must carry *when it was
taken*, or the consumer cannot tell a replay from a teleport. The rally backend's anti-teleport check divides
distance by elapsed time; without a real timestamp it would reject the whole flush.

## Task SA-3: Foreground streaming in the micro-app host

**Files:** `frontend/app/micro-app.tsx`, `frontend/types/microApp.types.ts`.

- [ ] Hold the subscription in a ref: `const locationSub = useRef<Location.LocationSubscription | null>(null)`.
- [ ] Handle the topics in `onMessage`, in the existing switch:
      ```ts
      case TOPIC.LOCATION_START:
        await startLocationUpdates(data);
        break;
      case TOPIC.LOCATION_STOP:
        await stopLocationUpdates();
        break;
      ```
- [ ] `startLocationUpdates(options)`:
      1. refuse unless the manifest declares it —
         `sendResponseToWeb("rejectLocationUpdates", "not_declared")`;
      2. `Location.requestForegroundPermissionsAsync()`; on denial send `"permission_denied"` **once** and
         do not re-prompt on every request;
      3. `Location.hasServicesEnabledAsync()` → `"services_disabled"`;
      4. `Location.watchPositionAsync({accuracy, distanceInterval, timeInterval}, fix =>
         sendResponseToWeb("resolveLocationUpdate", {lat, lng, accuracy, ts: new Date(fix.timestamp).toISOString()}))`.
- [ ] `stopLocationUpdates()` removes the subscription and clears the ref.
- [ ] **Clean up on unmount and on navigating away from the micro app.** A leaked watch drains the battery of
      a phone whose owner closed the app hours ago, and it is invisible — nothing on screen says it is running.
- [ ] Keep the screen awake while a `fullscreen` micro app is streaming (`useKeepAwake()`), released on stop.

## Task SA-4: Background updates with buffer-and-flush

**Files:** `frontend/tasks/locationTask.ts` (new), `frontend/app/micro-app.tsx`.

Backgrounding is not an edge case here: the rally's own route screen deep-links to Google Maps, so the
driver leaves the super app *by design* and comes back.

- [ ] `TaskManager.defineTask(LOCATION_TASK, ({data, error}) => …)` — append each fix to a bounded
      `AsyncStorage` ring buffer (cap it; a two-hour gap at 1 fix/5 s is ~1,400 fixes, and an unbounded
      buffer is a slow leak).
- [ ] When `options.background` is set and the permission is granted, also call
      `Location.startLocationUpdatesAsync(LOCATION_TASK, {accuracy, ...,
      foregroundService: {notificationTitle, notificationBody}})`; stop it in `stopLocationUpdates`.
- [ ] On `AppState` → `active`, drain the buffer oldest-first through
      `sendResponseToWeb("resolveLocationUpdate", {...fix, buffered: true})`, then clear it.
- [ ] Deduplicate on `ts` so a fix delivered live *and* buffered is not sent twice.
- [ ] **Test on a device, not a simulator:** start streaming, background the app, drive/walk 500 m, return,
      and assert the consumer received the intermediate fixes with their original timestamps in order.

## Task SA-5: Manifest-declared permission

**Files:** `frontend/types/microApp.types.ts`, the store's `micro_app` table + admin flow, `frontend/README.md`.

- [ ] Add `requiredPermissions?: string[]` to `microapp.json` and to the `micro_app` row.
- [ ] Show declared permissions on the store detail page before install.
- [ ] Enforce it in `startLocationUpdates` (SA-3 step 1).

## Task SA-6: Document the capability

**Files:** `frontend/README.md`.

- [ ] Extend the micro-app guide with the location contract from SA-2, the `requiredPermissions` field, the
      subscription semantics (`resolveLocationUpdate` fires many times), the `ts` guarantee, and the
      buffered-flush behaviour.
- [ ] State the platform caveat plainly: `navigator.geolocation` inside the WebView is **not** the supported
      path for an embedded micro app; the bridge topic is.

---

## Rally-side consequences (tracked in this repo)

1. **`MA-3`** — `sensors/position.ts` already specifies a pluggable `PositionSource`; `bridgePositionSource`
   becomes the primary and `webPositionSource` the fallback. See `docs/plans/2026-07-24-microapp.md`.
2. **`BE-20`** — `POST /sessions/me/location` must accept an optional client `ts` so a buffered flush is
   evaluated against the time each fix was actually taken. Without it, replayed fixes read as teleports and
   the anti-teleport check (`isPlausibleMove`, 60 m/s) discards the lot. Treat a missing `ts` as "now", so
   the live path is unchanged. Note the trade: the plausibility check then relies on a client-supplied
   timestamp, which is consistent with the MVP's existing decision to trust client GPS (spec §1 non-goals).
3. **Ordering** — SA-1..SA-4 must be merged and released in the super app before Milestone 2's exit criteria
   can be demonstrated. Nothing in Milestone 1 depends on them.

## Self-review

**Covers:** the blocking gap recorded in `docs/specs/2026-07-24-wso2-motor-rally-design.md` §9 and the
prerequisite section of the micro app plan. **Chosen approach** is the bridge topic (B) plus buffered flush
(D), because it is the only combination that works from a `file://` origin *and* keeps reporting while the
driver is in Google Maps. **Left out on purpose:** geofencing in native (`Location.startGeofencingAsync`) —
the rally evaluates geofences server-side against reported positions, and moving that decision into the
phone would put scoring logic on an untrusted device.
