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
- **Tables:** **2–6** players (pins pack up to 6 on one square — prefer **2×3** grid when crowded); **45-minute per-player time bank** (drains on your turn only; bank = 0 → eliminate); extra pause rules → Phase 13; notify players in hubs with a compact turn sheet.
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
| Presence sync  | Pins via game WS; **board** avatar poses Phase **7** (~10–20 Hz DataChannel); **hub** poses Phase **8**  |
| Voice          | Phase **10**; same WebRTC room model as presence                                                         |

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
- Turn time bank on `games.players[].timeRemainingMs` (+ `turnStartedAt` while current) — Phase **6.3** / **6.3b**; Redis mirror optional later
- Hot presence keys — **deferred**: Phase **7** uses SFU + service **memory** only; Redis presence keys when multi-instance / prod scale (typically **Phase 15**)

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

| Slice   | Done when                                                                                | Avoid           |
| ------- | ---------------------------------------------------------------------------------------- | --------------- |
| **4.0** | Landscape shell: 1:1 board placeholder + panel; reachable from home                      | Ring, walk      |
| **4.1** | Empty ring of slots from `boardIndex` (geometry only)                                    | Colors, icons   |
| **4.2** | Color bands + kind styling                                                               | Icons, walk     |
| **4.3** | Icons + `boardCode` / styling from `useLocations('africa-1')`                            | Walk, Enter     |
| **4.4** | ✅ Center brand + Chance/Chest deck shapes (`layout.decks` obstacles reserved)           | Movement        |
| **4.5** | ✅ Local avatar (pod + 1-letter) + joystick BR + edge/deck hard + pin soft; pin on GO    | Enter, net      |
| **4.6** | ✅ Walk-near Enter (nearest glow) + Details `InfoModal` + hub placeholder + BoardSession | SFU             |
| **4.7** | ✅ **DEV** multi-pin fan on GO (distinct colors; soft collide all); local pin from 4.5   | Full game rules |
| **4.8** | ✅ Menu home + board ⋯ (Leave / logout; health+locations `__DEV__`); block board back    | Lobby, WS, RTC  |
| **4.9** | ✅ Polish: attribution, Reanimated avatar, side-length, chrome; Phase 4 exit criteria    | New features    |

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

**Note on later phases:** Phase 5 adds World picker + lobby before board. Phase **7** syncs **board** avatar positions (DataChannels). Phase **8** adds hubs. Pins stay authoritative via game WS. Rolling moves pins without forcing avatars.

---

### Phase 5 — Tables lobby (2–6) + WebSocket basics

**Goal:** Play funnel: pick **World** → matchmaking lobby → **all Ready** → board. Seat sync eventually over **WebSocket** (not WebRTC). Ship a **mobile stub** first (5.0–5.5), then replace with real table HTTP/WS (5.6).

**Funnel (locked)**

```text
Menu → Play → World picker → Lobby (matchmaking pool) → all Ready (≥2) → Board
```

**Locks (Phase 5)**

| Topic       | Decision                                                                            |
| ----------- | ----------------------------------------------------------------------------------- |
| Matchmaking | Pick World → join waiting pool for that World (no invite codes in v1 stub)          |
| Worlds list | All worlds from `GET /worlds`                                                       |
| Capacity    | **2–6** hard cap                                                                    |
| Ready       | Disabled until ≥2 seated; toggle Ready anytime; bots auto-Ready after a short delay |
| New joiner  | Keep existing Readys; newcomer starts unready                                       |
| Start       | When **every seated** player is Ready and count ≥2 → board (no separate Start/host) |
| Leave       | Voluntary Leave frees seat immediately                                              |
| Disconnect  | Hold seat ~30–60s then free (stub timing OK)                                        |
| Transport   | Lobby seating = **WebSocket** later; stub is local-only until **5.6**               |
| WebRTC      | Not for lobby — Phase 7+                                                            |

- **World** = board pack (`africa-1`, …). Do **not** confuse with **Locations** (board spaces).
- Pins packing **2×3** when 6 on one square → Phase 6 UI if needed.

**Mobile — incremental slices (implement + test one at a time)**

| Slice   | Done when                                                                                           | Avoid                   |
| ------- | --------------------------------------------------------------------------------------------------- | ----------------------- |
| **5.0** | ✅ World picker from `useWorlds`; Play → picker; select World; temp Continue → board with `worldId` | Lobby, bots, Ready      |
| **5.1** | ✅ Lobby shell `lobby/[worldId]`; 6 seat slots UI; Leave → picker                                   | Matchmaking logic, bots |
| **5.2** | ✅ Local stub: enter pool as local seat; waiting copy until ≥2                                      | Bots, Ready, WS         |
| **5.3** | ✅ Slow fake joiners toward 6 (cap); newcomer unready rule                                          | Ready start, WS         |
| **5.4** | ✅ Ready toggle (≥2); bots delayed auto-Ready; all Ready → board                                    | Real WS                 |
| **5.5** | ✅ Disconnect-hold stub (45s); polish lobby chrome                                                  | Backend                 |
| **5.6** | ✅ Real `services/table` + WS matchmaking replaces local stub                                       | Game M1 rules           |

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

### Phase 6 — Game M1: movement + turn loop (authoritative), then light economy

**Goal:** Authoritative dice, pin movement, pass-GO income, turn order, **per-player time bank**. Property **buy / rent** follow once the movement loop is solid. Board avatar sync = Phase **7**; hubs = Phase **8**.

**Rules source:** classic Monopoly (see `meetopoly-mobile/resources/Monopoly_Complete_Rules_Guide.pdf`). Meetopoly keeps its own economy numbers and phased delivery — do **not** reshape phases just to match the booklet order.

**M1 economy / rules locks (do not re-litigate)**

| Topic                       | Decision                                                                                         |
| --------------------------- | ------------------------------------------------------------------------------------------------ |
| Currency                    | **MeetCoin** — double-bar capital **M**; HUD `[symbol] amount`                                   |
| Start cash                  | **2000** MeetCoin (ignore classic $1500 editions)                                                |
| Pass / land GO              | **+200** MeetCoin                                                                                |
| Free Parking                | **Nothing** (no jackpot — that is a house rule)                                                  |
| Rent (cities)               | `rents[0]` unimproved; color-set doubling when monopoly logic exists                             |
| Rent (airports / utilities) | Classic tables (rail 25/50/100/200; util 4× / 10× dice)                                          |
| Rent collection             | **Auto-pay** on land (digital; do not use “owner forgot → no rent”)                              |
| Unowned buyable             | Official: **Buy at list price** _or_ **Bank auctions** — auction ships in **Phase 13**, not here |
| Doubles                     | Re-roll after resolving the space; **three doubles → Jail** in M3 (Phase 12)                     |
| End turn                    | Explicit **End** after space is resolved (not auto-end mid-resolution)                           |
| Turn order                  | **Seat order** (lowest seatIndex first)                                                          |
| Devices                     | **2+** real accounts (no bots on production path)                                                |
| HUD balances                | Show **all** players’ MeetCoin                                                                   |

**Turn shape (official-aligned)**

```text
Roll → move (+pass GO if applicable) → resolve space →
  (later: buy / auction / rent / tax / cards / jail) →
  if doubles (and not 3rd): may Roll again → else End → next seat
```

**Sub-phases (ship one at a time)**

| Slice    | Done when                                                                                                                                                                                                                                                    | Avoid                                                                                           |
| -------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ | ----------------------------------------------------------------------------------------------- |
| **6.0**  | ✅ Create `games` on all-Ready; `GET /games/{id}`; pins on GO; MeetCoin HUD + toast; `gameId` nav                                                                                                                                                            | Dice, buy, timer                                                                                |
| **6.1**  | ✅ Dice + pin move + pass-GO; tile-by-tile motion; auto-advance (interim)                                                                                                                                                                                    | Buy, rent, End UI                                                                               |
| **6.2**  | ✅ Explicit **End** + **doubles** re-roll; stop auto-advance after roll; **game WS** push                                                                                                                                                                    | Buy, auction, Jail                                                                              |
| **6.2b** | ✅ Dice **roll animation** (Moti) on all devices when `lastRoll` updates; hold pin walk until dice land                                                                                                                                                      | Timer, buy, Lottie pack unless asked                                                            |
| **6.2c** | Leave board = **resign** (confirm); notify via game WS; last active player **wins**                                                                                                                                                                          | Full bankruptcy asset transfer (→ Phase 14); game WS disconnect hold → **7.5**                  |
| **6.3**  | ✅ Interim **3-min** per-turn AFK (`turnDeadline`) — **superseded by 6.3b**                                                                                                                                                                                  | —                                                                                               |
| **6.3b** | ✅ **45-min per-player time bank**; drain only on your turn; bank = 0 → eliminate; all banks on every HUD                                                                                                                                                    | Phase 13 pause actions (auction/trade/…); hubs                                                  |
| **6.4**  | ✅ **Buy at list price** for unowned city / airport / utility; ownership on game doc; classic price/rent ladder in seeds; buy modal shows 1–4 houses + hotel; **owner color chip** on tiles; **tap any tile** → info sheet (deed / Chance / Chest / tax / …) | Auction (→ Phase 13); if player skips buy, property stays unowned until 13; mortgage chip later |
| **6.5**  | ✅ **Rent** (+ tax to Bank; own tile = noop); classic rail/util formulas; auto-collect on land; block End/Roll if unpaid remainder                                                                                                                           | Houses, mortgage, cards; full bankruptcy raise-funds (→ Phase 14)                               |

**6.5 notes**

- On land (after Roll move): auto-collect **rent** (city `rents[0]`, ×2 if full color set owned; rail 25/50/100/200 by count; util 4×/10× dice total) or **tax** (`taxAmount` → Bank). Own tile / unowned buyable = no rent (buy still 6.4).
- `lastPayment` on Game for WS toasts; if cash short: pay all, set `pendingPayment`, `canEndTurn`/`canRoll` false until resign (Phase 14 bankruptcy later).
- Chance / Chest / Free Parking still noop this slice.

**6.4 notes**

- After landing on unowned `property` / `railroad` / `utility` with `price > 0`: `canBuy` + `buyOffer`.
- `POST /games/{id}/buy` deducts list price, appends deed (`boardIndex` + owner).
- Skip: End turn without buying — space stays unowned until Phase 13 auction.
- Classic US rent/price ladder by `boardIndex` in seeds (colors stay Meetopoly); buy modal lists Rent + 1–4 houses + Hotel.
- **Ownership chip:** outer-corner dot in owner `pinColor` on deed tiles (client from `deeds` + `players`).
- **Tap tile:** info overlay (buyable = deed + Available/Owned by; Chance/Chest = icon + name; tax = icon + name + amount). Disabled only while **local** buy modal is open.
- Rent on owned land is **6.5** (shipped).

**6.3b notes — time bank (locked)**

- **Bank:** each player starts with **45 minutes** (`timeRemainingMs` / display `mm:ss`). Same for 2–6 seats (no per-table scaling for v1).
- **Runs** only while that player is **current** (their turn). **Pauses** for everyone else (not their turn).
- **Does not reset** between turns — leftover bank carries forward until the game ends or they are eliminated.
- **Bank hits 0:** **auto-eliminate** (same outcome family as resign: `resigned` / out, skip turns, assets frozen until Phase 14). If one active player remains → they win (`status: finished`).
- **HUD (all devices):** show **every** player's remaining bank (not only local). Active player's bank ticks live; others show paused remainder.
- **Transport:** server is source of truth; debit on turn boundaries / tick; push via game WS. Clients display only.
- **Phase 13 (later):** list additional situations that **pause** the current player's bank (auction, open trade, raise-funds, …) without resetting it. Until then, the **only** pause rule is “not your turn.”
- **Interim 6.3** (`turnDeadline` 3‑min skip) remains in code until **6.3b** ships; then remove / replace.

**6.2c notes**

- ⋯ Leave → confirm (“leaving = resigning”) → `POST /games/{id}/resign`.
- Mark `resigned`; skip for turns; freeze assets until Phase 14.
- Game WS pushes updated `Game`; if one active player left → `status: finished` + winner fields.
- Explicit Leave only this slice. Game WS disconnect / app-kill hold → auto-resign is **Phase 7.5**.

**6.2b notes (client-only)**

- Server already authorizes faces (`die1`/`die2`) and pushes via `/ws/games/{id}`.
- Do **not** stream animation frames over the network — each client plays the same local tumble that **lands on** the server values.
- Gate pin tile-walk until the dice finish (~0.8–1.5s). Key off `lastRoll` so reconnects do not replay forever.

**Backend**

1. `services/game` state machine — **6.0+**
2. Persist `games` — **6.0**; ownership fields when **6.4**
3. Per-player time bank on game doc — **6.3b** (replaces interim `turnDeadline`); Redis mirror optional later
4. Game WS push on roll / end-turn — **done**; later events (`propertyBought`, `rentPaid`, …) as slices need
5. Table bridge: all-Ready → game — **6.0**

**Mobile**

1. HUD: MeetCoin + turn — **6.0**; **all players’ banks** ticking/paused — **6.3b**
2. Actions: Roll — **6.1**; End — **6.2**; Buy — **6.4**; (Auction UI — Phase 13)
3. Pins from game state — **6.0**; animate tile-walk — **6.1**; dice tumble — **6.2b**
4. `useGame` + **game WebSocket** (`/ws/games/{id}`) into Query cache; slow HTTP poll only if socket down — **done**
5. **Transport lock:** **HTTP = commands** (roll / end-turn / resign / buy); **WS = state fan-out**. Do not move mutations to WS for “performance” — see changelog / Phase 6 notes.

**Exit criteria**

- [ ] Movement + End + doubles + dice anim + time bank playable for 2–6
- [x] Buy + rent when 6.4–6.5 done (auction still Phase 13)
- [ ] Server is source of truth (client cannot forge money / position)
- [x] **6.0:** lobby start creates game; snapshot (2000, pins on GO, turn = seat 0)
- [x] **6.1:** roll + tile walk + pass GO +200; interim auto-advance
- [x] **6.2:** End turn + doubles re-roll; third doubles skips move (Jail later); game WS
- [x] **6.2b:** synced dice roll animation; pin walk waits for dice to land
- [x] **6.2c:** resign on Leave + last-player-wins
- [x] **6.3:** interim 3-min AFK (superseded)
- [x] **6.3b:** 45-min bank; eliminate at 0; all banks on HUD
- [x] **6.4:** buy at list price; deeds on game; skip leaves unowned
- [x] **6.5:** rent + tax auto-collect; pendingPayment blocks End/Roll

---

### Phase 7 — Board avatar sync (DataChannels)

**Goal:** Dual presence on the **2D board** — rules **pins** (game WS) vs social **avatars** (WebRTC DataChannels); prediction + bounce-back. **Board-only** this phase; **hubs = Phase 8**.

**Hot state (locked):** SFU + `services/presence` in **process memory** for v1. Redis presence keys → multi-instance / **Phase 15** (not required to exit Phase 7).

**Sub-phases (ship one at a time)**

| Slice   | Done when                                                                                                                                               | Avoid                                         |
| ------- | ------------------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------- |
| **7.0** | Pion SFU adapter + WS signaling; join/leave a **board presence room** per table/game; connect/disconnect lifecycle works (no pose payload yet)          | Pose messages, hubs, voice, Redis presence    |
| **7.1** | DataChannel up; locked **pose message shape**; SFU fans out unvalidated; mobile `useWebRTC` / `useRoom` joins the board room                            | Server validation, interpolation polish, hubs |
| **7.2** | Local avatar publishes ~10–20 Hz; remotes drawn + **interpolated** on the 2D board; pins still from game WS only                                        | Bounce-back, hubs, voice                      |
| **7.3** | ~~`services/presence` validates board moves; bounce-back~~ — **SKIPPED** (see notes)                                                                 | Hubs, voice, Redis presence                   |
| **7.4** | Dual-presence proof: **Roll moves pins** while avatars keep walking; presence reconnect / leave room / rate-limit hardening; room-id hook ready for Phase 8 hubs | Full hub enter/UX (→ 8); voice (→ 10); game resign-on-disconnect |
| **7.5** | Game WS disconnect hold (`GAME_DISCONNECT_HOLD`, default 3m / local 45s); reconnect cancels; expire → auto-resign; presence-only drops never resign; hide resigned pins | Resume-game CTA / SecureStore rejoin UI; pause time bank on disconnect; presence tear-down as resign |

**7.0 notes**

- Signaling over WebSocket (auth same family as table/game WS).
- One **board** presence room per active table/game — not lobby seating (lobby stays WS-only).
- No hub rooms yet; leave a stub room-id convention so Phase 8 can add `hub:{hubId}`.
- **Done (2026-09-24):** Pion SFU (`internal/adapters/webrtc`), `GET /ws/presence/board/{gameId}?token=`, room `board:{gameId}`, Google STUN, idle DC label `presence`, mobile `useBoardPresence` + join/leave toasts. Requires Expo **dev client** (not Expo Go) for `react-native-webrtc`.

**7.1–7.2 notes**

- Unvalidated fan-out first so remotes are visible early; wire validation in **7.3**.
- Pose payload: board-local `{ type:"pose", userId, username, x, y, rot?, t? }` (0..1 board-norm); SFU stamps identity; ~10–20 Hz send; client interpolates remotes in **7.2**.
- Pins remain authoritative via existing `/ws/games/{id}` — do not drive pins over DataChannel.
- **Done (2026-09-24) 7.1:** `StampPose` + SFU `forwardPose`; mobile `presencePose` + `useBoardPresence.sendPose` / `remotes`; board publishes ~10 Hz when DC open. Remotes not drawn yet (→ **7.2**).
- **Done (2026-09-24) 7.2:** `useInterpolatedBoardPose` + `BoardRemoteAvatar`; remotes drawn under local avatar with ~100 ms linear ease; accents from game `pinColor`.

**7.3 notes — SKIPPED (2026-09-24)**

- Decision: **do not** ship server presence validation / bounce-back / avatar↔avatar collision.
- Collision stays **client-only** as already implemented in board walk: hard edge + center Chance/Chest decks; soft shove vs **game pins**; avatars may overlap each other.
- Spoofed / teleported poses are out of scope for v1 Phase 7 (trust client + SFU identity stamp only). Revisit later if abuse shows up.
- Exit criterion for 7.3 marked skipped — not required to exit Phase 7.

**7.4 notes**

- Prove independence: dice/pin walk must not snap or force social avatars.
- Presence reconnect rejoins board room without breaking game WS; leave board tears down presence peer cleanly.
- Harden WebRTC when PC/DC dies while presence signaling WS stays up (renegotiate or bounce socket).
- Pose fan-out rate-limit on SFU (~20 Hz/peer); `HubRoomID` stub next to `BoardRoomID` for Phase 8.
- **Done (2026-09-24) 7.4:** SFU `allowPose` + `HubRoomID`; mobile PC/DC recover while WS open; `disconnect()` on leave/hub enter; pins remain game-WS-only (no avatar snap on roll).

**7.5 notes (locked)**

- Trigger: **`/ws/games/{gameId}`** closes only (app kill, long background, bad net). **Not** presence WS / WebRTC glitches.
- Hold via env **`GAME_DISCONNECT_HOLD`** (Go duration; default **3m**; local `.env` **45s** for faster testing). Silent (no banner/toast for others while held).
- Reconnect to game WS within hold → cancel; player stays in game.
- Hold expires → same **Resign** path as ⋯ Leave → game WS fan-out; if one active player left → `finished` + winner.
- ⋯ Leave remains **immediate** resign (confirm).
- Time bank **keeps draining** on their turn while disconnected (no pause — pausing would be exploitable).
- No Play-screen “resume game” / SecureStore rejoin UI in this slice (cold start after kill may miss rejoin; hold then resign is acceptable).
- Resigned players’ pins are hidden on the board (HUD still shows “out”).
- **Done (2026-09-24) 7.5:** `games.Disconnect` / `CancelDisconnectHold`; GameHub wires close→hold and reconnect→cancel; resigned pins filtered in `asPins`.

**Backend**

1. WebRTC (Pion) adapter + signaling WS — **7.0**
2. Board presence room join/leave — **7.0**
3. DataChannel fan-out of pose messages — **7.1**
4. ~~`services/presence` board validation + bounce-back — **7.3**~~ **SKIPPED** (client collision only; see 7.3 notes)
5. On dice: pin update via game WS only; do not force avatar pose — **7.4**
6. Game WS disconnect hold (3 min) → auto-resign — **7.5**
7. Pose rate-limit + PC/DC recover while signaling up + `HubRoomID` stub — **7.4**

**Mobile**

1. `hooks/useWebRTC` / `useRoom` / `useBoardPresence` — **7.0–7.1**
2. Publish local board avatar pose ~10–20 Hz — **7.2**
3. Interpolate / render remote board avatars — **7.2**
4. ~~Apply bounce-back corrections — **7.3**~~ **SKIPPED**
5. Pins from game WS; avatars from DataChannel — **throughout**
6. Presence PC/DC recover + explicit disconnect on leave — **7.4**
7. Game WS reconnect stays client-side; server owns hold/resign — **7.5**

**Exit criteria**

- [x] **7.0:** two clients join/leave the same board presence room reliably
- [x] **7.1:** pose messages fan out over DataChannel
- [x] **7.2:** two players see each other’s board avatars move smoothly
- [x] **7.3:** SKIPPED — no server bounce-back / avatar↔avatar collision (client edge+deck+pin soft only)
- [x] **7.4:** Roll updates pins while avatars keep walking (no forced avatar snap); presence reconnect hardened
- [x] **7.5:** game WS down hold → auto-resign + last-player-wins; presence-only drop does not resign
- [x] Hubs deferred from Phase 7 — start at **8.0**

---

### Phase 8 — Location hubs + turn notify while in hub

**Goal:** Enter landmark; **hub** WebRTC presence (extends Phase 7 room model); turn sheet without forcing the board screen. Board avatar sync already shipped in Phase 7.

**Hot state (locked):** Same in-memory SFU as Phase 7. Hub rooms keyed by `HubRoomID` → `hub:{hubId}`. Redis multi-instance → Phase 15.

**Cross-table (locked):** Players from **different tables/games may meet** in the same location hub (product default: **allow**).

**Collision / bounce-back:** Not required this phase (Phase **7.3 skipped**; hub walk stays client-side / simple scene).

**Sub-phases (ship one at a time)**

| Slice   | Done when                                                                                                                                                         | Avoid                                      |
| ------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------ |
| **8.0** | Hub presence **signaling + room lifecycle**: Enter leaves **board** presence and joins `hub:{hubId}`; Leave hub rejoins **board** presence; **game WS stays up** | Hub poses, turn sheet, voice, polish scene |
| **8.1** | Hub DataChannel poses (reuse locked **pose** shape; hub-local 0..1); remotes drawn + interpolated in the hub scene                                                  | Turn notify, voice, board “in hub” chrome  |
| **8.2** | Server/game knows `hubId` when entered; peers still on the **board** see that player as in-hub (frozen last board pose and/or clear “in hub” affordance)         | Turn sheet, voice                          |
| **8.3** | **Turn notify** while in hub + compact sheet: time bank + Roll / basic actions + **Open board**; pin updates on roll while avatar stays in hub                    | Voice (→ 10); auctions/trades              |
| **8.4** | Path hardening: Open board / Leave hub / presence reconnect without dropping game WS; no false resign; cross-table meet verified                                  | Voice; hub presence validation server-side |

**8.0 notes**

- Reuse Pion SFU + auth family as board presence; route e.g. `GET /ws/presence/hub/{hubId}?token=` (exact path in OpenAPI when implementing).
- `hubId` from location seeds / existing `hub:{world}:{slug}` convention; `HubRoomID` already stubbed in 7.4.
- Enter: tear down **board** peer cleanly → attach hub peer (no game resign). Leave: reverse.
- Upgrade hub placeholder only as needed to prove join/leave + roster toasts (full art later).
- Game WS + pins remain authoritative and connected the whole time.

**8.1 notes**

- Same `{ type:"pose", userId, username, x, y, rot?, t? }` as board; SFU stamps identity + rate-limit.
- Hub scene publishes ~10–20 Hz; remotes interpolate (mirror 7.2 patterns).
- No mic / media tracks.

**8.2 notes**

- Persist or fan-out `hubId` (game player field and/or presence service) so board clients know who left for a hub.
- Dual presence proof: board peers don’t require that player’s board DC; pin still moves on dice via game WS.
- Avoid treating hub enter as presence “left game” toast (already softened in 7.5 polish).

**8.3 notes**

- On turn start, if `hubId != null`, targeted notify to that player (and compact UI on their hub screen).
- Sheet: remaining bank (6.3b rules unchanged) + Roll / End (as allowed) + Open board.
- Rolling from hub moves **pin** on the board for everyone; hub avatar does not snap to the pin.
- Open board → board view without dropping game WS; decide whether leaving hub presence is required to walk the board again (default: Open board can keep `hubId` until explicit Leave hub — confirm in impl if UX fights that).

**8.4 notes**

- Hardening pass: reconnect hub/board presence; Leave hub ↔ board; Open board; app background does **not** resign (7.5 still owns that).
- Manual proof: two tables, same city hub, see each other; turn sheet while in hub; return to board cleanly.

**Backend**

1. Hub presence WS + SFU room join/leave — **8.0**
2. Hub pose fan-out (reuse StampPose / rate-limit) — **8.1**
3. `hubId` on player / enter-leave service — **8.2**
4. Turn-start notify when `hubId != null` — **8.3**
5. Reconnect / leave-path hardening — **8.4**

**Mobile**

1. Enter/Leave switches presence rooms; game WS untouched — **8.0**
2. Hub remotes + local pose publish — **8.1**
3. Board “in hub” / frozen pose for peers — **8.2**
4. Hub turn BottomSheet + Open board — **8.3**
5. Path polish / regression pass — **8.4**

**Exit criteria**

- [x] **8.0:** enter hub joins hub room and leaves board presence; leave hub rejoins board; game WS never drops for that alone
- [ ] **8.1:** two players see each other’s hub avatars move smoothly
- [x] **8.2:** board peers see in-hub players correctly; pins still update on roll
- [ ] **8.3:** turn notify + sheet works in hub; Open board reaches board view
- [ ] **8.4:** cross-table meet + leave/open/reconnect hardened; no false resign from hub flows
- [ ] Voice deferred — not required for Phase 8 exit (→ **Phase 10**)

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

**Official alignment:** even-build across a color group; max 4 houses then hotel; sell buildings back at half price; cannot build if any deed in the set is mortgaged; house shortage → auction for last houses.

**Exit criteria:** Even-build enforced server-side; UI to buy/sell houses (and hotels).

---

### Phase 12 — Rules M3 (jail + cards)

**Official alignment:** Go to Jail (land / card / three doubles); Just Visiting vs in Jail; exit via $50, Get Out of Jail Free, or doubles within 3 turns; Chance / Community Chest decks; Free Parking stays a noop.

**Exit criteria:** Jail entry/exit paths; Chance/Community Chest deck from config or seed.

---

### Phase 13 — Rules M4 (trading + auctions)

**Official alignment:** Players may trade deeds, cash, and Get Out of Jail Free cards. **When a player lands on unowned property and does not buy at list price, the Bank auctions it immediately** — all players may bid (including the one who declined). Also: house-shortage auctions if not done in M2; bankruptcy-to-bank may re-auction deeds (coord with Phase 14).

**Time bank pauses (encompassing — locked here)**

Baseline (**6.3b**): each player has a **45-minute** bank that drains **only on their turn** and **auto-eliminates at 0**. Until this phase, the only pause is “not your turn.”

When buy / auction / trade / debt flows exist, **also pause** (do not reset) the relevant bank(s) during:

- **Bank auction** (prefer a short shared auction sub-clock so bid time does not eat personal banks unfairly)
- **Open trade offers** awaiting accept/decline
- **Forced raise-funds** before bankruptcy (coord Phase 14)

Document the exact pause list in this phase’s implementation notes; do not invent pauses ad hoc in earlier slices.

**Exit criteria:** Multi-property / cash trades; **auction when purchase declined** (completes the official unowned-land rule started in Phase 6.4 buy); bank pause list enforced with those flows.

---

### Phase 14 — Rules M5 (bankruptcy)

**Official alignment:** Raise funds (sell buildings, mortgage, trade) before elimination; debt to player → assets transfer; debt to Bank → deeds return to Bank (auction where applicable); bankrupt token leaves the game.

**Exit criteria:** Debt resolution; player elimination; assets transfer correctly.

---

### Phase 15 — Production hardening (Contabo)

**Goal:** Deploy binary to Contabo; Redis on VPS; MongoDB Atlas; Google SMTP prod creds; TLS reverse proxy.

**Ask before:** systemd unit contents, nginx vs Caddy, TURN (coturn).

**Also consider here (if multi-instance):** Redis **hot presence** keys (Phase 7 ships memory-only).

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
→ 7 Presence WebRTC (board) → 8 Hubs 8.0–8.4 + turn sheet → 9 UX polish
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

| Date       | Change                                                                                                                                                                                                                                    |
| ---------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 2026-09-20 | Initial comprehensive plan from locked product/tech decisions                                                                                                                                                                             |
| 2026-09-20 | Worlds + SVGCities billboards; `locations.json` / `africa-1` replaces Nigeria districts                                                                                                                                                   |
| 2026-09-20 | Expanded `locations.json` with all SVGCities Worlds (Europe×5, Asia×2, NA, SA, ME, Oceania, Central America)                                                                                                                              |
| 2026-09-20 | Linked all 304 city properties to `city-icons/icons/{cc}-*.svg` + About/attribution from SVGCities metadata                                                                                                                               |
| 2026-09-20 | Phase 0: chi HTTP `/health`, Mongo+Redis wiring, Expo Router + NativeWind + TanStack health screen                                                                                                                                        |
| 2026-09-20 | Locked landscape orientation + theme tokens (forest/gold/warm paper)                                                                                                                                                                      |
| 2026-09-21 | Locked fonts: Fraunces (display) + Figtree (UI/body); wired via expo-font                                                                                                                                                                 |
| 2026-09-21 | Frontend perf rule: Compiler-first; selective memo only (skill `frontend.md`)                                                                                                                                                             |
| 2026-09-21 | Phase 1: orval OpenAPI codegen → `meetopoly-mobile/api/generated`; `useHealth` on generated client                                                                                                                                        |
| 2026-09-21 | Phase 2: email signup (6-digit SMTP) + bcrypt + opaque Redis sessions; Expo `(auth)` + secure-store                                                                                                                                       |
| 2026-09-21 | Locked forms: Formik `useFormik` + Yup in hooks (auth screens)                                                                                                                                                                            |
| 2026-09-21 | Auth UI: RN inputs + Moti city drift + Lottie sun; sunny paper afternoon vibe                                                                                                                                                             |
| 2026-09-22 | **Phase 4 pivot:** 2D Monopoly-style board (left) + right panel; drop GoG 3D overworld for v1; sub-phases 4.0–4.8                                                                                                                         |
| 2026-09-22 | **africa-1 → 40 spaces:** classic even sides (corners 0/10/20/30); dropped Zanzibar; reseed required                                                                                                                                      |
| 2026-09-21 | Password reset (OTP) + session revoke-all; branded toasts via `notify()`                                                                                                                                                                  |
| 2026-09-21 | Signup resume after password: `/auth/signup/status` + login `needsProfile`                                                                                                                                                                |
| 2026-09-21 | Phase 3: locations Mongo + seed CLI; Bearer `/worlds` + `/locations` (+ by id/slug); mobile `(app)/locations`                                                                                                                             |
| 2026-09-23 | **4.8:** menu home (not board-as-home); board leave via ⋯ + back lock; lobby/World funnel → Phase 5; 2–6 pins 2×3                                                                                                                         |
| 2026-09-23 | **4.9:** Reanimated avatar walk; attribution; BoardTile memo; Phase 4 exit criteria ticked                                                                                                                                                |
| 2026-09-23 | **Phase 5 split:** sub-phases 5.0–5.6; matchmaking + all-Ready locks; stub before WS                                                                                                                                                      |
| 2026-09-23 | **5.1:** lobby shell `lobby/[worldId]`; 6 empty seats; Leave → Worlds; worlds Continue → lobby                                                                                                                                            |
| 2026-09-23 | **5.2:** `useLobbyStub` seats local player; waiting copy until ≥2; Leave frees seat                                                                                                                                                       |
| 2026-09-23 | **5.3:** slow bot joiners (~3.2s) up to 6; newcomers always unready                                                                                                                                                                       |
| 2026-09-23 | **5.4:** Ready toggle (≥2); bots auto-Ready ~1.8s; all Ready → board                                                                                                                                                                      |
| 2026-09-23 | **5.5:** disconnect hold 45s (AppState + demo bot); Ready pills; hold banner                                                                                                                                                              |
| 2026-09-23 | **5.6:** `services/table` + Mongo `tables`; WS `/ws/tables/{id}`; mobile `useTableLobby`                                                                                                                                                  |
| 2026-09-23 | **Phase 6 split:** 6.0–6.5; M1 MeetCoin locks (2000 / pass GO 200 / seat-order / symbol HUD)                                                                                                                                              |
| 2026-09-23 | **6.0:** `games` repo+svc; create on all-Ready; `GET /games/{id}`; board HUD+pins+toast                                                                                                                                                   |
| 2026-09-23 | Fix lobby matchmaking: never persist `starting` without game; abandon broken half-starts on Join                                                                                                                                          |
| 2026-09-23 | **6.1:** Roll 2d6 + move + pass-GO; HTTP roll + poll; Reanimated tile-by-tile pins; auto-advance turn                                                                                                                                     |
| 2026-09-23 | Rules alignment: Phase 6 = movement+turn loop first; official Buy→Auction stays Phase 13; Free Parking noop; MeetCoin 2000/200                                                                                                            |
| 2026-09-23 | **6.2:** `end-turn` + doubles re-roll; `turnPhase` awaiting_roll/end; third doubles no move                                                                                                                                               |
| 2026-09-23 | **Game WS:** `/ws/games/{id}` push on roll/end-turn; mobile drops 2s poll (5s fallback if socket down)                                                                                                                                    |
| 2026-09-23 | **6.2b** added: synced dice roll animation (client Moti/Reanimated on `lastRoll`; pin walk waits)                                                                                                                                         |
| 2026-09-23 | **6.2b:** Moti dice overlay + `useDiceRollMotion`; pin `holdWalk` until tumble settles                                                                                                                                                    |
| 2026-09-23 | **6.2c:** Leave = resign (confirm); last active player wins; `POST /games/{id}/resign`                                                                                                                                                    |
| 2026-09-23 | **Timer:** 5→**3 min** AFK in 6.3; encompassing pause/sub-clock rules locked under **Phase 13**                                                                                                                                           |
| 2026-09-23 | **Timer 6.3b (locked):** **45 min/player** bank; drains on turn only; **0 → eliminate**; all banks on every HUD; Phase 13 adds auction/trade/raise-funds pauses. Interim 3‑min AFK superseded. HTTP commands + WS fan-out kept for scale. |
| 2026-09-23 | **6.3b shipped:** `timeRemainingMs` + `turnStartedAt`; bank drain/eliminate; HUD all banks                                                                                                                                                |
| 2026-09-23 | **6.4:** `POST /buy`; deeds + buyOffer/canBuy; skip = End without auction (→ 13)                                                                                                                                                          |
| 2026-09-23 | **6.4 polish:** classic rent/price ladder in seeds by boardIndex; buy modal 4 houses + icon/name align                                                                                                                                    |
| 2026-09-23 | **6.4 polish:** owner pinColor chip on tiles; tap-any-square TileInfoOverlay (buy taps disabled for buyer only)                                                                                                                           |
| 2026-09-23 | **6.5:** auto rent/tax on land; lastPayment + pendingPayment; block End/Roll if unpaid; monopoly ×2 base                                                                                                                                  |
| 2026-09-23 | **6.5 UX:** gate buy modal + Pass-GO/rent toasts until pin settles; `POST /pin-color` syncs avatar accent; cash tick animation                                                                                                            |
| 2026-09-24 | **UX polish:** buy toast titles by kind; hide local HUD row; Title-Case usernames on signup + display; winner modal row CTAs                                                                                                              |
| 2026-09-24 | **Lobby pin colors:** unique `pinColor` on seat join → game; HUD You · turn + filter; stop random accent overwrite                                                                                                                        |
| 2026-09-24 | **Phase 7 split:** 7.0–7.4 board avatar DataChannels (memory SFU); hubs stay **Phase 8**; Redis presence → 15 / multi-node                                                                                                                |
| 2026-09-24 | **7.0:** Pion SFU + `/ws/presence/board/{gameId}`; STUN; idle `presence` DC; mobile `useBoardPresence` + join/leave toasts (dev client)                                                                                                   |
| 2026-09-24 | **7.5 locked (plan):** game WS 3‑min silent hold → auto-resign; presence drop ≠ resign; bank keeps draining; no resume CTA this slice                                                                                                     |
| 2026-09-24 | **Phase 8 split:** 8.0 hub room lifecycle; 8.1 hub poses; 8.2 hubId + board in-hub affordance; 8.3 turn notify + sheet; 8.4 harden; cross-table meet **allow**; voice → 10                                                              |
| 2026-09-24 | **8.0:** `GET /ws/presence/hub/{hubId}`; Enter leaves board SFU room / joins hub; Leave reverse; game WS stays; mobile `useHubPresence` + board `useFocusEffect`                                                                 |
| 2026-09-25 | **8.2:** `POST enter-hub` / `leave-hub`; `GamePlayer.hubId`; panel `Name(in CODE)`; Board pin/avatar layers memoized to cut avatar hitch during pin walks                                                                              |
| 2026-09-24 | **Mobile UX:** hide status bar app-wide; board panel extra top padding so ⋯ clears the top edge                                                                                                                                         |
