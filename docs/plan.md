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
- **Interim (until Phase 13 auction):** ~~End turn blocked while `buyOffer` open~~ — **replaced in 13.0** by Buy \| start-auction; End blocked while buyOffer or active auction.
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

| Slice   | Done when                                                                                                                                                               | Avoid                                                                                                |
| ------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------- |
| **7.0** | Pion SFU adapter + WS signaling; join/leave a **board presence room** per table/game; connect/disconnect lifecycle works (no pose payload yet)                          | Pose messages, hubs, voice, Redis presence                                                           |
| **7.1** | DataChannel up; locked **pose message shape**; SFU fans out unvalidated; mobile `useWebRTC` / `useRoom` joins the board room                                            | Server validation, interpolation polish, hubs                                                        |
| **7.2** | Local avatar publishes ~10–20 Hz; remotes drawn + **interpolated** on the 2D board; pins still from game WS only                                                        | Bounce-back, hubs, voice                                                                             |
| **7.3** | ~~`services/presence` validates board moves; bounce-back~~ — **SKIPPED** (see notes)                                                                                    | Hubs, voice, Redis presence                                                                          |
| **7.4** | Dual-presence proof: **Roll moves pins** while avatars keep walking; presence reconnect / leave room / rate-limit hardening; room-id hook ready for Phase 8 hubs        | Full hub enter/UX (→ 8); voice (→ 10); game resign-on-disconnect                                     |
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

**Hub capacity (locked):** **Max 16** peers per hub SFU room (reject join when full). Driven by mobile pose/UI comfort; raise later only if measured OK. Redis presence (Phase 15) is multi-instance sharing — not required for this cap.

**Hub leave (locked):** Leave only via explicit **X** control. Block Android back + iOS swipe-back on the hub screen (same idea as board ⋯).

**Hub chrome (deferred → Phase 9):** 3-pane landscape — left media stub (cameras / Phase 10), center walk floor with location name + description underfoot, right worldwide roster (pin color + username + country) + joystick BR + X. Country on hub roster and board HUD when available. Placeholder hub UI is fine until that slice.

**Collision / bounce-back:** Not required this phase (Phase **7.3 skipped**; hub walk stays client-side / simple scene).

**Sub-phases (ship one at a time)**

| Slice   | Done when                                                                                                                                                        | Avoid                                      |
| ------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------ |
| **8.0** | Hub presence **signaling + room lifecycle**: Enter leaves **board** presence and joins `hub:{hubId}`; Leave hub rejoins **board** presence; **game WS stays up** | Hub poses, turn sheet, voice, polish scene |
| **8.1** | Hub DataChannel poses (reuse locked **pose** shape; hub-local 0..1); remotes drawn + interpolated in the hub scene                                               | Turn notify, voice, board “in hub” chrome  |
| **8.2** | Server/game knows `hubId` when entered; peers still on the **board** see that player as in-hub (frozen last board pose and/or clear “in hub” affordance)         | Turn sheet, voice                          |
| **8.3** | **Turn notify** while in hub + compact sheet: time bank + Roll / basic actions + **Open board**; pin updates on roll while avatar stays in hub                   | Voice (→ 10); auctions/trades              |
| **8.4** | Path hardening: Open board / Leave hub / presence reconnect without dropping game WS; no false resign; cross-table meet verified                                 | Voice; hub presence validation server-side |

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
- Board remotes: prefer live/linger poses; if `hubId` set and no pose (e.g. after board presence reconnect), draw a frozen avatar at the hub tile center.
- Avoid treating hub enter as presence “left game” toast (already softened in 7.5 polish).

**8.3 notes**

- On turn start, if local player `hubId != null`, client toast + compact turn sheet (game WS `state` is enough — no new event type).
- Sheet: remaining bank (6.3b) + **2×2** actions — Roll | End · Open board | Keep walking.
- **End / Roll gate:** same settle window as board (`turnBusy` = dice hold + pin walk estimate) so End does not light up mid-roll.
- **Unowned land from hub:** compact `HubBuySheet` — stripped deed name + time bank + **Buy** (affordability check) | **Open board**; after buy, primary becomes **End turn**; **Not now** dismisses to turn sheet / keep walking. Skip-buy still leaves deed unowned until Phase 13 auction.
- Hub = lightweight turn actions; board = full economy UI (buy deed, later auction/trade/raise-funds).
- Rolling from hub moves **pin** on the board for everyone; hub avatar does not snap to the pin.
- **Open board:** pop to board **without** `leave-hub` (keep `hubId` until explicit X Leave).
- Hub: `gestureEnabled: false` + `useBlockHardwareBack`; Leave = X icon only.
- SFU rejects new hub joins when room already has 16 distinct peers (reconnect of same userId still allowed).

**8.4 notes**

- Hardening pass: reconnect hub/board presence; Leave hub ↔ board; Open board; app background does **not** resign (7.5 still owns that).
- **Stale enter guard:** `GamePlayer.hubRevision` bumps on every `leave-hub` (and resign). `enter-hub` may send `hubRevision`; if older than server, enter is ignored. Mobile aborts in-flight enter before leave.
- **Hub presence:** seed remotes from `welcome.peers` (+ peer-joined) with frozen poses; clear remotes on soft reconnect; hub-full returns WS `{type:error}` and client stops retry thrash.
- Manual proof: two tables, same city hub, see each other; turn sheet while in hub; return to board cleanly; rapid enter→X leave does not leave stuck `hubId`.

**Backend**

1. Hub presence WS + SFU room join/leave — **8.0**
2. Hub pose fan-out (reuse StampPose / rate-limit) — **8.1**
3. `hubId` on player / enter-leave service — **8.2**
4. Hub max **16** on Attach + client turn detect on `state` — **8.3**
5. `hubRevision` stale-enter ignore + hub-full error frame — **8.4**

**Mobile**

1. Enter/Leave switches presence rooms; game WS untouched — **8.0**
2. Hub remotes + local pose publish — **8.1**
3. Board “in hub” / frozen pose for peers — **8.2**
4. Hub turn sheet + Open board + X-only leave + block system back — **8.3**
5. Enter/leave sequencing + welcome seed + hub-full stop + soft-reconnect clear — **8.4**

**Exit criteria**

- [x] **8.0:** enter hub joins hub room and leaves board presence; leave hub rejoins board; game WS never drops for that alone
- [x] **8.1:** two players see each other’s hub avatars move smoothly
- [x] **8.2:** board peers see in-hub players correctly; pins still update on roll
- [x] **8.3:** turn notify + sheet works in hub; Open board keeps hubId; X-only leave; hub cap 16
- [x] **8.4:** cross-table meet + leave/open/reconnect hardened; no false resign from hub flows
- [ ] Voice deferred — not required for Phase 8 exit (→ **Phase 10**)

---

### Phase 9 — Polish M1 UX + economy feedback

**Goal:** Property cards, rent toasts, Moti transitions, `@expo/ui` settings; **hub scene chrome** (3-pane + roster + country) per Phase 8 locked notes.

**Mobile**

- Branded cards (NativeWind + Moti)
- Settings switches via `@expo/ui`
- Error/empty states
- **Hub scene:** left media stub, center branded floor, right worldwide roster (≤16) + country; board HUD country when exposed

**Hub chrome sub-slices (9.0)**

| Slice    | Done when                                                                                                                                                              |
| -------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **9.0a** | Country on presence `welcome` / `peer-joined` / roster peers; `GamePlayer.country` enriched from user profile; board HUD shows country; presence hook exposes `roster` |
| **9.0b** | Equal 3-pane hub shell, full height, 2px middle borders, per-tile floor color                                                                                          |
| **9.0c** | Center copy: name + code + centered `about` with height-based ellipsis; walk square = avatars only                                                                     |
| **9.0d** | `HubRoster` (`IN HUB · n/16`, 2-col, country, X) + joystick BR                                                                                                         |
| **9.0e** | Plan smoke checklist                                                                                                                                                   |

**9.0a notes**

- Presence: `PeerInfo.country` + `Attach(..., country, ...)`; hub/board load ISO code from user profile.
- Game: `CountryLookup` enriches `PlayerView.country` on read (not persisted on game docs).
- Mobile: `PresenceRosterEntry` + `roster` on presence channel; `GamePlayer.country`; BoardPanel country label.

**9.0b notes**

- Equal `flex: 1` panes; horizontal safe-area only (rails stretch to bottom like board).
- Center: 2px side borders; `floorColor = tileVisual.bandColor ?? fill`; `HubScene` single-color square (no two-tone).
- Left media stub + Live/Time; right X leave + joystick stub until 9.0d.

**9.0c notes**

- `HubLocationCopy` on the **rail** (not inside the walk square): name + code + `about` (fallback aboutShort/description).
- Height-based `numberOfLines` + tail ellipsis — no ScrollView.
- `HubScene` = avatars only (`pointerEvents="box-none"`) above copy; rail owns floor color.
- Walk surface is the **full center rail** (rect) so avatars can cross the heading.

**9.0d notes**

- `HubRoster`: `IN HUB · n/16`; **2 per row** via `space-between` (equal L/R edge padding).
- Local label: `You · NG`. Remotes: `Name · NG`. X leave in header; joystick BR.
- Rows from `presence.roster` + local seed; accents via `accentAgainstFloor`.

**9.0e — smoke checklist (hub chrome)**

Manual (landscape device / simulator; BE running; 2 clients preferred):

1. **Shell (9.0b)** — Enter any hub: three equal panes; rails to bottom (no grey gap); middle 2px side borders; floor = tile color (airport blue / city group / etc.).
2. **Copy (9.0c)** — Short hub (e.g. airport): name + code + short about visible. Long hub (e.g. Benghazi/Tokyo): name + code stay on-screen; about ellipsizes with `…` (no mid-line clip, no ScrollView).
3. **Walk** — Joystick moves avatar over heading and body text; avatars draw above copy; contrast accents do not blend into floor.
4. **Roster (9.0d)** — `IN HUB · n/16`; two chips per row, left/right flush to panel padding; local `You · CC`, peer `Name · CC`.
5. **Country (9.0a)** — Board HUD shows country on You / others; hub roster matches profile ISO.
6. **Presence** — Second client joins same hub: count bumps, peer appears on floor + roster; leave drops count/avatar.
7. **Leave / Open board** — X leaves hub (clears hubId); turn/buy sheets still work; Open board keeps hubId; keep-awake on hub (screen stays awake).
8. **Hub full** — Optional: 17th join gets hub-full error and stops retrying.

**Remaining Phase 9 sub-slices (ship one at a time — ask before starting each)**

| Slice   | Done when                                                                                                                                                | Avoid                                                                                       |
| ------- | -------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------- |
| **9.1** | Shared branded `DeedCard` on **buy** + **tile-info** overlays (group-color strip border, Moti stagger, rent grid); specials keep a lighter branded sheet | Settings, economy toasts, hub buy compact redesign                                          |
| **9.2** | Home `/(app)/settings`: RN Mute mic Switch (SecureStore); Leave stays board ⋯; report deferred                                                           | Board settings sheet; report API; `@expo/ui` Switch on Android; property cards; rent toasts |
| **9.3** | Board economy **modals** (buy / rent / tax / salary) for involved players; spectators + hub get short toasts; 3s/5s `__DEV__` queue                      | Cash HUD flash; Chance/Auction/Jail modals; settings redesign                               |
| **9.4** | Leftover playtest bugs + smoke: signup → join → play → buy → rent → hub enter/exit → leave                                                               | New features                                                                                |

**9.1 notes**

- Extract `components/board/DeedCard.tsx` — strip color border + header + rent grid; strip world-pack name prefix.
- `BuyPropertyOverlay` + `TileInfoOverlay` both consume it; Moti sheet entrance + staggered deed header/body.
- Specials (chance / jail / tax / …): icon + title sheet, not a fake rent deed.
- Hub compact `HubBuySheet` stays as-is until a later polish ask.

**9.2 notes**

- Home `/(app)/settings` only (not board ⋯). Leave remains board overflow / hub X.
- Mute: RN `Switch` + SecureStore `muteMic` (`'1'` / `'0'`). No audio until Phase 10.
- **Report** skipped until player picker + report API exist.
- Inspect own deed shows **Owned by You** (board `inspectOwner`).

**9.3 notes**

- **Board:** branded economy modals (Meetopoly chrome; layout inspired by `meetopoly-mobile/resources/Screenshot_2026-09-27-*` — not a clone).
- **Hub:** same events → short toasts only (no modals).
- **Audience:** buy modal = buyer; rent modal = payer + owner; tax modal = payer; salary modal = GO passer. Everyone else → toast.
- **Dismiss:** auto **3s** (prod) / **5s** (`__DEV__`); overlapping events **queued** (one after another).
- **Buy flow:** close buy sheet → then LAND / **AIRPORT** / UTILITY BOUGHT (not “rail”).
- **No** cash HUD flash in 9.3.
- Replaces Phase 6.4/6.5 toast-only UX for involved players; keep toasts for spectators + hub + cannot-afford remainder.

**Reference screenshots → later phases (do not build in 9.3)**

| Ref                                          | Action                                                                   | Phase                     |
| -------------------------------------------- | ------------------------------------------------------------------------ | ------------------------- |
| CHANCE / CHEST card modals                   | Card draw UI                                                             | **12**                    |
| JUST VISITING                                | Jail visit notify                                                        | **12**                    |
| AUCTION bid/fold/slider                      | Bank auction                                                             | **13**                    |
| BUILD / SELL / MORTGAGE / REDEEM / TRADE bar | Economy CTAs in board panel; Roll/End/Hub dock icons                     | **11.4a** dock; **11.4b+** bar (+ trade **13**) |

**9.4 notes**

- No new features — scan + fix clear regressions from 9.0–9.3; document full-match smoke.
- Fixes landed: `HubRoster` `ReactNode` import from `react` (tsc); economy queue unique ids so identical toast copy still advances the timer.
- Buy/bought headers use location strip color (9.3 polish); mute = RN Switch; report deferred.

**9.4 — smoke checklist (Phase 9 close)**

Manual (2 clients preferred; BE running; landscape):

**A. End-to-end (Phase 9 exit bar)**

1. Fresh signup → verify → password → username/country → land on home menu.
2. Play → create/join table → Ready → game starts (pins, MeetCoin, turn HUD).
3. Walk board, roll, buy unowned, pay/collect rent, pass GO, pay tax if landed.
4. Enter a hub (X leave / Open board / roster / country) → leave hub → back on board.
5. Leave board via ⋯ (resign confirm) → home. No stuck WS / false resign for peer.

**B. Economy feedback (9.3)**

1. Buyer sees LAND/AIRPORT/UTILITY BOUGHT (strip-colored header, full deed, price + avatar); spectator toast.
2. Rent: payer + owner get PAID RENT (avatars); spectator toast.
3. Tax: payer PAID TAX (avatar); others toast.
4. Pass GO: passer SALARY modal; others toast. Queue if salary + rent same turn.
5. From hub: board actions → toast only (no modal).

**C. Hub chrome + settings (9.0–9.2)**

1. Hub 3 equal panes, strip floor, roster `You · CC` / 2 per row, keep-awake.
2. Settings → Mute persists across reopen; board ⋯ has Leave/Log out only.

**Exit criteria**

- [ ] New player can finish signup → join table → play M1 → visit a hub without developer intervention (**run 9.4 smoke A**)
- [x] Hub chrome matches locked 3-pane brief (Phase 9.0a–d; media cameras → Phase 10)
- [x] **9.0a:** country on presence roster + game players + board HUD
- [x] **9.0b:** equal 3-pane shell + per-tile floor + middle borders
- [x] **9.0c:** center name/code/about with ellipsis; full-rail walk over copy
- [x] **9.0d:** hub roster 2/row + X + joystick BR
- [x] **9.0e:** smoke checklist documented (run before declaring Phase 9 hub chrome done)
- [x] **9.1:** branded `DeedCard` on buy + tile-info
- [x] **9.2:** home Settings mute Switch (RN; `@expo/ui` Compose path avoided); report deferred
- [x] **9.3:** economy modals (board involved) + toasts (spectators / hub)
- [x] **9.4:** leftover fixes + full-match smoke checklist documented

---

### Phase 10 — Voice (same rooms) — DONE

**Goal:** Mic audio in **hub** and **board** rooms (same Pion presence SFU).

**Locked**

- Hub first; board voice = **10.4** (ask before).
- Mute: Settings `muteMic` is source of truth.
- Audio only (ignore video). STUN-only; TURN only if playtest proves need (ask first).
- Ship **one sub-slice at a time** — ask before each.

**Sub-slices**

| Slice    | Done when                                                                                                                                                          | Avoid                               |
| -------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------ | ----------------------------------- |
| **10.0** | SFU: on `hub:*` only, `OnTrack` audio → forward to other peers; video ignored; Detach stops forward; board rooms stay pose-only; WS accepts renegotiation `answer` | Mobile mic, mute UI, board voice    |
| **10.1** | Mobile hub: permission + `getUserMedia({audio})` + `pc.addTrack`; apply `muteMic`; teardown on leave                                                               | Remote playback polish, board voice |
| **10.2** | Mobile hub: `pc.ontrack` + handle SFU renegotiation offers; leave stops audio                                                                                      | Fancy speaking indicators           |
| **10.3** | Hub left rail Live + mute wired to `muteMic`; smoke checklist                                                                                                      | Cameras, board voice                |
| **10.4** | Board/table voice (ask before)                                                                                                                                     | —                                   |

**10.0 notes**

- [`sfu.go`](../internal/adapters/webrtc/sfu.go): hub rooms keep `audioPubs`; RTP relay via `TrackLocalStaticRTP`; `AddTrack` + SFU-created `offer` for mid-session peers; late joiners get existing pubs before `CreateAnswer`.
- Presence WS: handle client `answer` for renegotiation (`HandleAnswer`).
- Board `board:*`: no audio forward even if offer includes audio.

**10.1 notes**

- Hub only: `useHubPresence` → `getUserMedia({ audio: true })` + `pc.addTrack` before `createOffer`.
- `muteMic` (SecureStore) shared via `useMuteMic`; muted → `track.enabled = false` (no renegotiation).
- Leave / PC teardown stops local tracks. Board presence stays pose-only (no publish).
- Remote playback + SFU renegotiation offers → **10.2**.

**10.2 notes**

- Hub `pc.ontrack`: keep remote audio streams/tracks so RN playout stays alive.
- WS `offer` from SFU → `setRemoteDescription` + `createAnswer` + send `answer` (mid-session pubs).
- Leave / PC teardown stops remote tracks too. Board ignores (no `publishLocalAudio`).
- Loudspeaker: `react-native-incall-manager` via `lib/hubAudioRoute.ts` — `startHubSpeaker` on hub PC start, `stopHubSpeaker` on leave (requires rebuilt dev client).
- Hub mute CTA / left-rail Live → **10.3**.

**10.3 notes**

- Left rail: `HubMediaRail` — Voice eyebrow, Live/Connecting status, mute CTA → `useMuteMic` / SecureStore `muteMic` (same SoT as Settings).
- Cameras still deferred. Board voice → **10.4**.
- Settings hint updated to mention hub Voice rail.

**10.3 — smoke checklist (hub voice)**

1. **Live** — Enter hub: left rail shows Voice + Live (when DC open); Time bank when in a game.
2. **Mute CTA** — Tap Mic on → Muted (brand fill + mic-off); remote peers stop hearing you; tap again → Mic on.
3. **Settings sync** — Mute in hub → home Settings switch matches; toggle Settings → hub CTA matches (shared cache).
4. **Leave** — X leave hub stops local + remote audio; speaker session ends.
5. **Two-device** — Both unmuted → hear each other on loudspeaker; mute one → other goes silent for that pub.

**10.4 notes**

- SFU: `IsVoiceRoom` = hub + board; board rooms allocate `audioPubs` and forward mic like hub.
- Mobile: `useBoardPresence` publishes/plays audio + `muteMic`; speaker via same `hubAudioRoute`.
- Board panel: shared `MuteMicButton` above joystick. Hub handoff still disables board presence (one voice room).

**10.4 — smoke checklist (board voice)**

1. **Two on board** — Both unmuted → hear each other; mute CTA above joystick.
2. **Mute** — Board mute ↔ Settings / hub mute SoT.
3. **Hub handoff** — Enter hub leaves board SFU; return board restores board voice.
4. **Leave** — Leaving board presence stops board audio.

**Exit criteria**

- [x] Hear others in hub; leave hub stops hub audio (after 10.0–10.3)
- [x] **10.0:** hub SFU audio forward + board pose-only
- [x] **10.1:** hub mic publish + mute pref
- [x] **10.2:** hub remote playback + renegotiation
- [x] **10.3:** hub mute UI + smoke
- [x] **10.4:** board/table voice + mute CTA

**Deferred (not Phase 10):** With hubs up to **16**, selective listen (“choose who I hear”) ships with **video / camera tiles** — same roster UX. Locked under **Phase 16** (ask before). Until then: everyone-audio + global `muteMic` only.

---

### Phase 11 — Rules M2 (houses / hotels / light mortgage) — DONE

**Official alignment:** even-build across a color group; max 4 houses then hotel; sell buildings back at half price; cannot build if any deed in the set is mortgaged; house shortage → auction for last houses (**→ Phase 13**, not here).

**Locked**

- Ship **one sub-slice at a time** — ask before each.
- Board-only build/sell/mortgage UI (same as auction/trade rule: not in hub sheets).
- Classic house/hotel costs from seed `houseCost` (hotel step = same cost as a house).
- `Deed.houses` **0–5** (`5` = hotel); no separate `hotel` bool.
- `mortgaged` field on deed; actions in **11.3** (until then always false).
- Undeveloped monopoly rent = **2×** site (`Rents[0]`) — already in Phase 6.5; building tiers when monopoly + houses ≥ 1.
- **Build API (11.1):** `POST /games/{id}/build` body `{ boardIndex }` — **one step** per call; current player only (no off-turn); allowed in `awaiting_roll` **or** `awaiting_end`; blocked by `pendingPayment`; full color group + even-build + cash ≥ `houseCost`; WS `state` fan-out.
- **Sell API (11.2):** `POST /games/{id}/sell-building` body `{ boardIndex }` — one step down; refund `floor(houseCost/2)`; even-sell (sell from max houses first); current player only; **allowed during `pendingPayment`** (raise funds — refund applies toward debt); WS `state` fan-out.
- **Mortgage / redeem (11.3):** `POST /games/{id}/mortgage` + `POST /games/{id}/redeem` body `{ boardIndex }`; mortgage payout `floor(price/2)`; redeem = mortgage + 10%; cities must have **0 houses on the whole color group** before mortgage; mortgaged → **0 rent**; build blocked if any deed in set mortgaged; mortgage allowed during pendingPayment; redeem blocked while pending; current player only.
- **Board dock + economy UI (→ 11.4x):** **11.4a:** dock wire-up. **11.4b:** CTA bar + how-to sheet (Close exits mode) + highlight/tap. **11.4c:** house (1–4 green homes) / hotel (red) / **M** on color-band edge from `game.deeds`.

**Sub-slices**

| Slice     | Done when                                                                                                                                                                                             | Avoid                                 |
| --------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------- |
| **11.0**  | ✅ `Deed` gains `houses` (0–5; **5 = hotel**) + `mortgaged` (always false until 11.3); rent uses `Rents[houses]` when monopoly (undeveloped monopoly stays **2×** `Rents[0]`); OpenAPI + mobile types | Build/sell HTTP, UI, mortgage actions |
| **11.1**  | ✅ `POST /games/{id}/build` — buy house/hotel; even-build + full color group + cash; WS fan-out                                                                                                       | Sell, mortgage UI                     |
| **11.2**  | ✅ `POST /games/{id}/sell-building` — sell house/hotel at half cost; even-sell down; allowed during pendingPayment                                                                                    | Mortgage UI                           |
| **11.3**  | ✅ `POST` mortgage + redeem (list/half + 10%); mortgaged = 0 rent; block build if set mortgaged                                                                                                       | House-shortage auction                |
| **11.4a** | ✅ Dock icons wired (dice/end/hub); opacity disabled; text Roll/End/Enter removed                                                                                                                     | Economy CTA bar, markers              |
| **11.4b** | ✅ Economy CTA bar + mode → how-to sheet → highlight → tap → API; TRADE stub                                                                                                                          | House/hotel/M markers                 |
| **11.4c** | ✅ House / hotel / M markers on tiles                                                                                                                                                                 | Hub rebuild UI                        |
| **11.5**  | ✅ Smoke checklist for M2 path documented (+ `go test ./internal/services/game/` green)                                                                                | Jail/cards (12), auction (13)         |

**11.5 — smoke checklist (Phase 11 / M2 close)**

Manual (2 clients preferred; BE running; landscape). Backend gate: `cd meetopoly-be && go test ./internal/services/game/ -count=1`.

**A. Data + rent (11.0)**

1. Owned undeveloped monopoly → rent is **2×** site (`Rents[0]`).
2. Build 1+ houses with monopoly → rent uses `Rents[houses]` (hotel = tier 5).
3. Mortgaged deed → **0** rent when landed on.

**B. APIs (11.1–11.3)** — covered by unit tests; spot-check in play:

1. Build only on your turn; even-build; full color group; cash ≥ `houseCost`; blocked if set mortgaged or `pendingPayment`.
2. Sell one step; refund `floor(houseCost/2)`; even-sell; allowed during `pendingPayment`.
3. Mortgage `floor(price/2)` only with **0** houses on the color group; redeem = mortgage + 10%; redeem blocked while pending; mortgage OK while pending.

**C. Dock + must-buy (11.4a + interim)**

1. Dice / End / Hub ~48px green boxes; full opacity when usable / ~0.35 when not; no Roll/End/Enter text buttons.
2. Land unowned buyable → buy modal non-dismissible; End dimmed; no “skip” hint; Buy then End works.
3. Hub nearby → Hub icon active; tap enters hub.

**D. Economy UI (11.4b)**

1. Your turn: Build / Sell / Mortgage / Redeem / Trade above dock; Trade always dim.
2. Off-turn: CTAs visible but dim.
3. Tap Build → how-to sheet (brand green header + Close, sized to board free center − 12px); eligible tiles glow + amount; others dim.
4. **Close exits mode** (sheet + dim/highlight clear). Toggle same CTA also exits.
5. Tap eligible tile → one API step; markers/cash update via WS for both clients.
6. Sell / Mortgage / Redeem modes same pattern.

**E. Markers (11.4c)**

1. Houses 1–4 → green home icons on color-band edge.
2. Hotel (5) → single red hotel icon.
3. Mortgaged → black **M** badge (no houses).
4. Peer sees your markers update without reload.

**F. Raise funds (optional)**

1. Insufficient rent → `pendingPayment`; cannot Roll/End/Build/Redeem; Sell + Mortgage still work; after cash covers debt, turn continues.

**Exit criteria**

- [x] Even-build enforced server-side (11.1); even-sell (11.2)
- [x] UI to buy/sell houses (and hotels) — mode + tap (11.4b); markers (11.4c)
- [x] Light mortgage + redeem; mortgaged set blocks build
- [x] **11.0** … **11.5** ticked — Phase 11 M2 closed (house-shortage auction → **13**)

---

### Phase 12 — Rules M3 (jail + cards)

**Official alignment:** Go to Jail (land / card / three doubles); Just Visiting vs in Jail; exit via **100 MeetCoin**, Get Out of Jail Free, or doubles within 3 turns; Chance / Community Chest decks; Free Parking stays a noop.

**Exit criteria:** Jail entry/exit paths; Chance/Community Chest deck from config or seed.

**Locked**

- Ship **one sub-slice at a time** — ask before each.
- Jail fine = **100 MeetCoin** (not classic 50) — **12.1**.
- Cards = **Chance** + **Community Chest** only; destinations by **`boardIndex`** (world names differ; GO = 0).
- Free Parking stays **noop**.
- `GamePlayer`: `inJail`, `jailTurns`, `getOutOfJailFree` (schema in **12.0**; exit / draw later).
- Jail board index = seed `specialType: jail` (classic **10**); Go to Jail = `go_to_jail` (classic **30**).
- Teleport to Jail does **not** collect Pass GO; turn ends (`awaiting_end`); doubles streak cleared.
- While `inJail`, Roll tries doubles exit (**12.1**); after 3 failed attempts without cash for the fine, `canRoll` false until pay/card.
- Jail fine = **100** MeetCoin via `POST /games/{id}/pay-jail-fine`; GOOJF via `POST /games/{id}/use-jail-card` (cards drawn in 12.2+).
- Decks: shuffled at game start; persist remaining order on game doc; land Chance/Chest → draw; non-GOOJF returned to bottom; GOOJF held by player until used (then returned to its deck).

**Sub-slices**

| Slice    | Done when                                                                                                                                                         | Avoid                    |
| -------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------ |
| **12.0** | ✅ Player jail fields; land **Go to Jail** + **third doubles** → Jail; land Jail = Just Visiting; OpenAPI 0.20 + mobile types                                    | Exit APIs, card decks, UI |
| **12.1** | ✅ Jail exit: pay **100** MeetCoin (`POST .../pay-jail-fine`), GOOJF (`POST .../use-jail-card`), roll doubles (free + move, no re-roll), or fail 3 then forced pay+move (if broke stay until pay); OpenAPI 0.21 | Card decks, fancy UI |
| **12.2** | ✅ Chance + Chest catalog; shuffle at start; draw on land; persist decks; `lastCard`; GOOJF held; other effects stubbed → 12.3; OpenAPI 0.22 | Full effect resolve, UI |
| **12.3** | ✅ Card effects (tables below): cash, move by index, Jail, repairs, nearest RR/utility; apply on draw (lock **A**); OpenAPI 0.23 | Trade of GOOJF (→ 13); card modal UI |
| **12.4** | ✅ Card reveal (all seated) + move pause; Just Visiting modal; hub toasts; jail pin layout; jail options **v1** (next-turn only) | Jail modal UX polish → **12.4b** |
| **12.4b** | ✅ Jail options modal: avatar + **Pay** / **Roll a Double** / **Use card**; dock Roll gating while modal open | Negative cash / bankruptcy → **14** |
| **12.5** | ✅ Smoke checklist (manual playtest; M3 jail + cards closed)                                                                                                      | Auction/trade → **13**   |

**Chance deck (16 cards — locked; destinations = `boardIndex`)**

| # | Card id / title | Logic |
|---:|---|---|
| 1 | Advance to Boardwalk | Move to **39** |
| 2 | Advance to GO | Move to **0**, collect $200 |
| 3 | Advance to Illinois Avenue | Move to **24**; collect $200 if passing GO |
| 4 | Advance to St. Charles Place | Move to **11**; collect $200 if passing GO |
| 5 | Advance to nearest Railroad | Next of **5 / 15 / 25 / 35**; double rent if owned |
| 6 | Advance to nearest Railroad | Same — **2 copies** |
| 7 | Advance to nearest Utility | Next of **12 / 28**; rent **10×** dice if owned |
| 8 | Bank pays you dividend | +$50 |
| 9 | Get Out of Jail Free | Retain card |
| 10 | Go Back 3 Spaces | Position − 3 |
| 11 | Go to Jail | Move to **10**; do not pass GO |
| 12 | Make general repairs | Pay $25/house, $100/hotel |
| 13 | Speeding fine | −$15 |
| 14 | Take a trip to Reading Railroad | Move to **15** (left-side airport); collect $200 if passing GO |
| 15 | Elected Chairman of the Board | Pay each other player $50 |
| 16 | Building loan matures | +$150 |

**Community Chest deck (16 cards — locked)**

| # | Card id / title | Logic |
|---:|---|---|
| 1 | Advance to GO | Move to **0**, collect $200 |
| 2 | Bank error in your favor | +$200 |
| 3 | Doctor's fee | −$50 |
| 4 | From sale of stock | +$50 |
| 5 | Get Out of Jail Free | Retain card |
| 6 | Go to Jail | Move to **10**; do not pass GO |
| 7 | Holiday fund matures | +$100 |
| 8 | Income tax refund | +$20 |
| 9 | It's your birthday | Collect $10 from each other player |
| 10 | Life insurance matures | +$100 |
| 11 | Hospital fees | −$100 |
| 12 | School fees | −$50 |
| 13 | Consultancy fee | +$25 |
| 14 | Street repairs | Pay $40/house, $115/hotel |
| 15 | Second prize in beauty contest | +$10 |
| 16 | Inheritance | +$100 |

Amounts = MeetCoin 1:1 with classic dollars. UI may show the world tile **name** for an index; rules always use the index.

**12.0 notes**

- Third doubles no longer “skip move only” — pin → Jail + `inJail`.
- Land `go_to_jail` after normal move (Pass GO on the walk still applies) then teleport to Jail.

**12.1 notes**

- Pay / use card → leave Jail, `awaiting_roll`, then normal Roll to move (pin stays on Jail until then).
- Doubles from Jail → leave + move that total, **no** extra re-roll (`awaiting_end`).
- Failures 1–2 → `jailTurns++`, stay in Jail, End turn.
- Fail 3 with cash ≥ 100 → auto fine + leave + move.
- Fail 3 with cash < 100 → `jailTurns=3`, stay in Jail until pay/card (then Roll next) — **interim**; forced negative cash + bankruptcy gate → **Phase 14**.
- Board jail sheet UI → **12.4** / polish → **12.4b**.

**12.2 notes**

- Catalog in Go (`cards.go`); shuffled into `chanceDeck` / `chestDeck` on `CreateFromSeats`.
- Land `specialType` `chance` / `community_chest` → draw top; set `lastCard` on game view; non-GOOJF → bottom of same deck.
- GOOJF → remove from deck, append to player's held cards (`getOutOfJailFree` = count); use returns card to its deck bottom.
- Cash / move / repair effects → **12.3** (draw still happens so decks progress).

**12.3 notes**

- **Lock A:** server applies card effects in the same state update as the draw; client shows reveal modal 3s / `__DEV__` 5s in **12.4**, then animates the already-applied cash/move.
- Destinations by `boardIndex` (39 / 0 / 24 / 11 / **15** trip-RR / nearest RR 5·15·25·35 / nearest util 12·28 / jail 10).
- Nearest railroad → **2×** rent if owned; nearest utility → **10×** new dice roll if owned.
- Go back 3 → resolve landing on the new tile (tax/rent/another card OK).
- Card payments use `lastPayment.kind = card`; shortfall → `pendingPayment` (raise funds).
- Card reveal + jail action sheet UI → **12.4**.

**12.4 notes**

- **Chance/Chest:** **drawer** sees the card modal; **others** → toast (`{name} took a Chance` / `opened a Chest` + card text + MeetCoin delta when cash). Hub → toast only. Auto-dismiss `ECONOMY_MODAL_MS` (3s / `__DEV__` 5s), queued with other economy events.
- **Card move timing:** pin walks to Chance/Chest → **pause** for reveal modal → then animate to the card destination (jump for jail / go-back; walk onward for advance).
- **Pass-GO + card:** salary waits until card reveal (and any resume walk) finish — **card → pin action → salary** (Advance to GO). Board uses `salaryWaitIdle`; hub presents card before salary in effect order.
- **Just Visiting:** modal for the visitor; others → toast. Pins on Jail tile: **inJail → center**; **Just Visiting → outer L edges** (bottom/left).
- **Jail options timing (done):** only on **your next turn** while in Jail (`awaiting_roll`). After Go-to-Jail this turn: End first (no options). Exit (pay / card / doubles success) + **failed doubles roll** → **toast for the table**.
- Jail modal **layout / dock gating** → **12.4b** (v1 sheet was incomplete — Pay/Use only; no Roll-a-Double CTA; Use card could look forced when GOOJF = 0).

**12.4b notes — jail options UX (✅ done)**

Layout (centered economy chrome, same family as buy/rent/card):

- **Left:** jailed player avatar (pod + pin color).
- **Right:** three **vertical** buttons — **Pay** (100 MeetCoin) · **Roll a Double** · **Use card**.

Enable / disable:

- **Use card** — disabled unless `getOutOfJailFree ≥ 1`.
- **Pay** — disabled if cash &lt; 100 (`canPayJailFine`).
- Cash &lt; 100 **and** no GOOJF → only **Roll a Double** enabled.
- Hint copy matches real options.

Dock Roll gating:

- While the jail options modal is open → dock **Roll disabled**.
- **Roll a Double** → close modal → enable dock Roll for the jail attempt.
- **Pay** / **Use card** → leave Jail, close modal → dock Roll enabled to move.

Out of scope (→ **Phase 14**): negative MeetCoin after 3 fails when broke; bankruptcy gate.

Smoke → **12.5** ✅ (manual playtest 2026-09-28/29). **Phase 12 DONE.**

---

### Phase 13 — Rules M4 (trading + auctions)

**Official alignment:** Players may trade deeds, cash, and Get Out of Jail Free cards. **When a player lands on unowned property and does not buy at list price, the Bank auctions it immediately** — all players may bid (including the one who declined). House-shortage auctions **deferred** (not required for M4 exit). Bankruptcy wipe → deeds unowned until next landing (no wipe re-auction; Phase 14).

**Hub → board (locked with 8.3):** Auction, trade, and raise-funds UIs live **on the board only**. When an auction starts, clients still in a hub get a strong notify and **prefer auto Open board** (keep `hubId` like 8.3). Do not rebuild auction/trade clients inside the hub sheet.

**Time bank pauses (encompassing — locked here)**

Baseline (**6.3b**): each player has a **45-minute** bank that drains **only on their turn** and **auto-eliminates at 0**.

Also **pause everyone’s** personal banks (do not reset) for the **whole auction**; only the **per-bidder 60s** auction sub-clock ticks. Later: open trade offers; forced raise-funds (→ 14).

**Exit criteria:** Multi-property / cash trades; **auction when purchase declined** (completes Phase 6.4); bank pause list enforced with those flows. House-shortage **not** required to close M4.

**Locked — bank auction (from playtest refs + product decisions)**

- **All seated** see the auction modal (board-only UI in **13.1**); non-dismissible until settle/void.
- **Buy \| Auction** when lander can afford list price. **Cash &lt; list → skip buy modal, auto-start auction.**
- **Turn-based** bidding starting with the player who triggered the auction, then seat order among non-folded.
- **High bidder does not get a turn** while they hold the high bid. Turn comes back only after someone outbids them. If everyone else folds → **win immediately** at current high bid (no extra turn).
- **Min raise +1 MeetCoin.** First bid floor = **1** when high = 0.
- **BID / FOLD** only enabled on your auction turn.
- High bidder **may fold** on a later turn (after being outbid); high reverts to latest bid among remaining; if none → high 0 / min 1.
- On turn start: if cash &lt; min next → **auto-fold**. Not checked continuously.
- **Per-bidder timer 60s** (runs only on current bidder’s turn). Timeout → auto-**BID high+1** if affordable, else auto-**fold**.
- Last non-folded wins at their high bid. Never bid + others all folded → award at **1** if cash ≥ 1, else **void** (unowned). No legal winner → **void**.
- Settle: toast all `{player} won auction for {location}`; winner gets bought modal at **auction price** (**13.1**).
- Bid amount UI: **native numeric TextInput** (device keyboard). Peek board: dock hold **`eye-sharp`**, enabled only during auction (**13.1**).
- Ship **one sub-slice at a time** — ask before each.

**Sub-slices**

| Slice    | Done when                                                                                                                                                         | Avoid                         |
| -------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------- |
| **13.0** | ✅ Auction schema + APIs: auto-start if broke / `POST .../start-auction`; bid + fold; 30s timer; auto-bid/fold; settle/void; pause all banks; remove must-buy-only; OpenAPI 0.25 + unit tests | Fancy UI, trade               |
| **13.1** | ✅ Board auction modal (deed + feed + keypad + BID/FOLD); Buy\|Auction; dock peek; hub Open board; toasts + winner bought @ auction price | Trade                         |
| **13.2** | ✅ Trade schema + APIs (propose / accept / decline; deeds + cash + GOOJF); 60s reply; pause turn clock; **3m turn clock** + 2-strike forfeit; panel current-only green→red | Fancy trade UI → **13.3** |
| **13.3** | ✅ Trade board UI + wire TRADE CTA; hub notify / Open board                                                                                                          | House shortage                |
| **13.4** | ✅ Smoke checklist (manual playtest; M4 auction + trade closed)                                                                                                       | House shortage (deferred)     |

**13.0 notes**

- Persist `auction` on game doc; `lastAuction` result for clients after clear (like `lastPayment`).
- `POST /games/{id}/start-auction` — current player with buyOffer (can afford). Broke path starts auction in the same land resolve (no buyOffer).
- `POST /games/{id}/auction/bid` body `{ amount }`; `POST /games/{id}/auction/fold`.
- EndTurn blocked while buyOffer **or** auction; Roll/Buy/build blocked during auction.
- After auction settle/void: `suppressBuyOffer` so buyOffer does not reopen; cleared on EndTurn.
- Resign mid-auction → fold that player.
- UI + hub auto-open → **13.1**.

**13.1 notes**

- All seated see non-dismissible `AuctionOverlay` (deed + history + numeric TextInput + BID/FOLD).
- Buy modal → **Buy | Auction**; broke landers skip buy (server auto-auction).
- Dock **eye-sharp** hold-to-peek (enabled only while auction active).
- Hub: on auction start → toast + prefer Open board (keep `hubId`).
- Toast all on settle; winner board modal uses **auction amount** (`priceOverride`).
- Per-bidder clock **60s** (server `AuctionBidTurn`).

**13.2 notes**

- **Trade:** `POST …/trade/propose|accept|decline`; one open offer; current-turn propose only; target 60s auto-decline; no cash↔cash; block improved titles; mortgaged OK with `redeem_all`|`leave_all` (mortgage+10%); GOOJF tradable; Roll blocked while open; pause proposer's turn clock during reply wait.
- **Turn clock:** replaces 45m banks with **3:00 fresh each turn**; 1st timeout → force End Turn + strike; 2nd → auto-resign + `lastForfeit`; pause only for auction + trade-wait. Panel shows **current player only**; green → red at ≤1:00.
- Fancy two-panel trade UI → **13.3**.

**13.3 notes**

- Board `TradeOverlay`: compose (left you offer / right partner carousel ask) + Offer; incoming review with Accept (green) / Decline (red) + View board peek; mortgaged → redeem-all | leave-all sheet.
- TRADE dock CTA enabled when `canProposeTrade`.
- Hub: toast + prefer Open board when local player is in an open trade.
- Modal only for parties (proposer waiting / target review); third players get propose toast + accept/reject outcome toasts via `lastTrade` (OpenAPI 0.27).

**13.4 — smoke checklist (Phase 13 / M4 close)**

Manual (2–3 clients preferred; BE running; landscape). Backend gate: `cd meetopoly-be && go test ./internal/services/game/ -count=1`.

**A. Auction (13.0–13.1)**

1. Decline buy / broke land → auction starts; all seated see overlay; banks pause.
2. Bid / fold / 60s bidder clock; settle → toast + winner bought @ auction price; void path OK.
3. Hub client: toast + Open board on auction start; dock eye hold peeks board.

**B. Trade + turn clock (13.2–13.3)**

1. Propose on turn only; cash↔cash blocked; improved deeds blocked; GOOJF + mortgaged (`redeem_all` / `leave_all`) OK.
2. Target 60s reply; proposer waiting sheet; third players **no** modal (toast only); accept/reject toasts for all via `lastTrade`.
3. Panel **3:00** current-only green→red; 1st timeout strike; 2nd forfeit; clock pauses during auction + open trade.

Smoke → **13.4** ✅ (manual playtest 2026-09-29/30). **Phase 13 DONE.** House-shortage auction remains deferred.

---

### Phase 14 — Rules M5 (bankruptcy)

**Meetopoly debt model (locked — not classic asset-to-creditor):**

- Cash may go **negative**. HUD shows red while &lt; 0.
- On shortfall (rent / tax / card / jail fine): pay current cash to the payee, set cash to **−(remainder)**, remember `owedTo` (player id or Bank).
- **Landing turn:** **End allowed** while negative; **Roll blocked**.
- **Next turn** (indebted player): modal **Pay** | **Bankruptcy**.
  - **Pay** → **2:00** raise-funds window; pause turn clock; others toast + live cash; actions = **sell buildings + mortgage only** (**no trade/swap**).
  - As cash climbs toward 0, each raise chunk settles debt (**owed player receives MeetCoin**; Bank debt just clears).
  - **Bankruptcy** always available in modal → eliminate **that player only** (table continues; last active wins).
  - **2:00 expires** still &lt; 0 → **auto-bankrupt**.
  - **Auto-bankrupt (assets):** `cash < 0` and no buildings left and no unmortgaged deeds — no trade required.
- **Eliminate wipe (always → Bank):** clear houses; deeds → **unowned** (as if never bought; mortgage cleared); GOOJF → **bottom** of its deck; no player receives assets. If still owed a player, **Bank (infinite) credits** them the remaining unpaid amount.
- **Resign / turn-timeout / disconnect eliminate:** same Bank wipe (classic quit ≈ bankrupt to Bank).
- **No Bank re-auction on wipe** (14.3 cancelled): returned deeds stay open until a future **landing** → normal Buy \| Auction. A pin already sitting on a wiped tile does **not** get Buy \| Auction; they End / Roll away.
- Hub → board: debt Pay UI is **board-only** (toast + prefer Open board), same as auction/trade.

**Also from jail (locked with 12.4b deferral → 14.1):**

- After **3** failed doubles from Jail while cash &lt; 100 (and no GOOJF): force leave with MeetCoin **negative** by the unpaid 100 fine (Bank `owedTo`) — do **not** soft-lock forever as in **12.1** interim.
- On the next turn: Pay | Bankruptcy gate before Roll (same as other debt).

**Exit criteria:** Negative-cash debt + settle-on-raise; Pay/Bankruptcy gate + 2:00; player elimination with Bank wipe + creditor cash top-up; jail forced-debt path; wiped deeds unowned until next landing (no wipe re-auction).

| Slice | Deliverable | Not yet |
| --- | --- | --- |
| **14.0** | ✅ Negative-cash debt model + `owedTo`; shortfall → pay what you can then cash `−(remainder)`; sell/mortgage during debt settles creditor; End OK / Roll blocked while negative; declare-bankrupt + auto-bankrupt APIs; resign/timeout/disconnect use Bank wipe (deeds unowned, GOOJF deck bottom, Bank pays remaining owed); pause turn clock during debt-pay; OpenAPI 0.28 + unit tests | Jail path, UI |
| **14.1** | ✅ Jail 3-fail broke → leave Jail, cash `−` fine (`kind: jail`), move with failed roll + landing resolve; next-turn Pay \| Bankruptcy (same gate as 14.0) | Fancy UI |
| **14.2** | ✅ Board modal Pay \| Bankruptcy; **2:00** pay timer; red negative cash; live cash for all; toasts; hub Open board; wire Declare bankrupt | — |
| **14.3** | ✅ **Cancelled** — no Bank re-auction on wipe; deeds stay unowned until landed on; sitting pin suppresses Buy \| Auction (regression tests) | — |
| **14.4** | ✅ Smoke checklist → close M5 | — |

**14.0 notes**

- ✅ Shipped (2026-10-01): negative cash shortfall; `PendingPayment.fromUserId` + amount = `−cash`; End OK / Roll blocked while current cash &lt; 0; sell/mortgage → `applyRaiseTowardDebtLocked` (creditor gets chunks); trade blocked while in debt; `POST /games/{id}/bankrupt`; `POST /games/{id}/debt-pay/start` (2m, pause turn clock; `awaiting_roll` only); auto-bankrupt on insolvent / timer; resign/timeout wipe to Bank (deeds unowned, GOOJF deck bottom, Bank tops up creditor); OpenAPI **0.28** + mobile codegen; `go test ./internal/services/game/` green.
- Raise during debt: **sell + mortgage only**; block **trade** propose/accept while indebted / debt-pay active.
- UI modal / red cash / toasts → **14.2**. Jail path → **14.1**.

**14.1 notes**

- ✅ Shipped (2026-10-01): removed 12.1 soft-lock; 3rd failed doubles always charges JailFine (may go negative), leaves Jail, moves with that roll, resolves landing; `pendingPayment.kind = jail` / LastPayment `jail_fine`; resolveLanding no longer wipes open debt on no-op lands. UI gate → **14.2**.

**14.2 notes**

- ✅ Shipped (2026-10-01): board `DebtOverlay` — non-dismissible Pay \| Bankruptcy on next turn (`awaiting_roll`); Pay → `POST …/debt-pay/start` + compact 2:00 banner (sell/mortgage usable); Bankruptcy → `POST …/bankrupt`; hub auto Open board for debtor; toasts for debt-pay start + wipe (declare / auto); resign copy updated for Bank wipe; red negative cash already from 14.0 polish.

**14.3 notes**

- ✅ **Cancelled / locked (2026-10-01):** user chose no wipe re-auction. Wipe returns deeds to Bank as unowned (clean; mortgage/houses gone). Buy \| Auction only on a **future landing**. If another player is already sitting on a wiped tile, suppress Buy \| Auction so they End / Roll away. `SuppressBuyOffer` clears on the next `Roll` so doubles are not soft-locked. Regression: `TestWipeReturnsDeedsUnownedWithoutAuction`, `TestWipeUnderSittingLanderSuppressesBuyOffer`, `TestWipeSuppressClearsOnNextRoll`.

**14.4 — smoke checklist (Phase 14 / M5 close)** ✅

**A. Debt + raise (14.0–14.2)**

1. Rent/tax/card shortfall → cash negative; End OK; Roll blocked; creditor got partial cash.
2. Next turn → Pay \| Bankruptcy; Pay → 2:00; sell/mortgage only; cash climbs; creditor receives; at ≥ 0 Roll unlocks.
3. Bankruptcy / 2:00 expiry / no assets left → eliminate that player; deeds unowned; GOOJF deck bottom; owed player topped up by Bank; table continues.

**B. Jail + resign (14.1 + wipe)**

1. Jail 3-fail broke → negative fine; next-turn gate works.
2. Resign / timeout → same Bank wipe (no frozen deeds).

**C. Unowned after wipe (14.3 cancelled)**

1. Wiped deed stays unowned; no auction starts.
2. Pin already on wiped tile → no Buy \| Auction; Roll/End away.
3. Later land on that tile → normal Buy \| Auction.

Smoke → **14.4** ✅. **Phase 14 / M5 DONE.**

---


### Phase 15 — Production hardening (Contabo)

**Goal:** Deploy binary to Contabo; Redis on VPS; MongoDB Atlas; Google SMTP prod creds; TLS reverse proxy.

**Owner-run (locked 2026-10-01):** Phase 15 deploy is **handled by the project owner**. Agent does **not** drive Contabo/Atlas/SMTP/TLS unless the user asks for help (checklist, systemd/Caddy snippets, debugging).

**Ask before (when helping):** systemd unit contents, nginx vs Caddy, TURN (coturn).

**Also consider here (if multi-instance):** Redis **hot presence** keys (Phase 7 ships memory-only).

**Exit criteria**

- [ ] Mobile points at prod API
- [ ] Signup email works in prod
- [ ] One full M1 game on prod infra

---

### Encore — Location prestige reorder (post-M5)

**Goal:** Within each World’s color groups, place cities by relative prestige (not A–Z), keeping board indexes / prices / rents / houseCost fixed. Airports & utilities unchanged. Duplicate city names in other packs → fix when that world is touched.

| Slice | Deliverable | Not yet |
| --- | --- | --- |
| **E.1** | ✅ `africa-1` prestige reorder within color groups | — |
| **E.2** | ✅ All other worlds prestige reorder + CA/Oceania/SA dupe fixes | — |

**E.1 notes**

- ✅ Shipped (2026-10-01): swap city **identity** fields among slots in the same color group (name, slug, hubId, assets, copy, country, map, svgcities, …). Economy numbers stay with `boardIndex`.
- Rank (least → most prestigious within group): brown Antananarivo→Accra; lightBlue Asmara→Benghazi→Cairo; pink Dakar→Casablanca→Cape Town; orange Fez→Kigali→Lagos; red Lalibela→Maputo→Marrakesh; yellow Meroë→Maseru→Nairobi; green Timbuktu→Ouagadougou→Port Louis; darkBlue Yamoussoukro→Tunis. Airports/utilities unchanged.

**E.2 notes**

- ✅ Shipped (2026-10-01): same within-group identity swap for asia-1/2, central-america-1, europe-1…5, middle-east-1, north-america-1, oceania-1, south-america-1.
- **Duplicate `-r2` cities replaced** (slot kept; new identity):
  - **central-america-1:** Kingston, Bridgetown, Santo Domingo, Managua, Belize City, Port of Spain, Oranjestad, Panama City
  - **oceania-1:** Darwin, Christchurch, Adelaide, Gold Coast, Brisbane, Auckland, Cairns, Port Moresby
  - **south-america-1:** Rosario + Maracaibo on lightBlue; Buenos Aires + Caracas moved to darkBlue; Asunción + Salvador on green
- europe-4 lightBlue: Nicosia→Nuremberg→Odessa (no Paphos typo/mis-slot).
- Validation: every world 40 spaces / 22 properties; unique name/slug/hubId/boardCode within world; price/rent/houseCost stay on slots.
- Cross-world same display names (different places): Granada (CA vs EU), Córdoba (EU vs SA), Panama City (CA capital vs NA pack). Intentional.
- Re-seed Mongo after pull: `go run ./cmd/seed-locations -file seeds/locations.json`

---

### Phase 16 — Board video + deferred extras

**In progress — board cameras (ask before each sub-slice):**

| Slice | Deliverable | Not yet |
| --- | --- | --- |
| **16.0** | ✅ SFU board-only video forward (`videoPubs`, stream id `video-{userId}`); hub ignores video | Mobile publish/UI |
| **16.1** | ✅ Mobile `muteVideo` + board publish/play video tracks (`localVideoStream` / `remoteVideoByUserId`) | Seat grid UI |
| **16.2** | Meet-style `BoardSeatGrid` + local controls + long-press info modal | Turn ring polish |
| **16.3** | 3m turn-clock border ring on current seat + plan/skill docs | Smoke |
| **16.4** | Smoke checklist | — |

**16.0 notes**

- ✅ Shipped: `IsVideoRoom` = board only; `room.videoPubs`; `OnTrack` routes audio → existing relay, video → board relay with stream id `video-{userId}`; join gets existing video pubs; Attach/Detach/reconnect unpublish video; hub video drained+ignored; renegotiation queues while an SFU offer is in flight (audio+video). Unit tests for map alloc, detach, reconnect, renego queue.

**16.1 notes**

- ✅ Shipped: SecureStore `muteVideo` + `useMuteVideo`; Settings “Camera off”; board `getUserMedia` audio+front camera; hub stays `video: false`; remote video mapped by `video-{userId}` → `remoteVideoByUserId`; `localVideoStream` for preview; mute applies `track.enabled` (no renegotiation); peer-left clears remote video; wait for mic+video prefs before board presence connect.

**Still deferred until asked:**

- Location **admin** CRUD
- OAuth
- Web R3F client + Wails desktop
- TURN, recording, moderation tools
- More countries’ seed packs
- **Hub video (cameras)** + **selective listen** (pick who you hear/see in hubs ≤16) — do not build listen-matrix before hub video

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
| 2026-09-24 | **Phase 8 split:** 8.0 hub room lifecycle; 8.1 hub poses; 8.2 hubId + board in-hub affordance; 8.3 turn notify + sheet; 8.4 harden; cross-table meet **allow**; voice → 10                                                                |
| 2026-09-24 | **8.0:** `GET /ws/presence/hub/{hubId}`; Enter leaves board SFU room / joins hub; Leave reverse; game WS stays; mobile `useHubPresence` + board `useFocusEffect`                                                                          |
| 2026-09-25 | **8.2:** `POST enter-hub` / `leave-hub`; `GamePlayer.hubId`; panel `Name(in CODE)`; Board pin/avatar layers memoized to cut avatar hitch during pin walks                                                                                 |
| 2026-09-25 | **8.1:** hub walk surface + ~10 Hz pose publish; remotes via `BoardRemoteAvatar`; `useHubWalk` + `HubScene` (SFU pose path unchanged)                                                                                                     |
| 2026-09-25 | **8.2 polish:** `buildBoardRemoteAvatars` seeds frozen tile-center poses from `hubId` when board presence remotes were cleared (returner sees in-hub peers)                                                                               |
| 2026-09-25 | **Hub locked:** max **16**/room; Leave = X only; 3-pane + country → Phase 9; **8.3** turn sheet + Open board keeps hubId                                                                                                                  |
| 2026-09-25 | **8.3:** SFU `MaxHubPeers=16`; hub turn toast + `HubTurnSheet` (Roll/End/Open board); X leave + block system back; Open board skips leave-hub                                                                                             |
| 2026-09-25 | **8.3 polish:** turn sheet 2×2; hub End gated like board `turnBusy`; hub buy sheet Buy→End + Open board + time bank; strip world prefixes on deed names                                                                                   |
| 2026-09-25 | **8.4:** `hubRevision` stale-enter ignore; mobile abort enter-on-leave; hub welcome.peers seed + soft-reconnect clear; hub-full WS error stops retry                                                                                      |
| 2026-09-25 | **9.0a:** presence `country` on welcome/peers/peer-joined; `GamePlayer.country` via user lookup; board HUD country; presence `roster` for hub chrome                                                                                      |
| 2026-09-25 | **9.0b:** equal 3-pane hub shell; full-height rails; 2px center borders; per-tile `HubScene` floor                                                                                                                                        |
| 2026-09-25 | **9.0c:** `HubLocationCopy` on rail (about + ellipsis); `HubScene` avatars-only above copy                                                                                                                                                |
| 2026-09-25 | **9.0d:** `HubRoster` IN HUB n/16 · 2-col · country · X; right-rail joystick                                                                                                                                                              |
| 2026-09-25 | **9.0e:** hub chrome smoke checklist (shell/copy/walk/roster/country/presence/leave)                                                                                                                                                      |
| 2026-09-25 | **9.1:** shared `DeedCard` (strip border + Moti stagger); buy + tile-info overlays; Phase 9.1–9.4 sub-slices locked                                                                                                                       |
| 2026-09-25 | **9.2:** home Settings `@expo/ui` Mute mic (SecureStore); Leave board-only; report deferred until picker + API                                                                                                                            |
| 2026-09-27 | **9.3:** board economy modals (buy/rent/tax/salary, 2.5s queue, involved-only); hub/spectators toasts; Chance/Auction/Jail refs → later phases                                                                                            |
| 2026-09-27 | **9.4:** HubRoster ReactNode tsc fix; economy queue unique ids; Phase 9 full-match smoke checklist documented                                                                                                                             |
| 2026-09-27 | **Phase 10 split:** 10.0–10.4 hub-first voice; muteMic SoT; audio-only; STUN; board voice = 10.4 ask-before                                                                                                                               |
| 2026-09-27 | **10.0:** hub `OnTrack` audio → `TrackLocalStaticRTP` forward; SFU renegotiation `offer` + WS `answer`; board rooms stay pose-only; video ignored                                                                                         |
| 2026-09-27 | **10.1:** hub `getUserMedia` + `addTrack`; `muteMic` → `track.enabled`; teardown stops mic; board pose-only; remote playback → 10.2                                                                                                       |
| 2026-09-27 | **10.2:** hub `ontrack` remote audio; answer SFU renegotiation `offer`; leave stops remote playout                                                                                                                                        |
| 2026-09-27 | **10.2b:** hub loudspeaker via `react-native-incall-manager` (`startHubSpeaker` / `stopHubSpeaker`); needs rebuilt dev client                                                                                                             |
| 2026-09-27 | **10.3:** hub left-rail `HubMediaRail` Live + mute CTA (`muteMic`); Settings hint; smoke checklist; Phase 10 hub voice exit                                                                                                               |
| 2026-09-27 | **10.4:** board SFU voice (`IsVoiceRoom`); `useBoardPresence` mic + playback; shared `MuteMicButton` on board panel; Phase 10 complete                                                                                                    |
| 2026-09-27 | **Phase 10 DONE:** selective listen deferred → Phase 16 with video; everyone-audio + muteMic for now                                                                                                                                      |
| 2026-09-27 | **Phase 11 split:** 11.0–11.5 houses/hotels/light mortgage; house-shortage auction stays Phase 13; ask before each slice                                                                                                                  |
| 2026-09-27 | Skill + plan: Phase 10 voice locks; Phase 11.0 `houses` 0–5 (5=hotel); selective listen/video → 16                                                                                                                                        |
| 2026-09-27 | **11.0:** `Deed.houses` 0–5 + `mortgaged`; monopoly rent 2× site / `Rents[houses]` when built; OpenAPI 0.16 + mobile types; no build/sell UI                                                                                              |
| 2026-09-27 | **11.1 locks:** build body `{boardIndex}` one step; current-player only; economy CTA bar + Roll/End icon boxes → 11.4 UI                                                                                                                  |
| 2026-09-27 | **11.1:** `POST /games/{id}/build`; even-build + monopoly + `houseCost`; OpenAPI 0.17; no board UI yet                                                                                                                                    |
| 2026-09-27 | **11.2:** `POST /games/{id}/sell-building`; half `houseCost`; even-sell; allowed + auto-apply during pendingPayment; OpenAPI 0.18                                                                                                         |
| 2026-09-27 | **11.3:** `POST` mortgage + redeem (½ price / +10%); 0 rent when mortgaged; sell buildings first; OpenAPI 0.19                                                                                                                            |
| 2026-09-28 | **11.4a:** dock dice/end/hub wired (opacity 0.35 disabled); HUD Roll/End/Enter text removed; economy bar → 11.4b                                                                                                                          |
| 2026-09-28 | **Must-buy interim:** `canEndTurn` false + `ErrMustBuy` while `buyOffer` open; board buy modal non-dismissible; drop “Or End turn to skip” (auction → 13)                                                                                   |
| 2026-09-28 | **11.4b:** economy CTA bar above dock; mode + how-to sheet; eligible highlight + tap → build/sell/mortgage/redeem; TRADE stub; markers → 11.4c                                                                                            |
| 2026-09-28 | **11.4c:** house (1–4) / hotel (5) / M markers on color-band edge from `game.deeds`                                                                                                                     |
| 2026-09-28 | **11.5:** M2 smoke checklist (dock/must-buy/economy/markers/raise-funds); `go test ./internal/services/game/` green; Phase 11 DONE (auction shortage → 13)                                              |
| 2026-09-28 | **Phase 12 split:** 12.0–12.5 jail + Chance/Chest; jail fine **100** MeetCoin; classic card table locked for 12.2–12.3; ask before each slice                                                         |
| 2026-09-28 | **12.0:** `inJail` / `jailTurns` / `getOutOfJailFree`; Go to Jail + third doubles → Jail; Just Visiting; OpenAPI 0.20; exit → 12.1                                                                  |
| 2026-09-28 | **12.1:** pay-jail-fine (100) + use-jail-card; roll-from-jail doubles / 3-fail forced pay; `canPayJailFine` / `canUseJailCard`; OpenAPI 0.21; UI → 12.4                                              |
| 2026-09-28 | **12.2:** Chance/Chest catalog by boardIndex; shuffle + draw on land; persist decks; `lastCard`; GOOJF held; effects → 12.3; OpenAPI 0.22                                                         |
| 2026-09-28 | **12.3:** apply card effects on draw (lock A); move/cash/jail/repairs/nearest RR×2 / util 10×; `lastPayment.kind=card`; OpenAPI 0.23; modal → 12.4                                              |
| 2026-09-28 | **12.4:** board card modal (all seated) + Just Visiting modal; jail sheet (pay/card) + exit toasts; hub toasts; jail pin center vs visiting edges; smoke → 12.5                              |
| 2026-09-28 | **Jail UX plan lock:** **12.4b** = avatar + Pay / Roll a Double / Use card + dock Roll gating; negative cash after 3 fails + bankruptcy gate → **Phase 14**; keep 12.1 soft-lock until then |
| 2026-09-28 | **12.4b:** jail modal avatar + Pay / Roll a Double / Use card; dock Roll off while modal open; Use card disabled at 0 GOOJF; smoke → 12.5                                                              |
| 2026-09-28 | **12.4c:** `lastCard.cashDelta` (signed MeetCoin) for cash / pay-each / birthday / repairs; card modal + toast show +/- amount; OpenAPI 0.24 |
| 2026-09-28 | **12.4d:** Chance/Chest — drawer modal only; others toast (name + card text + cashDelta); jail failed-doubles toast after roll |
| 2026-09-28 | **12.4e:** Pass-GO salary deferred until after Chance/Chest reveal + pin resume (card → move → salary); OpenAPI unchanged |
| 2026-09-29 | **12.5:** Phase 12 smoke OK (manual); **Phase 12 DONE**                                                                                                                                  |
| 2026-09-29 | **Phase 13 split:** 13.0–13.4 auction then trade; house-shortage deferred; auction locks (30s, turn-based, high sits out, keypad, banks paused); ask before each slice                  |
| 2026-09-29 | **13.0:** bank auction APIs + 30s timer + auto-bid/fold + settle/void + bank pause; OpenAPI 0.25; must-buy → buy\|auction; UI → 13.1                                                                 |
| 2026-09-29 | **13.1:** AuctionOverlay + Buy\|Auction + dock peek + hub auto Open board; settle toast + winner bought @ auction price; trade → 13.2                                                              |
| 2026-09-29 | **13.1 polish:** auction bid = native TextInput (not custom keypad); turn timer **60s**; layout fix so controls stay under deed/feed                                                                |
| 2026-09-29 | **13.1 polish:** auction overlay (and hub Open board) wait for dice/pin idle like buy — no modal over mid-walk auto-auction                                                                        |
| 2026-09-29 | **13.2:** trade propose/accept/decline APIs + 60s reply; **3m turn clock** + 2-strike forfeit; OpenAPI 0.26; panel current-only green→red; trade UI → 13.3                                         |
| 2026-09-29 | **13.3:** TradeOverlay compose + accept/decline + mortgage choice; TRADE CTA; hub Open board on trade; party-only modal; `lastTrade` toasts (OpenAPI 0.27); smoke → 13.4                                                                               |
| 2026-09-30 | **13.4:** Phase 13 smoke OK (manual); **Phase 13 DONE** (house-shortage auction deferred)                                                                                              |
| 2026-09-30 | **Phase 14 split:** 14.0–14.4 negative-cash debt; Pay 2:00 (sell/mortgage, no trade) \| Bankruptcy; Bank wipe + creditor cash (no asset transfer); jail **14.1**; UI **14.2**; re-auction **14.3**; ask before each slice |
| 2026-10-01 | **14.0:** negative cash + `owedTo`/`fromUserId`; End OK / Roll blocked; sell/mortgage settle creditor; `POST …/bankrupt` + `…/debt-pay/start` (2m); resign wipe to Bank; OpenAPI 0.28; UI → **14.2** |
| 2026-10-01 | **14.2:** board Pay\|Bankruptcy + 2:00 debt-pay banner; hub Open board; debt/bankruptcy toasts; resign wipe copy |
| 2026-10-01 | **14.3 cancelled:** wipe → unowned only (no Bank re-auction); suppress Buy if sitting on wiped tile |
| 2026-10-01 | **14.4:** Phase 14 smoke OK; **Phase 14 / M5 DONE** |
| 2026-10-01 | **Phase 15:** Contabo deploy = **owner-run**; agent helps on request only |
| 2026-10-01 | **Encore E.1:** `africa-1` property cities reordered by prestige within color groups (prices stay on slots) |
| 2026-10-01 | **Encore E.2:** all other Worlds prestige reorder; CA/Oceania/SA `-r2` dupes replaced; reseed required |
| 2026-10-01 | **Phase 16 split:** board cameras 16.0–16.4; hub video + selective listen still deferred |
| 2026-10-01 | **16.0:** SFU board-only `videoPubs` + RTP relay (`video-{userId}`); hub ignores video; tests |
| 2026-10-01 | **16.1:** `muteVideo` + Settings; board publish/play camera; hub audio-only; stream maps on presence result |
| 2026-09-24 | **Mobile UX:** hide status bar app-wide; board panel extra top padding so ⋯ clears the top edge                                                                                                                                           |
