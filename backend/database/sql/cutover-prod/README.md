# SQL cutover production paddyadorable.biz.id

Dijalankan oleh `tools/vps/cutover-remote.sh restore` setelah `pg_restore` dump live paddydev ke DB production. Rencana lengkap: `docs/superpowers/plans/2026-10-23-cutover-production-paddyadorable.md`.

| File | Isi |
|---|---|
| `01-hris-only.sql` | Kosongkan 60 tabel non-HRIS yang berisi data contoh/seed di live tetapi kosong di DB baru hasil migrasi (POS, inventory, item, manufaktur, CRM WA, recruitment, spa, shop, purchasing, AI assistant, outbox). Tabel lain (±383) tidak boleh berubah. |
| `02-demo-dan-sesi.sql` | Hapus karyawan dummy `demo@paddy.id` (`DEMOPADDY`) beserta turunan cascade, lalu kosongkan `auth.sessions` dan `auth.login_attempts` (semua user login ulang). |

Keputusan data (user, 23-10-2026): hanya HRIS. `item.brands`, `accounting.*`, master hasil migrasi, dan seed `EMP-2026-00001` / `000000` tetap ada. `item.brands` wajib dipertahankan karena `hris.departments`, `hris.positions`, `hris.sections`, `hris.staff` punya FK `ON DELETE CASCADE` ke sana.

## Prasyarat

- Jalankan sebagai superuser. `01` memakai `session_replication_role = replica` supaya penghapusan tidak memicu trigger atau cascade FK.
- Kedua file menyetel `search_path` sendiri, sama dengan sesi aplikasi. Role DB production tidak menyetelnya, padahal trigger memakai nama tabel tanpa schema.
- Masing-masing file berjalan dalam satu transaksi. Bila verifikasi gagal, file itu `RAISE EXCEPTION` dan tidak ada yang berubah.

## Gladi lokal

```bash
B=~/Desktop/paddydev-backups/paddydev-db-cutover-test-$(date +%Y%m%d-%H%M%S).dump
docker exec paddydev-db sh -c 'pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Fc' > "$B"
T='postgres://postgres@127.0.0.1:55499'
psql "$T/postgres" -Xq -c 'DROP DATABASE IF EXISTS cutover_test' -c 'CREATE DATABASE cutover_test'
# pakai klien Postgres 16 (sama dengan server); pg_restore 17+ menulis SET transaction_timeout
docker run --rm -i --network host postgres:16-alpine pg_restore --no-owner --no-acl --exit-on-error -d "$T/cutover_test" < "$B"
psql "$T/cutover_test" -X -v ON_ERROR_STOP=1 -f 01-hris-only.sql
psql "$T/cutover_test" -X -v ON_ERROR_STOP=1 -f 02-demo-dan-sesi.sql
```

Hasil 24-10-2026 (dump live main `031e9d2`, 560 migrasi):
- `cutover 01 OK: 60 tabel dikosongkan, 383 tabel dipertahankan tidak berubah`;
- `cutover 02 OK`;
- karyawan 622 → 621, absensi 56.606 dan akun 331 tidak berubah, departemen 15, brand 52, produk POS 0, sesi 0;
- menjalankan `02` kedua kali gagal dengan `karyawan demo@paddy.id (DEMOPADDY) harus tepat 1, ditemukan 0`;
- `migrate -verify`: `10640 object dari migrasi; 0 hilang, 0 berbeda, 0 hanya ada di database`.

Bila skema berubah sebelum eksekusi, hitung ulang daftar tabel di `01`: tabel yang berisi data di live tetapi kosong di DB baru hasil `migrate -apply`, di luar schema yang dipertahankan.
