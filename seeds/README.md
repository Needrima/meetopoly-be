# Seeds

## `locations.json`

Worlds board content for MongoDB collection `locations`.

### Import (CLI)

From `meetopoly-be/`:

```bash
go run ./cmd/seed-locations -file seeds/locations.json
```

Uses `MONGO_URI` / `MONGO_DATABASE` from env (defaults: `mongodb://127.0.0.1:27017`, `meetopoly`). Replaces all documents in `locations`, then ensures indexes on `(worldId, boardIndex)` and unique `(worldId, slug)`.

Offline JSON editors (no Mongo): `cmd/remap-locations`, `cmd/polish-locations` — see **Seed tooling** below.
### Board template (all worlds)

Every `worldId` is a **classic 40-space** Monopoly ring:

| Kind | boardIndex |
|------|------------|
| Corners | GO `0`, Jail `10`, Layover `20`, Go to Jail `30` |
| Chance | `7`, `22`, `36` (tile label **CHA**) |
| Community Chest | `2`, `17`, `33` (tile label **CHE**) |
| Income / Luxury tax | `4`, `38` |
| Air (railroad) | `5`, `15`, `25`, `35` |
| Power / Water | `12`, `28` |
| Properties | 22 city slots |

`boardCode` is unique within `worldId` (`CHA`/`CHA2`/…, `CHE`/`CHE2`/…). The mobile board always displays Chance as **CHA** and Chest as **CHE**.

If a continent has extra cities after filling 22: leftovers **&lt; 15** stay unused until a future `-2` pack; **≥ 15** can form a `-2` pack padded with repeats from `-1` (same continent). Short packs pad with in-world repeats (`slug-rN`) to reach 22.

### Worlds in this file

| worldId | Board spaces | Notes |
| ----------------------- | ------------ | ------------------------------------------------------------ |
| `africa-1` | 40 | Ship first |
| `europe-1` … `europe-5` | 40 each | Trimmed to 22 cities each |
| `asia-1`, `asia-2` | 40 each | Trimmed to 22 cities each |
| `north-america-1` | 40 | Trimmed to 22 cities |
| `south-america-1` | 40 | Padded with in-world repeats where needed |
| `middle-east-1` | 40 | Trimmed to 22 cities |
| `oceania-1` | 40 | Padded with in-world repeats |
| `central-america-1` | 40 | Padded with in-world repeats |

Property `colorGroup` follows classic Monopoly bands by `boardIndex` (e.g. 31/32/34 green, 37/39 darkBlue) on every world.

City icons: [svgcities.com](https://svgcities.com/) / [anto1/city-icons](https://github.com/anto1/city-icons) (CC BY 4.0).

### Fields

- `worldId` — board pack
- `kind`: `property` (city) \| `railroad` \| `utility` \| `special`
- `boardIndex` — logical track order (game pin)
- `boardCode` — short tile label unique within `worldId`
- `map.x` / `map.z` — overworld placement
- `assets.icon` — art path relative to `meetopoly-mobile/`
- `hubId` — hub room key

### Art

- **Cities (runtime):** `meetopoly-mobile/city-icons/icons/*.svg` (SVGCities, CC BY 4.0)
- **Airports / utilities / specials:** `meetopoly-mobile/city-icons/generic/`
- `kind` stays `railroad` / `utility` for rules; display is Air hubs + Power/Water
- Specials: jail + go-to-jail → Tabler `prison`; tax → Tabler `tax`; free parking display **Layover**

See `docs/plan.md` Phases 3–4 and skill `product.md` Worlds table.

### Seed tooling (Go)

From `meetopoly-be/`:

```bash
# Rebuild every world to classic 40 (Chance/Chest/tax/rails/utils + colorGroups)
go run ./cmd/remap-locations -file seeds/locations.json

# Named airports, prison/tax icons, unique boardCodes within each world
go run ./cmd/polish-locations -file seeds/locations.json

# Push JSON → Mongo
go run ./cmd/seed-locations -file seeds/locations.json
```

Typical order after editing board data: `remap-locations` → `polish-locations` → `seed-locations`.
