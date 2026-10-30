# Audit Logs API Contract

**Version:** v1
**Base URL:** `/core/v1/audit-logs`
**Module:** Core - Audit Logs (global, polymorphic)
**Last Updated:** 2026-04-18
**Status:** ✅ Implemented

---

## Overview

API global untuk mengambil **audit trail** di seluruh aplikasi. Satu endpoint ini melayani semua feature module (finance, core, rental, dst.) melalui discriminator polymorphic `reff_type` + `reff_id`. Setiap mutation di backend (create, update, submit, approve, reject, post, delete, …) mencatat satu row event ke `core.audit_logs` dalam transaction DB yang sama dengan operasi bisnisnya — sehingga audit tidak bisa drift dari state.

**Key Features:**

- **Satu endpoint, semua resource** — FE tidak perlu tahu URL per feature; pakai filter `reff_type` + `reff_id`
- **Append-only** — row tidak pernah di-UPDATE/DELETE; history immutable by convention
- **Actor snapshot** — `actor_id`, `actor_name`, `actor_role` disnapshot saat event; perubahan profil user di kemudian hari tidak mengubah history lama (pola yang sama dengan `core.approval_request_levels.role_name`)
- **Field-level diff** — event `updated` membawa JSONB `changes` dengan pasangan `old`/`new` per field yang berubah
- **Request fingerprint** — `ip_address`, `user_agent`, `request_id` ikut dicatat untuk forensics
- **Multi-tenant** — scope `company_id` otomatis dari JWT; tenant lain tidak bisa membaca history kita
- **Company-wide feed** — tanpa filter `reff_type`, endpoint mengembalikan audit feed seluruh company (admin dashboard)
- **Newest first** — server selalu sort `occurred_at DESC, created_at DESC`

**Base Auth:** Login + CompanyContext (tidak ada permission khusus di v1)

---

## Reff Types Currently Supported

Daftar `reff_type` yang sudah di-emit event-nya oleh backend:

| reff_type | Feature Module | Status |
|---|---|---|
| `finance.cash_transactions` | Cash Book | ✅ Implemented |
| `finance.fund_transfers` | Transfer Dana | ✅ Implemented |
| `finance.journal_entries` | Jurnal Umum | ✅ Implemented |
| `rental.rentals` | Rentals | 🕓 Belum — roadmap |

> Feature module mendapat audit otomatis begitu service layer memanggil `audit.Service.RecordTx(...)`. FE tidak perlu perubahan saat module baru ditambah — `reff_type` string saja sudah cukup.

---

## Response Format

### Standard Response (paginated)

```json
{
  "data": [ ... ],
  "message": "Audit logs retrieved successfully",
  "meta": {
    "pagination": {
      "page": 1,
      "limit": 50,
      "total": 7,
      "total_pages": 1
    }
  }
}
```

### HTTP Status Codes

| Status | Deskripsi |
|---|---|
| 200 OK | Success |
| 400 Bad Request | Query params invalid, atau `reff_id` disertakan tanpa `reff_type` |
| 401 Unauthorized | Belum login |
| 500 Internal Server Error | Server error |

---

## Audit Log Object

```json
{
  "id": "c0ffee00-1234-5678-9abc-def012345678",
  "company_id": "20000000-0000-0000-0000-000000000001",
  "branch_id": "30000000-0000-0000-0000-000000000001",
  "reff_type": "finance.cash_transactions",
  "reff_id": "70000000-0000-0000-0000-000000000030",
  "reff_no": "CI/JKT-202604/0001",
  "action": "updated",
  "summary": "Diperbarui",
  "changes": {
    "total_amount": { "old": 15000000, "new": 17500000 },
    "description": { "old": "Penerimaan invoice ABC", "new": "Penerimaan invoice ABC + DEF" }
  },
  "metadata": {
    "line_count_before": 1,
    "line_count_after": 2,
    "status_at_update": "rejected",
    "linked_journal_entry": null
  },
  "actor_id": "10000000-0000-0000-0000-000000000005",
  "actor_name": "Siti Nurhaliza",
  "actor_role": "finance_manager",
  "ip_address": "10.1.2.34",
  "user_agent": "Mozilla/5.0 (...)",
  "request_id": "req-abc-123",
  "occurred_at": "2026-04-10T09:30:00Z",
  "created_at": "2026-04-10T09:30:00Z"
}
```

### Fields

| Field | Type | Keterangan |
|---|---|---|
| `id` | UUID | ID row audit |
| `company_id` | UUID | Scope company (auto dari JWT) |
| `branch_id` | UUID \| null | Branch entity saat event (diisi kalau entity branch-scoped) |
| `reff_type` | string | Schema-qualified entity type (contoh: `finance.cash_transactions`) |
| `reff_id` | UUID | ID entity |
| `reff_no` | string \| null | Snapshot nomor dokumen saat event (contoh: `CI/JKT-202604/0001`) — hemat join di admin dashboard |
| `action` | enum | Jenis event — lihat **Action Taxonomy** |
| `summary` | string \| null | Deskripsi satu-baris siap render di FE |
| `changes` | object \| null | Field-level diff. Hanya terisi saat `action = "updated"`. Key = nama field, value = `{old, new}` pair |
| `metadata` | object \| null | Konteks tambahan per action — isi tergantung feature & action |
| `actor_id` | UUID \| null | User yang melakukan aksi. Null untuk aksi system-triggered |
| `actor_name` | string \| null | Snapshot `claims.FullName` saat event |
| `actor_role` | string \| null | Snapshot role pertama di `claims.Roles` saat event |
| `ip_address` | string \| null | `c.ClientIP()` saat event |
| `user_agent` | string \| null | Header `User-Agent` saat event |
| `request_id` | string \| null | Header `X-Request-ID` saat event (null kalau FE tidak kirim) |
| `occurred_at` | datetime (ISO-8601 UTC) | Kapan event terjadi (business time) |
| `created_at` | datetime (ISO-8601 UTC) | Kapan row audit ditulis |

> Row audit bersifat **append-only** — tidak ada field `updated_at` / `deleted_at`.

### FieldChange Object

Setiap entry di `changes` memiliki bentuk:

```json
{ "old": <value-lama>, "new": <value-baru> }
```

Nilai `old`/`new` bisa `null` kalau field-nya baru ditambah atau dihapus. Tipe value mengikuti tipe asli field di domain object.

---

## Endpoints

### 1. List Audit Logs (global)

```
GET /core/v1/audit-logs
```

**Auth:** Bearer token + CompanyContext

**Query Parameters:**

| Param | Type | Default | Validasi | Keterangan |
|---|---|---|---|---|
| `reff_type` | string | - | - | Filter per tipe entity (contoh: `finance.cash_transactions`) |
| `reff_id` | UUID | - | uuid | Filter per ID entity. **Wajib disertai `reff_type`**. Kombinasi ini = per-entity history (pola detail page) |
| `actor_id` | UUID | - | uuid | Filter by user yang melakukan aksi (user activity view) |
| `action` | string | - | - | Filter by action (contoh: `approved`, `rejected`) |
| `date_from` | date | - | `YYYY-MM-DD` | Inclusive start, matched against `occurred_at` |
| `date_to` | date | - | `YYYY-MM-DD` | Inclusive end (server convert ke `< next-day`) |
| `page` | int | `1` | `min=1` | Halaman |
| `limit` | int | `50` | `min=1, max=200` | Items per page |

**Sorting:** fix — server selalu order `occurred_at DESC, created_at DESC`.

**Combinations:**

| Use case | Filter |
|---|---|
| **History di halaman detail transaksi** | `reff_type=finance.cash_transactions&reff_id={id}` |
| **Semua transaksi yang disentuh user X** | `actor_id={userId}` |
| **Semua rejection dalam rentang tanggal** | `action=rejected&date_from=2026-04-01&date_to=2026-04-30` |
| **Company-wide audit feed** | *(tanpa filter — cuma `company_id` dari JWT)* |

**Response (200):**

```json
{
  "data": [
    {
      "id": "c0ffee03-...",
      "company_id": "20000000-0000-0000-0000-000000000001",
      "branch_id": "30000000-0000-0000-0000-000000000001",
      "reff_type": "finance.cash_transactions",
      "reff_id": "70000000-0000-0000-0000-000000000030",
      "reff_no": "CI/JKT-202604/0001",
      "action": "posted",
      "summary": "Diposting ke buku besar",
      "metadata": {
        "total_amount": 15000000,
        "journal_entry_id": "60000000-0000-0000-0000-000000000010"
      },
      "actor_id": "10000000-0000-0000-0000-000000000009",
      "actor_name": "Budi Manager",
      "actor_role": "finance_manager",
      "ip_address": "10.1.2.34",
      "user_agent": "Mozilla/5.0 ...",
      "occurred_at": "2026-04-10T09:45:00Z",
      "created_at": "2026-04-10T09:45:00Z"
    }
  ],
  "message": "Audit logs retrieved successfully",
  "meta": {
    "pagination": {
      "page": 1,
      "limit": 50,
      "total": 1,
      "total_pages": 1
    }
  }
}
```

**Errors:**

| Status | Message |
|---|---|
| 400 | Invalid query parameters |
| 400 | reff_type is required when reff_id is provided |
| 500 | Failed to list audit logs |

> Tidak ada 404 khusus. Kalau filter menghasilkan 0 hit, endpoint mengembalikan `data: []` dengan `total: 0`. Ini sengaja — supaya cross-tenant UUID probing tidak bisa dibedakan dari query kosong.

---

## Action Taxonomy

Action adalah string lowercase snake_case yang divalidasi oleh regex di DB (`^[a-z][a-z0-9_]*$`). Modul baru boleh menambah action sendiri tanpa migration.

### Shared Actions

Didefinisikan di package `audit` dan disarankan dipakai semua feature:

| Action | Makna |
|---|---|
| `created` | Entity dibuat (draft atau langsung live) |
| `updated` | Field header atau child-set diubah |
| `submitted` | Dikirim ke approval flow |
| `approved` | Di-approve di 1 level (bisa level berapa saja) |
| `rejected` | Di-reject di approval flow |
| `cancelled` | Dibatalkan oleh submitter |
| `posted` | Masuk ke ledger / jadi state final |
| `voided` | Di-void setelah posted (reverse) |
| `deleted` | Soft delete |
| `restored` | Soft delete di-undo |
| `attachment_added` | File lampiran ditambah |
| `attachment_removed` | File lampiran dihapus |

### Per-Feature Event Catalog

Detail `summary`, `changes`, dan `metadata` per action tergantung feature. Lihat section berikutnya untuk tiap `reff_type` yang sudah implemented.

---

## Events for `reff_type = "finance.cash_transactions"`

Cash Book meng-emit tujuh action berikut. Nilai `action`, isi `summary`, `changes`, dan `metadata` dijamin persis sesuai tabel.

### `created`

**Di-emit saat:**
- `POST /finance/v1/cash-transactions` (Save Draft)

> Endpoint **Submit** (`POST /.../submit` atau `POST /.../:id/submit`) **tidak** meng-emit event `created` terpisah. Walau secara teknis record dibuat dulu dengan status `draft` lalu disubmit dalam satu tx, dari sudut pandang user aksinya adalah satu submit. Info yang biasanya di event `created` dibawa pindah ke event `submitted` (lihat field `is_new`, `transaction_type`, `total_amount`, `line_count`).

**Summary:** `"Draft dibuat"`
**Changes:** selalu `null`
**Metadata:**
```json
{
  "transaction_type": "receipt",
  "status": "draft",
  "total_amount": 15000000,
  "line_count": 1
}
```

---

### `updated`

**Di-emit saat:**
- `PUT /finance/v1/cash-transactions/:id` — kalau ada field header / jumlah line berubah
- `POST /finance/v1/cash-transactions/:id/submit` — kalau ada field header / jumlah line berubah

**Summary:**
- `"Diperbarui"` — dari endpoint Update
- `"Diperbarui sebelum submit"` — dari endpoint Submit (existing)

**Changes:** field-level diff. Field yang di-tracked:

| Field | Tipe value |
|---|---|
| `branch_id` | UUID string |
| `transaction_date` | string `YYYY-MM-DD` |
| `description` | string (empty string saat null) |
| `cash_account_id` | UUID string |
| `currency_code` | string |
| `exchange_rate` | number |
| `original_amount` | number |
| `total_amount` | number |

Kalau field header semua sama tapi **jumlah line** berubah, `changes` bisa `{}` kosong; info line dibaca dari `metadata.line_count_before/after`.

> Perubahan **isi line** (bukan jumlah) belum menghasilkan diff per-line di v1 — roadmap.

**Metadata:**
```json
{
  "line_count_before": 1,
  "line_count_after": 2,
  "status_at_update": "rejected",
  "linked_journal_entry": "60000000-..."
}
```

---

### `submitted`

**Di-emit saat:**
- `POST /finance/v1/cash-transactions/submit` (direct submit — tidak via draft)
- `POST /finance/v1/cash-transactions/:id/submit` (submit existing draft/rejected)

**Summary:**
- `"Disubmit untuk approval"` — normal path (ada level approval)
- `"Disubmit & otomatis di-approve (tidak ada level approval)"` — config 0-level

**Changes:** selalu `null`

**Metadata (selalu ada):**
```json
{
  "feature_key": "cash_in",
  "auto_approved": false,
  "is_new": true,
  "total_levels": 2,
  "approval_request_id": "40000000-..."
}
```

| Field | Keterangan |
|---|---|
| `feature_key` | `cash_in` untuk receipt, `cash_out` untuk disbursement |
| `auto_approved` | `true` kalau config punya 0 level (langsung posted) |
| `is_new` | `true` kalau submit **tanpa** draft sebelumnya (direct submit); `false` kalau submit existing draft/rejected |
| `total_levels` | hadir kalau `approvalResult.Request != nil` |
| `approval_request_id` | hadir kalau `approvalResult.Request != nil` |

**Metadata tambahan saat `is_new=true`** — dibawa dari event `created` yang tidak di-emit di jalur direct-submit:

```json
{
  "transaction_type": "receipt",
  "total_amount": 15000000,
  "line_count": 1
}
```

> Saat `auto_approved=true`, event `submitted` diikuti event `posted` di tx yang sama.

---

### `approved`

**Di-emit saat:**
- `POST /finance/v1/cash-transactions/:id/approve`

Satu event per level. Kalau level terakhir yang di-approve, event `approved` diikuti event `posted` di tx yang sama.

**Summary format:** `"Di-approve di level {N} dari {TOTAL}"`

**Changes:** selalu `null`
**Metadata:**
```json
{
  "level": 1,
  "total_levels": 2,
  "approval_request_id": "40000000-...",
  "comment": "Dokumen lengkap",
  "finalized": false
}
```

> `comment` hadir hanya kalau user isi.

---

### `posted`

**Di-emit saat:**
- `POST /finance/v1/cash-transactions/:id/approve` (level terakhir, setelah event `approved`)
- `POST /finance/v1/cash-transactions/submit` (auto-approve path, setelah event `submitted`)
- `POST /finance/v1/cash-transactions/:id/submit` (auto-approve path)

**Summary:** `"Diposting ke buku besar"`
**Changes:** selalu `null`
**Metadata:**
```json
{
  "total_amount": 15000000,
  "journal_entry_id": "60000000-..."
}
```

---

### `rejected`

**Di-emit saat:**
- `POST /finance/v1/cash-transactions/:id/reject`

**Summary format:** `"Ditolak di level {N}"`
**Changes:** selalu `null`
**Metadata:**
```json
{
  "level": 1,
  "reason": "Akun lawan salah, seharusnya Beban Operasional",
  "approval_request_id": "40000000-..."
}
```

---

### `deleted`

**Di-emit saat:**
- `DELETE /finance/v1/cash-transactions/:id`

**Summary:** `"Dihapus"`
**Changes:** selalu `null`
**Metadata:**
```json
{
  "status_before_delete": "rejected",
  "linked_journal_entry_id": "60000000-..."
}
```

> `linked_journal_entry_id` hadir kalau transaksi sebelumnya posted (journal ikut di-soft-delete).

---

## Events for `reff_type = "finance.fund_transfers"`

Transfer Dana meng-emit action yang sama dengan Cash Book. Struktur metadata menyesuaikan karena transfer selalu 2-line fixed (tidak ada `line_count`).

### `created`

**Di-emit saat:** `POST /finance/v1/fund-transfers` (Save Draft)

> Jalur **direct submit** (`POST /.../submit` tanpa id) tidak meng-emit `created` terpisah — lihat catatan di event `submitted`.

**Summary:** `"Draft dibuat"`
**Metadata:**
```json
{
  "status": "draft",
  "amount": 5000000,
  "from_account_id": "80000000-...",
  "to_account_id": "80000000-..."
}
```

### `updated`

**Di-emit saat:**
- `PUT /finance/v1/fund-transfers/:id` — kalau ada field header berubah
- `POST /finance/v1/fund-transfers/:id/submit` — kalau ada field header berubah

**Summary:**
- `"Diperbarui"` — dari endpoint Update
- `"Diperbarui sebelum submit"` — dari endpoint Submit (existing)

**Changes:** field-level diff pada salah satu dari:

| Field | Tipe value |
|---|---|
| `transfer_date` | string `YYYY-MM-DD` |
| `description` | string |
| `from_account_id` | UUID string |
| `from_branch_id` | UUID string |
| `to_account_id` | UUID string |
| `to_branch_id` | UUID string |
| `from_currency_code` | string |
| `to_currency_code` | string |
| `exchange_rate` | number |
| `original_amount` | number |
| `amount` | number |

**Metadata (endpoint Update saja):**
```json
{
  "status_at_update": "rejected",
  "linked_journal_entry": "60000000-..."
}
```

### `submitted`

**Di-emit saat:** `POST /finance/v1/fund-transfers/submit` atau `POST /finance/v1/fund-transfers/:id/submit`

**Summary:**
- `"Disubmit untuk approval"` — normal path
- `"Disubmit & otomatis di-approve (tidak ada level approval)"` — config 0-level

**Metadata (selalu ada):**
```json
{
  "feature_key": "transfer",
  "auto_approved": false,
  "is_new": true,
  "total_levels": 2,
  "approval_request_id": "40000000-..."
}
```

**Metadata tambahan saat `is_new=true`:**
```json
{
  "amount": 5000000,
  "from_account_id": "80000000-...",
  "to_account_id": "80000000-..."
}
```

### `approved`, `rejected`, `posted`, `deleted`

Persis sama dengan Cash Book (summary + metadata format identik). Lihat section cash_transactions untuk detail.

Perbedaan minor:
- `posted` metadata hanya membawa `amount` + `journal_entry_id` (tidak ada `total_amount` karena transfer pakai `amount`)
- `deleted` metadata: `status_before_delete` + `linked_journal_entry_id` (kalau ada)

---

## Events for `reff_type = "finance.journal_entries"`

Jurnal Umum meng-emit action yang sama dengan Cash Book. Perbedaan kunci: transaksi jurnal bisa **linked-to-source** (auto-posted dari cash transaction / fund transfer) — mutasi langsung ke journal entry yang seperti itu akan **ditolak** dengan error `LinkedToSourceError` dan **tidak meng-emit audit event** apapun (lifecycle di-own oleh dokumen sumber).

### `created`

**Di-emit saat:** `POST /finance/v1/journal-entries` (Save Draft)

**Summary:** `"Draft dibuat"`
**Metadata:**
```json
{
  "status": "draft",
  "total_debit": 1500000,
  "total_credit": 1500000,
  "line_count": 3
}
```

### `updated`

**Di-emit saat:**
- `PUT /finance/v1/journal-entries/:id` — kalau ada field header / jumlah line berubah
- `POST /finance/v1/journal-entries/:id/submit` — kalau ada field header / jumlah line berubah

**Summary:**
- `"Diperbarui"` — dari endpoint Update
- `"Diperbarui sebelum submit"` — dari endpoint Submit (existing)

**Changes:** field-level diff pada salah satu dari:

| Field | Tipe value |
|---|---|
| `branch_id` | UUID string |
| `journal_date` | string `YYYY-MM-DD` |
| `description` | string |
| `reff_type` | string (kalau jurnal di-link manual ke dokumen lain) |
| `reff_id` | UUID string |
| `reff_number` | string |
| `notes` | string |
| `tags` | array of string |
| `total_debit` | number |
| `total_credit` | number |

**Metadata:**
```json
{
  "line_count_before": 3,
  "line_count_after": 4,
  "status_at_update": "rejected"
}
```

### `submitted`

**Di-emit saat:** `POST /finance/v1/journal-entries/submit` atau `POST /finance/v1/journal-entries/:id/submit`

**Summary:**
- `"Disubmit untuk approval"` — normal path
- `"Disubmit & otomatis di-approve (tidak ada level approval)"` — config 0-level

**Metadata (selalu ada):**
```json
{
  "feature_key": "journal_entry",
  "auto_approved": false,
  "is_new": true,
  "total_levels": 2,
  "approval_request_id": "40000000-..."
}
```

**Metadata tambahan saat `is_new=true`:**
```json
{
  "total_debit": 1500000,
  "total_credit": 1500000,
  "line_count": 3
}
```

### `posted`

**Di-emit saat:**
- `POST /finance/v1/journal-entries/:id/approve` (level terakhir, setelah `approved`)
- `POST /finance/v1/journal-entries/submit` (auto-approve path, setelah `submitted`)

**Summary:** `"Diposting ke buku besar"`
**Metadata:**
```json
{
  "total_debit": 1500000,
  "total_credit": 1500000
}
```

> Tidak seperti cash_transactions / fund_transfers, jurnal umum **tidak** memiliki `journal_entry_id` terpisah — jurnalnya sendiri adalah dokumennya.

### `approved`, `rejected`, `deleted`

Format summary + metadata identik dengan Cash Book.

`deleted` pada journal entry **tidak** membawa `linked_journal_entry_id` (tidak relevan).

---

## Example Scenarios

### A. Halaman detail cash transaction — tab "History"

```
GET /core/v1/audit-logs?reff_type=finance.cash_transactions&reff_id=70000000-...
```

Alur "Save Draft → Submit → Approve L1 → Approve L2 (finalize)" menghasilkan 5 row (newest first):

1. `posted` — "Diposting ke buku besar"
2. `approved` — "Di-approve di level 2 dari 2" (`finalized: true`)
3. `approved` — "Di-approve di level 1 dari 2" (`finalized: false`)
4. `submitted` — "Disubmit untuk approval" (`is_new: false`)
5. `created` — "Draft dibuat"

### B. Alur "Direct Submit → Reject → Edit → Submit → Auto-Approve"

User tidak pernah klik "Save Draft". Pertama langsung submit, di-reject di L1, lalu edit + submit ulang ke config baru yang 0-level.

```
GET /core/v1/audit-logs?reff_type=finance.cash_transactions&reff_id=70000000-...
```

5 row (newest first) — **tanpa** event `created` karena tidak pernah disimpan sebagai draft:

1. `posted` — "Diposting ke buku besar"
2. `submitted` — "Disubmit & otomatis di-approve (tidak ada level approval)" (`is_new: false`, `auto_approved: true`)
3. `updated` — "Diperbarui sebelum submit" (`changes` berisi field yang direvisi)
4. `rejected` — "Ditolak di level 1"
5. `submitted` — "Disubmit untuk approval" (`is_new: true`, sekaligus membawa `transaction_type`/`total_amount`/`line_count`)

### C. Admin view — semua rejection dalam bulan ini

```
GET /core/v1/audit-logs?action=rejected&date_from=2026-04-01&date_to=2026-04-30&limit=100
```

Mengembalikan semua reject lintas feature dalam rentang tanggal, scoped ke company caller.

### D. User activity audit — semua aksi user X

```
GET /core/v1/audit-logs?actor_id=10000000-...&limit=100
```

### E. Update standalone (tanpa submit)

`PUT /:id` dengan payload yang identik persis dengan state saat ini (tidak ada field header berubah, jumlah line sama) → **tidak ada** row baru. Event `updated` hanya di-emit kalau ada perubahan real.

---

## Business Rules

| Rule | Keterangan |
|---|---|
| Append-only | Row audit tidak pernah di-UPDATE/DELETE. Satu event = satu row baru. |
| Same transaction | Audit ditulis dalam tx DB yang sama dengan mutation business-nya. Kalau audit gagal, seluruh operasi rollback → tidak ada gap yang lolos tanpa audit. |
| Actor snapshot | `actor_name` dan `actor_role` disnapshot saat event — perubahan profil user di kemudian hari tidak mengubah history lama. |
| Company-scoped read | `company_id` auto dari JWT — tidak bisa baca history company lain walau tahu UUID-nya. |
| No 404 | Query tanpa hit mengembalikan `data: []` + `total: 0`. Tidak ada cara cross-tenant probe resource existence lewat endpoint ini. |
| `reff_id` requires `reff_type` | Supaya tidak accidental cross-entity collision (UUID unik tapi konteksnya berbeda per feature). |
| Empty changes diff | Event `updated` bisa punya `changes: {}` atau `changes: null` kalau hanya jumlah line yang berubah. Baca `metadata` untuk detail. |
| Request ID optional | `request_id` diambil dari header `X-Request-ID` kalau ada — FE disarankan kirim untuk korelasi dengan server log. |
| Newest first | Sorting fix — tidak ada param `sort`. |
| Max limit 200 | Pagination cap agar query tetap cepat. |
| Extensible actions | Action string bebas (divalidasi regex). Module baru bisa emit action custom tanpa migration. |

---

## Notes for FE Implementers

- **Rendering summary** — field `summary` sudah diformat siap tampil. FE boleh tampilkan apa adanya, atau bangun tampilan custom dari `action` + `metadata`.
- **Badge warna per action** — disarankan mapping di FE:
  - `created` → biru
  - `updated` → abu-abu
  - `submitted` → ungu
  - `approved` → hijau muda
  - `posted` → hijau
  - `rejected` → merah
  - `cancelled` → oranye
  - `deleted` → hitam / coret
- **Diff rendering** — iterasi entry `changes`, render `field → old → new`. Value `null` disarankan render sebagai `—` atau `(empty)`.
- **Timestamp** — ISO-8601 UTC (suffix `Z`). FE convert ke timezone lokal.
- **Detail page pattern** — halaman detail cash tx cukup panggil `/core/v1/audit-logs?reff_type=finance.cash_transactions&reff_id={id}`; pattern yang sama berlaku untuk feature baru.
- **Polling** — endpoint aman di-poll (read-only, indexed). Tapi karena history hanya bertambah saat ada aksi, polling tidak perlu agresif.
- **Company-wide dashboard** — tanpa `reff_type`/`reff_id`, gunakan filter `actor_id` / `action` / date range untuk view agregat.

---

## Related

- [Cash Transactions Contract](../finance/cash-transactions.md) — operasi CRUD yang men-generate event cash-book ke audit-logs
- [Approval System Contract](approval.md) — detail level/approver yang direferensikan di `metadata.approval_request_id`
- Schema migration: [`internal/database/migrations/core/000010_audit_logs.up.sql`](../../../internal/database/migrations/core/000010_audit_logs.up.sql)
- Shared audit package: [`internal/shared/audit/`](../../../internal/shared/audit/)

---

**Last Updated:** 2026-04-18
