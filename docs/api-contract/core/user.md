# User API Contract

**Version:** v1
**Base URL:** `/core/v1/users`
**Module:** Core User
**Last Updated:** 2026-10-30

---

## Overview

Self-service profile endpoints for any signed-in user. Creating users, listing them, deactivating and resetting passwords are admin endpoints under `/core/v1/admin/users`, documented in [usage/README.md](../usage/README.md#admin-endpoints).

Response envelope: see [auth.md](auth.md#response-format).

---

## User Object

```json
{
  "id": "d6cb9822-0d8e-4361-9e47-97ef72633428",
  "email": "mia@claude-usage.local",
  "username": "mia",
  "full_name": "Mia",
  "phone": "0812...",
  "avatar_url": "https://...",
  "is_active": true,
  "is_email_verified": true,
  "last_login_at": "2026-10-30T04:18:58.217348Z",
  "created_at": "2026-10-30T04:18:58.316888Z",
  "updated_at": "2026-10-30T04:18:58.316888Z"
}
```

`phone`, `avatar_url` and `last_login_at` are omitted when empty. The admin list adds `roles`.

---

## Endpoints

### 1. Get Current User

```
GET /core/v1/users/me
```

**Auth:** JWT

**Response (200):** the user object, message `User retrieved successfully`.

---

### 2. Update Current User

```
PUT /core/v1/users/me
```

**Auth:** JWT

**Request:**
```json
{
  "full_name": "Mia Putri",
  "phone": "08123456789",
  "avatar_url": "https://example.com/mia.png"
}
```

All fields optional. `full_name` max 255, `phone` max 20, `avatar_url` a URL max 500.

**Response (200):** the updated user object, message `User updated successfully`.

---

### 3. Change Current User's Password

```
PUT /core/v1/users/me/password
```

**Auth:** JWT

**Request:**
```json
{
  "current_password": "MiaPass123!",
  "new_password": "NewPass123!"
}
```

**Response (200):**
```json
{ "data": null, "message": "Password changed successfully" }
```

**Errors:** 400 `Current password is incorrect`.

---

### 4. Get User by ID

```
GET /core/v1/users/:id
```

**Auth:** JWT with permission `user-management:read` (in practice, `super_admin`).

**Response (200):** the user object.

**Errors:** 403 Forbidden, 404 `User not found`.
