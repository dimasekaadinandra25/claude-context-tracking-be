# Claude Usage Server API Contract

**Last Updated:** 2026-10-30

---

## Documentation Structure

```
docs/api-contract/
├── README.md (this file)
├── core/
│   ├── auth.md
│   ├── user.md
│   ├── role.md
│   └── audit-logs.md
└── usage/
    └── README.md
```

---

## Core Module

Sign-in, the signed-in user's profile, roles, API keys and audit logs.

**Base Path:** `/core/v1`

| File | Endpoints | Description |
|------|-----------|-------------|
| [auth.md](core/auth.md) | 6 | Sign in, refresh, logout, me, change password |
| [user.md](core/user.md) | 4 | Own profile and password, user by ID |
| [role.md](core/role.md) | 6 | Role CRUD and permissions |
| [audit-logs.md](core/audit-logs.md) | 1 | Audit log list (`super_admin`) |

---

## Usage Module

Ingest from the desktop app, dashboard reports, and admin management of users and API keys.

**Base Paths:** `/usage/v1`, `/core/v1/admin`

| File | Endpoints | Description |
|------|-----------|-------------|
| [usage/README.md](usage/README.md) | 15 + 5 self-service key routes | Ingest, overview, timeseries, models, sessions, people, admin users and keys |

---

## Conventions

### Base URL

```
http://127.0.0.1:8080
```

The server binds to localhost only. CORS allows `http://localhost:5173` and `http://localhost:3000`.

### Authentication

| Caller | Header |
|--------|--------|
| Dashboard (signed-in user) | `Authorization: Bearer <access_token>` |
| Desktop app (ingest only) | `X-API-Key: wiz_live_<key_id>_<secret>` |

### Response Envelope

```json
{
  "data": { ... },
  "message": "Human readable message",
  "meta": { "pagination": { "page": 1, "limit": 25, "total": 1, "total_pages": 1 } },
  "errors": { "detail": ["..."] }
}
```

`meta` appears only on paged lists, `errors` only on failures with detail.

### Roles

| Code | Meaning |
|------|---------|
| `super_admin` | Admin. Sees everyone, manages users and keys, bypasses permission checks |
| `member` | Sees only their own usage. Permissions `{"usage":"viewer"}` |

### Time

All timestamps are RFC 3339. Storage is UTC. Report endpoints take `tz` (IANA name, default `Asia/Jakarta`) for day buckets.

### Health Check

```
GET /health
```

```json
{ "status": "ok", "message": "Claude Usage API is running" }
```
