# Usage API Contract

**Version:** v1
**Base URLs:** `/usage/v1` (ingest and reports), `/core/v1/admin` (user and API key administration)
**Module:** Usage
**Last Updated:** 2026-11-02

---

## Overview

The Claude Usage Monitor desktop app sends each person's Claude Code usage every 30 minutes. The server stores it in `usage.sessions` and `usage.session_hours`, and the dashboard reads it back.

- **Ingest** is authenticated with an API key (`X-API-Key`). The key identifies the person; the app never signs in.
- **Reports** are authenticated with a JWT from `POST /core/v1/auth/signin`. API keys are refused on reports.
- **Admin** routes need a JWT for a user with the `super_admin` role. API keys are refused on admin routes.

**Roles:**

| Role | Code | Sees |
|------|------|------|
| Admin | `super_admin` | Everyone's usage; can manage users and keys |
| Member | `member` | Only their own usage |

**Key Features:**
- Idempotent ingest: every send carries absolute lifetime values that are upserted, so retries, restarts and two copies of the app never double count
- Hourly history per session and model, dated by when the requests happened (backfill lands in the right hours)
- Member scoping on every read: a member's `user_id` parameter is ignored and replaced with their own ID
- Time grouping in any IANA time zone (default `Asia/Jakarta`)

---

## Response Format

Every response uses the shared envelope:

```json
{
  "data": { ... },
  "message": "Overview retrieved"
}
```

Paged lists add `meta.pagination`:

```json
{
  "data": [ ... ],
  "message": "Sessions retrieved",
  "meta": {
    "pagination": { "page": 1, "limit": 25, "total": 1, "total_pages": 1 }
  }
}
```

Errors:

```json
{
  "data": null,
  "message": "Invalid query parameters",
  "errors": { "detail": ["invalid input: unknown tz \"Mars/Base\""] }
}
```

`errors` is omitted when there is no detail.

---

## Authentication

| Route group | Header | Notes |
|-------------|--------|-------|
| `POST /usage/v1/sessions` | `X-API-Key: wiz_live_<key_id>_<secret>` | A JWT is refused with 403 |
| `GET /usage/v1/...` | `Authorization: Bearer <access_token>` | An API key is refused with 403 |
| `/core/v1/admin/...` | `Authorization: Bearer <access_token>` of a `super_admin` | Members get 403, API keys get 403 |

An API key stops working (401) when it is revoked, expired, or its owner is deactivated. A JWT stops working (401 `User account is not active`) on the next request after its user is deactivated or deleted.

Every route with an `:id` returns 404 for an unknown id or one that is not a UUID.

---

## Endpoints

| # | Method | Path | Auth |
|---|--------|------|------|
| 1 | POST | `/usage/v1/sessions` | API key |
| 2 | GET | `/usage/v1/overview` | JWT |
| 3 | GET | `/usage/v1/timeseries` | JWT |
| 4 | GET | `/usage/v1/models` | JWT |
| 5 | GET | `/usage/v1/sessions` | JWT |
| 6 | GET | `/usage/v1/sessions/:id` | JWT |
| 7 | GET | `/usage/v1/people` | JWT, admin |
| 8 | POST | `/core/v1/admin/users` | JWT, admin |
| 9 | GET | `/core/v1/admin/users` | JWT, admin |
| 10 | POST | `/core/v1/admin/users/:id/deactivate` | JWT, admin |
| 11 | POST | `/core/v1/admin/users/:id/activate` | JWT, admin |
| 12 | POST | `/core/v1/admin/users/:id/reset-password` | JWT, admin |
| 13 | POST | `/core/v1/admin/users/:id/api-keys` | JWT, admin |
| 14 | GET | `/core/v1/admin/users/:id/api-keys` | JWT, admin |
| 15 | POST | `/core/v1/admin/api-keys/:id/revoke` | JWT, admin |
| 16 | GET | `/core/v1/admin/api-keys/:id/reveal` | JWT, admin |

---

### 1. Ingest Sessions

Upserts a batch of sessions for the key's owner.

```
POST /usage/v1/sessions
```

**Auth:** `X-API-Key`

**Request:**
```json
{
  "machine": "DIMAS-PC",
  "app_version": "0.2.0",
  "sessions": [
    {
      "session_id": "9101114e-543c-4bb6-a36b-0d702dbc23d0",
      "project": "C:\\Code\\Claude Usage Monitor",
      "title": "Usage server design",
      "model": "claude-opus-5-5",
      "context_window": 1000000,
      "start_context": 21000,
      "latest_context": 180100,
      "peak_context": 210000,
      "new_context": 64000,
      "requests": 412,
      "compactions": 1,
      "first_at": "2026-10-30T02:11:09.123Z",
      "latest_at": "2026-10-30T06:58:40.456Z",
      "hours": [
        { "hour": "2026-10-30T02:00:00Z", "model": "claude-opus-5-5", "new_context": 40000, "requests": 120, "output_tokens": 15000, "subagent_requests": 4, "subagent_output_tokens": 6000 },
        { "hour": "2026-10-30T06:00:00Z", "model": "claude-sonnet-5-5", "new_context": 8000, "requests": 30, "output_tokens": 2500, "subagent_requests": 0, "subagent_output_tokens": 0 }
      ]
    }
  ]
}
```

| Field | Type | Required | Notes |
|-------|------|----------|-------|
| `machine` | string | Yes | Max 255 characters |
| `app_version` | string | No | Informational |
| `sessions` | array | Yes | Max 200 per request |
| `sessions[].session_id` | string | Yes | Claude Code session UUID, 1-100 characters |
| `sessions[].project` | string | No | Working directory |
| `sessions[].title` | string | No | Session title |
| `sessions[].model` | string | Yes | Current model, 1-100 characters |
| `sessions[].context_window` | int | No | Tokens, ≥ 0 |
| `sessions[].start_context` | int | No | First request's context, ≥ 0 |
| `sessions[].latest_context` | int | No | ≥ 0 |
| `sessions[].peak_context` | int | No | ≥ 0 |
| `sessions[].new_context` | int | No | Lifetime growth, ≥ 0 |
| `sessions[].requests` | int | No | ≥ 0 |
| `sessions[].compactions` | int | No | Lifetime compactions, ≥ 0 |
| `sessions[].first_at` | RFC 3339 | Yes | Must not be after `latest_at` |
| `sessions[].latest_at` | RFC 3339 | Yes | The app's watermark compares against this |
| `sessions[].hours` | array | No | Max 2,000 rows per session |
| `hours[].hour` | RFC 3339 | Yes | A whole UTC hour |
| `hours[].model` | string | Yes | 1-100 characters |
| `hours[].new_context` | int | No | Growth in that hour for that model, ≥ 0 |
| `hours[].requests` | int | No | ≥ 0 |
| `hours[].output_tokens` | int | No | Output tokens in that hour for that model, ≥ 0. Missing = 0 |
| `hours[].subagent_requests` | int | No | Subagent (sidechain) requests in that hour for that model, ≥ 0. Missing = 0 |
| `hours[].subagent_output_tokens` | int | No | Output tokens of those subagent requests, ≥ 0. Missing = 0 |

The body may be at most 64 MB.

**Behaviour:**
- Values are **absolute**. The session row and each `(session_id, hour, model)` row are replaced, never added to. Sending the same payload twice leaves the totals unchanged.
- Each session is saved in its own transaction (session row plus hour rows).
- A session that fails validation, or whose `session_id` already belongs to another user, is listed in `rejected`. The rest of the payload is still saved and the other user's data is untouched.
- `api_keys.last_used_at` is updated on every call. Reports show it as `last_synced_at`.

**Response (200):**
```json
{
  "data": {
    "accepted": 1,
    "rejected": []
  },
  "message": "Usage saved"
}
```

With a rejected session:
```json
{
  "data": {
    "accepted": 0,
    "rejected": [
      { "session_id": "9101114e-543c-4bb6-a36b-0d702dbc23d0", "reason": "session belongs to another user" }
    ]
  },
  "message": "Usage saved"
}
```

Rejection reasons: `session_id must be 1-100 characters`, `model must be 1-100 characters`, `numbers must be >= 0`, `first_at and latest_at are required`, `first_at must not be after latest_at`, `at most 2000 hours per session`, `hour must be a whole UTC hour` and `session belongs to another user`.

**Errors:**

| Status | Message | When |
|--------|---------|------|
| 400 | Invalid request payload | Bad JSON, missing `machine`, or more than 200 sessions |
| 401 | Unauthorized | No `X-API-Key` and no JWT |
| 401 | Invalid API key | Malformed, unknown, revoked or expired key, or owner deactivated. The reason is only logged on the server |
| 403 | Forbidden | Called with a JWT instead of an API key |
| 413 | Request body too large | Body over 64 MB |
| 500 | Internal server error | Database failure. Nothing is reported as accepted; the app keeps its watermark and resends |

---

## Report Parameters

Reports 2-5 accept these query parameters:

| Param | Type | Default | Notes |
|-------|------|---------|-------|
| `from` | RFC 3339 | `to` minus 7 days | Inclusive |
| `to` | RFC 3339 | now | Exclusive; `from` must be before `to` |
| `tz` | IANA name | `Asia/Jakarta` | Used for day buckets. Unknown names return 400 |
| `user_id` | UUID | none (everyone) | **Admins only.** For members it is ignored and forced to their own ID |

Ranges filter on the UTC hour of each request (`session_hours.hour`).

Write `from` and `to` in UTC with a `Z` suffix (`2026-10-29T00:00:00Z`). An offset such as `+07:00` must be URL-encoded as `%2B07:00`; a raw `+` in a query string is read as a space and the request fails with 400.

---

### 2. Overview

Per-person totals for the range, plus team totals. Every user is listed, active or not, including people with no usage, so the dashboard can show who isn't syncing.

```
GET /usage/v1/overview?from=2026-10-29T00:00:00Z&to=2026-10-31T00:00:00Z&tz=Asia/Jakarta
```

**Auth:** JWT

**Response (200):**
```json
{
  "data": {
    "people": [
      {
        "user_id": "d6cb9822-0d8e-4361-9e47-97ef72633428",
        "full_name": "Mia",
        "email": "mia@claude-usage.local",
        "current_model": "claude-opus-5-5",
        "new_context": 48000,
        "requests": 150,
        "sessions": 1,
        "output_tokens": 17500,
        "subagent_requests": 4,
        "subagent_output_tokens": 6000,
        "last_request_at": "2026-10-30T06:58:40.456Z",
        "last_synced_at": "2026-10-30T04:18:58.36942Z"
      },
      {
        "user_id": "b61e8255-5aef-42a3-8fdc-8c191295af10",
        "full_name": "Administrator",
        "email": "admin@claude-usage.local",
        "current_model": "",
        "new_context": 0,
        "requests": 0,
        "sessions": 0,
        "output_tokens": 0,
        "subagent_requests": 0,
        "subagent_output_tokens": 0,
        "last_request_at": null,
        "last_synced_at": null
      }
    ],
    "total_new_context": 48000,
    "total_requests": 150,
    "total_sessions": 1,
    "active_people": 1,
    "total_output_tokens": 17500,
    "total_subagent_requests": 4,
    "total_subagent_output_tokens": 6000
  },
  "message": "Overview retrieved"
}
```

| Field | Notes |
|-------|-------|
| `new_context`, `requests`, `output_tokens`, `subagent_requests`, `subagent_output_tokens` | Sums of the hour rows in the range |
| `total_*` | Sums over `people` |
| `sessions` | Sessions with at least one hour row in the range |
| `current_model`, `last_request_at` | From the person's most recent session overall |
| `last_synced_at` | Latest `last_used_at` across the person's API keys |
| `active_people` | People with `new_context > 0` in the range |

A member gets a single entry: themselves.

---

### 3. Timeseries

Buckets of `SUM(new_context)`, `SUM(requests)`, `SUM(output_tokens)`, `SUM(subagent_requests)` and `SUM(subagent_output_tokens)`.

```
GET /usage/v1/timeseries?interval=day&group_by=model&from=...&to=...&tz=Asia/Jakarta
```

**Auth:** JWT

| Param | Values | Default |
|-------|--------|---------|
| `interval` | `hour`, `day` | `day` |
| `group_by` | `user`, `model` | `user` |

**Response (200):**
```json
{
  "data": [
    { "bucket": "2026-10-30T00:00:00+07:00", "key": "claude-opus-5-5", "new_context": 40000, "requests": 120, "output_tokens": 15000, "subagent_requests": 4, "subagent_output_tokens": 6000 },
    { "bucket": "2026-10-30T00:00:00+07:00", "key": "claude-sonnet-5-5", "new_context": 8000, "requests": 30, "output_tokens": 2500, "subagent_requests": 0, "subagent_output_tokens": 0 }
  ],
  "message": "Timeseries retrieved"
}
```

- `bucket` is the start of the hour or day in `tz`, written with its UTC offset. Usage at 23:30 UTC lands on the next day for `Asia/Jakarta`.
- `key` is the user ID for `group_by=user`, or the model for `group_by=model`.
- Only buckets with data are returned.

---

### 4. Models

Per-model totals.

```
GET /usage/v1/models?from=...&to=...
```

**Auth:** JWT

**Response (200):**
```json
{
  "data": [
    { "model": "claude-opus-5-5", "new_context": 40000, "requests": 120, "sessions": 1, "output_tokens": 15000, "subagent_requests": 4, "subagent_output_tokens": 6000 },
    { "model": "claude-sonnet-5-5", "new_context": 8000, "requests": 30, "sessions": 1, "output_tokens": 2500, "subagent_requests": 0, "subagent_output_tokens": 0 }
  ],
  "message": "Models retrieved"
}
```

---

### 5. Session List

Paged `usage.sessions` rows with the owner's name. Only sessions with activity inside the range are listed.

```
GET /usage/v1/sessions?from=...&to=...&project=&model=&q=&page=1&per_page=25&sort=-last_request_at
```

**Auth:** JWT

| Param | Notes |
|-------|-------|
| `project` | Exact match |
| `model` | Exact match on any model the session used: its current model or the model of any of its hour rows |
| `q` | Case-insensitive substring of `title` or `project` |
| `page` | ≥ 1, default 1 |
| `per_page` | 1-100, default 25 |
| `sort` | `last_request_at`, `-last_request_at` (default), `new_context`, `-new_context` |

**Response (200):**
```json
{
  "data": [
    {
      "id": "9101114e-543c-4bb6-a36b-0d702dbc23d0",
      "user_id": "d6cb9822-0d8e-4361-9e47-97ef72633428",
      "machine": "DIMAS-PC",
      "project": "C:\\Code\\Claude Usage Monitor",
      "title": "Usage server design",
      "model": "claude-opus-5-5",
      "context_window": 1000000,
      "start_context": 21000,
      "latest_context": 180100,
      "peak_context": 210000,
      "new_context": 64000,
      "requests": 412,
      "compactions": 1,
      "first_request_at": "2026-10-30T02:11:09.123Z",
      "last_request_at": "2026-10-30T06:58:40.456Z",
      "last_reported_at": "2026-10-30T04:18:58.372578Z",
      "user_name": "Mia",
      "models": ["claude-opus-5-5", "claude-sonnet-5-5"],
      "output_tokens": 17500,
      "subagent_requests": 4,
      "subagent_output_tokens": 6000
    }
  ],
  "message": "Sessions retrieved",
  "meta": {
    "pagination": { "page": 1, "limit": 25, "total": 1, "total_pages": 1 }
  }
}
```

`new_context`, `output_tokens`, `subagent_requests` and `subagent_output_tokens` here are sums of the session's hour rows with `hour` in `[from, to)`, and sorting by `new_context` uses that value. A session is listed when, in the range, `new_context > 0`, `output_tokens > 0` or `subagent_requests > 0`. `requests` and the context fields are the session's lifetime values. `GET /usage/v1/sessions/:id` returns the lifetime `new_context`.

`model` is the session's current model. `models` lists every model the session used: the distinct models of its hour rows, ordered by their summed `new_context` (largest first), then by name. A session with no hour rows gets `[model]`.

---

### 6. Session Detail

The session plus all its hour rows.

```
GET /usage/v1/sessions/:id
```

**Auth:** JWT. A member asking for someone else's session gets 404.

**Response (200):**
```json
{
  "data": {
    "session": {
      "id": "9101114e-543c-4bb6-a36b-0d702dbc23d0",
      "user_id": "d6cb9822-0d8e-4361-9e47-97ef72633428",
      "machine": "DIMAS-PC",
      "project": "C:\\Code\\Claude Usage Monitor",
      "title": "Usage server design",
      "model": "claude-opus-5-5",
      "context_window": 1000000,
      "start_context": 21000,
      "latest_context": 180100,
      "peak_context": 210000,
      "new_context": 64000,
      "requests": 412,
      "compactions": 1,
      "first_request_at": "2026-10-30T02:11:09.123Z",
      "last_request_at": "2026-10-30T06:58:40.456Z",
      "last_reported_at": "2026-10-30T04:18:58.372578Z"
    },
    "user_name": "Mia",
    "models": ["claude-opus-5-5", "claude-sonnet-5-5"],
    "hours": [
      { "hour": "2026-10-30T02:00:00Z", "model": "claude-opus-5-5", "new_context": 40000, "requests": 120, "output_tokens": 15000, "subagent_requests": 4, "subagent_output_tokens": 6000 },
      { "hour": "2026-10-30T06:00:00Z", "model": "claude-sonnet-5-5", "new_context": 8000, "requests": 30, "output_tokens": 2500, "subagent_requests": 0, "subagent_output_tokens": 0 }
    ]
  },
  "message": "Session retrieved"
}
```

`models` follows the same rules as in the session list.

---

### 7. People

Users for the dashboard's person picker.

```
GET /usage/v1/people
```

**Auth:** JWT, admin. Members get 403 `admins only`.

**Response (200):**
```json
{
  "data": [
    { "id": "b61e8255-5aef-42a3-8fdc-8c191295af10", "full_name": "Administrator", "email": "admin@claude-usage.local", "is_active": true },
    { "id": "d6cb9822-0d8e-4361-9e47-97ef72633428", "full_name": "Mia", "email": "mia@claude-usage.local", "is_active": true }
  ],
  "message": "People retrieved"
}
```

**Report errors (2-7):**

| Status | Message | When |
|--------|---------|------|
| 400 | Invalid query parameters | Bad `from`/`to`, `from >= to`, unknown `tz`, non-UUID `user_id`, bad `interval`/`group_by`/`sort`/`page`/`per_page` |
| 401 | Unauthorized | Missing or expired JWT |
| 403 | Forbidden | Called with an API key, or a member calling `/people` |
| 404 | Session not found | Unknown session, or another user's session for a member |

---

## Admin Endpoints

All under `/core/v1/admin`, JWT of a `super_admin`.

### 8. Create User

Creates a user with the `member` role and `must_change_password = true`, so the dashboard asks them to pick their own password at first sign-in.

```
POST /core/v1/admin/users
```

**Request:**
```json
{
  "email": "mia@claude-usage.local",
  "username": "mia",
  "full_name": "Mia",
  "password": "MiaPass123!"
}
```

| Field | Type | Required | Notes |
|-------|------|----------|-------|
| `email` | string | Yes | Valid email, unique |
| `username` | string | Yes | 3-100 characters, unique |
| `full_name` | string | No | Max 255 |
| `password` | string | Yes | Min 8 characters |
| `phone` | string | No | Max 20 |

**Response (201):**
```json
{
  "data": {
    "id": "d6cb9822-0d8e-4361-9e47-97ef72633428",
    "email": "mia@claude-usage.local",
    "username": "mia",
    "full_name": "Mia",
    "is_active": true,
    "is_email_verified": true,
    "created_at": "2026-10-30T04:18:58.3168889Z",
    "updated_at": "2026-10-30T04:18:58.3168889Z"
  },
  "message": "User created successfully"
}
```

**Errors:** 400 invalid payload, 409 `Email already exists` / `Username already exists`.

---

### 9. List Users

```
GET /core/v1/admin/users?page=1&limit=10&search=&is_active=true
```

| Param | Notes |
|-------|-------|
| `page` | Default 1 |
| `limit` | 1-1000, default 10 |
| `search` | Substring of email, username or full name |
| `is_active` | `true` or `false` |

**Response (200):**
```json
{
  "data": [
    {
      "id": "d6cb9822-0d8e-4361-9e47-97ef72633428",
      "email": "mia@claude-usage.local",
      "username": "mia",
      "full_name": "Mia",
      "is_active": true,
      "is_email_verified": true,
      "roles": ["member"],
      "created_at": "2026-10-30T04:18:58.316888Z",
      "updated_at": "2026-10-30T04:18:58.316888Z"
    },
    {
      "id": "b61e8255-5aef-42a3-8fdc-8c191295af10",
      "email": "admin@claude-usage.local",
      "username": "admin",
      "full_name": "Administrator",
      "is_active": true,
      "is_email_verified": true,
      "roles": ["super_admin"],
      "last_login_at": "2026-10-30T04:18:58.217348Z",
      "created_at": "2026-10-30T04:18:14.090187Z",
      "updated_at": "2026-10-30T04:18:14.090187Z"
    }
  ],
  "message": "Users retrieved successfully",
  "meta": {
    "pagination": { "page": 1, "limit": 10, "total": 2, "total_pages": 1 }
  }
}
```

---

### 10. Deactivate User

Sets `is_active = false` and revokes all of the user's active API keys with reason `user deactivated`.

```
POST /core/v1/admin/users/:id/deactivate
```

**Response (200):**
```json
{ "data": null, "message": "User deactivated successfully" }
```

After this, the user's keys return 401 on ingest, and sign-in, token refresh and any access token issued before deactivation return 401 `User account is not active`.

**Errors:** 400 `Cannot deactivate yourself`, 404 `User not found`.

---

### 11. Activate User

```
POST /core/v1/admin/users/:id/activate
```

**Response (200):**
```json
{ "data": null, "message": "User activated successfully" }
```

Keys revoked on deactivation stay revoked; issue a new key.

**Errors:** 404 `User not found`.

---

### 12. Reset Password

```
POST /core/v1/admin/users/:id/reset-password
```

**Request:**
```json
{ "password": "Reset1234!" }
```

`password` is required, min 8 characters. Sets `must_change_password = true` and revokes the user's refresh tokens.

**Response (200):**
```json
{ "data": null, "message": "Password reset successfully" }
```

**Errors:** 400 invalid payload, 404 `User not found`.

---

### 13. Issue API Key for a User

Creates a key owned by the user. Validation uses only the SHA-256 hash of the secret; the full key is also stored AES-256-GCM encrypted so an admin can copy it later with [16. Reveal an API Key](#16-reveal-an-api-key).

```
POST /core/v1/admin/users/:id/api-keys
```

**Request:**
```json
{ "name": "DIMAS-PC" }
```

`name` is required, 1-100 characters.

**Response (201):**
```json
{
  "data": {
    "id": "69e4e903-5425-42d2-9994-bb467324dce7",
    "key": "wiz_live_<key_id>_<secret>",
    "key_prefix": "wiz_live_htg97w15_ea073062...",
    "name": "DIMAS-PC",
    "message": "API key created successfully. Save this key securely - it will not be shown again."
  },
  "message": "API key created successfully"
}
```

Paste `key` into the desktop app's Settings → API key.

**Errors:** 400 invalid payload, 404 `User not found or inactive`.

---

### 14. List a User's API Keys

```
GET /core/v1/admin/users/:id/api-keys?page=1&limit=20&is_active=true
```

**Response (200):**
```json
{
  "data": {
    "api_keys": [
      {
        "id": "69e4e903-5425-42d2-9994-bb467324dce7",
        "key_prefix": "wiz_live_htg97w15_ea073062...",
        "name": "DIMAS-PC",
        "environment": "live",
        "scoped_permissions": null,
        "rate_limit": 1000,
        "rate_limit_window": 3600,
        "last_used_at": "2026-10-30T04:18:58.500147Z",
        "last_used_ip": "127.0.0.1",
        "total_requests": 3,
        "is_active": true,
        "created_at": "2026-10-30T04:18:58.340749Z",
        "updated_at": "2026-10-30T04:18:58.340749Z"
      }
    ],
    "total": 1,
    "page": 1,
    "limit": 20
  },
  "message": "API keys retrieved successfully"
}
```

Revoked keys have `is_active: false` and a `revoked_at` timestamp.

**Errors:** 404 `User not found` (unknown user or not a UUID).

---

### 15. Revoke an API Key

Revokes any user's key. Revoking an already revoked key succeeds and changes nothing.

```
POST /core/v1/admin/api-keys/:id/revoke
```

**Request (optional body):**
```json
{ "reason": "lost laptop" }
```

**Response (200):**
```json
{ "data": null, "message": "API key revoked successfully" }
```

**Errors:** 404 `API key not found`.

---

### 16. Reveal an API Key

Returns the full key of an active key so an admin can copy it again. Admin JWT only: an API key in `X-API-Key` gets 403. The response carries `Cache-Control: no-store`; the key is never logged.

```
GET /core/v1/admin/api-keys/:id/reveal
```

**Response (200):**
```json
{ "data": { "key": "wiz_live_<key_id>_<secret>" }, "message": "API key revealed" }
```

**Errors:**

| Status | Message | When |
|--------|---------|------|
| 403 | `Forbidden` | Not `super_admin`, or called with an API key |
| 404 | `API key not found` | Unknown id or not a UUID |
| 409 | `key was issued before it could be revealed; issue a new key` | Key issued before reveal existed (no encrypted copy), or sealed with a rotated `API_KEY_ENCRYPTION_KEY` |
| 410 | `key is not active` | Key revoked, expired or deleted |

Rotating `API_KEY_ENCRYPTION_KEY` makes existing keys un-revealable (409); they keep authenticating. Issue a new key to get a copyable one.

---

## Self-Service API Keys

Any signed-in user can manage their own keys under `/core/v1/api-keys` (JWT):

| Method | Path | Notes |
|--------|------|-------|
| POST | `/core/v1/api-keys` | Body `{ "name": "..." }`; returns the full key once |
| GET | `/core/v1/api-keys` | Own keys only |
| GET | `/core/v1/api-keys/:id` | Own key only |
| PATCH | `/core/v1/api-keys/:id` | Rename or change limits |
| DELETE | `/core/v1/api-keys/:id` | Revoke; optional body `{ "reason": "..." }` |

---

## Typical Flow

1. Admin signs in: `POST /core/v1/auth/signin` with `{ "email": "admin@claude-usage.local", "password": "<SEED_ADMIN_PASSWORD>" }`.
2. Admin creates a member: `POST /core/v1/admin/users`.
3. Admin issues a key: `POST /core/v1/admin/users/:id/api-keys`, and gives the `key` to the member.
4. The member's desktop app sends `POST /usage/v1/sessions` with `X-API-Key` every 30 minutes.
5. The dashboard signs in and calls the report endpoints with `tz` set to the browser's time zone.
