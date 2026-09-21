# Meetopoly backend

Go API for Meetopoly (`module meetopoly-be`). Hexagonal layout: adapters → services → repository.

**No Docker.** Run MongoDB and Redis locally.

## Prerequisites

- Go 1.22+
- MongoDB on `127.0.0.1:27017`
- Redis on `127.0.0.1:6379`
- Gmail app password for SMTP (Phase 2 auth)

## Run

```bash
cd meetopoly-be
# Fill SMTP_* in `.env`, then:
go run .
```

Env is loaded with **joho/godotenv** from `.env` (gitignored).

### Hot reload (Air)

Air is already configured via `.air.toml` (watches `.go` files, rebuilds root `main.go`).

```bash
# once
go install github.com/air-verse/air@latest

cd meetopoly-be
air
```

Server listens on `:8080` by default.

### Env

| Variable | Default |
|----------|---------|
| `HTTP_ADDR` | `:8080` |
| `MONGO_URI` | `mongodb://127.0.0.1:27017` |
| `MONGO_DATABASE` | `meetopoly` |
| `REDIS_ADDR` | `127.0.0.1:6379` |
| `REDIS_PASSWORD` | _(empty)_ |
| `REDIS_DB` | `0` |
| `APP_VERSION` | `0.2.0-phase2` |
| `LOG_FILE` | `app.log` (set to `-` for stdout only) |
| `LOG_FORMAT` | `text` (use `json` in prod if you want) |
| `SIGNUP_TOKEN_TTL_MINUTES` | `30` |
| `VERIFICATION_CODE_TTL_MINUTES` | `2` |
| `SMTP_HOST` | `smtp.gmail.com` |
| `SMTP_PORT` | `587` |
| `SMTP_USER` | _(required for mail)_ |
| `SMTP_PASS` | _(Gmail app password)_ |
| `SMTP_FROM` | e.g. `Meetopoly <you@gmail.com>` |

Logs go to **stdout and `app.log`** via `log/slog` at **info** level (configured from `.env` for file/format only). Tail the file while debugging:

```bash
tail -f app.log
```

### Health

```bash
curl -s http://127.0.0.1:8080/health
```

### Auth (Phase 2)

Opaque Redis sessions (`session:{token}`) — no expiry; revoke on logout only. Signup flow:

1. `POST /auth/signup/start` `{ "email" }` → SMTP 6-digit code  
2. `POST /auth/signup/verify` `{ "email", "code" }` → `signupToken`  
3. `POST /auth/signup/password` + `Authorization: Bearer <signupToken>` `{ "password" }`  
4. `POST /auth/signup/profile` + Bearer signup token `{ "username", "country" }` → session  
5. `POST /auth/login` `{ "email", "password" }` → session  
6. `GET /me` + Bearer session → profile  
7. `POST /auth/logout` + Bearer session → revoke  

Password hashing: **bcrypt**. Username: 3–20 `[A-Za-z0-9_]`. Country: ISO alpha-2.

OpenAPI: `api/openapi.yaml`. Mobile codegen: **orval** → `meetopoly-mobile` (`npm run api:generate`).

HTTP router: **chi**. Seeds: `seeds/locations.json`. Plan: `docs/plan.md`.
