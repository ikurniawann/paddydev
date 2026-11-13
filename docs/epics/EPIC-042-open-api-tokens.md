# EPIC-042 — Open API Tokens (akses agent eksternal)

status: ready-for-qa

**Status:** Fase 1 selesai (2026-08-23)
**Pemilik:** Owner (ilham@wit.id)
**Konsumen pertama:** Agent OpenClaw milik owner — Add/Edit/Delete lewat API.

## Latar belakang

Seluruh endpoint `/api/*` selama ini hanya bisa diakses lewat cookie sesi login
browser. Owner ingin sistem eksternal (agent AI OpenClaw) bisa memanggil API
langsung — CRUD penuh — tanpa membajak sesi manusia.

## Desain

### Autentikasi
- Header `Authorization: Bearer nh_<64 hex>` (sejak Okt 2026; token lama `arkiv_` tetap diterima); DB hanya menyimpan hash
  SHA-256 (`configuration.api_tokens`), nilai token tampil SEKALI saat dibuat.
- Token menempel ke satu akun user (service account) — request berjalan
  SEBAGAI akun itu sehingga IAM/menu grant tetap berlaku, lalu dibatasi lagi
  oleh **scopes**.
- Validasi terjadi di **proxy/middleware** (Next 16 = Node runtime, bisa akses
  DB): token tidak dikenal / scope tidak cocok → 401 sebelum menyentuh route.
  Ini wajib karena sebagian route lama tidak punya cek sesi sendiri dan
  mengandalkan gerbang cookie middleware. Route yang punya cek sesi sendiri
  me-resolve ulang user via fallback Bearer di `getSessionUserFromCookies`.
- Host member (`member.mizuinwounderland.com/api/*`) kini juga melewati
  gerbang yang sama (sebelumnya /api lolos tanpa gerbang di host member).

### Scopes
- `*` = semua akses.
- `<modul>:read` / `<modul>:write` — modul: `pos`, `member`, `hris`,
  `inventory`, `crm`, `config`, `reports`, `other`.
- GET/HEAD/OPTIONS = read; method lain = write; `write` mencakup read.
- Pemetaan modul dari segmen pertama path (`src/lib/auth/api-token.ts`).
- `/api/openapi.json` boleh dibaca token valid mana pun (discovery agent).

### Guardrail
- **Token tidak bisa mengelola token** (403) — mencegah agent memperbanyak
  aksesnya sendiri. Kelola token = sesi manusia + menu `settings.integrations`.
- Guardrail bisnis tidak terlompati: void tetap butuh PIN supervisor, dst.
- Audit: satu baris per request di `configuration.api_token_request_logs`
  (method, path, allowed) + `last_used_at` per token.
- Revoke = soft delete (`revoked_at`) — jejak audit utuh, efek seketika.

### Kelola token
- UI: Dashboard → Settings → **Integrasi** (`/dashboard/settings/integrations`), section "Open API Tokens" — keputusan owner 2026-08-23; halaman terpisah dihapus.
- API: `GET/POST /api/admin/api-tokens`, `DELETE /api/admin/api-tokens/:id`.

### OpenAPI spec
- `GET /api/openapi.json` (butuh auth): OpenAPI 3.0.3 — 595 path / 896
  operasi hasil scan `scripts/generate-openapi.mjs` (di-commit sebagai
  `src/lib/api-docs/openapi-paths.generated.json`; jalankan ulang bila route
  bertambah) + kurasi skema endpoint inti POS.

## Migrasi

`database/migrations/deltas/20260914090000_api_tokens_port_from_legacy.sql`
— dijalankan bersama migrasi lain lewat `node database/scripts/apply-migrations.js --apply`.
Tanpa tabelnya: token auth diam-diam nonaktif, cookie auth normal, halaman
admin menampilkan peringatan migrasi.

Catatan 2026-09-14: berkas ini dulu berada di `migrations/013_api_tokens.sql`,
folder yang TIDAK pernah dibaca runner, sehingga tabelnya tidak ikut terpasang
di deploy baru dan pembuatan token gagal 503. Folder legacy itu sudah dihapus.

## Integrasi OpenClaw (ringkas)

1. Admin buat token di Settings → Integrasi (section Open API Tokens) (scope `*` untuk akses penuh),
   salin sekali.
2. Konfigurasi agent: base URL `https://dashboard.mizuinwounderland.com`,
   header `Authorization: Bearer <token>`.
3. Auto-discovery: ambil `GET /api/openapi.json` dengan token yang sama.

## Fase berikutnya (belum dikerjakan)
- Rate limiting per token.
- IP allowlist per token.
- Skema kurasi utk lebih banyak endpoint di OpenAPI spec.
- Halaman audit viewer (baca api_token_request_logs dari UI).
