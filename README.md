# Claude Usage Server

Collects Claude Code usage from the team's Claude Usage Monitor desktop apps and serves it to the usage dashboard.

- Each desktop app sends its sessions every 30 minutes to `POST /usage/v1/sessions` with an API key. Sends carry absolute values and are upserted, so retries and restarts never double count.
- Postgres (in Docker) keeps one row per Claude Code session and its growth per UTC hour per model.
- The dashboard signs in with a JWT and reads `/usage/v1/overview`, `/timeseries`, `/models`, `/sessions` and `/people`. Admins (`super_admin`) see everyone; members see only themselves.
- Admins create users and issue API keys under `/core/v1/admin`.

v1 runs on localhost only: every port is published on `127.0.0.1`, and CORS allows `http://localhost:5173` and `http://localhost:3000`.

Stack: Go (Gin, pgx/v5), Postgres 17, golang-migrate.

## Prerequisites

- Go 1.26 (the toolchain downloads itself from `go.mod`)
- Docker with Compose
- [golang-migrate](https://github.com/golang-migrate/migrate) CLI (`migrate`)
- GNU make
- Optional: [air](https://github.com/air-verse/air) for `make dev`

`psql` is not needed on the host; seeders run through `docker compose exec postgres psql`.

## Quick Start

```bash
cp .env.example .env              # set JWT_SECRET; the DB defaults match docker-compose.yml
docker compose up -d --build --wait
```

Then open **http://localhost:3000** and sign in with the seeded admin below.

Compose runs three services, all `restart: unless-stopped` and published only on `127.0.0.1`:

| Service | URL | Notes |
|---------|-----|-------|
| `postgres` | `127.0.0.1:5432` | Data in the `pgdata` volume |
| `api` | http://127.0.0.1:8080 | Runs migrations and the idempotent seeders on every start |
| `dashboard` | http://localhost:3000 | Built from `../claude-usage-dashboard` |

After changing server or dashboard code, run `docker compose up -d --build --wait` again. `docker compose down` stops everything and keeps the data.

To run the API on the host instead (for development), stop the container first so port 8080 is free:

```bash
docker compose stop api
make db-setup           # run migrations and seeders against 127.0.0.1:5432
make run                # start the API on http://127.0.0.1:8080
```

`JWT_SECRET` in `.env.example` is a placeholder and must be changed: the server refuses to start in every `ENV` when the secret is empty, shorter than 32 characters, or still the placeholder. Generate one with `openssl rand -hex 32`.

If port 5432 is taken, change the host port in `docker-compose.yml` and set `DB_PORT` to match.

Seeded admin (from `.env`):

| Email | Password |
|-------|----------|
| `admin@claude-usage.local` (`SEED_ADMIN_EMAIL`) | `<SEED_ADMIN_PASSWORD>` (`SEED_ADMIN_PASSWORD`) |

Change the password after the first sign-in (`POST /core/v1/auth/change-password`).

## Add a Team Member and Issue a Key

1. Sign in as the admin and copy `data.access_token`:

   ```bash
   curl -s -X POST http://127.0.0.1:8080/core/v1/auth/signin \
     -H "Content-Type: application/json" \
     -d '{"email":"admin@claude-usage.local","password":"<SEED_ADMIN_PASSWORD>"}'
   ```

2. Create the member. They get the `member` role. Copy `data.id`:

   ```bash
   curl -s -X POST http://127.0.0.1:8080/core/v1/admin/users \
     -H "Authorization: Bearer <access_token>" -H "Content-Type: application/json" \
     -d '{"email":"mia@example.com","username":"mia","full_name":"Mia","password":"MiaPass123!"}'
   ```

3. Issue an API key for the member:

   ```bash
   curl -s -X POST http://127.0.0.1:8080/core/v1/admin/users/<member_id>/api-keys \
     -H "Authorization: Bearer <access_token>" -H "Content-Type: application/json" \
     -d '{"name":"MIA-LAPTOP"}'
   ```

   `data.key` (`wiz_live_...`) is shown only once. The member pastes it into the desktop app's Settings → API key (or sets `CLAUDE_USAGE_API_KEY`), with Server URL `http://127.0.0.1:8080`.

To test ingest by hand:

```bash
curl -s -X POST http://127.0.0.1:8080/usage/v1/sessions \
  -H "X-API-Key: <key>" -H "Content-Type: application/json" \
  --data @testdata/ingest.json
```

Other admin actions: list users (`GET /core/v1/admin/users`), reset a password, deactivate or activate a user, list or revoke a user's keys. Deactivating a user revokes all their keys and blocks sign-in.

## Commands

| Command | What it does |
|---------|--------------|
| `make up` / `make down` | Build and start / stop the full stack (Postgres, API, dashboard) |
| `make db-setup` | Migrations + seeders |
| `make db-reset` | Drop and recreate `claude_usage`, then migrate and seed |
| `make run` | Run the API |
| `make dev` | Run with hot reload (air) |
| `make build` | Build `bin/claude-usage-server` |
| `make migrate-up` | Apply pending migrations |
| `make migrate-down MODULE=core` | Roll back one migration |
| `make migrate-version` | Show the schema version |
| `make migrate-create NAME=add_foo MODULE=core` | New timestamped migration pair |
| `make test-db` | Create, migrate and seed `claude_usage_test` |

## Tests

Unit tests run without a database:

```bash
go test ./...
```

Database tests skip unless `TEST_DATABASE_URL` is set. They use a separate database, `claude_usage_test`:

```bash
make test-db
TEST_DATABASE_URL="postgresql://postgres:postgres@localhost:5432/claude_usage_test?sslmode=disable" go test -count=1 ./...
```

The DB tests truncate `usage.*` and `core.api_keys` and delete every user except `admin`. Never point `TEST_DATABASE_URL` at `claude_usage`.

## Migrations

Files live in `internal/database/migrations/core` and are named by timestamp: `YYYYMMDDHHMMSS_name.up.sql` and `.down.sql`. Create them with `make migrate-create`, which passes `-format 20060102150405`. Use `TIMESTAMPTZ` for time columns.

| Migration | Contents |
|-----------|----------|
| `20261030000001_init_schema` | `core` schema, extensions |
| `20261030000002_users` | users, refresh tokens |
| `20261030000003_rbac` | roles, user roles |
| `20261030000004_api_keys` | API keys (secret stored as SHA-256) |
| `20261030000005_audit_logs` | audit logs |
| `20261030000006_usage` | `usage.sessions`, `usage.session_hours` |

## API

See [docs/api-contract](docs/api-contract/README.md). The ingest, report and admin endpoints are in [docs/api-contract/usage/README.md](docs/api-contract/usage/README.md).
