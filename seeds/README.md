# Seeds

## `nigeria-locations.json`

Import into MongoDB Compass:

1. Create database (e.g. `meetopoly`) and collection `locations`.
2. Collection → **Add Data** → **Import JSON or CSV file**.
3. Select `nigeria-locations.json` (JSON Array).
4. Import.

Fields:

- `kind`: `property` | `railroad` | `utility` | `special`
- `boardIndex`: logical track order for the game pin
- `map.x` / `map.z`: relative overworld placement (tune in Phase 4)
- `hubId`: WebRTC / hub room key when a player enters
- `assets.*`: placeholders until real art exists

See `docs/plan.md` Phase 3.
