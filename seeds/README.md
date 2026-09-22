# Seeds

## `locations.json`

Worlds board content for MongoDB collection `locations`.

### Import (CLI)

From `meetopoly-be/`:

```bash
go run ./cmd/seed-locations -file seeds/locations.json
```

Uses `MONGO_URI` / `MONGO_DATABASE` from env (defaults: `mongodb://127.0.0.1:27017`, `meetopoly`). Replaces all documents in `locations`, then ensures indexes on `(worldId, boardIndex)` and unique `(worldId, slug)`.

### Import (Compass)

1. Create database (e.g. `meetopoly`) and collection `locations`.
2. **Add Data** → **Import JSON or CSV file**.
3. Select `locations.json` (JSON Array).
4. Import.

Filter in app/API by `worldId`. **Gameplay v1 ships `africa-1` first**; other Worlds are seeded for later.

### Worlds in this file

| worldId                 | Cities (approx) | Board spaces | Notes                                    |
| ----------------------- | --------------- | ------------ | ---------------------------------------- |
| `africa-1`              | 24              | 40           | **Ship first** — classic even sides (corners 0/10/20/30) |
| `europe-1` … `europe-5` | 26 each         | 42 each      | Alphabetical packs from SVGCities Europe |
| `asia-1`, `asia-2`      | 24 each         | 38 each      | Split Asia pool                          |
| `north-america-1`       | 30              | 46           | Full NA pool                             |
| `south-america-1`       | 18              | 33           | Slightly short                           |
| `middle-east-1`         | 25              | 41           | Jerusalem once (`il-jerusalem`)          |
| `oceania-1`             | 14              | 24           | Short board                              |
| `central-america-1`     | 14              | 24           | Short board                              |

City icons: [svgcities.com](https://svgcities.com/) / [anto1/city-icons](https://github.com/anto1/city-icons) (CC BY 4.0). Duplicates in the site list (e.g. Hanoi×2, São Paulo×2) are deduped.

### Fields

- `worldId` — board pack
- `kind`: `property` (city) \| `railroad` \| `utility` \| `special`
- `boardIndex` — logical track order (game pin)
- `map.x` / `map.z` — overworld placement (tune in Phase 4)
- `assets.icon` — billboard art path relative to `meetopoly-mobile/` (cities: `city-icons/icons/{cc}-{slug}.svg`; airports/utilities: `city-icons/generic/*.svg`)
- `svgcities` — display name from SVGCities
- `svgcitiesUrl` / `svgcitiesPath` — source page + upstream file path
- `symbol` — landmark title (e.g. “Accra Independence Arch”)
- `about` / `aboutShort` — SVGCities About copy
- `attribution` — required credit (SVGCities CC BY for cities; Tabler MIT for generics)
- `hubId` — hub room key

### Art

- **Cities (runtime):** `meetopoly-mobile/city-icons/icons/*.svg` (SVGCities, CC BY 4.0)
- **Airports / utilities (runtime):** `meetopoly-mobile/city-icons/generic/` — `plane-tilt.svg`, `bolt.svg`, `droplet.svg` (Tabler Icons, MIT)
- **Backend mirror:** `meetopoly-be/city-icons/` (same layout)
- `kind` stays `railroad` / `utility` for rules; display names are **Air** hubs and Power/Water works
- Specials still use placeholder `icons/generic/...` until that art ships

See `docs/plan.md` Phases 3–4 and skill `product.md` Worlds table.
