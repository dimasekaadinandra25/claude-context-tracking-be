# Claude Usage Server

Collects Claude Code usage from the team's Claude Usage Monitor desktop apps and serves it to the usage dashboard.

- Each desktop app sends its sessions every 30 minutes to `POST /usage/v1/sessions` with an API key. Sends carry absolute values and are upserted, so retries and restarts never double count.
- Postgres (in Docker) keeps one row per Claude Code session and its growth per UTC hour per model.
- The dashboard signs in with a JWT and reads `/usage/v1/overview`, `/timeseries`, `/models`, `/projects`, `/sessions` and `/people`. Admins (`super_admin`) see everyone; members see only themselves.
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
| `backup` | none | Daily `pg_dump` of `claude_usage` into `./backups` (see [Backups](#backups)) |

After changing server or dashboard code, run `docker compose up -d --build --wait` again. `docker compose down` stops everything and keeps the data.

To run the API on the host instead (for development), stop the container first so port 8080 is free:

```bash
docker compose stop api
make db-setup           # run migrations and seeders against 127.0.0.1:5432
make run                # start the API on http://127.0.0.1:8080
```

`JWT_SECRET` in `.env.example` is a placeholder and must be changed: the server refuses to start in every `ENV` when the secret is empty, shorter than 32 characters, or still the placeholder. Generate one with `openssl rand -hex 32`.

`API_KEY_ENCRYPTION_KEY` must be generated too (`openssl rand -hex 32`, exactly 64 hex characters); the server refuses to start when it is empty, the wrong length, not hex, or still the placeholder. Every new API key is stored AES-256-GCM encrypted with it so an admin can copy the full key later (`GET /core/v1/admin/api-keys/:id/reveal`). Anyone with database access and this key can read working API keys, so keep it out of git. Rotating it makes existing keys un-revealable (they keep working; issue a new key to copy one again).

If port 5432 is taken, change the host port in `docker-compose.yml` and set `DB_PORT` to match.

Seeded admin (from `.env`):

| Email | Password |
|-------|----------|
| `admin@claude-usage.local` (`SEED_ADMIN_EMAIL`) | `<SEED_ADMIN_PASSWORD>` (`SEED_ADMIN_PASSWORD`) |

The seeded admin starts with `must_change_password`, so the dashboard asks for a new password at the first sign-in (`POST /core/v1/auth/change-password`). Members created by an admin, and users whose password an admin reset, get the same prompt.

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

   `data.key` (`wiz_live_...`). An admin can copy it again later with `GET /core/v1/admin/api-keys/<key_id>/reveal` while the key is active. The member pastes it into the desktop app's Settings → API key (or sets `CLAUDE_USAGE_API_KEY`), with Server URL `http://127.0.0.1:8080`.

To test ingest by hand:

```bash
curl -s -X POST http://127.0.0.1:8080/usage/v1/sessions \
  -H "X-API-Key: <key>" -H "Content-Type: application/json" \
  --data @testdata/ingest.json
```

Other admin actions: list users (`GET /core/v1/admin/users`), reset a password, deactivate or activate a user, list, reveal or revoke a user's keys. Deactivating a user revokes all their keys, blocks sign-in and ends their dashboard sessions on the next request.

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
| `make backup` | Dump `claude_usage` into `./backups` now |
| `make restore FILE=... RESTORE_CONFIRM=yes` | Replace `claude_usage` with a dump |
| `make report-now` | Send the report image to `REPORT_WEBHOOKS` now |
| `make report-test` | Run the reporter's unit tests |

## Backups

The `backup` service (`postgres:17-alpine`) wakes on start and then every hour, and runs `pg_dump --clean --if-exists` of `claude_usage` only when the newest `./backups/claude_usage-*.sql.gz` is missing or older than 24 hours, writing `./backups/claude_usage-YYYYMMDD-HHMMSS.sql.gz` (UTC). A machine that reboots daily still gets a dump each day, and a failed dump is retried the next hour. A dump is written to a `.tmp` file and renamed only when `pg_dump` succeeds, so a failed run leaves no partial file. Files are created with `umask 077` (owner-only). Dumps older than 14 days are deleted only after a successful dump. It uses `DB_PASSWORD` from `.env` as `PGPASSWORD`. `backups/` is ignored by git and by the Docker build.

Check it with `docker compose logs backup`. Take a dump now with:

```bash
make backup
```

Restore one (this replaces every table in `claude_usage`; the restore runs in one transaction, so a failure leaves the database unchanged):

```bash
make restore FILE=backups/claude_usage-20261102-020000.sql.gz RESTORE_CONFIRM=yes
```

Without `RESTORE_CONFIRM=yes` the target only prints what it would do. The dumps live on the same machine as the database; copy `./backups` somewhere else to survive a disk failure.

## Daily Report

The `reporter` service (Playwright on Chromium, `reporter/`) sends the dashboard's **Export image** PNG to webhooks once a day. At `REPORT_TIME` in `REPORT_TZ` it signs in to the API as `REPORT_EMAIL` / `REPORT_PASSWORD` (use a dashboard admin so every member is in the report), opens the overview inside the Docker network, clicks **Export image** and posts the file to every URL in `REPORT_WEBHOOKS`. It sends at most once per day, retries a failed day up to 3 times 10 minutes apart, never sends for a day it started after `REPORT_TIME`, and stays idle when `REPORT_WEBHOOKS` is empty. Logs show only the host of each webhook (`docker compose logs reporter`).

| Webhook | How to set it | What is sent |
|---------|---------------|--------------|
| Discord | `https://discord.com/api/webhooks/<id>/<token>` (channel → Integrations → Webhooks) | `payload_json` with the message as `content`, the PNG as `files[0]` |
| Telegram | `https://api.telegram.org/bot<token>/sendPhoto?chat_id=<chat id>` | `caption` and `photo` |
| Anything else | the URL | multipart `message` and `file` |
| WhatsApp | through a gateway such as Fonnte, as a JSON entry with `headers` and `fields` (see `.env.example`) | the gateway's fields plus `message` and `file` |

A JSON entry can also set `kind` (`discord`, `telegram`, `generic`), `file_field` and `message_field`. Webhook URLs carry their own tokens, so they live only in `.env`. Try a setup without waiting for the schedule:

```bash
make report-now
```

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
| `20261030000005_audit_logs` | audit logs table from the skeleton (dropped by `20261102064604`) |
| `20261030000006_usage` | `usage.sessions`, `usage.session_hours` |
| `20261102011256_api_key_secret_encrypted` | `api_keys.secret_encrypted` (full key, AES-256-GCM, for reveal) |
| `20261102014445_users_must_change_password` | `users.must_change_password`, set for the seeded `admin` |
| `20261102015038_session_hours_output_subagent` | `session_hours.output_tokens`, `subagent_requests`, `subagent_output_tokens` |
| `20261102033939_budgets` | `usage.budgets` (monthly new-context budget per member) |
| `20261102043258_app_version_billed_tokens` | `api_keys.app_version`, `session_hours.input_tokens`, `cache_write_tokens`, `cache_read_tokens` |
| `20261102064604_drop_audit_logs` | drops the unused `core.audit_logs` |
| `20261103022955_session_overhead` | nullable `sessions.overhead_system_tokens`, `overhead_memory_tokens`, `overhead_agents_tokens`, `overhead_skills_tokens` |
| `20261104010000_tool_activity_plan_limits` | new tables `usage.session_tool_hours` (calls, lines and commits per session, hour and tool) and `usage.plan_limits` (latest 5-hour and 7-day usage per member) |
| `20261111010000_plan_history_accounts` | new tables `usage.plan_limit_readings` (every plan limit reading, seeded from `usage.plan_limits`) and `usage.claude_accounts` (Claude account email, plan and tier per member) |

## API

See [docs/api-contract](docs/api-contract/README.md). The ingest, report and admin endpoints are in [docs/api-contract/usage/README.md](docs/api-contract/usage/README.md).
