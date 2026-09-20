# Meetopoly — Living implementation plan

> **Status:** Living document. Update this when phases complete or decisions change.  
> **Skill:** `.cursor/skills/meetopoly/` (parent repo).  
> **Repos:** `meetopoly-be` (Go) · `meetopoly-mobile` (Expo).  
> **Out of scope until asked:** web, Wails desktop, Docker, location admin UI, OAuth, LiveKit.

---

## 0. Product north star

Meetopoly is a **mobile-first**, worldwide social property game:

- **Guns of Glory–style** overworld: tilted camera, avatar walks 360°, **city billboards**; approach → Enter hub.
- **Dual presence:** **game pin** (board slot) vs **social avatar** (roam / hub). Rolling moves the pin immediately even if the avatar stays in a hub.
- **Worlds:** board packs (`africa-1` first). Cities from [svgcities.com](https://svgcities.com/); more Worlds later (Europe packs, Asia, etc.).
- **Tables:** 2–6 players; **5-minute** turn timer or lose turn; notify players in hubs with a compact turn sheet.
- **Content:** `seeds/locations.json` → Mongo; `worldId` filter.
- **Realtime:** Pion SFU; **positions** on DataChannels first; **voice later**.
- **Rules:** ship **M1** first, then M2–M5 (see skill `product.md`).

---

## 1. Locked technical decisions

| Area | Decision |
|------|----------|
| HTTP router | **chi** (`github.com/go-chi/chi/v5`) |
| Backend module | `meetopoly-be` |
| Architecture | Hexagonal: adapters → services → repository |
| Adapters | HTTP, WebSocket, WebRTC (Pion) |
| DB / cache | MongoDB (local → Atlas later), Redis (local → Contabo later) |
| Deploy | Contabo VPS, **no Docker** |
| Mobile | Expo + Expo Router + NativeWind + `@expo/ui` + Moti + TanStack Query |
| Orientation | **Landscape** (GoG-style) |
| Theme | `meetopoly-mobile/theme/` — forest brand, gold accent, warm paper bg |
| Fonts | **Fraunces** (display) + **Figtree** (UI/body) — `assets/fonts/` |
| 3D | expo-gl + plain Three.js |
| API contract | OpenAPI → **orval** → `meetopoly-mobile/api/` |
| Auth | Email → Google SMTP verify → password → username/country; login email/password |
| Overworld art | SVGCities icons as billboards; tilted camera; GLB optional later |
| Movement sync | Positions @ ~10–20 Hz; bounce-back corrections |
| Voice | Phase after positions; same room model |

---

## 2. Target package / file trees

Create in this order as phases unlock. Do not invent extra top-level packages without asking.

### 2.1 `meetopoly-be`

```
meetopoly-be/
  main.go                         # process entry
  go.mod                          # module meetopoly-be
  api/openapi.yaml
  docs/plan.md                    # this file
  seeds/locations.json            # Worlds (`africa-1` first)
  configs/                        # optional env samples — ask before adding
  internal/
    adapters/
      http/                       # chi (locked)
      websocket/
      webrtc/
    services/
      health/
      auth/
      user/
      location/
      table/                      # game table / lobby
      game/                       # Monopoly M1+ state machine
      hub/                        # location hub enter/leave
      presence/                   # avatar position validation
      mail/                       # Google SMTP
    repository/
      user/
      location/
      table/
      game/
      session/                    # redis-backed as needed
    platform/                     # mongo/redis clients, config — keep thin
```

### 2.2 `meetopoly-mobile`

```
meetopoly-mobile/
  app/                            # Expo Router routes
    (auth)/
    (app)/
  api/
    types.ts                      # OpenAPI codegen
    services.ts
    client.ts                     # fetch base URL, auth header
    queryKeys.ts
  hooks/
  components/
  scenes/                         # Three.js / expo-gl (overworld, hub)
  theme/                          # color tokens (colors.js) — locked
  assets/
```

---

## 3. Data model (initial)

Exact indexes: confirm when implementing. Collections below are the planned baseline.

### 3.1 MongoDB collections

| Collection | Purpose |
|------------|---------|
| `users` | Account, profile, password hash, emailVerified |
| `email_verifications` | Token, expiry for signup |
| `locations` | Board + map content (from seed) |
| `tables` | Lobby/session metadata |
| `games` | Authoritative game state (pins, money, ownership, turn) |

### 3.2 Location document shape (seed-aligned)

See `seeds/locations.json`. Core fields:

- `worldId` — e.g. `africa-1`
- `slug`, `name`, `countryCode`, `region`
- `kind`: `property` | `railroad` | `utility` | `special`
- `boardIndex` — order on the logical track
- `price`, `rents[]`, `colorGroup` (properties)
- `map`: `{ x, z, scale }` — overworld placement
- `assets.icon` — billboard path; cities: `city-icons/icons/{cc}-{slug}.svg` (under `meetopoly-mobile/`)
- `hubId` — hub room key when entered
- `enterRadius` — approach distance for Enter prompt
- `svgcities` / `svgcitiesUrl` / `svgcitiesPath` — SVGCities source
- `symbol`, `about`, `aboutShort`, `attribution` — landmark About + CC BY credit

### 3.3 Dual presence in game state

Per player in `games`:

- `pinLocationId` / `boardIndex` — rules position
- `avatar`: `{ x, z, rot, hubId? }` — social position (`hubId` null = overworld)

### 3.4 Redis (planned uses)

- Session / refresh tokens (if used)
- Turn deadline timestamps (`table:{id}:turnDeadline`)
- Hot presence keys optional (else SFU + service memory for v1 — decide at presence phase)

---

## 4. Phases

Each phase lists **goal**, **backend files**, **mobile files**, **exit criteria**. Complete exit criteria before starting the next phase unless the user says otherwise.

---

### Phase 0 — Repo bootstrap

**Goal:** Empty repos become runnable skeletons with skill-aligned layout.

**Backend**

1. `go.mod` (`module meetopoly-be`)
2. `main.go` (repo root) — wires config, mongo, redis, HTTP health only
3. `internal/adapters/http` — health handler
4. `internal/services/health` — interface + impl
5. `api/openapi.yaml` — `/health` only to start

**Mobile**

1. Expo app with **Expo Router**
2. NativeWind bootstrap
3. `api/client.ts` stub pointing at local backend
4. Placeholder `app/index` screen

**Exit criteria**

- [x] `GET /health` returns OK against local server
- [x] Mobile app launches and can call health via TanStack Query (Phase 0 scaffold)
- [x] No Docker; README notes local Mongo/Redis required

---

### Phase 1 — OpenAPI pipeline + shared HTTP client

**Goal:** FE types always track BE contract.

**Backend**

1. Expand `api/openapi.yaml` structure (info, servers, components/schemas)
2. Document codegen command — **orval** (locked)

**Mobile**

1. `api/types.ts`, `api/services.ts` (re-exports of orval `api/generated/`)
2. `api/queryKeys.ts`
3. Hook `useHealth` as pattern sample
4. Config: `orval.config.ts`; script: `npm run api:generate`

**Exit criteria**

- [x] Regenerating client after OpenAPI change updates generated types + endpoints
- [x] No hand-written duplicate DTOs for HTTP
- [x] Codegen tool: **orval** (MIT)

---

### Phase 2 — Auth (email signup + login)

**Goal:** Full locked signup/login with Google SMTP verification.

**Flow**

1. Enter email → send verification (Google SMTP)
2. Verify token/link/code
3. Create password
4. Finish profile: username, country
5. Login with email + password
6. Authenticated session (JWT or opaque token in Redis — **ask which** when implementing)

**Backend packages**

- `services/auth`, `services/mail`, `services/user`
- `repository/user`, `repository` for verifications
- HTTP routes in OpenAPI: signup start, verify, set password, complete profile, login, me

**Mobile**

- Expo Router `(auth)/` screens: email, verify, password, profile, login
- Forms: `@expo/ui` where suitable; Moti for transitions
- Hooks: `useSignup`, `useLogin`, `useMe`
- Secure token storage — ask before picking library (expo-secure-store expected)

**Exit criteria**

- [ ] Can register with real SMTP in a configured env
- [ ] Unverified users cannot complete login into app
- [ ] `/me` returns profile; mobile stores session and restores on launch

---

### Phase 3 — Locations API + `africa-1` seed

**Goal:** Configurable World board content from Mongo.

**Backend**

1. Import / seed `seeds/locations.json` into `locations` (all Worlds; gameplay filters `worldId: africa-1`)
2. `services/location` + `repository/location`
3. OpenAPI: list locations by `worldId`, get by id/slug
4. List Worlds endpoint (derive distinct `worldId`s from seed or static table)

**Mobile**

1. `hooks/useLocations(worldId)`
2. Dev screen listing Africa cities (name, kind, price)

**Seed / Compass**

- Import `seeds/locations.json` into collection `locations` (see `seeds/README.md`)

**Exit criteria**

- [ ] API returns seeded `africa-1` locations
- [ ] Mobile list matches Compass data
- [ ] Adding a document in Mongo appears in API without app rebuild

---

### Phase 4 — Overworld scene (single player, no net)

**Goal:** Guns of Glory feel — tilted camera + city billboards.

**Mobile**

1. `scenes/overworld/` — expo-gl + Three.js
2. World ground map + load city **billboards** from `assets.icon`
3. Joystick movement; camera follows at **isometric tilt**; pinch zoom + pan
4. Approach `enterRadius` → Enter prompt
5. Placeholder hub scene on Enter (local only; no SFU yet)
6. Optional: render static **pins** at `boardIndex` slots for debug

**Backend:** none required beyond locations.

**Exit criteria**

- [ ] Walk `africa-1` layout; billboards read as standing cities
- [ ] Enter prompt on approach; hub placeholder loads
- [ ] Stable ~30 FPS target on a mid-range phone for this sparse scene

---

### Phase 5 — Tables lobby (2–6) + WebSocket basics

**Goal:** Create/join a table; presence of seated players.

**Backend**

1. `services/table` + repos
2. WebSocket adapter: join lobby, seat updates
3. OpenAPI for create/list/join table (HTTP) + WS event list in plan/OpenAPI extensions
4. Enforce 2–6 players; host starts game when ready

**Mobile**

1. Lobby screens under `(app)/`
2. `hooks/useTable`, `hooks/useTableSocket`
3. Write WS events into TanStack cache

**Exit criteria**

- [ ] Two devices/users can join one table and see each other seated
- [ ] Cannot start with &lt;2 or &gt;6
- [ ] Disconnect handling stubbed (ask for exact policy when implementing)

---

### Phase 6 — Game M1 (authoritative) without hubs

**Goal:** Dice, pin movement, buy, rent, pass income, turn order, **5-minute timer**.

**Backend**

1. `services/game` state machine (M1 only)
2. Persist `games` documents
3. Redis turn deadlines
4. WS events: `turnStarted`, `turnTick`/`deadline`, `diceRolled`, `pinMoved`, `propertyBought`, `rentPaid`, `turnSkipped`
5. On timeout: skip turn (lose turn)

**Mobile**

1. Board HUD: money, turn indicator, timer
2. Turn sheet actions: Roll, Buy, Decline, End (as rules require)
3. Show **pins** on slots from game state; avatars can idle at table spawn for now

**Exit criteria**

- [ ] Full M1 round playable for 2–6 players
- [ ] Timer skip works
- [ ] Server is source of truth (client cannot forge money)

---

### Phase 7 — Dual presence + avatar position sync (DataChannels)

**Goal:** Pin vs avatar; roam overworld during others’ turns; prediction + bounce-back.

**Backend**

1. Pion SFU adapter + signaling over WebSocket
2. Table WebRTC room for in-session overworld presence
3. `services/presence` validates avatar moves; broadcast accepted; correct invalid to sender only
4. On dice: update pin for all; do not force avatar out of hub/overworld

**Mobile**

1. `hooks/useWebRTC` / `useRoom`
2. Send local avatar position 10–20 Hz
3. Interpolate remote avatars
4. Pins driven by game WS; avatars by DataChannel

**Exit criteria**

- [ ] Two players see each other’s avatars move smoothly
- [ ] Illegal teleport gets bounce-back only on offender
- [ ] Roll updates pins while avatars stay put

---

### Phase 8 — Location hubs + turn notify while in hub

**Goal:** Enter landmark; hub room; turn sheet without forcing board.

**Backend**

1. `services/hub` — enter/leave; hub WebRTC rooms by `hubId`
2. On `turnStarted`, if player `hubId != null`, emit targeted notify
3. Same room model ready for future voice (no mic yet)

**Mobile**

1. Hub scene load on Enter; leave returns to overworld
2. Turn BottomSheet in hub: timer + Roll / basic actions + Open board
3. Pin still updates on roll while staying in hub

**Exit criteria**

- [ ] Players from same or different tables can meet in one hub (define cross-table policy in impl — default: **allow** per product)
- [ ] Turn notify + 5‑min skip works while in hub
- [ ] Open board navigates to table/board view

---

### Phase 9 — Polish M1 UX + economy feedback

**Goal:** Property cards, rent toasts, Moti transitions, `@expo/ui` settings.

**Mobile**

- Branded cards (NativeWind + Moti)
- Settings switches via `@expo/ui`
- Error/empty states

**Exit criteria**

- [ ] New player can finish signup → join table → play M1 → visit a hub without developer intervention

---

### Phase 10 — Voice (same rooms)

**Goal:** Mic audio in table overworld room and/or hub rooms.

**Backend / mobile:** add media tracks to existing Pion rooms; mute/unmute UI; permissions.

**Exit criteria**

- [ ] Hear others in hub; leave hub stops hub audio

---

### Phase 11 — Rules M2 (houses / hotels / light mortgage)

**Exit criteria:** Even-build enforced server-side; UI to buy/sell houses.

---

### Phase 12 — Rules M3 (jail + cards)

**Exit criteria:** Jail entry/exit paths; Chance/Community Chest deck from config or seed.

---

### Phase 13 — Rules M4 (trading + auctions)

**Exit criteria:** Multi-property offers; auction when declined purchase.

---

### Phase 14 — Rules M5 (bankruptcy)

**Exit criteria:** Debt resolution; player elimination; assets transfer correctly.

---

### Phase 15 — Production hardening (Contabo)

**Goal:** Deploy binary to Contabo; Redis on VPS; MongoDB Atlas; Google SMTP prod creds; TLS reverse proxy.

**Ask before:** systemd unit contents, nginx vs Caddy, TURN (coturn).

**Exit criteria**

- [ ] Mobile points at prod API
- [ ] Signup email works in prod
- [ ] One full M1 game on prod infra

---

### Phase 16 — Later (explicitly deferred)

Only when the user asks:

- Location **admin** CRUD
- OAuth
- Web R3F client + Wails desktop
- TURN, recording, moderation tools
- More countries’ seed packs

---

## 5. Cross-cutting concerns

### 5.1 Testing

- Go: service unit tests for game M1 transitions and turn timeout.
- Mobile: hook tests optional; manual device checklist per phase exit criteria.

### 5.2 Observability

- Structured logging in Go (`slog`). Ask before adding metrics/tracing stacks.

### 5.3 Security

- Password hashing (argon2/bcrypt — ask when implementing).
- Rate-limit auth endpoints.
- Auth on WS/WebRTC signaling.
- Never trust client money/pin updates.

### 5.4 Performance (mobile 3D)

- Instancing, low-poly, limit active meshes, simple materials (see prior research). Target stable 30 FPS mid-range.

---

## 6. Suggested build order (summary)

```
0 Bootstrap → 1 OpenAPI → 2 Auth → 3 Locations+seed
→ 4 Overworld local → 5 Tables+WS → 6 Game M1
→ 7 Presence WebRTC → 8 Hubs+turn sheet → 9 UX polish
→ 10 Voice → 11–14 Rules M2–M5 → 15 Contabo → 16 Deferred
```

---

## 7. How to use this doc

1. Agent/user picks a **phase**.
2. Follow Meetopoly skill: clarify anything not locked; implement only that phase’s files.
3. Check off exit criteria.
4. Note deviations under a “Changelog” section below.

---

## 8. Changelog

| Date | Change |
|------|--------|
| 2026-09-20 | Initial comprehensive plan from locked product/tech decisions |
| 2026-09-20 | Worlds + SVGCities billboards; `locations.json` / `africa-1` replaces Nigeria districts |
| 2026-09-20 | Expanded `locations.json` with all SVGCities Worlds (Europe×5, Asia×2, NA, SA, ME, Oceania, Central America) |
| 2026-09-20 | Linked all 304 city properties to `city-icons/icons/{cc}-*.svg` + About/attribution from SVGCities metadata |
| 2026-09-20 | Phase 0: chi HTTP `/health`, Mongo+Redis wiring, Expo Router + NativeWind + TanStack health screen |
| 2026-09-20 | Locked landscape orientation + theme tokens (forest/gold/warm paper) |
| 2026-09-21 | Locked fonts: Fraunces (display) + Figtree (UI/body); wired via expo-font |
| 2026-09-21 | Frontend perf rule: Compiler-first; selective memo only (skill `frontend.md`) |
| 2026-09-21 | Phase 1: orval OpenAPI codegen → `meetopoly-mobile/api/generated`; `useHealth` on generated client |
