# Authentication API Contract

**Version:** v1
**Base URL:** `/core/v1/auth`
**Module:** Core Authentication
**Last Updated:** 2026-11-02

---

## Overview

Sign-in for the dashboard. JWT access token plus a rotating refresh token. There is no sign-up: an admin creates users (see [usage/README.md](../usage/README.md#8-create-user)). The desktop app never signs in; it uses an API key.

**Key Features:**
- JWT access token (default 24h) and opaque refresh token (default 7 days) with rotation
- Multi-device sessions, logout from one or all devices
- Account lockout after `MAX_LOGIN_ATTEMPTS` failed sign-ins (default 5) for `ACCOUNT_LOCKOUT_DURATION` (default 30m)
- Deactivated or deleted users cannot sign in or refresh, and their access tokens stop working on the next request
- `user.must_change_password` tells the dashboard to force a password change (new members, admin password resets, the seeded admin)

---

## Response Format

```json
{
  "data": { ... },
  "message": "Success message"
}
```

Error:

```json
{
  "data": null,
  "message": "Error message",
  "errors": { "detail": ["detail string"] }
}
```

`errors` is omitted when there is no detail.

---

## JWT Claims Structure

```json
{
  "user_id": "b61e8255-5aef-42a3-8fdc-8c191295af10",
  "email": "admin@claude-usage.local",
  "username": "admin",
  "full_name": "Administrator",
  "is_super_admin": true,
  "roles": ["super_admin"],
  "iss": "claude-usage-server",
  "sub": "b61e8255-5aef-42a3-8fdc-8c191295af10",
  "exp": 1793420338,
  "nbf": 1793333938,
  "iat": 1793333938
}
```

| Claim | Type | Notes |
|-------|------|-------|
| `user_id` | string (UUID) | Also in `sub` |
| `email`, `username`, `full_name` | string | Profile at sign-in time |
| `is_super_admin` | bool | `true` for the `super_admin` role: sees everyone's usage, can use `/core/v1/admin` |
| `roles` | string[] | Role codes: `super_admin` or `member` |
| `exp`, `iat`, `nbf` | int64 | Unix timestamps |

Permissions are not in the JWT. They are returned by sign-in and `GET /me`.

---

## Endpoints

### 1. Sign In

```
POST /core/v1/auth/signin
```

**Auth:** None

**Request:**
```json
{
  "email": "admin@claude-usage.local",
  "password": "<SEED_ADMIN_PASSWORD>"
}
```

| Field | Type | Required | Notes |
|-------|------|----------|-------|
| `email` | string | Yes | Email or username |
| `password` | string | Yes | Min 8 characters |

**Response (200):**
```json
{
  "data": {
    "access_token": "eyJhbGci...",
    "refresh_token": "903eedd70b88496a48f9107cbd6450cb9b2aa112a93efacf14e4555efb81d379",
    "token_type": "Bearer",
    "expires_in": 86400,
    "user": {
      "id": "d6cb9822-0d8e-4361-9e47-97ef72633428",
      "email": "mia@claude-usage.local",
      "username": "mia",
      "full_name": "Mia",
      "must_change_password": true
    },
    "roles": ["member"],
    "permissions": ["usage:read"]
  },
  "message": "Sign in successful"
}
```

| Field | Notes |
|-------|-------|
| `access_token` | Send as `Authorization: Bearer <token>` |
| `refresh_token` | Store securely; exchange at `/refresh` |
| `expires_in` | Access token lifetime in seconds |
| `user.must_change_password` | `true` for a member an admin created or whose password an admin reset, and for the seeded admin. Cleared by [6. Change Password](#6-change-password) (or `PUT /core/v1/users/me/password`). The server only reports it; the dashboard sends the user to change their password |
| `roles` | Role codes |
| `permissions` | Flat permission list. Empty or `null` for `super_admin`, who bypasses permission checks |

**Errors:**

| Status | Message | When |
|--------|---------|------|
| 400 | Invalid request payload | Missing field, password shorter than 8 |
| 401 | Invalid credentials | Wrong email/username or password |
| 401 | User account is not active | Deactivated by an admin |
| 401 | User account is locked. Try again later. | Too many failed attempts |

---

### 2. Refresh Token

Exchanges a refresh token for a new access token and a new refresh token. The old refresh token is revoked.

```
POST /core/v1/auth/refresh
```

**Auth:** None

**Request:**
```json
{ "refresh_token": "903eedd70b88496a48f9107cbd6450cb9b2aa112a93efacf14e4555efb81d379" }
```

**Response (200):**
```json
{
  "data": {
    "access_token": "eyJhbGci...",
    "refresh_token": "5c1f0d...",
    "token_type": "Bearer",
    "expires_in": 86400
  },
  "message": "Token refreshed successfully"
}
```

**Errors:**

| Status | Message |
|--------|---------|
| 400 | Invalid request payload |
| 401 | Invalid refresh token |
| 401 | Refresh token expired |
| 401 | Refresh token revoked |
| 401 | User account is not active |

---

### 3. Logout (Single Device)

```
POST /core/v1/auth/logout
```

**Auth:** JWT

**Request:**
```json
{ "refresh_token": "903eedd70b88496a48f9107cbd6450cb9b2aa112a93efacf14e4555efb81d379" }
```

**Response (200):**
```json
{ "data": null, "message": "Logged out successfully" }
```

---

### 4. Logout All Devices

Revokes every refresh token of the caller.

```
POST /core/v1/auth/logout-all
```

**Auth:** JWT

**Response (200):**
```json
{ "data": null, "message": "Logged out from all devices successfully" }
```

---

### 5. Get Me

Current profile with live roles and permissions.

```
GET /core/v1/auth/me
```

**Auth:** JWT

**Response (200):**
```json
{
  "data": {
    "user": {
      "id": "b61e8255-5aef-42a3-8fdc-8c191295af10",
      "email": "admin@claude-usage.local",
      "username": "admin",
      "full_name": "Administrator",
      "must_change_password": false
    },
    "roles": ["super_admin"],
    "permissions": [],
    "is_super_admin": true
  },
  "message": "Profile retrieved successfully"
}
```

`user.must_change_password` is read live from the database, as in sign-in.

**Errors:** 401 `User account is not active`.

---

### 6. Change Password

```
POST /core/v1/auth/change-password
```

**Auth:** JWT

**Request:**
```json
{
  "current_password": "<SEED_ADMIN_PASSWORD>",
  "new_password": "NewPass123!"
}
```

`new_password` is min 8 characters. A successful change sets `must_change_password` to `false`.

**Response (200):**
```json
{ "data": null, "message": "Password changed successfully" }
```

**Errors:** 400 `Current password is incorrect`, 401 `User account is not active`.

---

## Authentication Flow

1. `POST /signin` → access token + refresh token.
2. Call APIs with `Authorization: Bearer <access_token>`.
3. On 401, `POST /refresh` once; if that fails, sign in again.
4. `POST /logout` on sign-out.

### Token Expiry

| Token | Default | Env |
|-------|---------|-----|
| Access | 24h | `JWT_EXPIRATION` |
| Refresh | 168h | `JWT_REFRESH_EXPIRATION` |

Every JWT request checks the user in the database. Deactivating or deleting a user blocks sign-in and refresh, and their already issued access tokens get 401 `User account is not active` on the next request.
