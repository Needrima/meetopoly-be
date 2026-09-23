# Meetopoly — Living implementation plan

> **Status:** Living document. Update this when phases complete or decisions change.  
> **Skill:** `.cursor/skills/meetopoly/` (parent repo).  
> **Repos:** `meetopoly-be` (Go) · `meetopoly-mobile` (Expo).  
> **Out of scope until asked:** web, Wails desktop, Docker, location admin UI, OAuth, LiveKit.

---

## 0. Product north star

Meetopoly is a **mobile-first**, worldwide social property game:

- **Landscape 2D Monopoly-style board** (primary play surface): left = **1:1 board square**; right = controls / joystick / turn UI / later WebRTC.
- **City icons** on each square from SVGCities / generics (`assets.icon`).
- **Dual presence on the same board:**
  - **Game pin** — sits on a `boardIndex` slot (starts on GO; moves on dice).
  - **Social avatar** — walks the **whole board** (ring + center); pod + face callout (initial now, photo later).
- **Enter hub:** walk near a property / railroad / utility → Enter prompt (not tap-only).
- **Worlds:** board packs (`africa-1` first, **40** spaces — classic equal sides). Cities from [svgcities.com](https://svgcities.com/); more Worlds later.
- **Tables:** **2–6** players (pins pack up to 6 on one square — prefer **2×3** grid when crowded); **5-minute** turn timer or lose turn; notify players in hubs with a compact turn sheet.
- **Signed-in funnel (long-term):** menu home → **Play** → pick **World** → lobby → **Start** → board. Phase 4 Play goes **straight to board** (no lobby yet).
- **Content:** `seeds/locations.json` → Mongo; `worldId` filter. (**Locations** = board spaces; **World** = which board pack to play.)
- **Realtime:** Pion SFU; board + hub presence; **voice later**. Lobby seating = **WebSocket** (not WebRTC).
- **Rules:** ship **M1** first, then M2–M5 (see skill `product.md`).
- **No separate 3D overworld in v1** — roaming is on the 2D board surface.

---

## 1. Locked technical decisions

| Area           | Decision                                                                                                 |
| -------------- | -------------------------------------------------------------------------------------------------------- |
| HTTP router    | **chi** (`github.com/go-chi/chi/v5`)                                                                     |
| Backend module | `meetopoly-be`                                                                                           |
| Architecture   | Hexagonal: adapters → services → repository                                                              |
| Adapters       | HTTP, WebSocket, WebRTC (Pion)                                                                           |
| DB / cache     | MongoDB (local → Atlas later), Redis (local → Contabo later)                                             |
| Deploy         | Contabo VPS, **no Docker**                                                                               |
| Mobile         | Expo + Expo Router + NativeWind + `@expo/ui` + Moti + TanStack Query                                     |
| Orientation    | **Landscape**                                                                                            |
| Theme          | `meetopoly-mobile/theme/` — forest brand, gold accent, warm paper bg                                     |
| Fonts          | **Fraunces** (display) + **Figtree** (UI/body) — `assets/fonts/`                                         |
| Board UI (v1)  | **2D** RN + `react-native-svg`; board square 1:1 height; panel takes remaining width                     |
| API contract   | OpenAPI → **orval** → `meetopoly-mobile/api/`                                                            |
| Auth           | Email → Google SMTP verify → password → username/country; login email/password                           |
| Board art      | Monopoly-like ring + center; SVGCities / generic icons; original chrome (not Hasbro art)                 |
| Spaces         | **40** for `africa-1` (11 per side incl. corners; 9 between) — classic even ring                         |
| Hub enter      | **Walk near** property / railroad / utility → Enter; specials not enterable                              |
| Board walk     | Avatar walks whole board; joystick in **panel bottom-right**                                             |
| Collisions     | Hard: board outer edge + **center** Chance/Chest decks; soft: pins. Ring Chance/Chest **tiles** walkable |
| Pins on square | Up to **6**; fan OK for ≤4; crowded → **2×3** grid oriented to tile long edge                            |
| Avatar look    | Pod + Maps-style callout: colored circle + **initial** now; photo later (settings upload)                |
| App home       | Signed-in **menu** (Play / Settings / About / Log out) — **not** the board as root                       |
| Leave board    | Only via panel **⋯** (no on-art Back); block Android back + iOS swipe-back while on board                |
| Presence sync  | Pins via game WS; board + hub avatar positions later (~10–20 Hz)                                         |
| Voice          | Phase after presence; same room model                                                                    |

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
    board/                        # 2D Monopoly ring, tiles, pins
    game/                         # right-panel turn / actions (later phases)
  theme/                          # color tokens (colors.js) — locked
  city-icons/                     # SVGCities + generic SVGs
  assets/
```

---

## 3. Data model (initial)

Exact indexes: confirm when implementing. Collections below are the planned baseline.

### 3.1 MongoDB collections

| Collection            | Purpose                                                 |
| --------------------- | ------------------------------------------------------- |
| `users`               | Account, profile, password hash, emailVerified          |
| `email_verifications` | Token, expiry for signup                                |
| `locations`           | Board + map content (from seed)                         |
| `tables`              | Lobby/session metadata                                  |
| `games`               | Authoritative game state (pins, money, ownership, turn) |

### 3.2 Location document shape (seed-aligned)

See `seeds/locations.json`. Core fields:

- `worldId` — e.g. `africa-1`
- `slug`, `name`, `countryCode`, `region`
- `kind`: `property` | `railroad` | `utility` | `special`
- `boardIndex` — order on the logical track
- `price`, `rents[]`, `colorGroup` (properties)
- `map`: `{ x, z, scale }` — legacy / optional layout hints (v1 board uses `boardIndex`)
- `assets.icon` — tile icon path; cities: `city-icons/icons/{cc}-{slug}.svg` (under `meetopoly-mobile/`)
- `hubId` — hub room key when entered
- `enterRadius` — approach distance for Enter prompt
- `svgcities` / `svgcitiesUrl` / `svgcitiesPath` — SVGCities source
- `symbol`, `about`, `aboutShort`, `attribution` — landmark About + CC BY credit

### 3.3 Dual presence in game state

Per player in `games`:

- `pinLocationId` / `boardIndex` — rules position
- `avatar`: `{ x, y, rot, hubId? }` — on board when `hubId` null; inside hub when set. Board coords in board-local space.

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
7. Password reset: email → OTP → new password; revokes all sessions; returns to login

**Decisions (locked for Phase 2)**

- Opaque Redis session tokens (`session:{token}`)
- 6-digit email verification codes
- Password hash: **bcrypt**
- Device storage: **expo-secure-store**
- Env: `joho/godotenv` + `.env` SMTP placeholders
- Password reset only for fully registered users; soft toast when email not eligible
- Toasts: `react-native-toast-message` + branded `notify()`
- Signup resume: durable only after password is set; app open + login reissue → profile

**Backend packages**

- `services/auth`, `services/mail`, `services/user`
- `repository/user`, `repository` for verifications
- HTTP routes in OpenAPI: signup start, verify, set password, complete profile, login, me

**Mobile**

- Expo Router `(auth)/` screens: email, verify, password, profile, login
- Forms: `@expo/ui` where suitable; Moti for transitions
- Hooks: `useSignup*`, `useLogin`, `useMe`, `useSession`
- Secure token storage — ask before picking library (expo-secure-store expected)

**Exit criteria**

- [x] Can register with real SMTP in a configured env
- [x] Unverified users cannot complete login into app
- [x] `/me` returns profile; mobile stores session and restores on launch

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

- [x] API returns seeded `africa-1` locations (`GET /locations?worldId=africa-1`, Bearer)
- [x] Mobile list matches seed data (`(app)/locations` via `useLocations`)
- [x] Adding a document in Mongo appears in API without app rebuild
- [x] Seed CLI: `go run ./cmd/seed-locations -file seeds/locations.json`
- [x] `GET /worlds`, `GET /locations/{id}`, `GET /locations/by-slug?worldId=&slug=`

---

### Phase 4 — 2D board + walk + side panel (single player, no net)

**Goal:** Monopoly-style **2D board** (left, 1:1 square) with ring tiles + center; **right panel** for joystick / nearby Enter / later turn+WebRTC. Avatars **walk on the board**; pins mark game slots. No expo-gl / Three.js.

**Layout (landscape)**

```text
┌────────────────────────┬─────────────────────┐
│  Board square (1:1)    │  Right panel        │
│  ring + center decks   │  nearby / Enter     │
│  pins on track         │  joystick (BR)      │
│  avatars walk board    │  (later: turn/RTC)  │
└────────────────────────┴─────────────────────┘
```

**Dual presence (on this board)**

| Concept    | Placement                        | Behavior                                  |
| ---------- | -------------------------------- | ----------------------------------------- |
| **Pin**    | `boardIndex` slot (start **GO**) | Rules / dice; not the walk body           |
| **Avatar** | Anywhere on board surface        | Walk with joystick; pod + initial callout |

**Collisions (locked)**

| Collider                                                   | Type        | Notes                                |
| ---------------------------------------------------------- | ----------- | ------------------------------------ |
| Outer board edge                                           | Hard        | Cannot leave the board square        |
| **Center** Chance + Community Chest decks (parallelograms) | Hard        | Cannot walk through card decks       |
| Ring Chance / Chest / Tax / etc. **tiles**                 | Walkable    | Part of the track; approachable      |
| Pins                                                       | Soft radius | Slide around; avoid hard stuck-at-GO |

**Enter hub (locked)**

- Walk within mapped `enterRadius` of a **property | railroad | utility** → show Enter on panel.
- Specials (Jail, Tax, ring Chance/Chest, GO, etc.): no Enter.

**Art direction**

- Monopoly **layout language** (ring, corners, color bands, center decks) — **not** Hasbro copyrighted art.
- Icons from `assets.icon` via `react-native-svg`.
- Avatar: pod + Google-Maps-style callout (colored circle + username **initial**); photo when settings upload exists.

**Other locks**

1. **40** `africa-1` spaces; corners at boardIndex **0 / 10 / 20 / 30** (even sides like classic Monopoly).
2. Joystick lives in **panel bottom-right** (not over the board art).
3. Right panel in Phase 4 = nearby location + Enter (+ joystick); **no** turn/money chrome (Phase 6).
4. **Do not** make the board the app root. **4.8** replaces the health/debug home with a **menu home**; board stays at `/(app)/board`.
5. Phase 4 is **local only** (one avatar); remote avatar sync = Phase 7. **No lobby / World picker / fake joiners / WebRTC** in Phase 4 — those are Phase 5+.
6. On the board screen: **no** floating Back on the board art; leave only via panel **⋯**; block system/gesture back.

**Mobile — incremental slices (implement one at a time)**

| Slice   | Done when                                                                                  | Avoid           |
| ------- | ------------------------------------------------------------------------------------------ | --------------- |
| **4.0** | Landscape shell: 1:1 board placeholder + panel; reachable from home                        | Ring, walk      |
| **4.1** | Empty ring of slots from `boardIndex` (geometry only)                                      | Colors, icons   |
| **4.2** | Color bands + kind styling                                                                 | Icons, walk     |
| **4.3** | Icons + `boardCode` / styling from `useLocations('africa-1')`                               | Walk, Enter     |
| **4.4** | ✅ Center brand + Chance/Chest deck shapes (`layout.decks` obstacles reserved)              | Movement        |
| **4.5** | ✅ Local avatar (pod + 1-letter) + joystick BR + edge/deck hard + pin soft; pin on GO       | Enter, net      |
| **4.6** | ✅ Walk-near Enter (nearest glow) + Details `InfoModal` + hub placeholder + BoardSession   | SFU             |
| **4.7** | ✅ __DEV__ multi-pin fan on GO (distinct colors; soft collide all); local pin from 4.5     | Full game rules |
| **4.8** | ✅ Menu home + board ⋯ (Leave / logout; health+locations `__DEV__`); block board back       | Lobby, WS, RTC  |
| **4.9** | ✅ Polish: attribution, Reanimated avatar, side-length, chrome; Phase 4 exit criteria       | New features    |

**4.8 detail (locked)**

- **Home** (`/(app)/index`): branded menu — **Play**, **Settings** (stub OK), **About Meetopoly** (stub OK), **Log out**. Health API card and “View locations” move behind **⋯** or `__DEV__` only (not primary CTAs).
- **Play** → `/(app)/board` (direct; no World picker / lobby yet).
- **Board panel top:** **⋯** menu — **Leave** (→ menu home), **Log out**; in `__DEV__`: **Health**, **Locations** list.
- Remove on-board **Back** Pressable. `BackHandler` + `gestureEnabled: false` (or equivalent) so hardware/swipe back cannot leave the board; hub **Leave** stays explicit.
- Settings / About: placeholder screens or short modals are enough for 4.8.

**4.9 detail (locked)**

- Attribution: **About** credits + per-location `attribution` on Details `InfoModal` / hub when present.
- Walk feel: avatar position via **Reanimated** shared values (UI thread); JS owns collision + nearby; `BOARD_WALK` constants stay code-only until Settings.
- Light side-length: label/icon scale tweaks; `React.memo` on `BoardTile`.
- Chrome: drop leftover Phase 4.x labels from play surfaces.

**Suggested files**

- `components/board/Board.tsx`, `BoardTile.tsx`, `boardLayout.ts` (index → rect/side/rotation)
- `components/board/BoardPanel.tsx`, `BoardAvatar.tsx`, `BoardPin.tsx`, `boardPins.ts`, `BoardOverflowMenu.tsx`
- `components/board/Joystick.tsx` (PanResponder; panel BR)
- `hooks/useBoardWalk.ts`, `hooks/useBlockHardwareBack.ts`
- Routes: `(app)/index` menu home; `(app)/board`; `(app)/hub/[slug]`; `(app)/settings`, `(app)/about`, `(app)/health` (`__DEV__`)

**Backend:** none beyond locations API (already done).

**Exit criteria**

- [x] `africa-1` readable 2D ring + center decks; icons + color groups
- [x] Local avatar walks board; blocked by outer edge + center Chance/Chest decks; soft vs pins
- [x] Ring Chance/Chest tiles remain walkable
- [x] Walk near city/air/utility → Enter → hub placeholder → Leave → board
- [x] Joystick usable from panel bottom-right; board stays 1:1 dominant
- [x] Signed-in **menu home**; Play opens board; board leave only via **⋯**; system/gesture back blocked on board
- [x] Smooth walk on mid-range targets (Reanimated avatar pose; memoized tiles; no per-frame React setState)

**Note on later phases:** Phase 5 adds World picker + lobby before board. Phase 7 syncs **board** avatar positions (and hub poses). Pins stay authoritative via game WS. Rolling moves pins without forcing avatars.

---

### Phase 5 — Tables lobby (2–6) + WebSocket basics

**Goal:** Play funnel: pick **World** → matchmaking lobby → **all Ready** → board. Seat sync eventually over **WebSocket** (not WebRTC). Ship a **mobile stub** first (5.0–5.5), then replace with real table HTTP/WS (5.6).

**Funnel (locked)**

```text
Menu → Play → World picker → Lobby (matchmaking pool) → all Ready (≥2) → Board
```

**Locks (Phase 5)**

| Topic | Decision |
| ----- | -------- |
| Matchmaking | Pick World → join waiting pool for that World (no invite codes in v1 stub) |
| Worlds list | All worlds from `GET /worlds` |
| Capacity | **2–6** hard cap |
| Ready | Disabled until ≥2 seated; toggle Ready anytime; bots auto-Ready after a short delay |
| New joiner | Keep existing Readys; newcomer starts unready |
| Start | When **every seated** player is Ready and count ≥2 → board (no separate Start/host) |
| Leave | Voluntary Leave frees seat immediately |
| Disconnect | Hold seat ~30–60s then free (stub timing OK) |
| Transport | Lobby seating = **WebSocket** later; stub is local-only until **5.6** |
| WebRTC | Not for lobby — Phase 7+ |

- **World** = board pack (`africa-1`, …). Do **not** confuse with **Locations** (board spaces).
- Pins packing **2×3** when 6 on one square → Phase 6 UI if needed.

**Mobile — incremental slices (implement + test one at a time)**

| Slice | Done when | Avoid |
| ----- | --------- | ----- |
| **5.0** | ✅ World picker from `useWorlds`; Play → picker; select World; temp Continue → board with `worldId` | Lobby, bots, Ready |
| **5.1** | ✅ Lobby shell `lobby/[worldId]`; 6 seat slots UI; Leave → picker | Matchmaking logic, bots |
| **5.2** | ✅ Local stub: enter pool as local seat; waiting copy until ≥2 | Bots, Ready, WS |
| **5.3** | ✅ Slow fake joiners toward 6 (cap); newcomer unready rule | Ready start, WS |
| **5.4** | ✅ Ready toggle (≥2); bots delayed auto-Ready; all Ready → board | Real WS |
| **5.5** | ✅ Disconnect-hold stub (45s); polish lobby chrome | Backend |
| **5.6** | ✅ Real `services/table` + WS matchmaking replaces local stub | Game M1 rules |

**Backend (primarily 5.6)**

1. `services/table` + repos
2. WebSocket adapter: join pool/lobby, seat + ready updates
3. OpenAPI create/join/list + WS events
4. Enforce 2–6; start when all Ready

**Suggested mobile files**

- `(app)/worlds.tsx` — World picker (5.0)
- `(app)/lobby/[worldId].tsx` — lobby (5.1+)
- `hooks/useLobbyStub.ts` — local seats/bots/ready until 5.6
- Later: `hooks/useTable`, `hooks/useTableSocket`

**Exit criteria**

- [x] Menu Play → World → lobby → all Ready → board (stub OK through 5.5)
- [x] Seats capped at 6; cannot start with &lt;2
- [x] Ready UI shows who is ready; start only when all seated are Ready
- [x] Two devices can share a lobby via WS (5.6)
- [x] Disconnect hold stubbed (30–60s)

---

### Phase 6 — Game M1 (authoritative) without hubs

**Goal:** Dice, pin movement, buy, rent, pass income, turn order, **5-minute timer**.

**M1 economy locks (do not re-litigate)**

| Topic | Decision |
| ----- | -------- |
| Currency | **MeetCoin** — double-bar capital **M** mark (₦ / ₩ family); HUD shows `[symbol] amount` |
| Start cash | **2000** MeetCoin per player |
| Pass GO | **200** MeetCoin (applied when movement lands in later slices) |
| Rent (cities) | `rents[0]` from seed (unimproved) |
| Rent (airports / utilities) | Classic Monopoly tables |
| Doubles | Classic: roll again; three doubles → jail later (M3) |
| End turn | Explicit **End** (not auto-end after buy/decline alone) |
| Turn order | **Seat order** (lowest seatIndex first) |
| Devices | Real tables need **2+** devices / accounts (no bots in production path) |
| HUD balances | Show **all** players’ MeetCoin on the board panel |

**Sub-phases (ship one at a time)**

| Slice | Done when | Avoid |
| ----- | --------- | ----- |
| **6.0** | ✅ Create `games` on all-Ready; `GET /games/{id}`; board pins on GO; MeetCoin HUD + toast; navigate with `gameId` | Dice, buy, timer |
| **6.1** | ✅ Dice roll + pin move + pass-GO cash (server truth); tile-by-tile pin motion; auto-advance turn | Buy/rent UI |
| **6.2** | Buy / decline unowned property | Trading |
| **6.3** | Rent payment on land | Houses |
| **6.4** | Explicit End turn + doubles re-roll | Timer |
| **6.5** | 5-min turn timer + skip | Hubs / presence |

**Backend**

1. `services/game` state machine (M1 only) — **6.0:** create + get snapshot
2. Persist `games` documents — **6.0**
3. Redis turn deadlines — **6.5**
4. WS events: `turnStarted`, `turnTick`/`deadline`, `diceRolled`, `pinMoved`, `propertyBought`, `rentPaid`, `turnSkipped` — **6.1+**
5. On timeout: skip turn (lose turn) — **6.5**
6. Table bridge: all-Ready → `GameStarter` → `table.gameId` + `in_game` — **6.0**

**Mobile**

1. Board HUD: MeetCoin, turn indicator — **6.0**; timer — **6.5**
2. Turn sheet actions: Roll, Buy, Decline, End — **6.1–6.4**
3. Show **pins** on slots from game state — **6.0** (all on GO); move in **6.1**
4. `hooks/useGame` + `queryKeys.game` — **6.0**

**Exit criteria**

- [ ] Full M1 round playable for 2–6 players
- [ ] Timer skip works
- [ ] Server is source of truth (client cannot forge money)
- [x] **6.0:** lobby start creates game; board loads snapshot (balances 2000, pins on GO, turn = seat 0)
- [x] **6.1:** `POST /games/{id}/roll`; pin walks tile-by-tile; pass GO +200; turn advances (doubles/End → 6.4)

---

### Phase 7 — Dual presence + board/hub avatar sync (DataChannels)

**Goal:** Pins on track vs avatars walking the **2D board** (and inside hubs); prediction + bounce-back.

**Backend**

1. Pion SFU adapter + signaling over WebSocket
2. Table/board presence room + hub rooms
3. `services/presence` validates board/hub avatar moves (respect collision rules); bounce-back invalid
4. On dice: update pin for all; do not force avatar off the board or out of hub

**Mobile**

1. `hooks/useWebRTC` / `useRoom`
2. Send local board (or hub) avatar pose 10–20 Hz
3. Interpolate remote avatars
4. Pins driven by game WS; avatars by DataChannel

**Exit criteria**

- [ ] Two players see each other’s board avatars move smoothly
- [ ] Illegal teleport gets bounce-back only on offender
- [ ] Roll updates pins while avatars keep walking / stay in hub

---

### Phase 8 — Location hubs + turn notify while in hub

**Goal:** Enter landmark; hub room; turn sheet without forcing board.

**Backend**

1. `services/hub` — enter/leave; hub WebRTC rooms by `hubId`
2. On `turnStarted`, if player `hubId != null`, emit targeted notify
3. Same room model ready for future voice (no mic yet)

**Mobile**

1. Hub scene load on Enter; leave returns to **board**
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

**Goal:** Mic audio in hub rooms and/or table room.

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

### 5.4 Performance (mobile board)

- Prefer Views + SVG tiles over heavy bitmaps; recycle/memo tile list only if needed.
- Keep right-rail video (later) capped; don’t re-render full board on every RTC frame.
- Target smooth 60 FPS UI on mid-range phones for the 2D board.

---

## 6. Suggested build order (summary)

```
0 Bootstrap → 1 OpenAPI → 2 Auth → 3 Locations+seed
→ 4 2D board + walk + panel → 5 Tables+WS → 6 Game M1
→ 7 Presence WebRTC (board+hub) → 8 Hubs+turn sheet → 9 UX polish
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

| Date       | Change                                                                                                            |
| ---------- | ----------------------------------------------------------------------------------------------------------------- |
| 2026-09-20 | Initial comprehensive plan from locked product/tech decisions                                                     |
| 2026-09-20 | Worlds + SVGCities billboards; `locations.json` / `africa-1` replaces Nigeria districts                           |
| 2026-09-20 | Expanded `locations.json` with all SVGCities Worlds (Europe×5, Asia×2, NA, SA, ME, Oceania, Central America)      |
| 2026-09-20 | Linked all 304 city properties to `city-icons/icons/{cc}-*.svg` + About/attribution from SVGCities metadata       |
| 2026-09-20 | Phase 0: chi HTTP `/health`, Mongo+Redis wiring, Expo Router + NativeWind + TanStack health screen                |
| 2026-09-20 | Locked landscape orientation + theme tokens (forest/gold/warm paper)                                              |
| 2026-09-21 | Locked fonts: Fraunces (display) + Figtree (UI/body); wired via expo-font                                         |
| 2026-09-21 | Frontend perf rule: Compiler-first; selective memo only (skill `frontend.md`)                                     |
| 2026-09-21 | Phase 1: orval OpenAPI codegen → `meetopoly-mobile/api/generated`; `useHealth` on generated client                |
| 2026-09-21 | Phase 2: email signup (6-digit SMTP) + bcrypt + opaque Redis sessions; Expo `(auth)` + secure-store               |
| 2026-09-21 | Locked forms: Formik `useFormik` + Yup in hooks (auth screens)                                                    |
| 2026-09-21 | Auth UI: RN inputs + Moti city drift + Lottie sun; sunny paper afternoon vibe                                     |
| 2026-09-22 | **Phase 4 pivot:** 2D Monopoly-style board (left) + right panel; drop GoG 3D overworld for v1; sub-phases 4.0–4.8 |
| 2026-09-22 | **africa-1 → 40 spaces:** classic even sides (corners 0/10/20/30); dropped Zanzibar; reseed required              |
| 2026-09-21 | Password reset (OTP) + session revoke-all; branded toasts via `notify()`                                          |
| 2026-09-21 | Signup resume after password: `/auth/signup/status` + login `needsProfile`                                        |
| 2026-09-21 | Phase 3: locations Mongo + seed CLI; Bearer `/worlds` + `/locations` (+ by id/slug); mobile `(app)/locations`     |
| 2026-09-23 | **4.8:** menu home (not board-as-home); board leave via ⋯ + back lock; lobby/World funnel → Phase 5; 2–6 pins 2×3 |
| 2026-09-23 | **4.9:** Reanimated avatar walk; attribution; BoardTile memo; Phase 4 exit criteria ticked |
| 2026-09-23 | **Phase 5 split:** sub-phases 5.0–5.6; matchmaking + all-Ready locks; stub before WS |
| 2026-09-23 | **5.1:** lobby shell `lobby/[worldId]`; 6 empty seats; Leave → Worlds; worlds Continue → lobby |
| 2026-09-23 | **5.2:** `useLobbyStub` seats local player; waiting copy until ≥2; Leave frees seat |
| 2026-09-23 | **5.3:** slow bot joiners (~3.2s) up to 6; newcomers always unready |
| 2026-09-23 | **5.4:** Ready toggle (≥2); bots auto-Ready ~1.8s; all Ready → board |
| 2026-09-23 | **5.5:** disconnect hold 45s (AppState + demo bot); Ready pills; hold banner |
| 2026-09-23 | **5.6:** `services/table` + Mongo `tables`; WS `/ws/tables/{id}`; mobile `useTableLobby` |
| 2026-09-23 | **Phase 6 split:** 6.0–6.5; M1 MeetCoin locks (2000 / pass GO 200 / seat-order / symbol HUD) |
| 2026-09-23 | **6.0:** `games` repo+svc; create on all-Ready; `GET /games/{id}`; board HUD+pins+toast |
| 2026-09-23 | Fix lobby matchmaking: never persist `starting` without game; abandon broken half-starts on Join |
| 2026-09-23 | **6.1:** Roll 2d6 + move + pass-GO; HTTP roll + poll; Reanimated tile-by-tile pins; auto-advance turn |
