# Deployment Files

v1 runs on one PC: Postgres in Docker (`docker-compose.yml` at the project root) and the API with `make run`. There is no Kubernetes or CI setup.

## Structure

```
deployments/
└── docker/
    └── docker-entrypoint.sh   # container entrypoint used by the root Dockerfile
```

## Dockerfile

The root `Dockerfile` builds the API into an Alpine image with `migrate` and `psql` installed. It is not used for local development.

## docker-entrypoint.sh

Runs when the container starts:

- Waits up to 5 minutes for Postgres (`DB_HOST`, `DB_PORT`, `DB_USER`, `DB_PASSWORD`, `DB_NAME`).
- Runs the core migrations, tracked in `schema_migrations_core`.
- Starts the command passed to the container.

Set `SKIP_MIGRATION=true` to skip the wait and the migrations.

Seeders are not run by the entrypoint; run `make seed` against the database once.

## Environment Variables

See `.env.example`. The ones the server needs:

| Variable | Notes |
|----------|-------|
| `DB_HOST`, `DB_PORT`, `DB_USER`, `DB_PASSWORD`, `DB_NAME`, `DB_SSLMODE` | Postgres connection |
| `JWT_SECRET` | At least 32 characters |
| `JWT_EXPIRATION`, `JWT_REFRESH_EXPIRATION` | Default `24h`, `168h` |
| `SERVER_HOST`, `SERVER_PORT` | Default `127.0.0.1`, `8080` |
| `TZ` | Must be `UTC` |
