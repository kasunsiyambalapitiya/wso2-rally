# WSO2 Motor Rally 2027 — Architecture and Data Flow Diagrams

Companion to `WSO2 Motor Rally 2027 - Threat Model.docx`. Each DFD number below
matches the interaction number in that document.

**Scope note.** The Go backend and the organizer web app are drawn as
implemented (branch `feat/webapp-events`, commit `81dee73`). The in-car micro
app is drawn as designed in `docs/specs/2026-07-24-wso2-motor-rally-design.md` —
it has no implementation yet, so DFDs 3 through 7 describe intended behaviour on
the client side and implemented behaviour on the server side.

## Notation

| Boundary | Meaning |
|---|---|
| `[Untrusted]` | Untrusted entity — internet users, unmanaged participant devices |
| `[B-EX-EN]` | External entity — external APIs, IDPs, third-party systems |
| `[B-EX-DP]` | External dependency |
| `[B-EX-CP]` | External component, code owned by WSO2 |
| `[B-IN]` | Internal boundary |

| Medium | Meaning | | Confidentiality | Meaning |
|---|---|---|---|---|
| `[M-NT]` | Network (HTTP/HTTPS/WSS) | | `[C-High]` | Tokens, PII, secrets |
| `[M-DB]` | Database (TCP) | | `[C-Medium]` | Business data, non-PII |
| `[M-FS]` | File system | | `[C-Low]` | Public or non-sensitive |
| `[M-IN]` | Internal, in-process | | | |

---

## 1. High-Level System Architecture

```mermaid
graph TB
    subgraph untrusted["Untrusted — public internet"]
        ORG["Rally Organizer<br/>browser [Untrusted]"]
        CREW["Crew phones, ~600<br/>unmanaged [Untrusted]"]
        ATT["Malicious actor<br/>[Untrusted]"]
    end

    subgraph external["External entities [B-EX-EN]"]
        ASG["Asgardeo<br/>OIDC + JWKS"]
        OSM["OpenStreetMap<br/>tile servers"]
    end

    subgraph choreo["Choreo platform [B-EX-CP]"]
        GW["Gateway<br/>TLS termination · CORS<br/>rate limiting UNCONFIRMED"]

        subgraph backend["Rally backend component [B-IN]"]
            REST["REST API :8080"]
            WS["WebSocket /ws<br/>same listener"]
            AUTH["middleware.Auth<br/>RequireOrganizer / RequireTeam"]
            SVC["Domain services<br/>events · routes · tasks · vehicles<br/>sessions · scoring · alerts"]
            ENG["taskengine<br/>validate + score"]
            GEO["geo<br/>point-in-radius"]
            HUB["realtime.Hub<br/>topic fan-out"]
        end

        SPA["Rally Ops SPA<br/>static bundle [B-IN]"]
    end

    DB[("MySQL 8.4<br/>13 tables · all crew PII<br/>[B-IN]")]

    ORG -->|"[M-NT] HTTPS [C-High]<br/>OIDC PKCE sign-in"| ASG
    ORG -->|"[M-NT] HTTPS [C-Low]<br/>load static bundle"| SPA
    ORG -->|"[M-NT] HTTPS [C-Medium]<br/>map tiles, no API key"| OSM
    SPA -->|"[M-NT] HTTPS [C-High]<br/>Bearer + x-user-id-token"| GW
    CREW -->|"[M-NT] HTTPS [C-High]<br/>join, then team token"| GW
    ATT -.->|"[M-NT] HTTPS [C-High]<br/>unauthenticated: POST /sessions/join"| GW

    GW -->|"[M-NT] HTTPS [C-High]"| REST
    GW -->|"[M-NT] WSS [C-High]"| WS
    REST --> AUTH
    WS --> AUTH
    AUTH -->|"[M-IN] [C-High]"| SVC
    SVC -->|"[M-IN] [C-Medium]"| ENG
    SVC -->|"[M-IN] [C-High]"| GEO
    SVC -->|"[M-IN] [C-High]"| HUB
    HUB -->|"[M-NT] WSS [C-High]<br/>topic-scoped fan-out"| WS
    SVC -->|"[M-DB] TCP 3306 [C-High]<br/>TLS NOT ENFORCED"| DB
    AUTH -->|"[M-NT] HTTPS [C-Medium]<br/>cached JWKS refresh"| ASG

    classDef gap stroke:#c00,stroke-width:3px
    class GW,DB gap
```

Red-outlined nodes carry an open finding: the gateway because no rate limiting
is confirmed anywhere in the stack (RALLY-R5), the database because the
connection does not currently require TLS.

---

## 2. Container and Deployment Architecture

```mermaid
graph LR
    subgraph dev["Local development"]
        DSPA["Vite dev server<br/>:3000"]
        DBE["go run ./cmd/server<br/>:8080 · plain HTTP"]
        DDB[("MySQL 8.4 container<br/>:3306 published to host<br/>root/root · rally/rally")]
        DSPA -->|"[M-NT] HTTP [C-High]<br/>CORS_ALLOW_ORIGIN"| DBE
        DBE -->|"[M-DB] TCP [C-High]"| DDB
    end

    subgraph prod["Choreo"]
        PSPA["Rally Ops SPA<br/>static bundle + config.js"]
        PGW["Gateway · TLS 1.2+"]
        PBE["rally-backend<br/>Go static binary<br/>non-root container<br/>:8080 Public"]
        PDB[("MySQL 8.4<br/>DigiOps-managed<br/>network-internal only")]
        PSPA -->|"[M-NT] HTTPS [C-High]"| PGW
        PGW -->|"[M-NT] HTTPS [C-High]"| PBE
        PBE -->|"[M-DB] TCP 3306 [C-High]"| PDB
    end

    dev -.->|"same dist/, only config.js differs"| prod
```

Local development runs `TOKEN_VALIDATOR_ENABLED=false`, which accepts organizer
tokens **without signature verification** (RALLY-R3). Nothing in `config.Load()`
prevents that value reaching a deployed environment.

---

## DFD 1 — Organizer Authentication and API Access

```mermaid
sequenceDiagram
    autonumber
    participant O as Organizer browser [Untrusted]
    participant S as Rally Ops SPA [B-IN]
    participant A as Asgardeo [B-EX-EN]
    participant G as Choreo gateway [B-EX-CP]
    participant M as middleware.Auth [B-IN]
    participant V as OrganizerValidator [B-IN]

    Note over O,V: Boundary: Untrust → Trust<br/>[M-NT] HTTPS<br/>[C-High] id token, organizer email and group claims

    O->>S: Load application
    S->>A: OIDC PKCE authorize<br/>[M-NT] HTTPS [C-High]
    A-->>O: Authenticate (tenant MFA policy)
    A-->>S: id token (JWT RS256)<br/>[M-NT] HTTPS [C-High]
    S->>G: GET /users/me<br/>Authorization: Bearer + x-user-id-token<br/>[M-NT] HTTPS [C-High]
    G->>M: Forward<br/>[M-NT] HTTPS [C-High]
    M->>M: VerifyTeamToken first (cheap, local)
    M->>V: Not a team token → Validate()
    alt TOKEN_VALIDATOR_ENABLED = true
        V->>A: Cached JWKS (background refresh)<br/>[M-NT] HTTPS [C-Medium]
        V-->>M: Identity{organizer, email, groups}
    else TOKEN_VALIDATOR_ENABLED = false — RALLY-R3
        V-->>M: Claims decoded, SIGNATURE NOT VERIFIED
    end
    M-->>S: 200 {userId, email, groups}<br/>[M-NT] HTTPS [C-High]
    Note over M: RequireOrganizer checks Kind only.<br/>RequireAdmin is never mounted — RALLY-R2.
```

---

## DFD 2 — Event Administration

```mermaid
sequenceDiagram
    autonumber
    participant S as Rally Ops SPA [B-IN]
    participant G as Choreo gateway [B-EX-CP]
    participant H as events.Handler [B-IN]
    participant SV as events.Service [B-IN]
    participant DB as MySQL [B-IN]

    Note over S,DB: Boundary: Trust → Trust<br/>[M-NT] HTTPS then [M-DB] TCP<br/>[C-Medium] event configuration · [C-High] the cipher until 09:00

    S->>G: POST /events/search {offset, limit, filters}<br/>[M-NT] HTTPS [C-Medium]
    G->>H: Forward (organizer identity on context)
    H->>H: NormalizePage clamps limit to max 100
    H->>SV: Search(page, filter)
    SV->>DB: SELECT ... LIMIT ? OFFSET ?<br/>[M-DB] TCP 3306 [C-Medium]
    DB-->>SV: rows + total
    SV->>DB: SELECT event_id, id, name FROM route WHERE event_id IN (?,…)<br/>[M-DB] TCP 3306 [C-Low]
    SV-->>S: {items, totalCount}<br/>[M-NT] HTTPS [C-Medium]

    S->>H: POST /events/{id}/publish<br/>[M-NT] HTTPS [C-Medium]
    H->>SV: Publish(id)
    alt Both geofences placed and status = setup
        SV->>DB: UPDATE event SET status='active'<br/>[M-DB] TCP 3306 [C-Medium]
        SV-->>S: 200 event
    else Geofence missing, or already complete
        SV-->>S: 400 / 409 {"message": "<safe sentence>"}
    end
    Note over H: createdBy comes from the token, never the body.<br/>No admin role is checked — RALLY-R2.
```

---

## DFD 3 — Crew Join and Team Token Issuance

```mermaid
sequenceDiagram
    autonumber
    participant P as In-car phone [Untrusted]
    participant G as Choreo gateway [B-EX-CP]
    participant H as sessions Handler (PUBLIC) [B-IN]
    participant SV as sessions.Service [B-IN]
    participant T as HMACTokenMinter [B-IN]
    participant DB as MySQL [B-IN]

    Note over P,DB: Boundary: Untrust → Trust — THE ONLY UNAUTHENTICATED WRITE<br/>[M-NT] HTTPS<br/>[C-High] mints a 12-hour bearer token

    P->>G: POST /sessions/join<br/>{vehicleId, crewMemberId, phoneLast4}<br/>[M-NT] HTTPS [C-High]
    G->>H: Forward — NO bearer token required
    H->>SV: Join(input)
    SV->>DB: SELECT crew_member WHERE id = ?<br/>[M-DB] TCP 3306 [C-High] PII
    SV->>SV: checkPhoneLast4(roster, typed)
    alt Digits match
        SV->>DB: INSERT/UPDATE team_session (unique per vehicle)<br/>[M-DB] TCP 3306 [C-Medium]
        SV->>DB: UPSERT session_device (unique per member)<br/>[M-DB] TCP 3306 [C-Medium]
        SV->>T: MintTeamToken{session, vehicle, device, crew}
        T-->>SV: JWT HS256, iss=rally-team, 12h
        SV-->>P: {teamToken, session, device, crew}<br/>[M-NT] HTTPS [C-High]
    else Digits wrong
        SV-->>P: 4xx validation error
    end
    Note over P,SV: RALLY-R1 — 10,000 combinations, ids enumerable,<br/>no rate limit, no lockout, no CAPTCHA, no revocation.
```

---

## DFD 4 — Location Streaming and Geofence Unlock

```mermaid
sequenceDiagram
    autonumber
    participant N as Navigator phone [Untrusted]
    participant M as RequireTeam [B-IN]
    participant SV as sessions.Service [B-IN]
    participant GEO as internal/geo [B-IN]
    participant DB as MySQL [B-IN]
    participant HUB as realtime.Hub [B-IN]
    participant C as Other crew phones [Untrusted]

    Note over N,C: Boundary: Untrust → Trust<br/>[M-NT] HTTPS<br/>[C-High] live location of identifiable participants

    N->>M: POST /sessions/me/location {lat, lng, accuracy, ts}<br/>[M-NT] HTTPS [C-High]
    M->>M: Session id from token sub, never from body
    alt Caller is the navigator
        M->>SV: RecordLocation(...)
        SV->>GEO: Point-in-radius against waypoints<br/>[M-IN] [C-High]
        GEO-->>SV: newly entered waypoints
        SV->>DB: UPDATE last_lat/last_lng, INSERT visits<br/>[M-DB] TCP 3306 [C-High]
        SV->>HUB: publish task_unlocked / rest_lock / arrival<br/>[M-IN] [C-Medium]
        HUB-->>C: session:{id} messages<br/>[M-NT] WSS [C-Medium]
        SV-->>N: {unlockedTasks, events[]}<br/>[M-NT] HTTPS [C-Medium]
    else Caller is not the navigator
        M-->>N: 403
    end
    Note over N: GPS is trusted — RALLY-R10, accepted MVP non-goal.<br/>No posting-rate floor — RALLY-R5.
```

---

## DFD 5 — Task Definition Retrieval with Redaction

```mermaid
sequenceDiagram
    autonumber
    participant P as In-car phone [Untrusted]
    participant O as Organizer SPA [B-IN]
    participant H as tasks.Handler (shared) [B-IN]
    participant R as tasks.RedactForCrew [B-IN]
    participant DB as MySQL [B-IN]

    Note over P,DB: Boundary: Untrust → Trust<br/>[M-NT] HTTPS<br/>[C-High] the unredacted config holds every answer in the rally

    P->>H: GET /tasks/{id}<br/>[M-NT] HTTPS [C-Medium]
    H->>DB: SELECT task WHERE id = ?<br/>[M-DB] TCP 3306 [C-High]
    DB-->>H: task with full JSON config
    alt Identity.Kind = team
        H->>R: RedactForCrew(config)
        R->>R: Strip answer, answers, solution, payload,<br/>targetSec, tolerance, solvePoints,<br/>skipPoints, checkpointId
        R-->>H: presentation keys only
        H-->>P: task without answers<br/>[M-NT] HTTPS [C-Medium]
    else Identity.Kind = organizer
        H-->>O: full task including answers<br/>[M-NT] HTTPS [C-High]
    end
    Note over R: Deny-list, not allow-list. A new scoring key that is not<br/>added here leaks silently — RALLY-R8.
```

---

## DFD 6 — Task Submission and Server-Side Scoring

```mermaid
sequenceDiagram
    autonumber
    participant P as Crew phone [Untrusted]
    participant M as RequireTeam [B-IN]
    participant SV as sessions submit [B-IN]
    participant E as taskengine [B-IN]
    participant DB as MySQL [B-IN]
    participant HUB as realtime.Hub [B-IN]

    Note over P,HUB: Boundary: Untrust → Trust<br/>[M-NT] HTTPS<br/>[C-Medium] answers and scores

    P->>M: POST /sessions/me/tasks/{taskId}/submit {payload}<br/>[M-NT] HTTPS [C-Medium]
    M->>SV: session, device, crew member all from the token
    SV->>DB: SELECT task config (authoritative copy)<br/>[M-DB] TCP 3306 [C-High]
    SV->>E: Validate(payload, config) then Score(...)
    E-->>SV: correct?, awardedPoints
    alt Task not yet answered by this car
        SV->>DB: INSERT task_submission<br/>UNIQUE(session_id, task_id)<br/>[M-DB] TCP 3306 [C-Medium]
        SV->>DB: UPDATE team_session.total_score<br/>[M-DB] TCP 3306 [C-Medium]
        SV->>HUB: score_delta → session and event topics<br/>[M-IN] [C-Medium]
        HUB->>DB: Leaderboard rebuild, background ctx, 5s timeout<br/>[M-DB] TCP 3306 [C-Medium]
        SV-->>P: {correct, awardedPoints}<br/>[M-NT] HTTPS [C-Medium]
    else Already answered
        SV-->>P: 409 naming the winning crew member
    end
    Note over E: The client never decides correctness or points.
```

---

## DFD 7 — WebSocket Subscription and Fan-Out

```mermaid
sequenceDiagram
    autonumber
    participant O as Organizer browser [Untrusted]
    participant P as Crew phone [Untrusted]
    participant M as middleware.Auth [B-IN]
    participant W as wsHandler / maySubscribe [B-IN]
    participant HUB as realtime.Hub [B-IN]

    Note over O,HUB: Boundary: Untrust → Trust<br/>[M-NT] WSS<br/>[C-High] cipher reveal and every vehicle's live position

    O->>M: GET /ws?topic=event:{id} (Upgrade)<br/>[M-NT] WSS [C-High]
    M->>W: Identity resolved before upgrade
    W->>W: maySubscribe — organizer may watch any topic
    W->>HUB: ServeWS, OriginPatterns enforced
    HUB-->>O: vehicle_position, task_completed,<br/>score_delta, leaderboard, alert<br/>[M-NT] WSS [C-High]

    P->>M: GET /ws?topic=session:{id} (Upgrade)<br/>[M-NT] WSS [C-High]
    M->>W: Identity resolved before upgrade
    alt topic == SessionTopic(token.SessionID)
        W->>HUB: ServeWS
        HUB-->>P: task_unlocked, task_completed, navigator_changed,<br/>score_delta, start_signal, cipher_reveal,<br/>rest_lock, arrival<br/>[M-NT] WSS [C-High]
    else Any other topic
        W-->>P: 403 before upgrade, logged with topic and kind
    end
    Note over HUB: conn.CloseRead discards all client input,<br/>so inbound frames cannot be processed.<br/>No per-identity connection cap — RALLY-R5.
```

---

## DFD 8 — Vehicle and Crew Roster with CSV Import/Export

```mermaid
sequenceDiagram
    autonumber
    participant S as Rally Ops SPA [B-IN]
    participant M as RequireOrganizer [B-IN]
    participant H as vehicles.Handler [B-IN]
    participant CSV as encoding/csv [B-IN]
    participant DB as MySQL [B-IN]

    Note over S,DB: Boundary: Trust → Trust<br/>[M-NT] HTTPS multipart, then [M-DB] TCP<br/>[C-High] name, full phone number and country for ~600 people

    S->>M: POST /events/{id}/vehicles/import (multipart)<br/>[M-NT] HTTPS [C-High]
    M->>H: Organizer identity — no admin gate, RALLY-R2
    H->>H: MaxBytesReader caps the body at 2 MiB
    H->>CSV: parseCSV → checkHeader → splitCrew
    alt Header and rows valid
        CSV-->>H: []csvRow
        H->>DB: INSERT vehicle + crew_member rows<br/>event id from the URL, never the file<br/>[M-DB] TCP 3306 [C-High]
        H-->>S: 200 import summary
    else Malformed
        H-->>S: 400 {"message": "<safe sentence>"}
    end

    S->>H: GET /events/{id}/vehicles/export<br/>[M-NT] HTTPS [C-High]
    H->>DB: SELECT vehicles + crew<br/>[M-DB] TCP 3306 [C-High]
    H->>CSV: writeCSV
    CSV-->>S: text/csv with full phone numbers<br/>[M-NT] HTTPS [C-High]
    Note over CSV: No formula-prefix neutralisation on export — RALLY-R9.<br/>No actor recorded against imported rows — RALLY-R6.
```

---

## Boundary Crossing Summary

| DFD | Interaction | Boundary crossing | Medium | Confidentiality | Open risks |
|---|---|---|---|---|---|
| 1 | Organizer authentication | Untrust → Trust | `[M-NT]` HTTPS | `[C-High]` id token, email, groups | RALLY-R2, R3, R5, R6 |
| 2 | Event administration | Trust → Trust | `[M-NT]` + `[M-DB]` | `[C-Medium]`; cipher `[C-High]` | RALLY-R2 |
| 3 | Crew join | **Untrust → Trust, unauthenticated** | `[M-NT]` HTTPS | `[C-High]` mints a 12h token | **RALLY-R1**, R4 |
| 4 | Location streaming | Untrust → Trust | `[M-NT]` HTTPS | `[C-High]` participant location | RALLY-R5, R7, R10 |
| 5 | Task retrieval | Untrust → Trust | `[M-NT]` HTTPS | `[C-High]` unredacted config | RALLY-R8 |
| 6 | Task submission | Untrust → Trust | `[M-NT]` + `[M-DB]` | `[C-Medium]` answers and scores | RALLY-R5, R11 |
| 7 | WebSocket fan-out | Untrust → Trust | `[M-NT]` WSS | `[C-High]` cipher, all positions | RALLY-R5 |
| 8 | Roster and CSV | Trust → Trust | `[M-NT]` + `[M-DB]` | `[C-High]` PII for ~600 people | RALLY-R2, R6, R7, R9 |
| — | Backend → Asgardeo JWKS | Trust → Untrust | `[M-NT]` HTTPS | `[C-Medium]` public keys | — |
| — | Browser → OpenStreetMap | Trust → Untrust | `[M-NT]` HTTPS | `[C-Low]` tile coordinates | — |
| — | Backend → MySQL | Trust → Trust | `[M-DB]` TCP 3306 | `[C-High]` all data incl. PII | **TLS not enforced** |
