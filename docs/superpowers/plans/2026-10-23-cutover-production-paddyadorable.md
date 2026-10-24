# Cutover Production paddyadorable.biz.id — Rencana Implementasi

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Menjadikan https://paddyadorable.biz.id production Paddy HRIS dengan kode `main` terbaru dan data live paddydev.reddie.id (hanya HRIS, tanpa akun dummy demo@paddy.id), lalu menjadikan paddydev.reddie.id staging.

**Architecture:** Production sudah terpasang di VPS `43.134.250.152:2418` (user `wit`, grup `docker`). Container: `paddydev`, `paddydev-api`, `paddydev-db` (Postgres 16), `paddydev-caddy`. Volume: `paddydev-db-data`, `paddydev-storage`, `caddy-data`. Env ada di `~/.config/paddydev/{web,api,db}.env` di VPS. Isinya salinan live 22-10-2026 dengan kode `local-20261022-152655` (557 migrasi). Cutover dijalankan dari host paddydev (yang menjalankan live) lewat satu skrip:
1. Bekukan live.
2. Dump DB live dan arsipkan storage HRIS.
3. Kirim ke VPS.
4. Restore ke DB production.
5. Kosongkan data non-HRIS dan akun dummy dengan SQL yang sudah diuji.
6. Deploy kode `main` ke VPS dengan `tools/deploy-paddydev.sh`.
7. Nyalakan kembali paddydev sebagai staging dengan banner peringatan.

**Tech Stack:** Bash, Docker 29, PostgreSQL 16 (`pg_dump -Fc` / `pg_restore`), psql, rsync/ssh, Next.js 16.4.2 + Vitest (banner), Go migrate tool (`/usr/local/bin/migrate` di image API).

**Spec:** Keputusan user 23-10-2026 di sesi ini (tidak ada dokumen spec terpisah):
- Production = https://paddyadorable.biz.id, VPS di atas.
- Bawa data live paddydev, **hanya HRIS**. Data transaksi/contoh modul non-HRIS dikosongkan; master/config yang dibuat migrasi tetap ada.
- Data dummy yang dibuang: **hanya demo@paddy.id** (karyawan `DEMOPADDY`). Seed `EMP-2026-00001` (nabila.rahmawati+paddyseed@example.com) dan `000000` (takumihenjin7) **tetap ikut** karena tidak dipilih user.
- paddydev.reddie.id menjadi **staging** setelah cutover.
- Jadwal: **secepatnya** setelah rencana disetujui dan gladi lulus.

## Global Constraints

- Sebelum mengubah DB production, selalu backup DB production (`pg_dump -Fc`) dan storage production (`tar`) di VPS. Keduanya chmod 600, di `~/backups/` VPS.
- Sebelum dump, selalu backup DB live (`pg_dump -Fc`) ke `~/Desktop/paddydev-backups/` (chmod 600).
- Isi env, password, dan hash tidak pernah dicetak ke log/terminal. Cetak hanya nama key.
- Live paddydev hanya dihentikan (`docker stop paddydev paddydev-api`) selama cutover. DB live tidak diubah.
- Penghapusan data hanya lewat `backend/database/sql/cutover-prod/*.sql`, dalam satu transaksi dengan verifikasi yang `RAISE EXCEPTION` bila gagal. Tidak ada `TRUNCATE ... CASCADE`.
- Tabel yang **dipertahankan**:
  - semua tabel di schema `hris`, `auth`, `iam`, `configuration`, `audit`, `performance`, `accounting`;
  - `public.schema_migrations`, `public.notifications`, `platform.outbox_subscriptions`;
  - semua tabel yang terisi oleh migrasi (jumlah baris DB baru hasil migrasi > 0), termasuk `item.brands`.
- `item.brands` wajib dipertahankan: `hris.departments`, `hris.positions`, `hris.sections`, `hris.staff` memiliki FK `ON DELETE CASCADE` ke tabel itu.
- Storage yang dibawa hanya `private/attendance`, `private/attendance-requests`, `private/leave-attachments`, `private/overtime-attachments`.
- Env production: hapus `XENDIT_MOCK` dan `MEMBER_OTP_DEMO_CODE` dari `api.env` VPS (setting testing). Key lain tidak diubah.
- Downtime live paddydev selama cutover ≤ 45 menit. Umumkan ke karyawan sebelum mulai.
- Kode Next.js: baca `frontend/AGENTS.md`; versi Next 16.4.2.

## Review Focus

1. **Karyawan tetap absen di paddydev.reddie.id setelah cutover.** Data itu tidak ikut ke production. Mitigasi: banner staging di semua halaman paddydev (Task 2) dan pengumuman URL baru. Test banner ada di Task 2.
2. **Data production tertulis setelah pemeriksaan 23-10 tetapi sebelum cutover** (ada karyawan yang sudah memakai paddyadorable). Data itu tertimpa. Mitigasi: skrip menghentikan cutover bila ada tulis-baru di production setelah `PROD_WRITE_CUTOFF` (Task 4, langkah preflight; diuji di gladi Task 5).
3. **Hapus berantai ke HRIS lewat FK** (contoh `item.brands` → `hris.departments` CASCADE). Mitigasi: `01-hris-only.sql` menghapus dengan `session_replication_role = replica`, lalu memverifikasi jumlah baris semua tabel yang dipertahankan tidak berubah dan tidak ada baris yatim. Diuji di Task 3.
4. **Restore gagal di tengah jalan sehingga production kosong.** Mitigasi: `pg_restore --exit-on-error` sudah diuji dengan flag yang sama di Task 3 dan Task 5, lalu ada `rollback-prod.sh` dari backup VPS (Task 4). Rollback di-review di Task 5 langkah 5.
5. **Build Next di VPS kehabisan memori** (7,5 GB RAM). Mitigasi: deploy dijalankan setelah container web/api production dihentikan dan build diulang sekali bila exit ≠ 0 (Task 6). `deploy-paddydev.sh` otomatis menjalankan image lama bila start gagal.

---

## Task 0: Akses SSH dengan kunci (prasyarat, perlu persetujuan user)

Penyedia VPS meminta kunci publik dan penggantian password. Password sekarang hanya disimpan sementara (chmod 600) di scratchpad sesi.

**Files:** tidak ada di repo.

- [ ] **Step 1: Buat kunci khusus deploy di host paddydev**

```bash
ssh-keygen -t ed25519 -N '' -C 'paddy-prod-deploy@paddydev-host' -f ~/.ssh/paddy_prod_ed25519
```

- [ ] **Step 2: Tambahkan kunci publik ke VPS** (sekali, memakai password)

```bash
ssh -p 2418 wit@43.134.250.152 'umask 077; cat >> ~/.ssh/authorized_keys' < ~/.ssh/paddy_prod_ed25519.pub
```

- [ ] **Step 3: Tambah alias SSH**

```bash
cat >> ~/.ssh/config <<'EOF'
Host paddy-prod
  HostName 43.134.250.152
  Port 2418
  User wit
  IdentityFile ~/.ssh/paddy_prod_ed25519
  IdentitiesOnly yes
EOF
chmod 600 ~/.ssh/config
```

- [ ] **Step 4: Verifikasi**

Run: `ssh -o BatchMode=yes paddy-prod 'id -nG'`
Expected: `wit docker` (tanpa prompt password).

- [ ] **Step 5: User mengganti password VPS** (`passwd` di VPS) dan memberi tahu penyedia. Lalu hapus salinan password sementara di scratchpad.

---

## Task 1: Gabungkan toolkit VPS ke main

Branch `ci/deploy-vps-paddyadorable` (commit `695819c`, `4fa3a0c`) berisi `tools/vps/bootstrap.sh`, `tools/vps/Caddyfile`, `.github/workflows/deploy-vps.yml`, dan override `PADDYDEV_URL` di `tools/deploy-paddydev.sh`. Isinya sudah terpasang di VPS, tetapi belum ada di main.

**Files:**
- Modify: `tools/deploy-paddydev.sh` (dari branch)
- Create: `tools/vps/bootstrap.sh`, `tools/vps/Caddyfile`, `.github/workflows/deploy-vps.yml` (dari branch)

- [ ] **Step 1: Rebase branch ke main**

```bash
git fetch paddydev main
git worktree add .worktrees/deploy-vps ci/deploy-vps-paddyadorable
cd .worktrees/deploy-vps && git rebase paddydev/main
```
Expected: rebase bersih (branch hanya menyentuh 4 file tersebut).

- [ ] **Step 2: Cek sintaks**

Run: `bash -n tools/deploy-paddydev.sh tools/vps/bootstrap.sh && echo ok`
Expected: `ok`

- [ ] **Step 3: Pastikan Caddyfile di repo sama dengan yang berjalan**

Run: `diff <(ssh paddy-prod 'docker exec paddydev-caddy cat /etc/caddy/Caddyfile') tools/vps/Caddyfile && echo sama`
Expected: `sama`

- [ ] **Step 4: Push dan buat PR** (`ci: toolkit deploy VPS paddyadorable.biz.id`). Merge setelah CI lulus, atas izin user.

---

## Task 2: Banner staging di paddydev.reddie.id

Setelah cutover, paddydev tetap hidup sebagai staging. Setiap halaman harus jelas menyatakan "STAGING", dengan tautan ke production. Banner aktif hanya bila env `STAGING_BANNER_PRODUCTION_URL` diisi. Production tidak mengisi env ini.

**Files:**
- Create: `frontend/src/components/layout/staging-banner.tsx`
- Create: `frontend/src/components/layout/staging-banner.test.tsx`
- Modify: `frontend/src/app/layout.tsx` (di dalam `<body>`, sebelum `<ErrorBoundary>`)

**Interfaces:**
- Produces: `StagingBanner({ productionUrl }: { productionUrl?: string }): JSX.Element | null`

- [ ] **Step 1: Tulis test yang gagal**

```tsx
// frontend/src/components/layout/staging-banner.test.tsx
import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { StagingBanner } from "./staging-banner";

describe("StagingBanner", () => {
  it("tidak merender apa pun tanpa URL production", () => {
    const { container } = render(<StagingBanner />);
    expect(container.innerHTML).toBe("");
  });

  it("tidak merender apa pun bila URL bukan https", () => {
    const { container } = render(<StagingBanner productionUrl="javascript:alert(1)" />);
    expect(container.innerHTML).toBe("");
  });

  it("menandai STAGING dan menautkan ke production", () => {
    render(<StagingBanner productionUrl="https://paddyadorable.biz.id" />);
    expect(screen.getByRole("status").textContent).toContain("STAGING");
    const link = screen.getByRole("link", { name: "paddyadorable.biz.id" });
    expect(link.getAttribute("href")).toBe("https://paddyadorable.biz.id/login");
  });
});
```

- [ ] **Step 2: Jalankan, pastikan gagal**

Run: `cd frontend && npx vitest run src/components/layout/staging-banner.test.tsx`
Expected: FAIL, `Failed to resolve import "./staging-banner"`

- [ ] **Step 3: Implementasi**

```tsx
// frontend/src/components/layout/staging-banner.tsx
// Banner server-side untuk paddydev.reddie.id setelah cutover: data di sini tidak
// lagi dipakai production. Aktif hanya bila STAGING_BANNER_PRODUCTION_URL diisi.
export function StagingBanner({ productionUrl }: { productionUrl?: string }) {
  if (!productionUrl) return null;
  let url: URL;
  try {
    url = new URL(productionUrl);
  } catch {
    return null;
  }
  if (url.protocol !== "https:") return null;
  return (
    <div
      role="status"
      className="sticky top-0 z-[100] bg-amber-500 px-4 py-2 text-center text-sm font-semibold text-black"
    >
      STAGING — data di sini tidak dipakai. Gunakan{" "}
      <a className="underline" href={`${url.origin}/login`}>
        {url.host}
      </a>
    </div>
  );
}
```

Di `frontend/src/app/layout.tsx` tambahkan import `import { StagingBanner } from "@/components/layout/staging-banner";`, lalu ganti `<body suppressHydrationWarning>` menjadi:

```tsx
      <body suppressHydrationWarning>
        <StagingBanner productionUrl={process.env.STAGING_BANNER_PRODUCTION_URL} />
```

- [ ] **Step 4: Jalankan test, tsc, lint**

Run: `cd frontend && npx vitest run src/components/layout/staging-banner.test.tsx && npx tsc --noEmit -p . && npx eslint src/components/layout/staging-banner.tsx src/app/layout.tsx`
Expected: 3 test PASS, tsc dan eslint bersih.

- [ ] **Step 5: Commit, PR** (`feat(web): banner staging untuk paddydev setelah cutover production`). Merge setelah CI lulus, atas izin user.

---

## Task 3: SQL cutover "hanya HRIS" + hapus demo, diuji di salinan

**Files:**
- Create: `backend/database/sql/cutover-prod/01-hris-only.sql`
- Create: `backend/database/sql/cutover-prod/02-demo-dan-sesi.sql`
- Create: `backend/database/sql/cutover-prod/README.md`
- Test: gladi di DB test lokal `postgres://postgres@127.0.0.1:55499/cutover_test`

**Interfaces:**
- Produces: dua file SQL yang dijalankan `psql -v ON_ERROR_STOP=1 -f`. Masing-masing satu transaksi dan gagal (exit ≠ 0, tanpa perubahan) bila verifikasi tidak lulus. Output terakhir: `NOTICE: cutover 01 OK: <n> tabel dikosongkan, <m> tabel dipertahankan tidak berubah` dan `NOTICE: cutover 02 OK`.

Daftar 60 tabel di bawah dihitung 23-10-2026 dengan dua syarat:
- ada isinya di live, tetapi kosong di DB baru hasil `migrate -apply` (560 migrasi);
- berada di luar daftar tabel yang dipertahankan.

Bila skema berubah sebelum eksekusi, hitung ulang dengan langkah 1.

- [ ] **Step 1: Siapkan salinan live dan DB baru-migrasi di DB test lokal**

```bash
B=~/Desktop/paddydev-backups/paddydev-db-cutover-test-$(date +%Y%m%d-%H%M%S).dump
docker exec paddydev-db sh -c 'pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Fc' > "$B" && chmod 600 "$B"
T='postgres://postgres@127.0.0.1:55499'
psql "$T/postgres" -Xq -c 'DROP DATABASE IF EXISTS cutover_test' -c 'CREATE DATABASE cutover_test'
pg_restore --no-owner --no-acl --exit-on-error -d "$T/cutover_test" "$B"; echo exit=$?
```
Expected: `exit=0`. Flag sama dengan restore di VPS, supaya error restore muncul di sini lebih dulu.

- [ ] **Step 2: Tulis `01-hris-only.sql`**

```sql
-- Cutover production (EPIC: cutover paddyadorable, 23-10-2026): kosongkan data
-- non-HRIS yang tidak dibuat migrasi. Satu transaksi; gagal = tidak ada perubahan.
-- Wajib superuser (session_replication_role). Lihat README.md di folder ini.
\set ON_ERROR_STOP on
BEGIN;

CREATE TEMP TABLE cutover_empty (tbl text PRIMARY KEY) ON COMMIT DROP;
INSERT INTO cutover_empty (tbl) VALUES
  ('crm.wa_messages'),
  ('dataroom.nodes'),
  ('inventory.finished_goods_inventory'),
  ('inventory.finished_goods_movements'),
  ('inventory.inventory'),
  ('inventory.inventory_movements'),
  ('inventory.stock_batches'),
  ('inventory.stock_batch_movements'),
  ('item.product_categories'),
  ('item.products'),
  ('item.raw_material_categories'),
  ('item.raw_materials'),
  ('item.raw_material_unit_conversions'),
  ('item.units'),
  ('manufacturing.bom_items'),
  ('manufacturing.production_batches'),
  ('manufacturing.production_order_materials'),
  ('manufacturing.production_orders'),
  ('manufacturing.raw_material_bom_items'),
  ('platform.outbox_deliveries'),
  ('platform.outbox_events'),
  ('platform.rate_limits'),
  ('pos.pos_customers'),
  ('pos.pos_inventory_settings'),
  ('pos.pos_product_images'),
  ('pos.pos_products'),
  ('pos.pos_product_skus'),
  ('pos.pos_wallet_sweep_runs'),
  ('public.ai_assistant_logs'),
  ('public.ai_assistant_messages'),
  ('public.ai_assistant_sessions'),
  ('purchasing.supplier_price_lists'),
  ('purchasing.suppliers'),
  ('recruitment.candidate_activities'),
  ('recruitment.candidate_ai_analysis'),
  ('recruitment.candidate_notes'),
  ('recruitment.candidate_offers'),
  ('recruitment.candidate_psikotes_summary'),
  ('recruitment.candidates'),
  ('recruitment.candidate_screenings'),
  ('recruitment.interview_ai_proctor_events'),
  ('recruitment.interview_ai_sessions'),
  ('recruitment.interview_ai_turns'),
  ('recruitment.interviews'),
  ('recruitment.job_openings'),
  ('recruitment.live_monitor_frames'),
  ('recruitment.live_signals'),
  ('recruitment.psikotes_proctor_events'),
  ('recruitment.psikotes_questions'),
  ('recruitment.psikotes_sessions'),
  ('recruitment.psikotes_session_tests'),
  ('shop.product_channels'),
  ('shop.shipping_settings'),
  ('spa.commission_rules'),
  ('spa.outlets'),
  ('spa.public_booking_funnel_events'),
  ('spa.treatments'),
  ('spa.treatment_variants'),
  ('spa.variant_outlet_prices'),
  ('spa.variant_pos_products');

-- Semua tabel yang dikosongkan harus ada (nama salah = batal).
DO $$
DECLARE missing text;
BEGIN
  SELECT string_agg(tbl, ', ') INTO missing FROM cutover_empty WHERE to_regclass(tbl) IS NULL;
  IF missing IS NOT NULL THEN RAISE EXCEPTION 'tabel tidak ditemukan: %', missing; END IF;
END $$;

-- Snapshot jumlah baris semua tabel lain (yang dipertahankan).
CREATE TEMP TABLE cutover_kept ON COMMIT DROP AS
SELECT t.table_schema || '.' || t.table_name AS tbl,
       (xpath('/row/c/text()', query_to_xml(format('select count(*) as c from %I.%I', t.table_schema, t.table_name), false, true, '')))[1]::text::bigint AS n
FROM information_schema.tables t
WHERE t.table_type = 'BASE TABLE'
  AND t.table_schema NOT IN ('pg_catalog', 'information_schema')
  AND t.table_schema NOT LIKE 'pg_temp%'
  AND t.table_schema || '.' || t.table_name NOT IN (SELECT tbl FROM cutover_empty);

-- Hapus tanpa trigger/cascade FK supaya tidak ada efek ke tabel yang dipertahankan.
SET LOCAL session_replication_role = replica;
DO $$
DECLARE r record;
BEGIN
  FOR r IN SELECT tbl FROM cutover_empty ORDER BY tbl LOOP
    EXECUTE format('DELETE FROM %s', r.tbl);
  END LOOP;
END $$;
SET LOCAL session_replication_role = origin;

-- Verifikasi 1: tabel yang dipertahankan tidak berubah.
DO $$
DECLARE changed text;
BEGIN
  SELECT string_agg(k.tbl || ' ' || k.n || '→' || now_n, ', ') INTO changed
  FROM (SELECT tbl, n, (xpath('/row/c/text()', query_to_xml(format('select count(*) as c from %s', tbl), false, true, '')))[1]::text::bigint AS now_n
        FROM cutover_kept) k
  WHERE k.n <> k.now_n;
  IF changed IS NOT NULL THEN RAISE EXCEPTION 'tabel dipertahankan berubah: %', changed; END IF;
END $$;

-- Verifikasi 2: tidak ada baris di tabel lain yang masih menunjuk tabel yang dikosongkan.
DO $$
DECLARE r record; n bigint; orphans text := '';
BEGIN
  FOR r IN
    SELECT cn.nspname || '.' || c.relname AS src, con.conname,
           (SELECT string_agg(format('%I', a.attname), ',') FROM unnest(con.conkey) k JOIN pg_attribute a ON a.attrelid = con.conrelid AND a.attnum = k) AS cols
    FROM pg_constraint con
    JOIN pg_class c ON c.oid = con.conrelid JOIN pg_namespace cn ON cn.oid = c.relnamespace
    JOIN pg_class f ON f.oid = con.confrelid JOIN pg_namespace fn ON fn.oid = f.relnamespace
    WHERE con.contype = 'f'
      AND fn.nspname || '.' || f.relname IN (SELECT tbl FROM cutover_empty)
      AND cn.nspname || '.' || c.relname NOT IN (SELECT tbl FROM cutover_empty)
  LOOP
    EXECUTE format('SELECT count(*) FROM %s WHERE (%s) IS NOT NULL', r.src, replace(r.cols, ',', ') IS NOT NULL AND (')) INTO n;
    IF n > 0 THEN orphans := orphans || r.src || '(' || r.conname || ')=' || n || ' '; END IF;
  END LOOP;
  IF orphans <> '' THEN RAISE EXCEPTION 'baris yatim: %', orphans; END IF;
END $$;

DO $$
BEGIN
  RAISE NOTICE 'cutover 01 OK: % tabel dikosongkan, % tabel dipertahankan tidak berubah',
    (SELECT count(*) FROM cutover_empty), (SELECT count(*) FROM cutover_kept);
END $$;
COMMIT;
```

- [ ] **Step 3: Tulis `02-demo-dan-sesi.sql`**

```sql
-- Cutover production: buang karyawan dummy demo@paddy.id (DEMOPADDY, tanpa akun
-- login) beserta data turunannya (cascade: onboarding, riwayat, saldo cuti,
-- penempatan, KPI), lalu kosongkan sesi login supaya semua user login ulang.
\set ON_ERROR_STOP on
BEGIN;

CREATE TEMP TABLE cutover_demo ON COMMIT DROP AS
SELECT id, user_id FROM hris.employees WHERE nip = 'DEMOPADDY' AND lower(email) = 'demo@paddy.id';

DO $$
BEGIN
  IF (SELECT count(*) FROM cutover_demo) <> 1 THEN
    RAISE EXCEPTION 'karyawan demo@paddy.id (DEMOPADDY) harus tepat 1, ditemukan %', (SELECT count(*) FROM cutover_demo);
  END IF;
END $$;

DELETE FROM hris.employees WHERE id IN (SELECT id FROM cutover_demo);
DELETE FROM iam.user_roles WHERE user_id IN (SELECT user_id FROM cutover_demo WHERE user_id IS NOT NULL);
DELETE FROM configuration.users WHERE id IN (SELECT user_id FROM cutover_demo WHERE user_id IS NOT NULL)
   OR lower(email) = 'demo@paddy.id';
DELETE FROM auth.users WHERE id IN (SELECT user_id FROM cutover_demo WHERE user_id IS NOT NULL)
   OR lower(email) = 'demo@paddy.id';

DELETE FROM auth.sessions;
DELETE FROM auth.login_attempts;

DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM hris.employees WHERE nip = 'DEMOPADDY' OR lower(email) = 'demo@paddy.id')
     OR EXISTS (SELECT 1 FROM auth.users WHERE lower(email) = 'demo@paddy.id') THEN
    RAISE EXCEPTION 'demo@paddy.id masih ada';
  END IF;
  RAISE NOTICE 'cutover 02 OK';
END $$;
COMMIT;
```

- [ ] **Step 4: Gladi di salinan lokal — jalankan dan cocokkan hasil**

```bash
T='postgres://postgres@127.0.0.1:55499/cutover_test'
psql "$T" -Xq -At -c "select count(*) from hris.employees" > /tmp/emp_before
psql "$T" -X -v ON_ERROR_STOP=1 -f backend/database/sql/cutover-prod/01-hris-only.sql
psql "$T" -X -v ON_ERROR_STOP=1 -f backend/database/sql/cutover-prod/02-demo-dan-sesi.sql
psql "$T" -Xq -At -c "select count(*) from hris.employees"
```
Expected:
- `NOTICE:  cutover 01 OK: 60 tabel dikosongkan, <m> tabel dipertahankan tidak berubah` (m = semua tabel lain, ±383);
- `NOTICE:  cutover 02 OK`;
- jumlah karyawan = isi `/tmp/emp_before` − 1.

- [ ] **Step 5: Uji jalur gagal (wajib)**

```bash
# a) jalankan 02 kedua kali → harus gagal tanpa perubahan
psql "$T" -X -v ON_ERROR_STOP=1 -f backend/database/sql/cutover-prod/02-demo-dan-sesi.sql; echo exit=$?
# b) salin 01 ke /tmp, sisipkan tabel palsu ke daftar → harus gagal "tabel tidak ditemukan"
sed "s/^  ('crm.wa_messages'),/  ('crm.tidak_ada'),\n  ('crm.wa_messages'),/" backend/database/sql/cutover-prod/01-hris-only.sql > /tmp/01-bad.sql
psql "$T" -X -v ON_ERROR_STOP=1 -f /tmp/01-bad.sql; echo exit=$?
```
Expected:
- a) `ERROR: karyawan demo@paddy.id (DEMOPADDY) harus tepat 1, ditemukan 0`, `exit=3`;
- b) `ERROR: tabel tidak ditemukan: crm.tidak_ada`, `exit=3`.

- [ ] **Step 6: Cek skema masih utuh dengan migrate tool**

```bash
IMG=$(docker inspect --format '{{.Config.Image}}' paddydev-api)
docker run --rm --network host -e DATABASE_URL="$T?sslmode=disable" -e MIGRATIONS_DIR=/app/database/migrations \
  --entrypoint /usr/local/bin/migrate "$IMG" -allow-remote -verify
```
Expected: `... 0 hilang, 0 berbeda, 0 hanya ada di database.` (Nilai `MIGRATIONS_DIR` diambil dari `~/.config/paddydev/api.env`.)

- [ ] **Step 7: Tulis README.md folder** (urutan, prasyarat superuser, cara gladi, daftar tabel dan alasannya). Lalu commit:

```bash
git add backend/database/sql/cutover-prod
git commit -m "ops(db): SQL cutover production hanya HRIS + hapus demo@paddy.id (diuji di salinan)"
```

---

## Task 4: Skrip cutover, helper VPS, dan rollback

Skrip lokal hanya mengatur urutan dan memindahkan file. Semua perintah DB di VPS ada di helper `cutover-remote.sh`, supaya tidak ada quoting berlapis ssh → sh → psql.

**Files:**
- Create: `tools/vps/cutover-remote.sh` (dijalankan di VPS)
- Create: `tools/vps/cutover-from-paddydev.sh` (dijalankan di host paddydev, dari checkout `main` yang bersih, mis. `.worktrees/deploy-main`)
- Create: `tools/vps/rollback-prod.sh` (dijalankan di host paddydev)

**Interfaces:**
- Consumes:
  - SSH alias `paddy-prod` (Task 0);
  - `backend/database/sql/cutover-prod/01-hris-only.sql` dan `02-demo-dan-sesi.sql` (Task 3);
  - `tools/deploy-paddydev.sh` dengan `PADDYDEV_URL` (Task 1);
  - banner env `STAGING_BANNER_PRODUCTION_URL` (Task 2).
- Produces:
  - `cutover-remote.sh writes <iso>|backup <stamp>|restore <db|PROD>|verify <db>|drop <db>|storage|env`;
  - `cutover-from-paddydev.sh rehearse|execute` (execute wajib env `PROD_WRITE_CUTOFF`);
  - `rollback-prod.sh <stamp>`.

- [ ] **Step 1: Tulis `tools/vps/cutover-remote.sh`**

```bash
#!/usr/bin/env bash
# Helper di VPS untuk cutover production (dipanggil tools/vps/cutover-from-paddydev.sh).
# Input di ~/cutover: live.dump, storage-hris.tgz, 01-hris-only.sql, 02-demo-dan-sesi.sql.
set -Eeuo pipefail
CMD="${1:?perintah}"; ARG="${2:-}"
DIR="$HOME/cutover"; ENV_DIR="$HOME/.config/paddydev"
pg() { docker exec -i paddydev-db sh -c 'psql -U "$POSTGRES_USER" -X -v ON_ERROR_STOP=1 -q "$@"' _ "$@"; }
prod_db() { docker exec paddydev-db sh -c 'printf %s "$POSTGRES_DB"'; }
db_name() { if [ "$1" = PROD ]; then prod_db; else printf %s "$1"; fi; }

case "$CMD" in
  writes) # jumlah tulis-baru di production setelah ARG (ISO timestamp)
    pg -d "$(prod_db)" -At -v cutoff="$ARG" <<'SQL'
SET default_transaction_read_only = on;
SELECT (SELECT count(*) FROM audit.audit_log WHERE created_at > :'cutoff')
     + (SELECT count(*) FROM hris.attendance WHERE updated_at > :'cutoff')
     + (SELECT count(*) FROM hris.leaves WHERE created_at > :'cutoff')
     + (SELECT count(*) FROM hris.overtime_requests WHERE created_at > :'cutoff')
     + (SELECT count(*) FROM hris.employees WHERE updated_at > :'cutoff');
SQL
    ;;
  backup) # backup DB, storage, dan api.env production dengan STAMP = ARG
    umask 077; mkdir -p "$HOME/backups"
    docker exec paddydev-db sh -c 'pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Fc' > "$HOME/backups/prod-db-before-cutover-$ARG.dump"
    docker exec -i paddydev-db pg_restore -l < "$HOME/backups/prod-db-before-cutover-$ARG.dump" > /dev/null
    docker run --rm -v paddydev-storage:/s:ro alpine tar -C /s -czf - . > "$HOME/backups/prod-storage-before-cutover-$ARG.tgz"
    cp "$ENV_DIR/api.env" "$HOME/backups/api.env.before-cutover-$ARG"
    ls -la "$HOME/backups" | tail -3
    ;;
  restore) # buat ulang DB (ARG = nama DB atau PROD), restore live.dump, jalankan SQL cutover
    DB="$(db_name "$ARG")"
    pg -d postgres -c "DROP DATABASE IF EXISTS \"$DB\" WITH (FORCE)" -c "CREATE DATABASE \"$DB\""
    docker exec -i paddydev-db sh -c 'pg_restore -U "$POSTGRES_USER" -d "$1" --no-owner --no-acl --exit-on-error' _ "$DB" < "$DIR/live.dump"
    for f in 01-hris-only.sql 02-demo-dan-sesi.sql; do pg -d "$DB" < "$DIR/$f"; done
    ;;
  verify) # cek skema DB ARG terhadap migrasi image API yang berjalan (exit 3 = berbeda)
    DB="$(db_name "$ARG")"
    URL="$(sed -n 's/^DATABASE_URL=//p' "$ENV_DIR/api.env")"
    URL="$(printf %s "$URL" | sed -E "s#/[^/?]+(\?|\$)#/$DB\1#")"
    IMG="$(docker inspect --format '{{.Config.Image}}' paddydev-api)"
    docker run --rm --network paddydev-net --env-file "$ENV_DIR/api.env" -e DATABASE_URL="$URL" \
      --entrypoint /usr/local/bin/migrate "$IMG" -allow-remote -verify
    ;;
  drop)
    pg -d postgres -c "DROP DATABASE IF EXISTS \"$ARG\" WITH (FORCE)"
    ;;
  storage) # ganti isi storage production dengan folder HRIS dari live
    docker run --rm -i -v paddydev-storage:/s alpine sh -c \
      'rm -rf /s/private /s/uploads /s/assistant-memory && tar -C /s -xzf - && chown -R 1001:1001 /s' < "$DIR/storage-hris.tgz"
    ;;
  env) # hapus setting testing dari api.env; cetak nama key saja
    sed -i '/^XENDIT_MOCK=/d;/^MEMBER_OTP_DEMO_CODE=/d' "$ENV_DIR/api.env"
    sed -n 's/=.*//p' "$ENV_DIR/api.env" | tr '\n' ' '; echo
    ;;
  *) echo "perintah tidak dikenal: $CMD" >&2; exit 2 ;;
esac
```

Catatan `verify`: URL diubah dengan mengganti segmen path terakhir. Password DB yang berisi `/` atau `?` akan merusak penggantian ini. Password production sekarang dibuat acak heksadesimal (cek tanpa mencetak: `grep -c 'DATABASE_URL=.*[?/].*@' api.env` harus 0).

- [ ] **Step 2: Tulis `tools/vps/cutover-from-paddydev.sh`**

```bash
#!/usr/bin/env bash
# Cutover production paddyadorable.biz.id dari live paddydev (host ini).
#   rehearse : restore ke DB sementara cutover_rehearsal di VPS + SQL cutover + cek skema,
#              lalu hapus DB itu. Live & production tidak berubah.
#   execute  : cek tulis-baru production, backup production, bekukan live, restore + SQL
#              cutover ke DB production, salin storage HRIS, rapikan env, deploy main,
#              lalu nyalakan paddydev sebagai staging (banner).
# Jalankan dari checkout main yang bersih. Wajib SSH alias paddy-prod.
set -Eeuo pipefail
cd "$(dirname "$0")/../.."
MODE="${1:?pakai: rehearse | execute}"
STAMP="$(TZ=Asia/Jakarta date +%Y%m%d-%H%M%S)"
BK="$HOME/Desktop/paddydev-backups"
SQL_DIR=backend/database/sql/cutover-prod
R=paddy-prod
STORAGE_DIRS=(private/attendance private/attendance-requests private/leave-attachments private/overtime-attachments)
log() { printf '[%s] %s\n' "$(date +%H:%M:%S)" "$*"; }
remote() { ssh "$R" bash cutover/cutover-remote.sh "$@"; }

log "Preflight ($MODE, $STAMP)"
test -z "$(git status --porcelain --untracked-files=no)" || { echo 'Checkout tidak bersih.' >&2; exit 1; }
ssh -o BatchMode=yes "$R" 'umask 077; mkdir -p ~/cutover; df -h / | tail -1'
df -h / | tail -1
docker inspect paddydev paddydev-api paddydev-db > /dev/null
scp -q tools/vps/cutover-remote.sh "$SQL_DIR/01-hris-only.sql" "$SQL_DIR/02-demo-dan-sesi.sql" "$R:cutover/"

if [ "$MODE" = execute ]; then
  : "${PROD_WRITE_CUTOFF:?isi PROD_WRITE_CUTOFF, mis. 2026-10-22T08:30:00Z}"
  writes="$(remote writes "$PROD_WRITE_CUTOFF" | tr -d '[:space:]')"
  if [ "$writes" != 0 ]; then
    echo "Production punya $writes perubahan setelah $PROD_WRITE_CUTOFF. Cutover dihentikan; minta keputusan user." >&2
    exit 1
  fi
  log "Backup production"
  remote backup "$STAMP"
  log "Bekukan live paddydev"
  docker stop paddydev paddydev-api
fi

log "Dump live + storage HRIS"
umask 077
DUMP="$BK/paddydev-db-cutover-$MODE-$STAMP.dump"
TGZ="$BK/paddydev-storage-hris-$STAMP.tgz"
docker exec paddydev-db sh -c 'pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Fc' > "$DUMP"
docker exec -i paddydev-db pg_restore -l < "$DUMP" > /dev/null
docker run --rm -v paddydev-storage:/s:ro alpine tar -C /s -czf - "${STORAGE_DIRS[@]}" > "$TGZ"

log "Kirim ke VPS"
scp -q "$DUMP" "$R:cutover/live.dump"
scp -q "$TGZ" "$R:cutover/storage-hris.tgz"

if [ "$MODE" = rehearse ]; then
  log "Restore + SQL cutover (cutover_rehearsal)"
  status=0
  remote restore cutover_rehearsal && remote verify cutover_rehearsal || status=$?
  remote drop cutover_rehearsal
  [ "$status" = 0 ] || { echo "Gladi GAGAL (exit $status)." >&2; exit "$status"; }
  log "Gladi selesai. Live & production tidak berubah."
  exit 0
fi

log "Hentikan web/API production"
ssh "$R" 'docker stop paddydev paddydev-api'
log "Restore + SQL cutover (PROD)"
remote restore PROD
log "Salin storage HRIS"
remote storage
log "Rapikan env production"
remote env

log "Deploy main ke production"
git archive --format=tar HEAD | ssh "$R" 'mkdir -p paddydev-src && tar -x -C paddydev-src'
ssh "$R" 'cd paddydev-src && PADDYDEV_URL=https://paddyadorable.biz.id bash tools/deploy-paddydev.sh' \
  || ssh "$R" 'cd paddydev-src && PADDYDEV_URL=https://paddyadorable.biz.id bash tools/deploy-paddydev.sh'

log "Verifikasi production"
curl -fsS --max-time 15 https://paddyadorable.biz.id/api/ready; echo
test "$(curl -s -o /dev/null -w '%{http_code}' https://paddyadorable.biz.id/login)" = 200

log "Nyalakan paddydev sebagai staging (banner)"
grep -q '^STAGING_BANNER_PRODUCTION_URL=' "$HOME/.config/paddydev/web.env" \
  || printf 'STAGING_BANNER_PRODUCTION_URL=https://paddyadorable.biz.id\n' >> "$HOME/.config/paddydev/web.env"
docker start paddydev-api paddydev
bash tools/deploy-paddydev.sh
log "Cutover selesai ($STAMP). Rollback: tools/vps/rollback-prod.sh $STAMP"
```

- [ ] **Step 3: Tulis `tools/vps/rollback-prod.sh`**

```bash
#!/usr/bin/env bash
# Pulihkan production ke kondisi sebelum cutover: tools/vps/rollback-prod.sh <STAMP>
set -Eeuo pipefail
STAMP="${1:?pakai: rollback-prod.sh <STAMP dari log cutover>}"
ssh paddy-prod bash -s -- "$STAMP" <<'REMOTE'
set -Eeuo pipefail
STAMP="$1"; B="$HOME/backups"
test -s "$B/prod-db-before-cutover-$STAMP.dump"
test -s "$B/prod-storage-before-cutover-$STAMP.tgz"
docker stop paddydev paddydev-api || true
DB="$(docker exec paddydev-db sh -c 'printf %s "$POSTGRES_DB"')"
docker exec -i paddydev-db sh -c 'psql -U "$POSTGRES_USER" -d postgres -X -v ON_ERROR_STOP=1 -q -c "DROP DATABASE IF EXISTS \"$1\" WITH (FORCE)" -c "CREATE DATABASE \"$1\""' _ "$DB"
docker exec -i paddydev-db sh -c 'pg_restore -U "$POSTGRES_USER" -d "$1" --no-owner --no-acl --exit-on-error' _ "$DB" < "$B/prod-db-before-cutover-$STAMP.dump"
docker run --rm -i -v paddydev-storage:/s alpine sh -c 'rm -rf /s/* && tar -C /s -xzf -' < "$B/prod-storage-before-cutover-$STAMP.tgz"
cp "$B/api.env.before-cutover-$STAMP" "$HOME/.config/paddydev/api.env"
docker start paddydev-api paddydev
REMOTE
docker start paddydev-api paddydev 2>/dev/null || true
echo "Production dipulihkan ke kondisi sebelum cutover $STAMP; live paddydev dinyalakan kembali."
```

Catatan: bila deploy main sudah mengganti image production, `docker start` menjalankan image baru di atas DB lama (22-10). Kondisi ini aman karena migrasi bersifat aditif dan deploy berikutnya menerapkan migrasi yang kurang. Image lama tersimpan sebagai `paddydev:previous` / `paddydev-api:previous` (pola `deploy-paddydev.sh`).

- [ ] **Step 4: Cek sintaks dan lint**

Run: `bash -n tools/vps/cutover-remote.sh tools/vps/cutover-from-paddydev.sh tools/vps/rollback-prod.sh && echo ok; command -v shellcheck && shellcheck -S warning tools/vps/*.sh`
Expected: `ok`. Bila shellcheck tersedia, tidak ada peringatan yang belum dijelaskan.

- [ ] **Step 5: Uji helper `writes` (read-only) terhadap production sekarang**

Run: `scp tools/vps/cutover-remote.sh paddy-prod:cutover/ && ssh paddy-prod bash cutover/cutover-remote.sh writes 2026-10-22T08:30:00Z`
Expected: `0` (sesuai pemeriksaan 23-10). Lalu uji dengan `2026-10-21T00:00:00Z`; Expected: angka > 0.

- [ ] **Step 6: Commit** (`ops(vps): skrip cutover production dari paddydev, helper VPS, rollback`).

---

## Task 5: Gladi di VPS (tanpa mengubah live/production)

- [ ] **Step 1: Jalankan gladi**

Run (dari `.worktrees/deploy-main` setelah `git checkout --detach paddydev/main` yang sudah memuat Task 1–4): `bash tools/vps/cutover-from-paddydev.sh rehearse 2>&1 | tee ~/Desktop/paddydev-backups/cutover-rehearse-$(date +%Y%m%d-%H%M).log`
Expected urutan log:
- `Preflight (rehearse, ...)`, `Dump live + storage HRIS`, `Kirim ke VPS`, `Restore + SQL cutover (cutover_rehearsal)`;
- `NOTICE: cutover 01 OK: 60 tabel ...`, `NOTICE: cutover 02 OK`;
- `0 hilang, 0 berbeda, 0 hanya ada di database.`;
- `Gladi selesai. Live & production tidak berubah.`

- [ ] **Step 2: Pastikan live dan production tidak berubah**

Run: `docker ps --format '{{.Names}} {{.Status}}' | grep -E '^paddydev(-api)? ' && ssh paddy-prod "docker ps --format '{{.Names}} {{.Status}}'; docker exec paddydev-db sh -c 'psql -U \"\$POSTGRES_USER\" -d postgres -XAtc \"select count(*) from pg_database where datname=\$\$cutover_rehearsal\$\$\"'"`
Expected: semua container `Up ... (healthy)` dan jumlah DB `cutover_rehearsal` = 0.

- [ ] **Step 3: Catat waktu tiap tahap** dari log. Total harus < 45 menit; bila lebih, laporkan ke user sebelum eksekusi.

- [ ] **Step 4: Uji preflight tulis-baru.** Jalankan `PROD_WRITE_CUTOFF=2026-10-21T00:00:00Z bash tools/vps/cutover-from-paddydev.sh execute`.
Expected: berhenti dengan `Production punya N perubahan setelah ...`, exit 1, sebelum `Backup production`. Live tetap berjalan (`docker ps` menunjukkan paddydev Up).

- [ ] **Step 5: Review rollback.** Rollback tidak bisa diuji tanpa mengubah production. Baca `rollback-prod.sh` baris demi baris dan pastikan nama file backup cocok dengan yang dibuat `cutover-remote.sh backup` (`prod-db-before-cutover-$STAMP.dump`, `prod-storage-before-cutover-$STAMP.tgz`, `api.env.before-cutover-$STAMP`).

---

## Task 6: Eksekusi cutover (jendela maintenance, atas izin user)

- [ ] **Step 1: Pengumuman** ke karyawan/HR: paddydev berhenti ±45 menit. Setelahnya gunakan https://paddyadorable.biz.id. Akun dan password sama, tetapi semua orang perlu login ulang.

- [ ] **Step 2: Tentukan `PROD_WRITE_CUTOFF`.** Pakai waktu pemeriksaan terakhir production yang dinyatakan bersih (23-10-2026: tidak ada tulis setelah `2026-10-22T08:30:00Z`). Jalankan ulang pemeriksaan sesaat sebelum eksekusi dan pakai nilai itu.

- [ ] **Step 3: Eksekusi**

Run: `PROD_WRITE_CUTOFF=<nilai langkah 2> bash tools/vps/cutover-from-paddydev.sh execute 2>&1 | tee ~/Desktop/paddydev-backups/cutover-execute-$(date +%Y%m%d-%H%M).log`
Expected: diakhiri `Cutover selesai (<STAMP>)`. Catat STAMP untuk rollback.

- [ ] **Step 4: Bila gagal** sebelum `Cutover selesai`, jalankan `bash tools/vps/rollback-prod.sh <STAMP>` dan laporkan isi log ke user.

---

## Task 7: Verifikasi pasca-cutover dan dokumentasi

**Files:**
- Create: `docs/ops/production-paddyadorable.md`

- [ ] **Step 1: Cek fungsi production**

```bash
for u in / /login /api/ready /api/health; do printf '%s ' $u; curl -s -o /dev/null -w '%{http_code} %{redirect_url}\n' https://paddyadorable.biz.id$u; done
ssh paddy-prod "docker ps --format '{{.Names}} {{.Image}} {{.Status}}'"
```
Expected: `/` 302 → `/login`, `/login` 200, `/api/ready` 200, semua container healthy dengan image tag baru.

- [ ] **Step 2: Cek data production (read-only)**

```bash
ssh paddy-prod "docker exec -i paddydev-db sh -c 'psql -U \"\$POSTGRES_USER\" -d \"\$POSTGRES_DB\" -X'" <<'SQL'
SET default_transaction_read_only = on;
SELECT (SELECT count(*) FROM hris.employees) employees, (SELECT count(*) FROM hris.attendance) attendance,
       (SELECT count(*) FROM auth.users) users, (SELECT count(*) FROM auth.sessions) sessions,
       (SELECT count(*) FROM hris.employees WHERE nip='DEMOPADDY') demo,
       (SELECT count(*) FROM pos.pos_products) pos_products, (SELECT count(*) FROM recruitment.candidates) candidates,
       (SELECT count(*) FROM hris.departments) departments, (SELECT count(*) FROM item.brands) brands;
SQL
```
Expected:
- employees = live − 1;
- attendance = live;
- users = live;
- sessions = 0 sebelum ada yang login;
- demo = 0, pos_products = 0, candidates = 0;
- departments dan brands = live.

Ambil angka live dari dump sebelum cutover.

- [ ] **Step 3: Login uji** dengan akun HR (paddy.peopledevelopment@gmail.com, oleh user). Buka Karyawan, Absensi, Lembur, dan Dokumentasi bukti lembur (file storage harus terbuka di modal).

- [ ] **Step 4: Cek paddydev staging** menampilkan banner "STAGING — ... paddyadorable.biz.id" di `/login`.

- [ ] **Step 5: Tulis `docs/ops/production-paddyadorable.md`.** Isinya:
  - arsitektur VPS;
  - lokasi env dan backup;
  - cara deploy (`rsync` + `tools/deploy-paddydev.sh` dengan `PADDYDEV_URL`, atau workflow Actions);
  - STAMP cutover dan rollback;
  - keputusan data (hanya HRIS, demo dibuang, seed Nabila dan 000000 tetap);
  - banner staging.

  Commit dan PR.

- [ ] **Step 6: Jadwalkan backup harian production** mengikuti `docs/ops/backup-restore.md`. Ini tugas lanjutan terpisah; usulkan ke user.
