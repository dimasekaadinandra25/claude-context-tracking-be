# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Claude Usage Server stores Claude Code usage sent by the team's Claude Usage Monitor desktop apps and serves it to the usage dashboard. Go 1.26, Gin, pgx/v5, Postgres 17 in Docker. It is a stripped copy of the Venturo Go skeleton: one team, no companies, no Redis. v1 runs on localhost only.

- Desktop apps send usage with an API key: `POST /usage/v1/sessions` (`X-API-Key`).
- The dashboard signs in with a JWT and reads `/usage/v1/*`.
- Admins (`super_admin`) see everyone and manage users and keys under `/core/v1/admin`. Members (`member`) see only themselves.

Design spec: `C:\Code\Claude Usage Monitor\docs\superpowers\specs\2026-10-30-usage-server-design.md`.

## Development Commands

```bash
cp .env.example .env           # set JWT_SECRET
make up                        # postgres + api + dashboard in Docker (docker compose up -d --build --wait); open http://localhost:3000
docker compose stop api        # free 8080 before running the API on the host
make db-setup                  # migrations + seeders (roles, seeded admin)
make run                       # API on 127.0.0.1:8080
make dev                       # hot reload (air)
make build                     # bin/claude-usage-server
make down                      # stop the stack
```

Seeded admin: `admin@claude-usage.local` / `<SEED_ADMIN_PASSWORD>` (`SEED_ADMIN_EMAIL`, `SEED_ADMIN_PASSWORD`).

### Database

```bash
make migrate-up                                  # apply pending migrations
make migrate-down MODULE=core                    # roll back one
make migrate-version                             # current version
make migrate-create NAME=add_foo MODULE=core     # new YYYYMMDDHHMMSS_add_foo.{up,down}.sql
make migrate-force V=20261030000006 MODULE=core  # fix a dirty version
make seed                                        # run seeders
make db-reset                                    # drop, recreate, migrate, seed claude_usage
```

`psql` is not installed on the host. Run SQL with `docker compose exec -T postgres psql -U postgres -d claude_usage`.

### Testing

```bash
go test ./...                                    # unit tests; DB tests skip
make test-db                                     # create/migrate/seed claude_usage_test
TEST_DATABASE_URL="postgresql://postgres:postgres@localhost:5432/claude_usage_test?sslmode=disable" go test -count=1 ./...
```

DB tests use `internal/modules/usage/testdb`: `Open(t)` skips without `TEST_DATABASE_URL`, takes a Postgres advisory lock so packages don't race, truncates `usage.*` and `core.api_keys`, and deletes every user except `admin`. `User(t, pool, email, admin)` creates a member or super admin. Never point it at `claude_usage`.

## Architecture

### Modules

```
internal/
├── modules/
│   ├── core/
│   │   ├── auth/        # signin, refresh, logout, me, change-password
│   │   ├── user/        # /users/me, /admin/users (create, list, deactivate, activate, reset-password)
│   │   ├── role/        # roles and permission levels
│   │   └── api_key/     # self-service /api-keys, /admin/users/:id/api-keys, /admin/api-keys/:id/revoke
│   └── usage/           # ingest + reports (/usage/v1)
├── shared/              # audit, authz, query builder, response envelope
├── middleware/          # JWTAuth (JWT or X-API-Key), RequireRole, RequirePermission, RejectApiKey
└── router/router.go     # module wiring and route registration
```

Each module follows `domain/ dto/ handler/ repository/ service/ main.<module>.go`, where `main.<module>.go` exports `Initialize(db, ...)` and `SetupRoutes(router)`. Cross-module dependencies are wired in `router.go` with setters (e.g. `middleware.SetApiKeyValidator(apiKeyModule.Service)`, `userModule.Service.SetKeyRevoker(apiKeyModule.Service)`).

### Routes

- `/core/v1/auth/*`: sign-in takes `{"email", "password"}`; `email` also matches the username. No sign-up.
- `/core/v1/users/me`, `/core/v1/users/me/password`: self-service profile.
- `/core/v1/api-keys`: a user's own keys.
- `/core/v1/admin/*`: `JWTAuth` + `RejectApiKey` + `RequireRole("super_admin")`.
- `/usage/v1/sessions` POST: API key only (403 with a JWT).
- `/usage/v1/overview|timeseries|models|sessions|sessions/:id|people` GET: JWT only (403 with an API key).

Full contract: `docs/api-contract/`.

### Usage data

- `usage.sessions`: one row per Claude Code session, latest absolute values.
- `usage.session_hours`: `(session_id, hour, model)` → `new_context`, `requests`; `hour` is a whole UTC hour; `user_id` copied for join-free range queries.
- Ingest is an idempotent upsert of absolute values. Each session is one transaction. A `session_id` owned by another user is rejected without touching the owner's data.
- Reads are scoped in the service: non-admins always get `user_id` forced to themselves. Default `tz` is `Asia/Jakarta`; default range is the last 7 days.

### Auth and RBAC

- Roles are level-based JSON permissions on `core.roles`. `super_admin` bypasses all checks; `member` has `{"usage":"viewer"}`.
- Permission checks read Postgres through `internal/shared/authz` (no cache).
- API keys are `wiz_<env>_<key_id>_<secret>`; only the SHA-256 of the secret is stored. A key fails if revoked, expired, or its owner is inactive (`"user is inactive"`).
- Deactivating a user revokes all their keys; sign-in and refresh already refuse inactive users. Issued access tokens live until `JWT_EXPIRATION`.

## Conventions

- New code has no comments of any kind. `gofmt -l .` must print nothing; `go vet ./...` and `go build ./...` must pass.
- Responses use `internal/shared/response`: `response.Success`, `response.SuccessWithPagination`, `response.Error(c, status, message, detail)`.
- Logging: `pkg/logger` (`logger.Info`, `logger.Error(..., logger.Err(err))`).
- Validation: Gin `binding` tags on DTOs; trust-boundary checks (sizes, ≥ 0) in the service.
- Migrations: timestamp names via `make migrate-create`, `TIMESTAMPTZ` columns, UUID primary keys for core tables.
- Commits: one-line messages, only when asked.

## Testing Endpoints

```bash
curl http://127.0.0.1:8080/health

curl -X POST http://127.0.0.1:8080/core/v1/auth/signin \
  -H "Content-Type: application/json" \
  -d '{"email": "admin@claude-usage.local", "password": "<SEED_ADMIN_PASSWORD>"}'

curl "http://127.0.0.1:8080/usage/v1/overview?tz=Asia/Jakarta" \
  -H "Authorization: Bearer YOUR_ACCESS_TOKEN"

curl -X POST http://127.0.0.1:8080/usage/v1/sessions \
  -H "X-API-Key: YOUR_KEY" -H "Content-Type: application/json" \
  --data @testdata/ingest.json
```
