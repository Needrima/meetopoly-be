# Meetopoly backend

Go API for Meetopoly (`module meetopoly-be`). Hexagonal layout: adapters → services → repository.

**No Docker.** Run MongoDB and Redis locally.

## Prerequisites

- Go 1.22+
- MongoDB on `127.0.0.1:27017`
- Redis on `127.0.0.1:6379`

## Run

```bash
cd meetopoly-be
go run .
```

### Hot reload (Air)

Air is already configured via `.air.toml` (watches `.go` files, rebuilds root `main.go`).

```bash
# once
go install github.com/air-verse/air@latest

cd meetopoly-be
air
```

Server listens on `:8080` by default.

### Env (optional)

| Variable | Default |
|----------|---------|
| `HTTP_ADDR` | `:8080` |
| `MONGO_URI` | `mongodb://127.0.0.1:27017` |
| `MONGO_DATABASE` | `meetopoly` |
| `REDIS_ADDR` | `127.0.0.1:6379` |
| `REDIS_PASSWORD` | _(empty)_ |
| `REDIS_DB` | `0` |
| `APP_VERSION` | `0.0.1-phase0` |
| `LOG_FILE` | `app.log` (set to `-` for stdout only) |
| `LOG_LEVEL` | `info` (`debug` / `warn` / `error`) |
| `LOG_FORMAT` | `text` (use `json` in prod if you want) |

Logs go to **stdout and `app.log`** via `internal/platform/logging` (`log/slog`). Tail the file while debugging:

```bash
tail -f app.log
```

### Health

```bash
curl -s http://127.0.0.1:8080/health
```

OpenAPI: `api/openapi.yaml` (Phase 0: `/health` only).

HTTP router: **chi**. Seeds: `seeds/locations.json`. Plan: `docs/plan.md`.
