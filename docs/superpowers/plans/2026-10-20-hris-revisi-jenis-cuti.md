# Revisi Jenis Cuti HRIS — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Daftar jenis cuti untuk pengajuan baru disesuaikan dengan permintaan user:

| Perubahan | Jenis | Kode | Aturan |
|---|---|---|---|
| Dihapus | Cuti Haid | `menstrual` | Tidak bisa diajukan lagi; riwayat tetap terbaca |
| Dihapus | Izin Darurat | `emergency` | Tidak bisa diajukan lagi; riwayat tetap terbaca |
| Dihapus | Izin Tanpa Gaji | `unpaid` | Tidak bisa diajukan lagi; riwayat tetap terbaca |
| Ditambah | Izin Leader | `leader` (baru) | Jabatan Supervisor ke atas, kuota 1 hari per bulan, tidak memotong gaji/THP |
| Ditambah | Izin Pribadi | `personal` (sudah ada sejak EPIC-055) | Kuota 3 hari per bulan, memotong 1 hari THP per hari izin |

Ketentuan umum: antar jenis cuti dapat dikombinasikan (lihat Ruling R6).

**Architecture:**
- **Satu katalog jenis cuti per sisi.** Saat ini label jenis cuti tersalin di 4 tempat Go, 7 tempat frontend, dan 1 fungsi SQL. Rencana ini menggantinya dengan satu katalog di Go (`backend/internal/modules/hris/domain/leave_types.go`) dan satu di frontend (`frontend/src/lib/hris/leave-types.ts`). Katalog memuat label, apakah jenis itu masih bisa diajukan (`Selectable`), kuota bulanan, aturan satu hari, dan syarat jabatan.
- **Enum tidak dihapus.** Nilai `menstrual`, `emergency`, dan `unpaid` tetap ada di `public.leave_type`, karena PostgreSQL tidak bisa membuang nilai enum yang masih dipakai baris lama. Penghapusan dilakukan di validasi pengajuan baru (Go) dan di form (frontend). Daftar, detail, filter, ekspor CSV, laporan, absensi harian, dan payroll tetap membaca baris lama.
- **Validasi di `CreateLeave` (Go).** Jenis yang sudah tidak berlaku ditolak. Izin Leader dicek terhadap level jabatan karyawan (`hris.positions.level` lewat `hris.employees.job_title_id`). Kuota bulanan dan tabrakan tanggal dicek di dalam transaksi yang sama dengan insert. Baris karyawan dikunci dulu (`FOR NO KEY UPDATE`), supaya dua pengajuan bersamaan tidak sama-sama lolos kuota.
- **Payroll tidak diubah.** Izin Pribadi sudah memotong THP lewat rule PIP EPIC-055: `THP / pembagi hari × hari izin`, aktif bila setting payroll `personal_leave_deduction` menyala (di live sudah menyala). Izin Leader tidak dihitung payroll, jadi tidak ada potongan, sama seperti Cuti Tahunan. Rencana ini hanya menambah test yang mengunci perilaku itu.
- **Endpoint baru `GET /api/hris/leave-quota`** memberi sisa kuota bulan berjalan dan kelayakan Izin Leader, untuk form ESS dan form HR. Aksesnya mengikuti pola EPIC-057: pemilik data, atau akun HR yang memegang menu `hris.workforce`. Endpoint dan form tidak memuat angka rupiah; potongan dalam rupiah hanya tampil di slip gaji (dijaga `hris.compensation`, EPIC-057).

**Tech Stack:** Go modular monolith (`backend/internal/modules/{hris,payroll}`), PostgreSQL 16 (migrasi `backend/database/migrations/deltas`), Next.js versi repo (baca `frontend/node_modules/next/dist/docs/` sebelum menyentuh halaman), TanStack Query, vitest + testing-library.

**Spec:** tidak ada file spec terpisah. Permintaan user (disampaikan lewat sesi induk, 20-10-2026) disalin di tabel Goal di atas. Ruling R1–R9 di bawah adalah tafsiran rencana ini dan **harus dikonfirmasi user** lewat bagian Pertanyaan Terbuka sebelum eksekusi.

## Temuan investigasi (dasar rencana)

- **Definisi jenis cuti:**
  - Enum `public.leave_type` (`backend/database/migrations/bootstrap/00000000000001_prelude.sql:26`) berisi `annual, sick, maternity, paternity, unpaid, emergency, pilgrimage, menstrual`, ditambah `personal` oleh `deltas/20261017090000_leave_type_personal.sql` (EPIC-055).
  - Kolom `hris.leaves.leave_type leave_type NOT NULL`. Tidak ada tabel master jenis cuti.
  - `hris.leave_balances` punya kolom `*_used` per jenis. Yang benar-benar dipakai kode hanya `annual_leave_*`.
- **Daftar & label tersebar di banyak file:**
  - Go: `hris/http_leaves.go:13` (`leaveTypes` untuk validasi), `hris/domain/workforce.go:122` (label WA), `:233` (label CSV), `hris/domain/insights.go:182` (label laporan).
  - Frontend: `types/hris.ts:379,544`, `features/hris/ess/components/cuti-request-dialog.tsx:21` (opsi ESS), `features/hris/ess/components/beranda-widgets.tsx:22`, `lib/hris/leave-wa.ts:20`, `lib/hris/leaves-export.ts:9`, `lib/hris/reports-summary.ts:44`, `lib/hris/leaves-repo.ts:21` (zod route fallback Next).
  - SQL: `public.fn_notify_leave_changes()` (`bootstrap/00000000001000_functions.sql:794`), notifikasi in-app. Fungsi ini belum mengenal `personal` (tampil "Lainnya").
- **Bug yang sudah ada:** form ESS dan daftar Go menawarkan `marriage` (Menikah) dan `bereavement` (Duka), padahal kedua nilai itu **tidak ada** di enum DB. Pengajuan dengan jenis itu gagal di insert (error enum → 500). Lihat Ruling R9.
- **Kuota & saldo:**
  - Hanya Cuti Tahunan yang punya kuota (`leave_balances.annual_leave_total`, prorata tahun masuk).
  - Saat pengajuan dicek sisa kuota. Saat approve, kuota dipakai dalam transaksi yang sama. Saat cuti yang sudah disetujui dibatalkan HR, kuota dikembalikan.
  - Hari cuti = hari kerja; akhir pekan dan libur nasional tidak dihitung, cuti bersama tetap dihitung (`domain.DescribeLeaveDays`).
- **Validasi tabrakan:** **tidak ada** sama sekali. `CreateLeave` tidak mengecek pengajuan lain pada tanggal yang sama, baik jenis sama maupun berbeda.
- **Approve:** HR (`hrRoles`: super_admin, admin, hrd, hiring_manager) atau atasan langsung (`reporting_to`).
- **Absensi:** `postgres_attendance.go:104` menampilkan satu cuti approved per hari di roster harian. Approve cuti tidak menulis baris `hris.attendance`.
- **Payroll** (`backend/internal/app/adapters_payroll.go:299`, `payroll/domain/inputs.go:192`, `payroll/domain/calculator.go:389`):
  - Hanya cuti approved yang dibaca. Hari dihitung per tanggal kalender di dalam periode (`ClampedLeaveDays`).
  - `unpaid` → potongan `base / hari kerja × hari`.
  - `personal` → PIP `THP / pembagi × hari`.
  - `sick` → PIS `(transport + makan) / pembagi × hari`.
  - Jenis lain tidak dipotong. Tunjangan per hari mengikuti hari hadir di absensi.
- **"SPV ke atas":**
  - `hris.positions.level` (teks, default `'Staff'`). Master Posisi (`features/master-data/positions/components/positions-page.tsx:40`) sudah punya tangga level: Staff, Senior Staff, Supervisor, Assistant Manager, Manager, Senior Manager, General Manager, Director, C-Level.
  - Masalahnya, 192 jabatan hasil import Odoo tidak membawa level (CSV import tanpa kolom `level`), jadi di live kemungkinan besar semuanya `Staff`. Ini perlu dicek read-only saat Task 8.
  - Judul jabatan memang memuat "Supervisor", "SPV", "Manager", "Coordinator", "Lead", dan sebagainya.
  - Di DB test nilainya ditulis huruf kecil (`staff`, `manager`), jadi perbandingan harus case-insensitive.
- **Data cuti lama:**
  - DB test (`paddy_test`): `hris.leaves` kosong (0 baris), dan tidak ada `leave_balances` dengan `emergency/menstrual/unpaid_leave_used > 0`.
  - Live **tidak disentuh**. Perkiraan dari paket import Odoo 15-10-2026: 660 cuti, termasuk 162 `emergency` (Odoo "Cuti Menikah" dan "Izin Pribadi") dan 0 `unpaid`/`menstrual`. EPIC-055 memindahkan 157 baris "[Odoo: Izin Pribadi]" ke `personal`.
  - Jadi sisa `emergency` di live ≈ 5 baris, semuanya Cuti Menikah dari Odoo, ditambah pengajuan ESS 16–20 Oktober yang jumlahnya belum diketahui. Angka pasti diambil read-only di Task 8.

## Ruling (tafsiran rencana — dikonfirmasi lewat Pertanyaan Terbuka)

- **R1 — Kode baru `leader`.** Ditambahkan ke enum lewat migrasi terpisah (pola EPIC-055: nilai enum baru baru bisa dipakai setelah transaksinya selesai). Izin Pribadi memakai kode `personal` yang sudah ada, sehingga rule PIP payroll langsung berlaku.
- **R2 — Tiga jenis yang dihapus disembunyikan, bukan dibuang.**
  - Pengajuan baru dengan `menstrual`/`emergency`/`unpaid` ditolak 400 `Jenis cuti ini sudah tidak berlaku untuk pengajuan baru`. Jenis itu juga tidak muncul di form ESS maupun form HR.
  - Riwayat tetap utuh: daftar, detail, filter jenis di halaman HR, ekspor CSV, laporan, roster absensi, dan notifikasi tetap memakai label lamanya.
  - Pengajuan lama yang masih `pending` tetap bisa di-approve, ditolak, atau dibatalkan.
  - Payroll tetap memotong `unpaid` lama yang approved, supaya hitung ulang periode lama tidak berubah.
- **R3 — Izin Leader & Izin Pribadi diajukan per satu hari** (tanggal mulai = tanggal selesai).
  - "1x/3x per bulan" dan "memotong 1 hari THP" dibaca sebagai satuan hari.
  - Rentang lebih dari satu hari ditolak: `<Label> diajukan per satu hari — tanggal mulai dan selesai harus sama`.
  - Tanggal libur atau akhir pekan tetap ditolak oleh aturan yang sudah ada.
- **R4 — Kuota dihitung per bulan kalender** dari tanggal izin.
  - Yang dihitung: `SUM(total_days)` pengajuan berstatus `pending` + `approved` dengan `start_date` di bulan itu. Pengajuan yang ditolak atau dibatalkan mengembalikan kuota.
  - Kuota berlaku juga saat HR menginput atas nama karyawan; tidak ada override.
  - Kuota tidak disimpan di `leave_balances`, karena dihitung langsung dari `hris.leaves`.
- **R5 — "Supervisor ke atas"** = `hris.positions.level` (case-insensitive, spasi dirapikan) salah satu dari `Supervisor, Assistant Manager, Manager, Senior Manager, General Manager, Director, C-Level`.
  - Karyawan tanpa jabatan, atau dengan level `Staff`/`Senior Staff`/kosong, tidak bisa mengajukan Izin Leader: 400 `Izin Leader hanya untuk jabatan Supervisor ke atas`.
  - Yang dicek adalah karyawan yang **diajukan** (bukan akun yang mengklik), termasuk saat HR menginput atas nama karyawan.
  - Level jabatan live harus diisi HR lebih dulu lewat master Posisi. Task 8 menyediakan laporan read-only jabatan yang judulnya seperti SPV tetapi levelnya masih Staff.
- **R6 — "Dapat dikombinasikan"** = satu karyawan boleh memakai beberapa jenis berdampingan di bulan yang sama, dengan kuota masing-masing berdiri sendiri. Contoh: Izin Leader Senin, Izin Pribadi Selasa, Cuti Tahunan Rabu–Jumat.
  - Satu pengajuan tetap satu jenis; tidak ada pengajuan campuran dalam satu rentang.
  - **Aturan baru:** satu tanggal hanya boleh tercakup satu pengajuan `pending`/`approved`. Tabrakan ditolak 400 `Tanggal ini bertabrakan dengan pengajuan <Label> (<mulai> s.d. <selesai>, <status>)`. Tujuannya mencegah hari yang sama terhitung dua kali, misalnya terkena potongan PIP sekaligus memakai kuota tahunan.
  - Saat ini tidak ada cek tabrakan sama sekali.
- **R7 — Payroll:**
  - Izin Pribadi memakai PIP yang sudah ada (THP / pembagi × 1 hari), selama setting `personal_leave_deduction` menyala.
  - Izin Leader tidak masuk perhitungan potongan mana pun.
  - Tunjangan per hari (transport/makan mode `per_day`) dan gaji pokok mode `daily`/`monthly_prorated` tetap mengikuti hari hadir, sama persis seperti Cuti Tahunan sekarang. Lihat Q5.
- **R8 — Akses:**
  - Endpoint kuota: pemilik data, atau `IsHR` + menu `hris.workforce`. Selain itu 403 `Insufficient permissions`.
  - Akses daftar/approve cuti yang sekarang masih berbasis peran (`hrRoles`) tidak diubah di rencana ini (lihat Follow-up).
  - Tidak ada angka rupiah di endpoint cuti maupun form. Teks yang dipakai: "Memotong 1 hari THP".
- **R9 — Menikah & Duka:** kedua nilai ditambahkan ke enum (`marriage`, `bereavement`) di migrasi yang sama dengan `leader`.
  - Opsinya sudah ditawarkan form, tetapi selalu gagal karena enumnya tidak ada.
  - Payroll tidak memotong keduanya (bukan `unpaid`/`personal`/`sick`).
  - Kalau user memilih menyembunyikan saja (Q7), cukup set `Selectable: false` untuk kedua kode di kedua katalog dan buang dua baris `ADD VALUE` dari migrasi Task 1.

## Global Constraints

- **Kode & label (persis, sama di Go dan TS):**

| Kode | Label | Label pendek (laporan) | Label ekspor CSV | Bisa diajukan | Kuota/bulan | Satu hari | Syarat jabatan |
|---|---|---|---|---|---|---|---|
| `annual` | Cuti Tahunan | Tahunan | Cuti Tahunan | ya | — | — | — |
| `sick` | Sakit | Sakit | Cuti Sakit | ya | — | — | — |
| `personal` | Izin Pribadi | Izin Pribadi | Izin Pribadi | ya | 3 | ya | — |
| `leader` | Izin Leader | Izin Leader | Izin Leader | ya | 1 | ya | Supervisor ke atas |
| `maternity` | Cuti Melahirkan | Melahirkan | Cuti Melahirkan | ya | — | — | — |
| `paternity` | Cuti Ayah | Ayah | Cuti Ayah | ya | — | — | — |
| `pilgrimage` | Cuti Ibadah Haji/Umrah | Haji/Umrah | Cuti Haji/Umrah | ya | — | — | — |
| `marriage` | Cuti Menikah | Pernikahan | Cuti Menikah | ya | — | — | — |
| `bereavement` | Cuti Duka | Duka Cita | Cuti Duka | ya | — | — | — |
| `emergency` | Izin Darurat | Darurat | Cuti Darurat | **tidak** (riwayat) | — | — | — |
| `unpaid` | Izin Tanpa Gaji | Tidak Dibayar | Cuti Tanpa Upah | **tidak** (riwayat) | — | — | — |
| `menstrual` | Cuti Haid | Haid | Cuti Haid | **tidak** (riwayat) | — | — | — |

  Urutan baris = urutan opsi di form. Label ekspor CSV lama dipertahankan, karena berkasnya dibaca di luar aplikasi (`lib/hris/leaves-export.ts:2`).
- **Level "Supervisor ke atas":** `supervisor`, `assistant manager`, `manager`, `senior manager`, `general manager`, `director`, `c-level` (dibandingkan setelah `strings.ToLower(strings.TrimSpace(level))`).
- **Pesan error (persis):**
  - `Jenis cuti ini sudah tidak berlaku untuk pengajuan baru`
  - `Izin Leader hanya untuk jabatan Supervisor ke atas`
  - `<Label> diajukan per satu hari — tanggal mulai dan selesai harus sama`
  - `Kuota <Label> bulan <YYYY-MM> sudah habis (<terpakai> dari <kuota> hari)`
  - `Tanggal ini bertabrakan dengan pengajuan <Label> (<YYYY-MM-DD> s.d. <YYYY-MM-DD>, <menunggu persetujuan|disetujui>)`
  - `Format bulan harus YYYY-MM`

  Semua dikirim sebagai 400, kecuali yang sudah ditentukan lain.
- **Test integrasi Go:**
  - Pakai `TEST_DATABASE_URL='postgres://postgres@127.0.0.1:55499/paddy_test?sslmode=disable'` (container `paddydev-sdd-testdb`), **bukan** port 55432. Go ada di `$HOME/.local/go/bin`.
  - Kegagalan lama yang sudah diketahui sama persis dengan baseline main (EPIC-057: 27 nama). Catat baseline di awal Task 1.
- **Migrasi ke DB test:** `cd backend && DATABASE_URL='postgres://postgres@127.0.0.1:55499/paddy_test?sslmode=disable' $HOME/.local/go/bin/go run ./cmd/migrate -apply`.
- **Route Go baru** butuh `make -C backend generate` (memperbarui `frontend/src/lib/go-routes.generated.json`). Route Next tidak perlu dibuat; route Next hanya fallback.
- **Commit:** satu per task, pesan diakhiri `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`. Kerjakan di branch fitur `feat/epic058-revisi-jenis-cuti`, bukan main.
- **Live:** rencana ini **tidak** men-deploy, tidak menjalankan migrasi, dan tidak menulis apa pun ke DB live. Query read-only ke live (Task 8 Step 1) pun hanya dijalankan setelah user mengizinkan. Deploy dilakukan terpisah lewat `tools/deploy-paddydev.sh` setelah user menyetujui hasil stack uji.

## Review Focus

1. **Dua pengajuan Izin Pribadi bersamaan untuk slot terakhir kuota.** Harapannya hanya satu yang lolos. Penjaganya: lock baris karyawan (`LockLeaveEmployee`) sebelum hitung kuota, di transaksi yang sama dengan insert. Test sekuensial mengunci logika hitungnya: `TestLeaveMonthlyQuota` (Task 3). Reviewer wajib memastikan lock dan hitung ada di dalam `InTx`.
2. **Membatalkan atau menolak pengajuan mengembalikan kuota.** Izin Pribadi ke-4 di bulan yang sama harus bisa diajukan setelah salah satu dari 3 sebelumnya ditolak. Test: `TestLeaveMonthlyQuota` langkah reject (Task 3).
3. **Batas bulan.** Izin Pribadi 31 Maret dan 1 April masuk kuota bulan berbeda; kuota Maret penuh tidak menghalangi April. Test: `TestLeaveMonthlyQuota` (Task 3) dan `TestMonthBounds` (Task 2).
4. **Pengajuan lama jenis yang dihapus.** Baris `emergency` yang masih pending harus tetap bisa di-approve. Filter `leave_type=emergency` di daftar HR tetap jalan, dan ekspor CSV tetap berlabel `Cuti Darurat`. Test: `TestRemovedLeaveTypesStayReadable` (Task 3).
5. **Level jabatan yang ditulis tidak seragam** (`manager` huruf kecil, ` Supervisor ` berspasi, karyawan tanpa jabatan). Harapannya: dua yang pertama layak, yang ketiga ditolak. Test: `TestIsLeaderLevel` (Task 2) dan `TestLeaderLeaveEligibility` (Task 3).

---

### Task 1: Enum `leader`/`marriage`/`bereavement` + label notifikasi in-app

**Files:**
- Create: `backend/database/migrations/deltas/20261020150000_leave_type_leader.sql`
- Create: `backend/database/migrations/deltas/20261020150100_leave_notify_labels.sql`
- Test: `backend/internal/modules/hris/leave_types_integration_test.go`

**Interfaces:**
- Produces: nilai enum `public.leave_type` `'leader'`, `'marriage'`, `'bereavement'`; `public.fn_notify_leave_changes()` mengenal label `personal`, `leader`, `marriage`, `bereavement`.

- [ ] **Step 0: Catat baseline**

Run: `cd backend && TEST_DATABASE_URL='postgres://postgres@127.0.0.1:55499/paddy_test?sslmode=disable' $HOME/.local/go/bin/go test ./... > /tmp/epic058-baseline.txt 2>&1; grep -- '--- FAIL' /tmp/epic058-baseline.txt | sort > /tmp/epic058-baseline-fails.txt; wc -l /tmp/epic058-baseline-fails.txt`
Expected: jumlah dan nama kegagalan dicatat. Nilai ini dipakai sebagai pembanding di Task 8.

- [ ] **Step 1: Tulis test yang gagal**

`backend/internal/modules/hris/leave_types_integration_test.go`:

```go
package hris

import (
	"strings"
	"testing"
)

// EPIC-058: kode cuti baru ada di enum, dan notifikasi in-app memberi label
// yang benar (sebelumnya "personal" tampil sebagai "Lainnya").
func TestLeaveEnumAndNotifyLabels(t *testing.T) {
	h := newHarness(t)
	hr, _ := h.staff("hrd", []string{menuWorkforce}, true)
	_, own := h.staff("pos", nil, true, emp{name: "Pemohon Label"})
	for _, v := range []string{"leader", "marriage", "bereavement", "personal"} {
		if got := h.scalar(`SELECT $1::text::leave_type::text`, v); got != v {
			t.Fatalf("enum %s = %q", v, got)
		}
	}
	for _, c := range []struct{ code, date, label string }{
		{"leader", "2031-03-04", "Izin Leader"},
		{"personal", "2031-03-05", "Izin Pribadi"},
		{"marriage", "2031-03-06", "Menikah"},
	} {
		h.exec(`INSERT INTO hris.leaves (employee_id, leave_type, start_date, end_date, total_days, reason)
			VALUES ($1, $2::text::leave_type, $3::date, $3::date, 1, 'Uji label notifikasi')`, own, c.code, c.date)
		// now() sama sepanjang transaksi fixture, jadi cari berdasarkan isi pesan, bukan created_at.
		n := h.scalar(`SELECT count(*)::text FROM notifications WHERE user_id = $1 AND message LIKE $2`,
			hr.UserID, "Pemohon Label mengajukan cuti "+c.label+" (%")
		if n != "1" {
			t.Fatalf("%s: notifikasi berlabel %q = %s", c.code, c.label, n)
		}
	}
}
```

- [ ] **Step 2: Jalankan, pastikan gagal**

Run: `cd backend && TEST_DATABASE_URL='postgres://postgres@127.0.0.1:55499/paddy_test?sslmode=disable' $HOME/.local/go/bin/go test ./internal/modules/hris -run TestLeaveEnumAndNotifyLabels -v`
Expected: FAIL `invalid input value for enum leave_type: "leader"`.

- [ ] **Step 3: Tulis migrasi enum**

`backend/database/migrations/deltas/20261020150000_leave_type_leader.sql`:

```sql
-- =============================================================================
-- Jenis cuti EPIC-058.
--
-- leader      : Izin Leader (Supervisor ke atas, 1 hari/bulan, tidak memotong).
-- marriage,
-- bereavement : Cuti Menikah & Cuti Duka. Form sudah menawarkan keduanya,
--               tetapi enum belum punya nilainya, sehingga insert gagal.
--
-- menstrual, emergency, unpaid TIDAK dihapus dari enum: riwayat masih
-- memakainya. Penghapusan untuk pengajuan baru ada di validasi Go.
-- File terpisah: nilai enum baru baru bisa dipakai setelah transaksi ini selesai.
-- =============================================================================

ALTER TYPE public.leave_type ADD VALUE IF NOT EXISTS 'leader';
ALTER TYPE public.leave_type ADD VALUE IF NOT EXISTS 'marriage';
ALTER TYPE public.leave_type ADD VALUE IF NOT EXISTS 'bereavement';
```

- [ ] **Step 4: Tulis migrasi label notifikasi**

`backend/database/migrations/deltas/20261020150100_leave_notify_labels.sql`. Salin **utuh** badan `public.fn_notify_leave_changes()` dari `backend/database/migrations/bootstrap/00000000001000_functions.sql:795-877`. Yang diganti hanya blok `CASE`:

```sql
-- =============================================================================
-- Label jenis cuti di notifikasi in-app (EPIC-058): personal, leader,
-- marriage, bereavement. Label jenis lama tetap, karena riwayat masih memicu
-- notifikasi saat status pengajuan lama berubah.
-- =============================================================================

CREATE OR REPLACE FUNCTION public.fn_notify_leave_changes()
 RETURNS trigger
 LANGUAGE plpgsql
 SECURITY DEFINER
AS $function$
DECLARE
  v_employee_name text;
  v_employee_user_id uuid;
  v_leave_label text;
BEGIN
  SELECT full_name, user_id INTO v_employee_name, v_employee_user_id
  FROM employees WHERE id = COALESCE(NEW.employee_id, OLD.employee_id);

  v_leave_label := CASE COALESCE(NEW.leave_type, OLD.leave_type)::text
    WHEN 'annual'      THEN 'Tahunan'
    WHEN 'sick'        THEN 'Sakit'
    WHEN 'maternity'   THEN 'Melahirkan'
    WHEN 'paternity'   THEN 'Ayah'
    WHEN 'personal'    THEN 'Izin Pribadi'
    WHEN 'leader'      THEN 'Izin Leader'
    WHEN 'pilgrimage'  THEN 'Ibadah Haji/Umrah'
    WHEN 'marriage'    THEN 'Menikah'
    WHEN 'bereavement' THEN 'Duka'
    WHEN 'unpaid'      THEN 'Tidak Dibayar'
    WHEN 'emergency'   THEN 'Darurat'
    WHEN 'menstrual'   THEN 'Haid'
    ELSE 'Lainnya'
  END;

  IF TG_OP = 'INSERT' THEN
    PERFORM notify_hrd(
      '📋 Pengajuan Cuti Baru',
      v_employee_name || ' mengajukan cuti ' || v_leave_label ||
        ' (' || NEW.total_days || ' hari) — ' ||
        to_char(NEW.start_date::date, 'DD Mon YYYY') || ' s/d ' ||
        to_char(NEW.end_date::date, 'DD Mon YYYY'),
      'approval',
      '/dashboard/hris/leaves'
    );

  ELSIF TG_OP = 'UPDATE' AND OLD.status IS DISTINCT FROM NEW.status THEN

    IF NEW.status = 'approved' THEN
      PERFORM notify_user(
        v_employee_user_id,
        '✅ Cuti Disetujui',
        'Pengajuan cuti ' || v_leave_label || ' kamu (' || NEW.total_days || ' hari) telah disetujui.',
        'status_change',
        '/dashboard/hris/leaves'
      );
      PERFORM notify_hrd(
        '✅ Cuti Disetujui: ' || v_employee_name,
        'Cuti ' || v_leave_label || ' (' || NEW.total_days || ' hari) telah disetujui.',
        'status_change',
        '/dashboard/hris/leaves'
      );

    ELSIF NEW.status = 'rejected' THEN
      PERFORM notify_user(
        v_employee_user_id,
        '❌ Cuti Ditolak',
        'Pengajuan cuti ' || v_leave_label || ' kamu (' || NEW.total_days || ' hari) ditolak.' ||
          CASE WHEN NEW.rejection_reason IS NOT NULL
               THEN ' Alasan: ' || NEW.rejection_reason
               ELSE '' END,
        'alert',
        '/dashboard/hris/leaves'
      );

    ELSIF NEW.status = 'cancelled' THEN
      PERFORM notify_hrd(
        '🚫 Cuti Dibatalkan',
        v_employee_name || ' membatalkan pengajuan cuti ' || v_leave_label || '.',
        'status_change',
        '/dashboard/hris/leaves'
      );
    END IF;

  ELSIF TG_OP = 'DELETE' THEN
    PERFORM notify_hrd(
      '🗑️ Pengajuan Cuti Dihapus',
      'Pengajuan cuti ' || v_leave_label || ' dari ' || v_employee_name || ' telah dihapus.',
      'alert',
      '/dashboard/hris/leaves'
    );
  END IF;

  RETURN COALESCE(NEW, OLD);
END;
$function$;
```

Badan di luar blok `CASE` adalah salinan persis bootstrap baris 820–877. Sebelum commit, cek dengan `diff <(sed -n 820,877p backend/database/migrations/bootstrap/00000000001000_functions.sql) <(sed -n '/IF TG_OP/,/RETURN COALESCE/p' backend/database/migrations/deltas/20261020150100_leave_notify_labels.sql)`. Harapannya: kosong.

- [ ] **Step 5: Terapkan migrasi ke DB test, jalankan test**

Run: `cd backend && DATABASE_URL='postgres://postgres@127.0.0.1:55499/paddy_test?sslmode=disable' $HOME/.local/go/bin/go run ./cmd/migrate -apply && TEST_DATABASE_URL='postgres://postgres@127.0.0.1:55499/paddy_test?sslmode=disable' $HOME/.local/go/bin/go test ./internal/modules/hris -run TestLeaveEnumAndNotifyLabels -v`
Expected: dua migrasi diterapkan; test PASS.

- [ ] **Step 6: Commit**

```bash
git add backend/database/migrations/deltas/20261020150000_leave_type_leader.sql backend/database/migrations/deltas/20261020150100_leave_notify_labels.sql backend/internal/modules/hris/leave_types_integration_test.go
git commit -m "feat(hris): enum cuti leader/marriage/bereavement & label notifikasi, EPIC-058"
```

**Exit criteria:** enum memuat tiga nilai baru; notifikasi in-app untuk `personal`/`leader`/`marriage` tidak lagi "Lainnya"; test PASS.

---

### Task 2: Katalog jenis cuti Go (`domain/leave_types.go`)

**Files:**
- Create: `backend/internal/modules/hris/domain/leave_types.go`
- Create: `backend/internal/modules/hris/domain/leave_types_test.go`
- Modify: `backend/internal/modules/hris/domain/workforce.go:122-136` (hapus `leaveTypeLabels` + `LeaveTypeLabel`, pindah ke katalog), `:233-237` (hapus `csvLeaveTypes`; `LeavesCSV` memakai `LeaveTypeExportLabel`)
- Modify: `backend/internal/modules/hris/domain/insights.go:182-187` (hapus `reportLeaveLabels`; pemakainya memakai `LeaveTypeShort`)
- Modify: `backend/internal/modules/hris/domain/domain_test.go:357-360` (kode tak dikenal untuk uji "apa adanya" diganti `"lainnya"`, karena `marriage` kini punya label)

**Interfaces:**
- Produces (package `domain`):
  - `type LeaveType struct { Code, Label, Short, Export string; Selectable bool; MonthlyQuota int; SingleDay, LeaderOnly bool }`
  - `func LookupLeaveType(code string) (LeaveType, bool)`
  - `func LeaveTypeCodes() []string` — semua kode enum, untuk validasi enum dan filter
  - `func QuotaLeaveTypes() []LeaveType` — jenis dengan `MonthlyQuota > 0`, urut katalog (`personal`, `leader`)
  - `func LeaveTypeLabel(code string) string`, `func LeaveTypeShort(code string) string`, `func LeaveTypeExportLabel(code string) string` — kode tak dikenal dikembalikan apa adanya
  - `func IsLeaderLevel(level string) bool`
  - `func MonthBounds(month string) (from, to string, ok bool)` — `"2031-03"` → `"2031-03-01"`, `"2031-03-31"`
  - `func MonthOf(date string) string` — `"2031-03-31"` → `"2031-03"`

- [ ] **Step 1: Tulis test yang gagal**

`backend/internal/modules/hris/domain/leave_types_test.go`:

```go
package domain

import (
	"slices"
	"testing"
)

func TestLeaveTypeCatalog(t *testing.T) {
	var selectable []string
	for _, c := range LeaveTypeCodes() {
		if lt, _ := LookupLeaveType(c); lt.Selectable {
			selectable = append(selectable, c)
		}
	}
	want := []string{"annual", "sick", "personal", "leader", "maternity", "paternity", "pilgrimage", "marriage", "bereavement"}
	if !slices.Equal(selectable, want) {
		t.Fatalf("selectable = %v", selectable)
	}
	for _, removed := range []string{"emergency", "unpaid", "menstrual"} {
		lt, ok := LookupLeaveType(removed)
		if !ok || lt.Selectable {
			t.Fatalf("%s harus tetap dikenal tetapi tidak bisa diajukan: %+v %v", removed, lt, ok)
		}
	}
	if len(LeaveTypeCodes()) != 12 {
		t.Fatalf("codes = %v", LeaveTypeCodes())
	}
	leader, _ := LookupLeaveType("leader")
	personal, _ := LookupLeaveType("personal")
	if leader.MonthlyQuota != 1 || !leader.SingleDay || !leader.LeaderOnly || leader.Label != "Izin Leader" {
		t.Fatalf("leader = %+v", leader)
	}
	if personal.MonthlyQuota != 3 || !personal.SingleDay || personal.LeaderOnly || personal.Label != "Izin Pribadi" {
		t.Fatalf("personal = %+v", personal)
	}
	q := QuotaLeaveTypes()
	if len(q) != 2 || q[0].Code != "personal" || q[1].Code != "leader" {
		t.Fatalf("quota types = %+v", q)
	}
	for code, want := range map[string][3]string{
		"sick":      {"Sakit", "Sakit", "Cuti Sakit"},
		"emergency": {"Izin Darurat", "Darurat", "Cuti Darurat"},
		"unpaid":    {"Izin Tanpa Gaji", "Tidak Dibayar", "Cuti Tanpa Upah"},
		"menstrual": {"Cuti Haid", "Haid", "Cuti Haid"},
		"marriage":  {"Cuti Menikah", "Pernikahan", "Cuti Menikah"},
		"lainnya":   {"lainnya", "lainnya", "lainnya"},
	} {
		got := [3]string{LeaveTypeLabel(code), LeaveTypeShort(code), LeaveTypeExportLabel(code)}
		if got != want {
			t.Fatalf("%s labels = %v, want %v", code, got, want)
		}
	}
}

func TestIsLeaderLevel(t *testing.T) {
	for _, lv := range []string{"Supervisor", " supervisor ", "manager", "Assistant Manager", "General Manager", "Director", "C-Level", "SENIOR MANAGER"} {
		if !IsLeaderLevel(lv) {
			t.Fatalf("%q harus layak Izin Leader", lv)
		}
	}
	for _, lv := range []string{"", "Staff", "staff", "Senior Staff", "Coordinator", "Lead"} {
		if IsLeaderLevel(lv) {
			t.Fatalf("%q tidak boleh layak Izin Leader", lv)
		}
	}
}

func TestMonthBounds(t *testing.T) {
	cases := map[string][2]string{"2031-03": {"2031-03-01", "2031-03-31"}, "2032-02": {"2032-02-01", "2032-02-29"}, "2031-12": {"2031-12-01", "2031-12-31"}}
	for m, want := range cases {
		from, to, ok := MonthBounds(m)
		if !ok || from != want[0] || to != want[1] {
			t.Fatalf("%s = %s %s %v", m, from, to, ok)
		}
	}
	for _, bad := range []string{"", "2031-3", "2031-13", "maret"} {
		if _, _, ok := MonthBounds(bad); ok {
			t.Fatalf("%q harus ditolak", bad)
		}
	}
	if MonthOf("2031-03-31") != "2031-03" || MonthOf("2031-04-01") != "2031-04" {
		t.Fatal("MonthOf")
	}
}
```

- [ ] **Step 2: Jalankan, pastikan gagal**

Run: `cd backend && $HOME/.local/go/bin/go test ./internal/modules/hris/domain -run 'TestLeaveTypeCatalog|TestIsLeaderLevel|TestMonthBounds' -v`
Expected: FAIL kompilasi `undefined: LeaveTypeCodes`.

- [ ] **Step 3: Tulis katalog**

`backend/internal/modules/hris/domain/leave_types.go`:

```go
package domain

import (
	"strings"
	"time"
)

// LeaveType is one hris.leaves.leave_type value (EPIC-058). The enum keeps
// every value ever used; Selectable says whether new requests may use it.
type LeaveType struct {
	Code   string
	Label  string // form, daftar, WhatsApp
	Short  string // ringkasan laporan HRIS
	Export string // ekspor CSV (label lama dipertahankan, dibaca di luar aplikasi)

	Selectable   bool // false = riwayat saja
	MonthlyQuota int  // >0: hari per bulan kalender (pending + approved)
	SingleDay    bool // diajukan per satu hari
	LeaderOnly   bool // hanya jabatan Supervisor ke atas
}

// leaveTypes is the catalog, in form order. Mirror: frontend/src/lib/hris/leave-types.ts.
var leaveTypes = []LeaveType{
	{Code: "annual", Label: "Cuti Tahunan", Short: "Tahunan", Export: "Cuti Tahunan", Selectable: true},
	{Code: "sick", Label: "Sakit", Short: "Sakit", Export: "Cuti Sakit", Selectable: true},
	{Code: "personal", Label: "Izin Pribadi", Short: "Izin Pribadi", Export: "Izin Pribadi", Selectable: true, MonthlyQuota: 3, SingleDay: true},
	{Code: "leader", Label: "Izin Leader", Short: "Izin Leader", Export: "Izin Leader", Selectable: true, MonthlyQuota: 1, SingleDay: true, LeaderOnly: true},
	{Code: "maternity", Label: "Cuti Melahirkan", Short: "Melahirkan", Export: "Cuti Melahirkan", Selectable: true},
	{Code: "paternity", Label: "Cuti Ayah", Short: "Ayah", Export: "Cuti Ayah", Selectable: true},
	{Code: "pilgrimage", Label: "Cuti Ibadah Haji/Umrah", Short: "Haji/Umrah", Export: "Cuti Haji/Umrah", Selectable: true},
	{Code: "marriage", Label: "Cuti Menikah", Short: "Pernikahan", Export: "Cuti Menikah", Selectable: true},
	{Code: "bereavement", Label: "Cuti Duka", Short: "Duka Cita", Export: "Cuti Duka", Selectable: true},
	// Dihapus untuk pengajuan baru (EPIC-058); riwayat tetap terbaca.
	{Code: "emergency", Label: "Izin Darurat", Short: "Darurat", Export: "Cuti Darurat"},
	{Code: "unpaid", Label: "Izin Tanpa Gaji", Short: "Tidak Dibayar", Export: "Cuti Tanpa Upah"},
	{Code: "menstrual", Label: "Cuti Haid", Short: "Haid", Export: "Cuti Haid"},
}

func LookupLeaveType(code string) (LeaveType, bool) {
	for _, t := range leaveTypes {
		if t.Code == code {
			return t, true
		}
	}
	return LeaveType{}, false
}

// LeaveTypeCodes is every enum value, for validation and filters.
func LeaveTypeCodes() []string {
	out := make([]string, len(leaveTypes))
	for i, t := range leaveTypes {
		out[i] = t.Code
	}
	return out
}

// QuotaLeaveTypes are the types with a monthly quota, in catalog order.
func QuotaLeaveTypes() []LeaveType {
	var out []LeaveType
	for _, t := range leaveTypes {
		if t.MonthlyQuota > 0 {
			out = append(out, t)
		}
	}
	return out
}

func leaveText(code string, pick func(LeaveType) string) string {
	if t, ok := LookupLeaveType(code); ok {
		return pick(t)
	}
	return code
}

// LeaveTypeLabel names a leave type (forms, lists, WhatsApp).
func LeaveTypeLabel(code string) string { return leaveText(code, func(t LeaveType) string { return t.Label }) }

// LeaveTypeShort is the report label.
func LeaveTypeShort(code string) string { return leaveText(code, func(t LeaveType) string { return t.Short }) }

// LeaveTypeExportLabel is the CSV export label.
func LeaveTypeExportLabel(code string) string {
	return leaveText(code, func(t LeaveType) string { return t.Export })
}

// leaderLevels are the hris.positions.level values from Supervisor up
// (ladder of the master Posisi form).
var leaderLevels = map[string]bool{
	"supervisor": true, "assistant manager": true, "manager": true, "senior manager": true,
	"general manager": true, "director": true, "c-level": true,
}

// IsLeaderLevel reports whether a position level may take Izin Leader.
func IsLeaderLevel(level string) bool { return leaderLevels[strings.ToLower(strings.TrimSpace(level))] }

// MonthBounds is the first and last date of a YYYY-MM month.
func MonthBounds(month string) (from, to string, ok bool) {
	if len(month) != 7 {
		return "", "", false
	}
	t, err := time.Parse("2006-01", month)
	if err != nil {
		return "", "", false
	}
	return t.Format(DateLayout), t.AddDate(0, 1, -1).Format(DateLayout), true
}

// MonthOf is the YYYY-MM of a YYYY-MM-DD date.
func MonthOf(date string) string {
	if len(date) < 7 {
		return ""
	}
	return date[:7]
}
```

- [ ] **Step 4: Ganti peta label lama**
  - `workforce.go`: hapus `var leaveTypeLabels` dan fungsi `LeaveTypeLabel` (baris 122–136). Fungsi baru di katalog bernama sama, jadi `LeaveRequestWaMessage` tidak berubah.
  - `workforce.go`: hapus `var csvLeaveTypes`. Di `LeavesCSV`, ganti lookup `csvLeaveTypes[r.LeaveType]` (beserta fallback-nya) dengan `LeaveTypeExportLabel(r.LeaveType)`.
  - `insights.go`: hapus `var reportLeaveLabels`. Ganti setiap `reportLeaveLabels[x]` (beserta fallback-nya) dengan `LeaveTypeShort(x)`.
  - `domain_test.go:357,360`: `"marriage"` → `"lainnya"` (dua tempat).

Run: `cd backend && grep -n "leaveTypeLabels\|csvLeaveTypes\|reportLeaveLabels" -r internal/`
Expected: tidak ada hasil.

- [ ] **Step 5: Jalankan test domain**

Run: `cd backend && $HOME/.local/go/bin/go test ./internal/modules/hris/domain/... -v 2>&1 | tail -20`
Expected: PASS semua, termasuk test CSV (`"Cuti Sakit"`) dan laporan (`{"Sakit", 3}, {"Tahunan", 2}`) yang lama.

- [ ] **Step 6: Commit**

```bash
git add backend/internal/modules/hris/domain/leave_types.go backend/internal/modules/hris/domain/leave_types_test.go backend/internal/modules/hris/domain/workforce.go backend/internal/modules/hris/domain/insights.go backend/internal/modules/hris/domain/domain_test.go
git commit -m "refactor(hris): satu katalog jenis cuti di Go, EPIC-058"
```

**Exit criteria:** satu sumber label di Go; test domain lama dan baru PASS; label CSV dan laporan untuk jenis lama tidak berubah.

---

### Task 3: Validasi pengajuan — jenis dihapus, Izin Leader, satu hari, kuota bulanan, tabrakan

**Files:**
- Modify: `backend/internal/modules/hris/http_leaves.go:13-14` (hapus `leaveTypes`; `f.Enum("leave_type", …, domain.LeaveTypeCodes())`)
- Modify: `backend/internal/modules/hris/leaves.go` (`LeaveRepo` + `CreateLeave`)
- Modify: `backend/internal/modules/hris/postgres_leaves.go` (4 method store baru)
- Test: `backend/internal/modules/hris/leave_types_integration_test.go` (tambah 4 test)

**Interfaces:**
- Consumes: `domain.LookupLeaveType`, `domain.IsLeaderLevel`, `domain.MonthBounds`, `domain.MonthOf`, `domain.LeaveTypeLabel` (Task 2).
- Produces (dipakai Task 4), method baru di `LeaveRepo`:
  - `EmployeePositionLevel(ctx context.Context, employeeID string) (level string, found bool, err error)`
  - `LockLeaveEmployee(ctx context.Context, employeeID string) error`
  - `OverlappingLeave(ctx context.Context, employeeID, start, end string) (*Row, error)` — kolom `leave_type`, `status`, `start_date`, `end_date` (teks); `nil` bila tidak ada
  - `MonthlyLeaveUse(ctx context.Context, employeeID, from, to string) (map[string]float64, error)` — `SUM(total_days)` per jenis untuk status pending/approved, `start_date` di `[from, to]`

- [ ] **Step 1: Tulis test yang gagal**

Tambahkan ke `backend/internal/modules/hris/leave_types_integration_test.go`. Tanggal 2031-03-04 = Selasa, 2031-03-31 = Senin, 2031-04-01 = Selasa.

```go
// leaderPosition gives an employee a position with the given level.
func (h *harness) leaderPosition(employeeID, level string) {
	h.t.Helper()
	pos := h.scalar(`INSERT INTO hris.positions (title, level) VALUES ('Posisi Uji '||$1, $1) RETURNING id::text`, level)
	h.exec(`UPDATE hris.employees SET job_title_id = $2 WHERE id = $1`, employeeID, pos)
}

func leaveBody(kind, start, end string) map[string]any {
	return map[string]any{"leave_type": kind, "start_date": start, "end_date": end, "reason": "Keperluan uji EPIC-058"}
}

func TestRemovedLeaveTypesRefusedForNewRequests(t *testing.T) {
	h := newHarness(t)
	staff, _ := h.staff("pos", nil, true)
	for _, kind := range []string{"emergency", "unpaid", "menstrual"} {
		expect(t, h.do(&staff, "POST", "/api/hris/leaves", leaveBody(kind, "2031-03-04", "2031-03-04")), 400,
			"Jenis cuti ini sudah tidak berlaku untuk pengajuan baru")
	}
	// Menikah kini tersimpan (dulu error enum).
	expect(t, h.do(&staff, "POST", "/api/hris/leaves", leaveBody("marriage", "2031-03-04", "2031-03-06")), 200, "")
	expect(t, h.do(&staff, "POST", "/api/hris/leaves", leaveBody("lainnya", "2031-03-10", "2031-03-10")), 400, "Validation failed")
}

func TestRemovedLeaveTypesStayReadable(t *testing.T) {
	h := newHarness(t)
	hr, _ := h.staff("hrd", []string{menuWorkforce}, true)
	_, own := h.staff("pos", nil, true, emp{name: "Riwayat Darurat"})
	id := h.scalar(`INSERT INTO hris.leaves (employee_id, leave_type, start_date, end_date, total_days, reason)
		VALUES ($1, 'emergency', '2031-03-04', '2031-03-04', 1, 'Import Odoo: Cuti Menikah') RETURNING id::text`, own)
	list := h.do(&hr, "GET", "/api/hris/leaves?leave_type=emergency&employee_id="+own, nil)
	expect(t, list, 200, "")
	if len(list.list()) != 1 {
		t.Fatalf("filter emergency = %s", list.raw)
	}
	expect(t, h.do(&hr, "POST", "/api/hris/leaves/approve", map[string]any{"leave_id": id, "action": "approve"}), 200, "")
	csv := h.do(&hr, "GET", "/api/hris/leaves/export?leave_type=emergency", nil)
	expect(t, csv, 200, "")
	if !strings.Contains(csv.raw, `"Cuti Darurat"`) {
		t.Fatalf("csv = %s", csv.raw)
	}
}

func TestLeaderLeaveEligibility(t *testing.T) {
	h := newHarness(t)
	hr, _ := h.staff("hrd", []string{menuWorkforce}, true)
	staff, staffID := h.staff("pos", nil, true)
	spv, spvID := h.staff("pos", nil, true)
	lower, lowerID := h.staff("pos", nil, true)
	_, noPosID := h.staff("pos", nil, true)
	h.leaderPosition(staffID, "Staff")
	h.leaderPosition(spvID, "Supervisor")
	h.leaderPosition(lowerID, "manager")

	expect(t, h.do(&staff, "POST", "/api/hris/leaves", leaveBody("leader", "2031-03-04", "2031-03-04")), 400,
		"Izin Leader hanya untuk jabatan Supervisor ke atas")
	expect(t, h.do(&hr, "POST", "/api/hris/leaves", map[string]any{"employee_id": noPosID, "leave_type": "leader",
		"start_date": "2031-03-04", "end_date": "2031-03-04", "reason": "Diinput HR tanpa jabatan"}), 400,
		"Izin Leader hanya untuk jabatan Supervisor ke atas")
	expect(t, h.do(&spv, "POST", "/api/hris/leaves", leaveBody("leader", "2031-03-04", "2031-03-05")), 400,
		"Izin Leader diajukan per satu hari — tanggal mulai dan selesai harus sama")
	expect(t, h.do(&spv, "POST", "/api/hris/leaves", leaveBody("leader", "2031-03-04", "2031-03-04")), 200, "")
	expect(t, h.do(&lower, "POST", "/api/hris/leaves", leaveBody("leader", "2031-03-04", "2031-03-04")), 200, "")
	// Kuota 1/bulan.
	expect(t, h.do(&spv, "POST", "/api/hris/leaves", leaveBody("leader", "2031-03-11", "2031-03-11")), 400,
		"Kuota Izin Leader bulan 2031-03 sudah habis (1 dari 1 hari)")
}

func TestLeaveMonthlyQuota(t *testing.T) {
	h := newHarness(t)
	hr, _ := h.staff("hrd", []string{menuWorkforce}, true)
	staff, _ := h.staff("pos", nil, true)
	var ids []string
	for _, d := range []string{"2031-03-04", "2031-03-05", "2031-03-06"} {
		res := h.do(&staff, "POST", "/api/hris/leaves", leaveBody("personal", d, d))
		expect(t, res, 200, "")
		ids = append(ids, res.data()["id"].(string))
	}
	expect(t, h.do(&staff, "POST", "/api/hris/leaves", leaveBody("personal", "2031-03-31", "2031-03-31")), 400,
		"Kuota Izin Pribadi bulan 2031-03 sudah habis (3 dari 3 hari)")
	// Bulan lain punya kuota sendiri.
	expect(t, h.do(&staff, "POST", "/api/hris/leaves", leaveBody("personal", "2031-04-01", "2031-04-01")), 200, "")
	// Ditolak → kuota kembali.
	expect(t, h.do(&hr, "POST", "/api/hris/leaves/approve", map[string]any{"leave_id": ids[0], "action": "reject",
		"rejection_reason": "Uji kuota"}), 200, "")
	expect(t, h.do(&staff, "POST", "/api/hris/leaves", leaveBody("personal", "2031-03-31", "2031-03-31")), 200, "")
	// Kombinasi: jenis lain di bulan yang sama tetap boleh, kuotanya terpisah.
	expect(t, h.do(&staff, "POST", "/api/hris/leaves", leaveBody("annual", "2031-03-11", "2031-03-12")), 200, "")
}

func TestLeaveOverlapRefused(t *testing.T) {
	h := newHarness(t)
	hr, _ := h.staff("hrd", []string{menuWorkforce}, true)
	staff, _ := h.staff("pos", nil, true)
	annual := h.do(&staff, "POST", "/api/hris/leaves", leaveBody("annual", "2031-03-04", "2031-03-06"))
	expect(t, annual, 200, "")
	expect(t, h.do(&staff, "POST", "/api/hris/leaves", leaveBody("personal", "2031-03-05", "2031-03-05")), 400,
		"Tanggal ini bertabrakan dengan pengajuan Cuti Tahunan (2031-03-04 s.d. 2031-03-06, menunggu persetujuan)")
	// Bersebelahan boleh (dikombinasikan).
	expect(t, h.do(&staff, "POST", "/api/hris/leaves", leaveBody("personal", "2031-03-07", "2031-03-07")), 200, "")
	// Setelah dibatalkan, tanggalnya bebas lagi.
	id := annual.data()["id"].(string)
	expect(t, h.do(&hr, "PUT", "/api/hris/leaves/"+id, map[string]any{"status": "cancelled"}), 200, "")
	expect(t, h.do(&staff, "POST", "/api/hris/leaves", leaveBody("personal", "2031-03-05", "2031-03-05")), 200, "")
}
```

- [ ] **Step 2: Jalankan, pastikan gagal**

Run: `cd backend && TEST_DATABASE_URL='postgres://postgres@127.0.0.1:55499/paddy_test?sslmode=disable' $HOME/.local/go/bin/go test ./internal/modules/hris -run 'TestRemovedLeaveTypes|TestLeaderLeaveEligibility|TestLeaveMonthlyQuota|TestLeaveOverlapRefused' -v`
Expected: FAIL. Contoh: `emergency` diterima 200, `leader` milik Staff diterima, dan tidak ada tolakan kuota/tabrakan.

- [ ] **Step 3: Method store baru**

Tambahkan ke `backend/internal/modules/hris/postgres_leaves.go` (tambahkan import `errors` bila belum ada):

```go
// EmployeePositionLevel is the level of the employee's position ("" without
// one); found is false when the employee does not exist.
func (s *store) EmployeePositionLevel(ctx context.Context, employeeID string) (string, bool, error) {
	var level *string
	err := s.db.QueryRow(ctx, `SELECT p.level FROM hris.employees e
		LEFT JOIN hris.positions p ON p.id = e.job_title_id WHERE e.id = $1`, employeeID).Scan(&level)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil || level == nil {
		return "", err == nil, err
	}
	return *level, true, nil
}

// LockLeaveEmployee serialises leave requests of one employee so the quota
// and overlap checks cannot both pass for two concurrent requests.
func (s *store) LockLeaveEmployee(ctx context.Context, employeeID string) error {
	_, err := s.db.Exec(ctx, `SELECT 1 FROM hris.employees WHERE id = $1 FOR NO KEY UPDATE`, employeeID)
	return err
}

// OverlappingLeave is a pending/approved request of the employee covering any
// day of [start, end]; nil when none.
func (s *store) OverlappingLeave(ctx context.Context, employeeID, start, end string) (*Row, error) {
	return queryRow(ctx, s.db, `SELECT leave_type::text AS leave_type, status::text AS status,
		       start_date::text AS start_date, end_date::text AS end_date
		FROM hris.leaves
		WHERE employee_id = $1 AND status IN ('pending', 'approved')
		  AND start_date <= $3::date AND end_date >= $2::date
		ORDER BY start_date LIMIT 1`, employeeID, start, end)
}

// MonthlyLeaveUse sums total_days of pending/approved requests per type whose
// start_date falls in [from, to].
func (s *store) MonthlyLeaveUse(ctx context.Context, employeeID, from, to string) (map[string]float64, error) {
	rows, err := s.db.Query(ctx, `SELECT leave_type::text, COALESCE(SUM(total_days), 0)::float8
		FROM hris.leaves
		WHERE employee_id = $1 AND status IN ('pending', 'approved')
		  AND start_date BETWEEN $2::date AND $3::date
		GROUP BY leave_type`, employeeID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	use := map[string]float64{}
	for rows.Next() {
		var kind string
		var days float64
		if err := rows.Scan(&kind, &days); err != nil {
			return nil, err
		}
		use[kind] = days
	}
	return use, rows.Err()
}
```

Tambahkan keempat signature di atas ke interface `LeaveRepo` di `leaves.go`. Kalau ada fake repo di test unit yang mengimplementasi `LeaveRepo`, tambahkan juga stub-nya (cek dengan `go vet ./internal/modules/hris/...`).

- [ ] **Step 4: Validasi di `CreateLeave`**

Di `backend/internal/modules/hris/leaves.go`, `CreateLeave`:

1. Di awal fungsi, sebelum resolusi `employeeID`:

```go
	lt, known := domain.LookupLeaveType(in.LeaveType)
	if !known || !lt.Selectable {
		return nil, httpx.BadRequest("Jenis cuti ini sudah tidak berlaku untuk pengajuan baru")
	}
```

2. Setelah cek `in.EndDate < in.StartDate`:

```go
	if lt.SingleDay && in.StartDate != in.EndDate {
		return nil, httpx.BadRequest(lt.Label + " diajukan per satu hari — tanggal mulai dan selesai harus sama")
	}
	if lt.LeaderOnly {
		level, _, err := s.repo.EmployeePositionLevel(ctx, *employeeID)
		if err != nil {
			return nil, err
		}
		if !domain.IsLeaderLevel(level) {
			return nil, httpx.BadRequest("Izin Leader hanya untuk jabatan Supervisor ke atas")
		}
	}
```

3. Di dalam `s.repo.InTx(ctx, func(r Repository) error { … })`, **sebelum** `r.InsertLeave`:

```go
		if err := r.LockLeaveEmployee(ctx, *employeeID); err != nil {
			return err
		}
		clash, err := r.OverlappingLeave(ctx, *employeeID, in.StartDate, in.EndDate)
		if err != nil {
			return err
		}
		if clash != nil {
			state := "menunggu persetujuan"
			if clash.Str("status") == "approved" {
				state = "disetujui"
			}
			return httpx.BadRequest(fmt.Sprintf("Tanggal ini bertabrakan dengan pengajuan %s (%s s.d. %s, %s)",
				domain.LeaveTypeLabel(clash.Str("leave_type")), clash.Str("start_date"), clash.Str("end_date"), state))
		}
		if lt.MonthlyQuota > 0 {
			month := domain.MonthOf(in.StartDate)
			from, to, _ := domain.MonthBounds(month)
			use, err := r.MonthlyLeaveUse(ctx, *employeeID, from, to)
			if err != nil {
				return err
			}
			if used := use[lt.Code]; used+float64(totalDays) > float64(lt.MonthlyQuota) {
				return httpx.BadRequest(fmt.Sprintf("Kuota %s bulan %s sudah habis (%s dari %d hari)",
					lt.Label, month, strconv.FormatFloat(used, 'f', -1, 64), lt.MonthlyQuota),
					obj("quota", lt.MonthlyQuota, "used", used, "month", month))
			}
		}
```

(Tambahkan import `strconv`.) Variabel `leave` dan `err` di closure yang sudah ada tetap dipakai. Perhatikan `var err error` sudah dideklarasikan di closure; sesuaikan supaya tidak terjadi shadowing yang menelan error.

4. Di `http_leaves.go`: hapus `var leaveTypes`, lalu ganti `f.Enum("leave_type", validate.Rule{}, leaveTypes)` → `f.Enum("leave_type", validate.Rule{}, domain.LeaveTypeCodes())`.

- [ ] **Step 5: Jalankan test baru + test cuti lama**

Run: `cd backend && TEST_DATABASE_URL='postgres://postgres@127.0.0.1:55499/paddy_test?sslmode=disable' $HOME/.local/go/bin/go test ./internal/modules/hris -run 'TestRemovedLeaveTypes|TestLeaderLeaveEligibility|TestLeaveMonthlyQuota|TestLeaveOverlapRefused|TestLeaves|TestLeaveEnumAndNotifyLabels' -v`
Expected: PASS semua. `TestLeaves` lama tidak berubah, karena isinya hanya satu cuti tahunan.

- [ ] **Step 6: Paket hris lengkap**

Run: `cd backend && TEST_DATABASE_URL='postgres://postgres@127.0.0.1:55499/paddy_test?sslmode=disable' $HOME/.local/go/bin/go test ./internal/modules/hris/... 2>&1 | grep -- '--- FAIL' | sort | comm -13 /tmp/epic058-baseline-fails.txt -`
Expected: kosong (tidak ada kegagalan baru dibanding baseline).

- [ ] **Step 7: Commit**

```bash
git add backend/internal/modules/hris/http_leaves.go backend/internal/modules/hris/leaves.go backend/internal/modules/hris/postgres_leaves.go backend/internal/modules/hris/leave_types_integration_test.go
git commit -m "feat(hris): validasi jenis cuti baru — kuota bulanan, Izin Leader, tabrakan tanggal, EPIC-058"
```

**Exit criteria:** tiga jenis lama ditolak untuk pengajuan baru tetapi riwayatnya tetap bisa dibaca dan diproses; Izin Leader hanya untuk Supervisor ke atas; kuota 1 dan 3 per bulan berlaku dan kembali setelah ditolak/dibatalkan; tanggal yang bertabrakan ditolak dan yang bersebelahan diterima; tidak ada kegagalan baru di paket hris.

---

### Task 4: Endpoint `GET /api/hris/leave-quota`

**Files:**
- Modify: `backend/internal/modules/hris/service.go` (field `WorkforceMenu bool` di `Actor`)
- Modify: `backend/internal/modules/hris/leaves.go` (method `LeaveQuota`)
- Modify: `backend/internal/modules/hris/http_leaves.go` (handler `getLeaveQuota`)
- Modify: `backend/internal/modules/hris/routes.go:71` (route baru setelah leave-balances)
- Regenerate: `frontend/src/lib/go-routes.generated.json` (`make -C backend generate`)
- Test: `backend/internal/modules/hris/leave_types_integration_test.go` (`TestLeaveQuotaEndpoint`)

**Interfaces:**
- Consumes: `MonthlyLeaveUse`, `EmployeePositionLevel` (Task 3); `domain.QuotaLeaveTypes`, `domain.IsLeaderLevel`, `domain.MonthBounds`, `domain.NowWIB` (Task 2 / sudah ada).
- Produces: `GET /api/hris/leave-quota?employee_id=<uuid opsional>&month=<YYYY-MM opsional>` →

```json
{ "data": { "employee_id": "…", "month": "2031-03", "position_level": "Supervisor", "leader_eligible": true,
  "types": [ { "leave_type": "personal", "label": "Izin Pribadi", "quota": 3, "used": 1, "remaining": 2, "available": true },
             { "leave_type": "leader", "label": "Izin Leader", "quota": 1, "used": 0, "remaining": 1, "available": true } ] } }
```

Tanpa `employee_id`, karyawan yang dipakai adalah karyawan akun yang login. Tanpa `month`, dipakai bulan berjalan (WIB).

- [ ] **Step 1: Tulis test yang gagal**

```go
func TestLeaveQuotaEndpoint(t *testing.T) {
	h := newHarness(t)
	hr, _ := h.staff("hrd", []string{menuWorkforce}, true)
	hrNoMenu, _ := h.staff("hrd", nil, true)
	spv, spvID := h.staff("pos", nil, true)
	stranger, _ := h.staff("pos", nil, true)
	h.leaderPosition(spvID, "Supervisor")
	expect(t, h.do(&spv, "POST", "/api/hris/leaves", leaveBody("personal", "2031-03-04", "2031-03-04")), 200, "")

	own := h.do(&spv, "GET", "/api/hris/leave-quota?month=2031-03", nil)
	expect(t, own, 200, "")
	d := own.data()
	if d["leader_eligible"] != true || d["month"] != "2031-03" {
		t.Fatalf("quota = %s", own.raw)
	}
	types := d["types"].([]any)
	personal := types[0].(map[string]any)
	if personal["leave_type"] != "personal" || personal["used"] != 1.0 || personal["remaining"] != 2.0 {
		t.Fatalf("personal = %v", personal)
	}
	if strings.Contains(own.raw, "Rp") || strings.Contains(own.raw, "salary") {
		t.Fatalf("kuota tidak boleh memuat angka gaji: %s", own.raw)
	}

	expect(t, h.do(&hr, "GET", "/api/hris/leave-quota?month=2031-03&employee_id="+spvID, nil), 200, "")
	expect(t, h.do(&hrNoMenu, "GET", "/api/hris/leave-quota?month=2031-03&employee_id="+spvID, nil), 403, "Insufficient permissions")
	expect(t, h.do(&stranger, "GET", "/api/hris/leave-quota?month=2031-03&employee_id="+spvID, nil), 403, "Insufficient permissions")
	expect(t, h.do(&spv, "GET", "/api/hris/leave-quota?month=2031-3", nil), 400, "Format bulan harus YYYY-MM")
	staffQ := h.do(&stranger, "GET", "/api/hris/leave-quota?month=2031-03", nil)
	expect(t, staffQ, 200, "")
	if staffQ.data()["leader_eligible"] != false || staffQ.data()["types"].([]any)[1].(map[string]any)["available"] != false {
		t.Fatalf("staff quota = %s", staffQ.raw)
	}
}
```

- [ ] **Step 2: Jalankan, pastikan gagal**

Run: `cd backend && TEST_DATABASE_URL='postgres://postgres@127.0.0.1:55499/paddy_test?sslmode=disable' $HOME/.local/go/bin/go test ./internal/modules/hris -run TestLeaveQuotaEndpoint -v`
Expected: FAIL 404 (route belum ada).

- [ ] **Step 3: Implementasi**

`service.go`, struct `Actor`, tambahkan setelah `PerformanceMenu`:

```go
	// WorkforceMenu: the account holds the Cuti/Absensi menu (hris.workforce);
	// set by handlers that need it (EPIC-058 leave quota).
	WorkforceMenu bool
```

`leaves.go`:

```go
// LeaveQuota is the monthly quota of Izin Pribadi / Izin Leader (EPIC-058):
// the employee themself, or an HR account holding the Cuti menu. It carries
// days only, never salary amounts.
func (s *Service) LeaveQuota(ctx context.Context, a *Actor, employeeID *string, month string) (*Row, error) {
	target := a.EmployeeID
	if employeeID != nil && *employeeID != "" {
		target = employeeID
	}
	if target == nil {
		return nil, httpx.Forbidden("Akun ini tidak terhubung ke data karyawan")
	}
	own := a.EmployeeID != nil && *target == *a.EmployeeID
	if !own && !(a.IsHR && a.WorkforceMenu) {
		return nil, httpx.Forbidden("Insufficient permissions")
	}
	if month == "" {
		month = domain.NowWIB(s.now()).Format("2006-01")
	}
	from, to, ok := domain.MonthBounds(month)
	if !ok {
		return nil, httpx.BadRequest("Format bulan harus YYYY-MM")
	}
	level, found, err := s.repo.EmployeePositionLevel(ctx, *target)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, httpx.NotFound("Employee not found")
	}
	use, err := s.repo.MonthlyLeaveUse(ctx, *target, from, to)
	if err != nil {
		return nil, err
	}
	eligible := domain.IsLeaderLevel(level)
	types := []any{}
	for _, t := range domain.QuotaLeaveTypes() {
		used := use[t.Code]
		types = append(types, obj("leave_type", t.Code, "label", t.Label, "quota", t.MonthlyQuota,
			"used", used, "remaining", max(0, float64(t.MonthlyQuota)-used), "available", !t.LeaderOnly || eligible))
	}
	return obj("employee_id", *target, "month", month, "position_level", level,
		"leader_eligible", eligible, "types", types), nil
}
```

`http_leaves.go`:

```go
func (h handlers) getLeaveQuota(w http.ResponseWriter, r *http.Request, a *Actor) error {
	f := queryForm(r)
	employeeID := f.Str("employee_id", optional, validate.StrOpts{})
	month := strOr(f.Str("month", optional, validate.StrOpts{}))
	if err := formErr(f, ""); err != nil {
		return err
	}
	granted, err := h.auth.GrantedMenuCodes(r.Context(), a.UserID, a.Role)
	if err != nil {
		return err
	}
	a.WorkforceMenu = iam.HasAnyMenuPrefix(granted, iam.HrisWorkforce)
	row, err := h.svc.LeaveQuota(r.Context(), a, employeeID, month)
	if err != nil {
		return err
	}
	return ok(w, dataOf(row))
}
```

(Tambahkan import `nuhabit/backend/internal/platform/iam`. Kalau `strOr` tidak menerima `*string` nil, pakai pola yang sama dengan `leaveFilterOf`.)

`routes.go`, setelah baris `PUT /api/hris/leave-balances/{employee_id}`:

```go
		r("GET /api/hris/leave-quota", h.withActor(h.getLeaveQuota)),
```

- [ ] **Step 4: Regenerate manifest route, jalankan test**

Run: `make -C backend generate && cd backend && TEST_DATABASE_URL='postgres://postgres@127.0.0.1:55499/paddy_test?sslmode=disable' $HOME/.local/go/bin/go test ./internal/modules/hris -run 'TestLeaveQuotaEndpoint|TestLeaveMonthlyQuota' -v && git diff --stat ../frontend/src/lib/go-routes.generated.json`
Expected: PASS; manifest bertambah satu entri `/api/hris/leave-quota`.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/modules/hris/service.go backend/internal/modules/hris/leaves.go backend/internal/modules/hris/http_leaves.go backend/internal/modules/hris/routes.go backend/internal/modules/hris/leave_types_integration_test.go frontend/src/lib/go-routes.generated.json
git commit -m "feat(hris): endpoint sisa kuota Izin Pribadi/Leader per bulan, EPIC-058"
```

**Exit criteria:** pemilik dan HR bermenu Cuti bisa membaca kuota; HR tanpa menu dan karyawan lain 403; respons tanpa angka gaji; manifest route diperbarui.

---

### Task 5: Payroll — kunci perilaku Izin Leader & Izin Pribadi

**Files:**
- Test: `backend/internal/modules/payroll/domain/parity_test.go` (tambah `TestLeaderAndPersonalLeavePayroll`)
- Modify (komentar saja): `backend/internal/modules/payroll/domain/inputs.go:192` (catat bahwa `leader` sengaja tidak dihitung)

**Interfaces:**
- Consumes: `BuildInput`, `Calculate`, `DefaultConfig`, `parityInput` (sudah ada di paket `payroll/domain`).
- Produces: tidak ada kode produksi baru.

- [ ] **Step 1: Tulis test**

Tambahkan ke `parity_test.go`:

```go
// EPIC-058: Izin Leader tidak memotong apa pun; Izin Pribadi satu hari
// memotong THP / pembagi × 1 (rule PIP EPIC-055, aktif lewat setting).
func TestLeaderAndPersonalLeavePayroll(t *testing.T) {
	emp := Employee{ID: "e", JoinDate: "2025-01-01", EmploymentStatus: "permanent"}
	sal := Salary{BaseSalary: "2600000.00", WorkingDaysDivisor: "26.00"}
	rec := WorkforceRecords{Leaves: []Leave{
		{ptr("2026-09-08"), ptr("2026-09-08"), "leader"},
		{ptr("2026-09-09"), ptr("2026-09-09"), "personal"},
		{ptr("2026-09-10"), ptr("2026-09-10"), "marriage"},
	}}
	in := BuildInput(emp, sal, rec, nil, nil, 9, 2026, false)
	if in.PersonalLeaveDays != 1 || in.UnpaidLeaveDays != 0 || in.SickLeaveDays != 0 {
		t.Fatalf("leave days personal=%v unpaid=%v sick=%v", in.PersonalLeaveDays, in.UnpaidLeaveDays, in.SickLeaveDays)
	}

	c := DefaultConfig()
	c.PersonalLeaveDeduction = true
	r := Calculate(parityInput(func(in *Input) {
		in.BaseSalary, in.DivisorDays, in.PersonalLeaveDays = 2_600_000, 26, 1
	}), c)
	eq(t, "PIP satu hari", r.PersonalLeaveDeduction, Round(r.GrossSalary/26))
	none := Calculate(parityInput(func(in *Input) { in.BaseSalary, in.DivisorDays = 2_600_000, 26 }), c)
	eq(t, "leader tanpa potongan", none.PersonalLeaveDeduction+none.UnpaidLeaveDeduction+none.SickLeaveDeduction, 0)
}
```

Catatan: `r.GrossSalary` sama dengan THP selama `parityInput` tidak mengisi komponen non-pajak manual. Kalau `parityInput` ternyata mengisinya, ganti dengan `(r.GrossSalary + nontax) / 26` sesuai rumus di `calculator.go:381`.

- [ ] **Step 2: Jalankan**

Run: `cd backend && $HOME/.local/go/bin/go test ./internal/modules/payroll/domain -run 'TestLeaderAndPersonalLeavePayroll|TestLeaveDeductions|TestBuildInputParity' -v`
Expected: PASS tanpa perubahan kode produksi. Kalau FAIL, **berhenti** dan laporkan, karena berarti rumus PIP tidak seperti yang dicatat di rencana ini.

- [ ] **Step 3: Komentar di `inputs.go`**

Di atas `switch l.LeaveType` (baris ±192):

```go
		// Hanya unpaid/personal/sick yang memotong. leader (Izin Leader) dan
		// jenis cuti lain sengaja tidak dihitung (EPIC-058).
```

- [ ] **Step 4: Commit**

```bash
git add backend/internal/modules/payroll/domain/parity_test.go backend/internal/modules/payroll/domain/inputs.go
git commit -m "test(payroll): Izin Leader tanpa potongan, Izin Pribadi = 1 hari THP, EPIC-058"
```

**Exit criteria:** test mengunci bahwa `leader` dan `marriage` tidak memotong, dan Izin Pribadi satu hari = THP / pembagi.

---

### Task 6: Katalog jenis cuti frontend + ganti peta label lama

**Files:**
- Create: `frontend/src/lib/hris/leave-types.ts`
- Create: `frontend/src/lib/hris/leave-types.test.ts`
- Modify: `frontend/src/types/hris.ts:378-389, 544-556` (`LeaveType` & `LEAVE_TYPE_LABELS` di-re-export dari katalog)
- Modify: `frontend/src/lib/hris/leave-wa.ts:20-36` (`LEAVE_TYPE_LABELS`/`leaveTypeLabel` dari katalog)
- Modify: `frontend/src/lib/hris/leaves-export.ts:9-19` (pakai `leaveTypeExportLabel`)
- Modify: `frontend/src/lib/hris/leaves-export.test.ts:38,40` (`"marriage"` → `"lainnya"`)
- Modify: `frontend/src/lib/hris/reports-summary.ts:44-56` (pakai `leaveTypeShort`)
- Modify: `frontend/src/features/hris/ess/components/beranda-widgets.tsx:22-25` (pakai `leaveTypeLabel`)
- Modify: `frontend/src/lib/hris/leaves-repo.ts:21-24,38` (zod `leave_type` = kode yang bisa diajukan)

**Interfaces:**
- Produces (`@/lib/hris/leave-types`):
  - `type LeaveTypeCode = "annual" | "sick" | "personal" | "leader" | "maternity" | "paternity" | "pilgrimage" | "marriage" | "bereavement" | "emergency" | "unpaid" | "menstrual"`
  - `interface LeaveTypeInfo { code: LeaveTypeCode; label: string; short: string; exportLabel: string; selectable: boolean; monthlyQuota?: number; singleDay?: boolean; leaderOnly?: boolean; deductsThpDay?: boolean }`
  - `const LEAVE_TYPES: readonly LeaveTypeInfo[]`
  - `const LEAVE_TYPE_LABELS: Record<LeaveTypeCode, string>`
  - `const SELECTABLE_LEAVE_CODES: readonly [LeaveTypeCode, ...LeaveTypeCode[]]`
  - `function leaveTypeLabel(code: string): string`, `leaveTypeShort(code: string): string`, `leaveTypeExportLabel(code: string): string`
  - `function leaveTypeInfo(code: string): LeaveTypeInfo | undefined`
  - `function leaveTypeOptions(opts: { leaderEligible: boolean }): { value: LeaveTypeCode; label: string }[]`

- [ ] **Step 1: Tulis test yang gagal**

`frontend/src/lib/hris/leave-types.test.ts`:

```ts
import { describe, expect, it } from "vitest";
import {
  LEAVE_TYPES,
  SELECTABLE_LEAVE_CODES,
  leaveTypeExportLabel,
  leaveTypeInfo,
  leaveTypeLabel,
  leaveTypeOptions,
  leaveTypeShort,
} from "./leave-types";

describe("katalog jenis cuti (EPIC-058)", () => {
  it("hanya jenis yang berlaku yang bisa diajukan, urut form", () => {
    expect([...SELECTABLE_LEAVE_CODES]).toEqual([
      "annual", "sick", "personal", "leader", "maternity", "paternity", "pilgrimage", "marriage", "bereavement",
    ]);
    expect(LEAVE_TYPES).toHaveLength(12);
  });

  it("jenis yang dihapus tetap punya label riwayat", () => {
    expect(leaveTypeLabel("emergency")).toBe("Izin Darurat");
    expect(leaveTypeLabel("unpaid")).toBe("Izin Tanpa Gaji");
    expect(leaveTypeLabel("menstrual")).toBe("Cuti Haid");
    expect(leaveTypeInfo("menstrual")?.selectable).toBe(false);
  });

  it("label pendek & ekspor sama dengan katalog Go", () => {
    expect(leaveTypeShort("annual")).toBe("Tahunan");
    expect(leaveTypeExportLabel("sick")).toBe("Cuti Sakit");
    expect(leaveTypeExportLabel("emergency")).toBe("Cuti Darurat");
    expect(leaveTypeLabel("lainnya")).toBe("lainnya");
  });

  it("aturan Izin Leader & Izin Pribadi", () => {
    expect(leaveTypeInfo("leader")).toMatchObject({ monthlyQuota: 1, singleDay: true, leaderOnly: true });
    expect(leaveTypeInfo("personal")).toMatchObject({ monthlyQuota: 3, singleDay: true, deductsThpDay: true });
  });

  it("opsi form menyembunyikan Izin Leader untuk non-SPV", () => {
    const staff = leaveTypeOptions({ leaderEligible: false }).map((o) => o.value);
    const spv = leaveTypeOptions({ leaderEligible: true }).map((o) => o.value);
    expect(staff).not.toContain("leader");
    expect(spv).toContain("leader");
    expect(spv).not.toContain("emergency");
    expect(spv).not.toContain("unpaid");
    expect(spv).not.toContain("menstrual");
  });
});
```

- [ ] **Step 2: Jalankan, pastikan gagal**

Run: `cd frontend && npx vitest run src/lib/hris/leave-types.test.ts`
Expected: FAIL `Failed to resolve import "./leave-types"`.

- [ ] **Step 3: Tulis katalog**

`frontend/src/lib/hris/leave-types.ts`:

```ts
/**
 * Katalog jenis cuti (EPIC-058). Cermin dari
 * backend/internal/modules/hris/domain/leave_types.go: ubah keduanya bersamaan.
 * Jenis dengan `selectable: false` sudah dihapus untuk pengajuan baru, tetapi
 * tetap dikenal supaya riwayat terbaca.
 */

export type LeaveTypeCode =
  | "annual" | "sick" | "personal" | "leader" | "maternity" | "paternity"
  | "pilgrimage" | "marriage" | "bereavement" | "emergency" | "unpaid" | "menstrual";

export interface LeaveTypeInfo {
  code: LeaveTypeCode;
  label: string;
  short: string;
  exportLabel: string;
  selectable: boolean;
  monthlyQuota?: number;
  singleDay?: boolean;
  leaderOnly?: boolean;
  /** Memotong 1 hari THP per hari izin (rule PIP payroll). */
  deductsThpDay?: boolean;
}

export const LEAVE_TYPES: readonly LeaveTypeInfo[] = [
  { code: "annual", label: "Cuti Tahunan", short: "Tahunan", exportLabel: "Cuti Tahunan", selectable: true },
  { code: "sick", label: "Sakit", short: "Sakit", exportLabel: "Cuti Sakit", selectable: true },
  { code: "personal", label: "Izin Pribadi", short: "Izin Pribadi", exportLabel: "Izin Pribadi", selectable: true,
    monthlyQuota: 3, singleDay: true, deductsThpDay: true },
  { code: "leader", label: "Izin Leader", short: "Izin Leader", exportLabel: "Izin Leader", selectable: true,
    monthlyQuota: 1, singleDay: true, leaderOnly: true },
  { code: "maternity", label: "Cuti Melahirkan", short: "Melahirkan", exportLabel: "Cuti Melahirkan", selectable: true },
  { code: "paternity", label: "Cuti Ayah", short: "Ayah", exportLabel: "Cuti Ayah", selectable: true },
  { code: "pilgrimage", label: "Cuti Ibadah Haji/Umrah", short: "Haji/Umrah", exportLabel: "Cuti Haji/Umrah", selectable: true },
  { code: "marriage", label: "Cuti Menikah", short: "Pernikahan", exportLabel: "Cuti Menikah", selectable: true },
  { code: "bereavement", label: "Cuti Duka", short: "Duka Cita", exportLabel: "Cuti Duka", selectable: true },
  // Dihapus untuk pengajuan baru (EPIC-058); riwayat tetap terbaca.
  { code: "emergency", label: "Izin Darurat", short: "Darurat", exportLabel: "Cuti Darurat", selectable: false },
  { code: "unpaid", label: "Izin Tanpa Gaji", short: "Tidak Dibayar", exportLabel: "Cuti Tanpa Upah", selectable: false },
  { code: "menstrual", label: "Cuti Haid", short: "Haid", exportLabel: "Cuti Haid", selectable: false },
];

const byCode = new Map<string, LeaveTypeInfo>(LEAVE_TYPES.map((t) => [t.code, t]));

export const LEAVE_TYPE_LABELS = Object.fromEntries(
  LEAVE_TYPES.map((t) => [t.code, t.label])
) as Record<LeaveTypeCode, string>;

export const SELECTABLE_LEAVE_CODES = LEAVE_TYPES.filter((t) => t.selectable).map((t) => t.code) as unknown as readonly [
  LeaveTypeCode,
  ...LeaveTypeCode[],
];

export const leaveTypeInfo = (code: string): LeaveTypeInfo | undefined => byCode.get(code);
export const leaveTypeLabel = (code: string): string => byCode.get(code)?.label ?? code;
export const leaveTypeShort = (code: string): string => byCode.get(code)?.short ?? code;
export const leaveTypeExportLabel = (code: string): string => byCode.get(code)?.exportLabel ?? code;

/** Opsi form pengajuan: hanya jenis yang berlaku; Izin Leader hanya untuk Supervisor ke atas. */
export function leaveTypeOptions({ leaderEligible }: { leaderEligible: boolean }) {
  return LEAVE_TYPES.filter((t) => t.selectable && (!t.leaderOnly || leaderEligible)).map((t) => ({
    value: t.code,
    label: t.label,
  }));
}
```

- [ ] **Step 4: Ganti peta label lama**
  - `types/hris.ts`: hapus union `LeaveType` dan objek `LEAVE_TYPE_LABELS`, lalu ganti dengan:
    ```ts
    export type { LeaveTypeCode as LeaveType } from "@/lib/hris/leave-types";
    export { LEAVE_TYPE_LABELS } from "@/lib/hris/leave-types";
    ```
    Kalau `LeaveType` dipakai di file yang sama (`interface Leave`), tambahkan `import type { LeaveTypeCode as LeaveType } from "@/lib/hris/leave-types";`.
  - `leave-wa.ts`: hapus `LEAVE_TYPE_LABELS` dan `leaveTypeLabel` lokal, lalu `export { LEAVE_TYPE_LABELS, leaveTypeLabel } from "./leave-types";` (nama ekspor tetap, sehingga `leave-wa.test.ts` tidak berubah).
  - `leaves-export.ts`: hapus `LEAVE_TYPE_LABELS` lokal; pemakainya diganti `leaveTypeExportLabel(row.leave_type)`.
  - `leaves-export.test.ts:38,40`: `"marriage"` → `"lainnya"`.
  - `reports-summary.ts`: hapus `LEAVE_TYPE_LABELS` lokal; pemakainya diganti `leaveTypeShort(type)`.
  - `beranda-widgets.tsx`: hapus `LEAVE_TYPE_LABELS` lokal; pemakainya diganti `leaveTypeLabel(code)`. Perubahan tampilan yang disengaja: "Tanpa Gaji" → "Izin Tanpa Gaji", "Darurat" → "Izin Darurat", "Ibadah" → "Cuti Ibadah Haji/Umrah".
  - `leaves-repo.ts`: hapus `LEAVE_TYPES` lokal, lalu `leave_type: z.enum(SELECTABLE_LEAVE_CODES)` (route fallback Next menolak jenis yang dihapus; kuota dan tabrakan hanya dicek di Go).

Run: `cd frontend && grep -rn "menstrual" src --include=*.ts --include=*.tsx | grep -v "leave-types"`
Expected: tidak ada hasil.

- [ ] **Step 5: Jalankan test & tsc**

Run: `cd frontend && npx vitest run src/lib/hris src/features/hris && npx tsc --noEmit -p . 2>&1 | tail -5`
Expected: vitest PASS (`leave-types`, `leave-wa`, `leaves-export`, `reports-summary`); tsc tanpa error baru.

- [ ] **Step 6: Commit**

```bash
git add frontend/src/lib/hris/leave-types.ts frontend/src/lib/hris/leave-types.test.ts frontend/src/types/hris.ts frontend/src/lib/hris/leave-wa.ts frontend/src/lib/hris/leaves-export.ts frontend/src/lib/hris/leaves-export.test.ts frontend/src/lib/hris/reports-summary.ts frontend/src/features/hris/ess/components/beranda-widgets.tsx frontend/src/lib/hris/leaves-repo.ts
git commit -m "refactor(hris): satu katalog jenis cuti di frontend, EPIC-058"
```

**Exit criteria:** tidak ada lagi peta label jenis cuti di luar `leave-types.ts`; label riwayat tetap; vitest & tsc bersih.

---

### Task 7: Form pengajuan ESS & HR — opsi, aturan satu hari, sisa kuota

**Files:**
- Modify: `frontend/src/features/hris/leaves/types.ts` (tipe `LeaveQuota`)
- Modify: `frontend/src/features/hris/leaves/api.ts` (`fetchLeaveQuota`)
- Modify: `frontend/src/features/hris/leaves/query-keys.ts` (`quota`)
- Modify: `frontend/src/features/hris/leaves/queries.ts` (`useLeaveQuota`)
- Modify: `frontend/src/features/hris/ess/components/cuti-request-dialog.tsx` (opsi dari katalog, kuota, satu hari)
- Modify: `frontend/src/features/hris/ess/components/cuti-page.tsx:8,24` (label riwayat via `leaveTypeLabel`, bukan `LEAVE_TYPE_OPTIONS`)
- Modify: `frontend/src/features/hris/leaves/components/new-leave-page.tsx:163-177` (opsi dari katalog + kuota karyawan terpilih)
- Modify: `frontend/src/features/hris/leaves/components/leaves-page.tsx:142` (filter: semua jenis, yang dihapus diberi akhiran " (lama)")
- Create: `frontend/src/lib/hris/leave-form.ts` + `frontend/src/lib/hris/leave-form.test.ts` (logika murni form)

**Interfaces:**
- Consumes: `GET /api/hris/leave-quota` (Task 4); `leaveTypeInfo`, `leaveTypeOptions`, `leaveTypeLabel`, `LEAVE_TYPES` (Task 6).
- Produces:
  - `interface LeaveQuota { employee_id: string; month: string; position_level: string; leader_eligible: boolean; types: { leave_type: string; label: string; quota: number; used: number; remaining: number; available: boolean }[] }`
  - `fetchLeaveQuota(month: string, employeeId?: string): Promise<LeaveQuota | null>`
  - `useLeaveQuota(month: string, employeeId?: string, enabled?: boolean)`
  - `leaveFormNote(code: string, quota: LeaveQuota | null): string | null` dan `applySingleDay(code: string, start: string, end: string): { start: string; end: string }` di `@/lib/hris/leave-form`

- [ ] **Step 1: Tulis test yang gagal**

`frontend/src/lib/hris/leave-form.test.ts`:

```ts
import { describe, expect, it } from "vitest";
import { applySingleDay, leaveFormNote } from "./leave-form";
import type { LeaveQuota } from "@/features/hris/leaves/types";

const quota: LeaveQuota = {
  employee_id: "e", month: "2031-03", position_level: "Supervisor", leader_eligible: true,
  types: [
    { leave_type: "personal", label: "Izin Pribadi", quota: 3, used: 1, remaining: 2, available: true },
    { leave_type: "leader", label: "Izin Leader", quota: 1, used: 1, remaining: 0, available: true },
  ],
};

describe("leaveFormNote", () => {
  it("Izin Pribadi: sisa kuota + memotong 1 hari THP, tanpa angka rupiah", () => {
    const note = leaveFormNote("personal", quota)!;
    expect(note).toContain("Sisa kuota Izin Pribadi bulan 2031-03: 2 dari 3 hari");
    expect(note).toContain("Memotong 1 hari THP");
    expect(note).not.toMatch(/Rp/);
  });
  it("Izin Leader habis: tidak memotong gaji, kuota habis", () => {
    const note = leaveFormNote("leader", quota)!;
    expect(note).toContain("Sisa kuota Izin Leader bulan 2031-03: 0 dari 1 hari");
    expect(note).toContain("Tidak memotong gaji");
  });
  it("jenis tanpa kuota: tidak ada catatan", () => {
    expect(leaveFormNote("annual", quota)).toBeNull();
    expect(leaveFormNote("personal", null)).toBeNull();
  });
});

describe("applySingleDay", () => {
  it("jenis satu hari menyamakan tanggal selesai dengan tanggal mulai", () => {
    expect(applySingleDay("personal", "2031-03-04", "2031-03-06")).toEqual({ start: "2031-03-04", end: "2031-03-04" });
    expect(applySingleDay("annual", "2031-03-04", "2031-03-06")).toEqual({ start: "2031-03-04", end: "2031-03-06" });
  });
});
```

- [ ] **Step 2: Jalankan, pastikan gagal**

Run: `cd frontend && npx vitest run src/lib/hris/leave-form.test.ts`
Expected: FAIL `Failed to resolve import "./leave-form"`.

- [ ] **Step 3: Data layer kuota**

`features/hris/leaves/types.ts`, tambahkan:

```ts
/** GET /api/hris/leave-quota (EPIC-058): kuota bulanan, hari saja (tanpa angka gaji). */
export interface LeaveQuota {
  employee_id: string;
  month: string;
  position_level: string;
  leader_eligible: boolean;
  types: { leave_type: string; label: string; quota: number; used: number; remaining: number; available: boolean }[];
}
```

`features/hris/leaves/api.ts`:

```ts
export const fetchLeaveQuota = (month: string, employeeId?: string) => {
  const qs = new URLSearchParams({ month });
  if (employeeId) qs.set("employee_id", employeeId);
  return apiGet<{ data: LeaveQuota }>(`/api/hris/leave-quota?${qs}`)
    .then((res) => res.data)
    .catch(() => null);
};
```

`query-keys.ts`: `quota: (month: string, employeeId?: string) => ["hris", "leaves", "quota", month, employeeId ?? "me"] as const,`

`queries.ts`:

```ts
export const useLeaveQuota = (month: string, employeeId?: string, enabled = true) =>
  useQuery({
    queryKey: leavesQueryKeys.quota(month, employeeId),
    queryFn: () => fetchLeaveQuota(month, employeeId),
    enabled: enabled && /^\d{4}-\d{2}$/.test(month),
  });
```

Mutation pengajuan (`useSubmitMyLeave` di ESS dan `useCreateLeave` di HR) juga harus meng-invalidate `["hris", "leaves", "quota"]` di `onSuccess`, supaya sisa kuota langsung berkurang.

- [ ] **Step 4: Logika form murni**

`frontend/src/lib/hris/leave-form.ts`:

```ts
import type { LeaveQuota } from "@/features/hris/leaves/types";
import { leaveTypeInfo } from "./leave-types";

/** Catatan di bawah form untuk jenis berkuota (EPIC-058). Hari saja, tanpa rupiah. */
export function leaveFormNote(code: string, quota: LeaveQuota | null): string | null {
  const info = leaveTypeInfo(code);
  if (!info?.monthlyQuota || !quota) return null;
  const q = quota.types.find((t) => t.leave_type === code);
  if (!q) return null;
  const effect = info.deductsThpDay ? "Memotong 1 hari THP per hari izin." : "Tidak memotong gaji.";
  return `Sisa kuota ${info.label} bulan ${quota.month}: ${q.remaining} dari ${q.quota} hari. ${effect} Diajukan per satu hari.`;
}

/** Jenis satu hari: tanggal selesai mengikuti tanggal mulai. */
export function applySingleDay(code: string, start: string, end: string) {
  return leaveTypeInfo(code)?.singleDay ? { start, end: start } : { start, end };
}
```

- [ ] **Step 5: Pasang di form ESS**

`cuti-request-dialog.tsx`:
- Hapus `LEAVE_TYPE_OPTIONS` beserta ekspornya; perbarui import di `cuti-page.tsx` (label riwayat → `leaveTypeLabel` dari `@/lib/hris/leave-types`).
- `const month = (form.start_date || todayWib()).slice(0, 7);` (`todayWib` dari `@/lib/dates`).
- `const quota = useLeaveQuota(month, undefined, open).data ?? null;`
- Opsi: `options={leaveTypeOptions({ leaderEligible: quota?.leader_eligible ?? false })}`.
- Saat jenis atau tanggal mulai berubah, terapkan `applySingleDay`. Untuk jenis satu hari, input "Selesai" di-`disabled` dan nilainya = tanggal mulai.
- Di bawah blok pratinjau hari, tampilkan `leaveFormNote(form.leave_type, quota)` (kalau tidak null) dalam kotak `rounded-lg bg-sky-50 px-3 py-2 text-xs text-sky-700`, mengikuti gaya catatan cuti tahunan.
- Server tetap menjadi penentu: pesan 400 dari Go (kuota habis, tabrakan, bukan SPV) sudah muncul lewat `toast.error(error.message)`.

- [ ] **Step 6: Pasang di form HR**

`new-leave-page.tsx`:
- `const month = (formData.start_date || todayWib()).slice(0, 7);`
- `const quota = useLeaveQuota(month, formData.employee_id || undefined, Boolean(formData.employee_id)).data ?? null;`
- Ganti `Object.entries(LEAVE_TYPE_LABELS)` dengan `leaveTypeOptions({ leaderEligible: quota?.leader_eligible ?? false })`.
- Kalau karyawan diganti dan jenis yang terpilih tidak ada lagi di opsi, reset ke `"annual"`.
- Terapkan aturan satu hari dan catatan kuota yang sama seperti Step 5.

`leaves-page.tsx:142` (filter daftar HR): pakai `LEAVE_TYPES.map((t) => ({ value: t.code, label: t.selectable ? t.label : \`${t.label} (lama)\` }))`, supaya riwayat jenis yang dihapus tetap bisa difilter.

- [ ] **Step 7: Jalankan test, tsc, lint**

Run: `cd frontend && npx vitest run src/lib/hris src/features/hris && npx tsc --noEmit -p . 2>&1 | tail -5 && npx eslint src/features/hris/ess src/features/hris/leaves src/lib/hris 2>&1 | tail -5`
Expected: PASS; tanpa error tsc/eslint baru.

- [ ] **Step 8: Commit**

```bash
git add frontend/src/lib/hris/leave-form.ts frontend/src/lib/hris/leave-form.test.ts frontend/src/features/hris/leaves frontend/src/features/hris/ess/components/cuti-request-dialog.tsx frontend/src/features/hris/ess/components/cuti-page.tsx frontend/src/features/hris/ess/mutations.ts
git commit -m "feat(hris): form cuti ESS & HR — Izin Leader/Pribadi, sisa kuota, satu hari, EPIC-058"
```

**Exit criteria:** form ESS dan HR hanya menawarkan jenis yang berlaku; Izin Leader hanya muncul untuk Supervisor ke atas; jenis satu hari mengunci tanggal selesai; sisa kuota tampil tanpa angka rupiah; filter HR masih bisa memilih jenis lama.

---

### Task 8: Data level jabatan, uji menyeluruh di stack salinan DB, dokumentasi

**Files:**
- Create: `backend/database/sql/checks/2026-10-20_epic058_leave_types_readonly.sql`
- Modify: `docs/epics/EPIC-058-hris-revisi-jenis-cuti.md` (Automation Log)

**Interfaces:** tidak ada kode produksi.

- [ ] **Step 1: Skrip cek read-only**

`backend/database/sql/checks/2026-10-20_epic058_leave_types_readonly.sql`:

```sql
-- EPIC-058 — cek read-only sebelum rilis revisi jenis cuti. Tidak menulis apa pun.
BEGIN READ ONLY;

-- 1. Riwayat jenis yang dihapus (per status).
SELECT leave_type::text, status::text, count(*) AS jumlah, COALESCE(sum(total_days), 0) AS hari
FROM hris.leaves
WHERE leave_type::text IN ('emergency', 'unpaid', 'menstrual')
GROUP BY 1, 2 ORDER BY 1, 2;

-- 2. Pengajuan pending jenis yang dihapus: tetap bisa diproses, tetapi HR perlu tahu.
SELECT l.id, e.nip, e.full_name, l.leave_type::text, l.start_date, l.end_date, l.reason
FROM hris.leaves l JOIN hris.employees e ON e.id = l.employee_id
WHERE l.status = 'pending' AND l.leave_type::text IN ('emergency', 'unpaid', 'menstrual')
ORDER BY l.start_date;

-- 3. Sebaran level jabatan karyawan aktif.
SELECT COALESCE(p.level, '(tanpa jabatan)') AS level, count(*) AS karyawan
FROM hris.employees e LEFT JOIN hris.positions p ON p.id = e.job_title_id
WHERE e.is_active GROUP BY 1 ORDER BY 2 DESC;

-- 4. Jabatan yang judulnya seperti SPV ke atas tetapi levelnya masih Staff/kosong.
SELECT p.id, p.title, p.level, count(e.id) FILTER (WHERE e.is_active) AS karyawan_aktif
FROM hris.positions p LEFT JOIN hris.employees e ON e.job_title_id = p.id
WHERE p.title ~* '(supervisor|spv|manager|head|chief|director|direktur|general manager|ceo)'
  AND lower(trim(COALESCE(p.level, ''))) NOT IN
      ('supervisor', 'assistant manager', 'manager', 'senior manager', 'general manager', 'director', 'c-level')
GROUP BY p.id ORDER BY p.title;

-- 5. Tanggal yang sudah tumpang tindih (pending/approved): akan menolak pengajuan baru di tanggal itu.
SELECT a.employee_id, a.id AS leave_a, b.id AS leave_b, a.start_date, a.end_date, b.start_date, b.end_date
FROM hris.leaves a JOIN hris.leaves b
  ON a.employee_id = b.employee_id AND a.id < b.id
 AND a.status IN ('pending', 'approved') AND b.status IN ('pending', 'approved')
 AND a.start_date <= b.end_date AND b.start_date <= a.end_date
LIMIT 50;

ROLLBACK;
```

Run (DB test): `psql 'postgres://postgres@127.0.0.1:55499/paddy_test?sslmode=disable' -f backend/database/sql/checks/2026-10-20_epic058_leave_types_readonly.sql`
Expected: berjalan tanpa error. Hasilnya kosong atau kecil karena DB test tidak punya data cuti.

- [ ] **Step 2: Suite penuh**

Run: `cd backend && TEST_DATABASE_URL='postgres://postgres@127.0.0.1:55499/paddy_test?sslmode=disable' $HOME/.local/go/bin/go test ./... > /tmp/epic058-go.txt 2>&1; grep -- '--- FAIL' /tmp/epic058-go.txt | sort | comm -13 /tmp/epic058-baseline-fails.txt -; cd ../frontend && npx vitest run 2>&1 | tail -5 && npx tsc --noEmit -p . 2>&1 | tail -3`
Expected: tidak ada kegagalan Go baru dibanding baseline Task 1; vitest PASS; tsc bersih.

- [ ] **Step 3: Stack salinan DB (pola EPIC-056/057)**
  - Dump live ke `ui-db` (port 55521). Ini hanya **membaca** live (`pg_dump`) dan perlu izin user.
  - Bangun `ui-api`/`ui-web` dari branch di `127.0.0.1:8148`, lalu jalankan migrasi di **salinan**.
  - Jalankan skrip Step 1 di salinan dan simpan hasilnya (angka saja, tanpa nama karyawan) untuk dilaporkan.
  - Di salinan, set level satu jabatan uji ke `Supervisor`. Buat dua akun uji di salinan saja: karyawan Staff dan karyawan Supervisor, masing-masing role `employee`, dengan password sementara. Akun uji dihapus bersama stack.

- [ ] **Step 4: Cek manual (Playwright, di salinan)**

| Skenario | Harapan |
|---|---|
| ESS Staff buka "Ajukan Izin / Cuti" | Opsi tanpa Izin Leader, Izin Darurat, Izin Tanpa Gaji, Cuti Haid |
| ESS Supervisor | Izin Leader muncul; pilih → tanggal selesai terkunci; catatan "Sisa kuota Izin Leader … 1 dari 1 hari. Tidak memotong gaji." |
| Supervisor ajukan Izin Leader kedua di bulan sama | Toast "Kuota Izin Leader bulan … sudah habis (1 dari 1 hari)" |
| Staff ajukan Izin Pribadi 4× di satu bulan | Ke-4 ditolak; catatan memuat "Memotong 1 hari THP", tanpa "Rp" |
| HR `/dashboard/hris/leaves/insert` pilih karyawan Staff lalu Supervisor | Opsi Izin Leader muncul hanya untuk Supervisor |
| HR `/dashboard/hris/leaves` filter "Izin Darurat (lama)" | Riwayat `emergency` dari Odoo tampil; ekspor CSV berlabel "Cuti Darurat" |
| Payroll (run uji di salinan, periode yang memuat 1 Izin Pribadi approved) | `personal_leave_deduction` = THP / pembagi; Izin Leader tidak menambah potongan |

Simpan screenshot ke `.share/epic058/`; console tanpa error baru.

- [ ] **Step 5: Catat & berhenti**

Tulis hasil Step 2–4 dan angka Step 1 (salinan) ke Automation Log EPIC-058, lalu set `status: ready-for-qa` **hanya** di branch. Laporkan ke user. **Berhenti di sini.** Deploy (`tools/deploy-paddydev.sh`: backup, migrasi `20261020150000` & `20261020150100`, build API & web) dan pengisian level jabatan live adalah keputusan terpisah setelah user menyetujui.

```bash
git add backend/database/sql/checks/2026-10-20_epic058_leave_types_readonly.sql docs/epics/EPIC-058-hris-revisi-jenis-cuti.md
git commit -m "docs(hris): cek read-only & hasil uji stack EPIC-058"
```

**Exit criteria:** tidak ada kegagalan baru; skenario Step 4 sesuai harapan di salinan DB; Automation Log memuat bukti; live tidak tersentuh.

---

## Pertanyaan terbuka (wajib dijawab user sebelum eksekusi)

1. **Arti "dapat dikombinasikan" (R6).** Rencana ini membaca: beberapa jenis boleh dipakai berdampingan di bulan yang sama dan bersebelahan (mis. Izin Leader Senin + Cuti Tahunan Selasa–Jumat), kuota masing-masing terpisah, satu pengajuan tetap satu jenis. Rencana juga **menambah** larangan dua pengajuan di tanggal yang sama (saat ini tidak dicek sama sekali). Apakah benar? Atau maksudnya satu pengajuan boleh berisi beberapa jenis sekaligus? Atau dua jenis boleh jatuh di tanggal yang sama?
2. **"SPV ke atas" (R5).** Apakah definisinya level jabatan Supervisor, Assistant Manager, Manager, Senior Manager, General Manager, Director, dan C-Level? Apakah jabatan **Coordinator/Koordinator** dan **Lead** (mis. "Koordinator Gudang", "Sample Maker Lead") ikut? Level jabatan hasil import Odoo kemungkinan besar masih `Staff` semua. Siapa yang mengisi level di master Posisi sebelum rilis: HR lewat UI, atau kami siapkan skrip berdasarkan judul jabatan untuk disetujui?
3. **Satuan kuota (R3/R4).** "1x/3x per bulan" = 1/3 **hari** per bulan, dan setiap pengajuan hanya satu hari? Bolehkah Izin Pribadi setengah hari atau beberapa hari sekaligus?
4. **Bulan kuota (R4).** Bulan kalender (1–akhir bulan), atau periode cut-off absensi/payroll (mis. 21–20)? Apakah pengajuan yang masih `pending` ikut menghabiskan kuota? Rencana ini: ya.
5. **"Tidak memotong" untuk Izin Leader (R7).** Gaji pokok bulanan memang tidak dipotong. Tetapi tunjangan transport/makan mode per hari dan upah harian/`monthly_prorated` mengikuti hari hadir, jadi hari Izin Leader tidak mendapat tunjangan harian (sama seperti Cuti Tahunan sekarang). Apakah Izin Leader harus dihitung "hadir" untuk tunjangan harian?
6. **Izin Pribadi & tunjangan harian.** Selain potongan 1 hari THP (PIP), hari Izin Pribadi juga tidak mendapat tunjangan harian, karena tidak hadir. Ini potensi potong ganda yang sudah tercatat di analisis payroll 19-10 dan masih menunggu slip Odoo September. Apakah tetap seperti sekarang?
7. **Cuti Menikah & Cuti Duka (R9).** Form saat ini menawarkan keduanya tetapi pengajuannya selalu gagal (nilai enum tidak ada). Rencana ini menambahkan keduanya ke enum. Setuju, atau disembunyikan saja?
8. **Riwayat `emergency` dari Odoo** (≈5 baris, aslinya "Cuti Menikah"). Biarkan berlabel "Izin Darurat", atau dipindah ke `marriage` lewat skrip data terpisah (setelah Q7)?
9. **Pengajuan pending jenis yang dihapus.** Rencana ini membiarkan HR tetap bisa approve/tolak. Atau HR ingin semua yang pending ditolak/dibatalkan otomatis saat rilis?
10. **Override HR.** Apakah HR boleh melewati kuota/syarat jabatan saat menginput atas nama karyawan (mis. kasus khusus)? Rencana ini: tidak boleh.
11. **Kuota tetap atau bisa diatur.** Angka 1 dan 3 ditanam di kode (katalog). Perlu dibuat bisa diatur HR di pengaturan?
12. **Cuti Haid.** Sekadar konfirmasi kebijakan: setelah dihapus, karyawan yang haid dan tidak bisa bekerja mengajukan lewat jenis apa (Sakit)? Catatan: UU 13/2003 Pasal 81 mengatur hak istirahat haid hari ke-1 dan ke-2.

## Follow-up (di luar rencana ini)

- Akses daftar/approve/hapus cuti dan saldo cuti masih berbasis peran (`hrRoles`, `isLineManagerRole`, `a.Role != "hrd"`), belum menu `hris.workforce` seperti pola EPIC-057.
- Payroll menghitung hari cuti per tanggal kalender (`ClampedLeaveDays`), sedangkan `total_days` di HRIS memakai hari kerja. Untuk Izin Pribadi satu hari hasilnya sama, tetapi Izin Pribadi multi-hari lama dari Odoo yang melewati akhir pekan bisa terhitung lebih.
- Kolom `hris.leave_balances.{emergency,menstrual,unpaid}_leave_used` tidak dipakai kode; bisa dirapikan nanti.
