# EPIC-119 Gaji HQ Tidak Dibagi, Cuti Ulang Tahun 2 Bulan, dan Koreksi Terlambat Bungsu Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Biaya gaji staf yang penempatan utamanya kantor pusat dibebankan penuh ke HQ, Cuti Ulang Tahun bisa diambil sejak tanggal ulang tahun sampai 2 bulan sesudahnya, dan tiga status terlambat Bungsu yang salah jadwal dikoreksi lewat skrip ops.

**Architecture:** Cabang kantor pusat ditandai `configuration.branches.is_head_office`; kalkulator payroll memakai basis alokasi baru `head_office` (satu share 100%) bila penempatan utama periode adalah cabang itu. Aturan Cuti Ulang Tahun pindah dari "bulan lahir" ke jendela tanggal per ulang tahun (`domain.BirthdayLeaveWindow`), dan kuota 1x dihitung per jendela, bukan per tahun kalender. Koreksi Bungsu memakai pola skrip ops: pratinjau, eksekusi, audit, pembalik.

**Tech Stack:** Go + pgx + PostgreSQL 16, Next.js (baca `apps/frontend/AGENTS.md` dulu), Vitest, psql.

**Spec:** laporan user 19-11-2026 dan jawaban AskUserQuestion di sesi yang sama:
- "perhitungan gaji di kantor pusat tidak di bagi" → "Penempatan utama HQ: 100% HQ". Staf toko tetap dibagi per jam kerja per toko.
- "cuti ulang tahun (valid 2 bulan dari tgl ulang tahunnya)" → "Tgl ultah s.d. 2 bulan". Contoh ultah 15 Nov: 15 Nov s.d. 14 Jan; sebelum tanggal ultah tidak bisa. Tetap 1x per ulang tahun, H-3, lampiran wajib, sesudah kontrak awal (K-21).
- "Mas Bungsu HQ (2 Penempatan Warehouse) ke detect Warehouse jadi ke detect terlambat" → "Koreksi via skrip". Penyebab sebenarnya: departemen Bungsu sempat Warehouse 9 s.d. 13 Nov (jadwal OPS 08.00, toleransi 10), lalu HR mengembalikannya ke Operations pada 13-11 09.13 WIB (jadwal NS, masuk sampai 09.00). Terlambat 10-11 (08.14), 11-11 (08.10), 13-11 (08.11) tidak terlambat bila memakai NS. Hanya Bungsu yang terdampak (dicek di production).

Temuan tanpa perubahan kode (dicatat di epic, dilaporkan ke user):
- "karyawan yang sudah lebih dari 1 tahun tidak bisa mengajukan izin pribadi": syarat "Cuti Tahunan habis" sudah dihapus EPIC-112 (keputusan user 17-11), live di production sejak 19-11 01.55 WIB. Task 4 menambah tes regresinya.
- "Yudho (seting dlu jadwal kerja) => clock in lagi": Yudho (Marketing, `schedule_type = shift`) tidak punya roster maupun pola jadwal. Ada 50 karyawan shift aktif lain yang sama sekali tidak punya jadwal untuk 1 s.d. 20 Nov. HR mengisi roster lewat menu Jadwal; Task 5 menyiapkan daftarnya.
- "Yana cuti salah input malah di server reddie": tidak ada pengajuan cuti Yana di staging (paddydev.reddie.id) maupun production. Satu-satunya data Yana di staging sesudah salinan 10-11 adalah clock-in 10-11 09.07, yang bentrok dengan clock-in production 18.12 (salah satu dari 8 bentrok EPIC-117). HR perlu menyebut tanggal dan jenis cutinya, lalu mencatatnya di production dari kalender kerja.

## Global Constraints

- Repo erp-paddy. Worktree baru `/mnt/hdd1/project/apps/paddydev/.worktrees/erp-epic119`, branch `feat/epic119-hq-ultah` dari `origin/development`. Jangan push langsung ke `development`; lewat PR.
- EPIC-118 (`feat/epic118-slip-gaji`) juga mengubah `service_runs.go` (`calculateRun`). Perubahan EPIC-119 di file itu dibatasi pada pemanggilan alokasi; branch yang di-merge kedua di-rebase.
- Tidak ada docs dan seeder di repo erp-paddy (`.gitignore`). Dokumen epic dan acuan ada di repo paddydev (`.worktrees/rilis-prod/docs/`).
- Test DB: `CREATE DATABASE epic119_test TEMPLATE epic117_test` di 127.0.0.1:55499; salin `.superpowers/env.sh` dari worktree erp-epic117 dan ganti nama DB. Go test dengan `-p 1`.
- Migration: `apps/backend/database/migrations/deltas/20261219100000_branches_head_office.sql`.
- Skrip ops: pratinjau bawaan (ROLLBACK), `-v eksekusi=1` mengeksekusi, `SET LOCAL search_path = public,iam,configuration,hris,performance,recruitment,item,purchasing,inventory,manufacturing,pos,crm,accounting,auth` sesudah BEGIN, NOTICE bertag STRUKTUR/PINDAH/INVARIAN/BLOKIR/HASIL, snapshot audit, skrip `_balik.sql`.
- Production: hanya baca sampai user menyetujui pratinjau. Backup DB prod dulu sebelum eksekusi skrip. Hindari jam 07–08, 13, 16–17, dan 21 WIB. Jangan membuat sesi atau akun sementara di prod.
- Kode cabang HQ di staging dan production: `ERP-BANDUNG` ("Paddy HQ Bandung").
- Setiap keputusan aturan HRIS dicatat di epic, lalu file acuan `docs/hris/acuan-mom-20261019/` diperbarui di commit yang sama (skill paddy-hris-acuan).

## Review Focus

1. Staf HQ yang absen sebelum 10-11 tercatat di cabang lama Odoo (`ODOO-PADDY-PUDAK`, `ODOO-PADDY-SALAM`): sesudah hitung ulang, satu share HQ 100%, jumlahnya sama persis dengan slip. Tes di Task 1.
2. Ulang tahun 31 Desember dan 29 Februari: jendela berakhir di akhir Februari (tanpa meluap ke Maret), dan 29 Feb jatuh ke 28 Feb di tahun bukan kabisat. Tes di Task 2.
3. Ulang tahun Desember, cuti diambil Januari tahun berikutnya: lolos, dan kuota dihitung untuk ulang tahun Desember itu, bukan tahun kalender Januari. Tes di Task 2.
4. Update absensi di skrip Bungsu memicu trigger `trg_notify_attendance_changes` dan `attendance_closed_period_guard`: pratinjau menunjukkan berapa notifikasi yang terbentuk, dan skrip menolak bila periodenya sudah ditutup. Tes di Task 3.
5. Karyawan HQ yang penempatan utamanya pindah ke toko di tengah periode: `PeriodPrimary` memilih penempatan utama terbaru; bila itu toko, slip tetap dibagi seperti biasa. Tes di Task 1.

---

### Task 1: Gaji staf kantor pusat 100% ke HQ

**Files:**
- Create: `apps/backend/database/migrations/deltas/20261219100000_branches_head_office.sql`
- Modify: `apps/backend/internal/modules/payroll/domain/allocation.go` (konstanta basis baris 26-29, fungsi baru)
- Modify: `apps/backend/internal/modules/payroll/ports.go` (`Entities` baris 162-169), `apps/backend/internal/app/adapters_payroll.go`
- Modify: `apps/backend/internal/modules/payroll/service_runs.go` (pemanggilan `domain.AllocateSlip`, baris ~519)
- Modify: `apps/frontend/src/lib/payroll/store-allocation.ts` (`ALLOCATION_BASIS_LABEL`, tipe `AllocationBasisCode`)
- Test: `apps/backend/internal/modules/payroll/domain/allocation_test.go`, `apps/backend/internal/modules/payroll/head_office_allocation_integration_test.go`, `apps/frontend/src/lib/payroll/store-allocation.test.ts`

**Interfaces:**
- Migration (cek dulu nama constraint basis yang ada lewat `\d hris.payroll_detail_allocations` di test DB):

```sql
ALTER TABLE configuration.branches ADD COLUMN IF NOT EXISTS is_head_office boolean NOT NULL DEFAULT false;
UPDATE configuration.branches SET is_head_office = true WHERE code = 'ERP-BANDUNG';
ALTER TABLE hris.payroll_detail_allocations DROP CONSTRAINT IF EXISTS payroll_detail_allocations_basis_check,
  ADD CONSTRAINT payroll_detail_allocations_basis_check CHECK (basis IN ('hours', 'days', 'primary', 'none', 'head_office'));
```

- `const BasisHeadOffice = "head_office"`
- `func AllocateHeadOffice(amounts map[string]float64, days []BranchDay, branchID string) SlipAllocation`: satu share `BranchID = &branchID`, `Ratio = 1`, `Amounts` = semua `AllocationColumns` utuh, `Days`/`Hours` = total hari kerja dan jam semua baris absensi (`countsAsWork`), `Basis = BasisHeadOffice`, `Unassigned = false`.
- `Entities.HeadOffices(ctx context.Context, q database.Querier) (map[string]bool, error)`: id cabang dengan `is_head_office = true`.
- Di kalkulasi: `primary := domain.PeriodPrimary(scopedPlacements, start, end)`; bila `primary != nil && headOffices[*primary]` pakai `AllocateHeadOffice`, selain itu `AllocateSlip` seperti sekarang. `headOffices` dimuat sekali per run bersama `BranchCompanies`.
- Label frontend: `head_office: "Kantor pusat (tidak dibagi)"`.

- [ ] **Step 1: Tulis tes yang gagal**

```go
// domain/allocation_test.go
func TestAllocateHeadOfficeKeepsWholeSlip(t *testing.T) // sampleAmounts(); days HQ 2 + Pudak 11; satu share HQ, Ratio 1, sumsExact, Basis "head_office", Days 13
// head_office_allocation_integration_test.go
func TestHeadOfficeStaffNotSplit(t *testing.T)             // penempatan utama HQ (is_head_office), absen di HQ dan toko → 1 baris alokasi HQ, gross_amount = gross slip
func TestStoreStaffStillSplit(t *testing.T)                // penempatan utama toko A, absen di A dan B → 2 baris, basis "hours"
func TestHeadOfficeSecondaryPlacementIgnored(t *testing.T) // utama HQ + penempatan Warehouse aktif (kasus Bungsu) → 1 baris HQ
func TestPrimaryMovedToStoreMidPeriodSplits(t *testing.T)  // utama HQ s.d. tgl 5, utama toko sejak tgl 6 → dibagi seperti biasa
```

```ts
// store-allocation.test.ts
it("label basis kantor pusat", () => {
  expect(ALLOCATION_BASIS_LABEL.head_office).toBe("Kantor pusat (tidak dibagi)");
});
```

- [ ] **Step 2: Jalankan, pastikan gagal**

Run: `cd apps/backend && source ../../.superpowers/env.sh && go test ./internal/modules/payroll/... -run 'HeadOffice|StoreStaffStillSplit|PrimaryMovedToStore' -p 1`
Expected: FAIL kompilasi (`AllocateHeadOffice` belum ada).

- [ ] **Step 3: Implementasi** sesuai Interfaces. Terapkan migration ke test DB lewat runner (`-apply`). Perbarui fake `Entities` di tes payroll lain yang mengimplementasikan interface itu.

- [ ] **Step 4: Jalankan lagi**

Run: `go test ./internal/modules/payroll/... ./internal/app/... -p 1` dan `cd apps/frontend && pnpm exec vitest run src/lib/payroll src/features/hris/payroll`
Expected: PASS semua.

- [ ] **Step 5: Commit**

```bash
git add apps/backend apps/frontend/src/lib/payroll
git commit -m "feat(payroll): gaji staf kantor pusat dibebankan penuh ke HQ, tidak dibagi per toko (EPIC-119)"
```

### Task 2: Cuti Ulang Tahun berlaku sejak tanggal ulang tahun sampai 2 bulan

**Files:**
- Modify: `apps/backend/internal/modules/hris/domain/leave_rules.go` (dekat `BirthdayInYear` baris 66-78)
- Modify: `apps/backend/internal/modules/hris/leaves.go` (`checkBirthdayLeave` baris 235-254; kuota birthday di dalam transaksi baris ~375-384)
- Modify: `apps/frontend/src/lib/hris/leave-form.ts` (baris 32)
- Test: `apps/backend/internal/modules/hris/domain/leave_rules_test.go`, `apps/backend/internal/modules/hris/leave_tnc_integration_test.go` (blok birthday baris 41-56)

**Interfaces:**
- `func BirthdayLeaveWindow(birthDate, date string) (from, to string, ok bool)`: cari ulang tahun `b` di tahun `date`, lalu tahun sebelumnya (pakai `BirthdayInYear`), dengan `b <= date <= to`. `to` = hari sebelum "tanggal yang sama 2 bulan kemudian", dan tanggal itu dijepit ke akhir bulan tujuan. `ok=false` bila tanggal lahir kosong/tidak valid atau `date` di luar kedua jendela; `from`/`to` lalu berisi jendela ulang tahun di tahun `date`.
- Contoh yang dipatok tes:
  - lahir 1995-11-15, date 2026-11-15 → `2026-11-15..2027-01-14`, ok
  - date 2027-01-14 → ok (jendela 2026); date 2027-01-15 → tidak ok; date 2026-11-14 → tidak ok
  - lahir 1990-12-31, date 2027-02-27 → ok dengan `to` 2027-02-27; date 2027-02-28 → tidak ok
  - lahir 2000-02-29, tahun 2027 → from 2027-02-28, to 2027-04-27
- Pesan error:
  - di luar jendela: `Cuti Ulang Tahun berlaku mulai tanggal ulang tahun sampai 2 bulan sesudahnya (DD-MM-YYYY s.d. DD-MM-YYYY)`, dari `from`/`to` hasil fungsi.
  - kuota: `Cuti Ulang Tahun untuk ulang tahun DD-MM-YYYY sudah dipakai`. Kuota menghitung pengajuan birthday berstatus pending/approved dengan `start_date` di `[from, to]`.
- Cek kontrak awal (`InitialContractPassed`) memakai tanggal ulang tahun jendela (`from`).
- Hint frontend: `birthday: "1 hari, mulai tanggal ulang tahun sampai 2 bulan sesudahnya"`.

- [ ] **Step 1: Tulis tes yang gagal**

```go
func TestBirthdayLeaveWindow(t *testing.T) // tabel semua contoh di Interfaces, plus tanggal lahir kosong → ok=false
```

Ubah blok birthday di `leave_tnc_integration_test.go` (lahir 1995-11-20; "hari ini" di tes itu 2026-11-03, terlihat dari Sakit hari H):
- `2026-11-19` → 400 `Cuti Ulang Tahun berlaku mulai tanggal ulang tahun sampai 2 bulan sesudahnya (20-11-2026 s.d. 19-01-2027)`
- `2027-01-19` → 200, lalu `2026-11-23` → 400 `Cuti Ulang Tahun untuk ulang tahun 20-11-2026 sudah dipakai`
- `2027-01-20` → 400 pesan jendela (20-11-2027 s.d. 19-01-2028)

Pesan lampiran dan kontrak awal tetap seperti sekarang.

- [ ] **Step 2: Jalankan, pastikan gagal**

Run: `go test ./internal/modules/hris/... -run 'TestBirthdayLeaveWindow|<nama tes induk blok birthday>' -p 1`
Expected: FAIL.

- [ ] **Step 3: Implementasi** sesuai Interfaces. Hapus pesan lama "Birthday Leave hanya dalam bulan ulang tahun Anda" dan "Birthday Leave sudah dipakai tahun %s".

- [ ] **Step 4: Jalankan lagi**

Run: `go test ./internal/modules/hris/... -p 1` dan `cd apps/frontend && pnpm exec vitest run src/lib/hris src/features/hris`
Expected: PASS.

- [ ] **Step 5: Commit** `feat(hris): cuti ulang tahun berlaku sejak tanggal ulang tahun sampai 2 bulan (EPIC-119)`

### Task 3: Skrip koreksi terlambat Bungsu (jadwal HQ 9 s.d. 13 Nov)

**Files:**
- Create: `apps/backend/database/sql/fixes/2026-11-19_bungsu_jadwal_hq_terlambat.sql`, `2026-11-19_bungsu_jadwal_hq_terlambat_balik.sql`
- Test: `apps/backend/internal/modules/hris/bungsu_late_fix_integration_test.go` (pakai `opsScript` dari `business_structure_integration_test.go`)

**Interfaces:**
- Target dipatok di skrip: employee `10e1888b-902d-42d0-a0fb-7f7899338478`; baris `hris.employee_shifts` miliknya dengan `created_by_name = 'Sistem (pindah departemen)'`, `effective_from = '2026-11-09'`, `effective_to = '2026-11-12'` (7 baris); absensi tanggal 2026-11-10, 11, 12, 13.
- Eksekusi:
  1. 7 baris pola: Senin–Jumat `shift_id` = shift `NS`; Sabtu `shift_id` NULL (libur), sama dengan pola HQ sejak 13-11.
  2. Absensi 4 tanggal: `shift_id` = `NS`, `scheduled_start`/`scheduled_end` dari shift NS pada tanggal itu, `late_tolerance_minutes = 0`. `is_late` dan `late_minutes` dihitung ulang dengan aturan NS (terlambat bila `clock_in` WIB lewat 09.00). Ketiga baris yang kini terlambat menjadi `false`/0.
  3. Audit `data_restructure.bungsu_late_fix` dengan snapshot `before` (pola dan absensi) untuk pembalik.
- BLOKIR bila salah satu berlaku:
  - jumlah baris pola bukan 7;
  - ada absensi target dengan `clock_in` ≥ 09.00 WIB;
  - periode absensi tanggal itu sudah ditutup;
  - pola sejak 13-11 bukan NS (keadaan berubah sejak dicek).
- NOTICE `STRUKTUR` mencetak jumlah notifikasi yang dibuat trigger selama transaksi (`public.notifications` sebelum/sesudah).
- Pembalik: kembalikan pola dan absensi dari snapshot audit terakhir yang belum dibalik; BLOKIR `diubah_sesudah_koreksi` bila baris berubah sesudah skrip.

- [ ] **Step 1: Tulis tes yang gagal**

```go
func TestBungsuLateFixPreviewChangesNothing(t *testing.T)     // fixture meniru data prod (OPS 9–12, NS sejak 13, 4 absensi); pratinjau → data sama
func TestBungsuLateFixExecute(t *testing.T)                   // 3 baris is_late false, late_minutes 0, shift NS, pola 9–12 NS, 1 audit
func TestBungsuLateFixBlocksChangedData(t *testing.T)         // clock_in 13-11 diubah ke 09.30 → BLOKIR, tidak ada perubahan
func TestBungsuLateFixRevert(t *testing.T)                    // eksekusi lalu balik → data kembali persis
func TestBungsuLateFixWorksWithoutAppSearchPath(t *testing.T) // dijalankan tanpa search_path aplikasi
```

- [ ] **Step 2: Jalankan, pastikan gagal**

Run: `go test ./internal/modules/hris/ -run TestBungsuLateFix -p 1`
Expected: FAIL (berkas skrip belum ada).

- [ ] **Step 3: Tulis skrip** sesuai Interfaces dan pola `2026-11-18_staging_tutup_login_karyawan.sql`.

- [ ] **Step 4: Jalankan lagi**

Run: perintah Step 2. Expected: PASS 5/5. Lalu jalankan pratinjau di psql biasa terhadap test DB (`psql "$TEST_DATABASE_URL" -v ON_ERROR_STOP=1 -f ...`). Expected: NOTICE `HASIL PRATINJAU` tanpa error.

- [ ] **Step 5: Commit** `fix(hris): skrip koreksi terlambat Bungsu dengan jadwal HQ 9–13 Nov (EPIC-119)`

- [ ] **Step 6 (sesudah rilis, butuh izin user):** backup DB prod, jalankan pratinjau di prod, tunjukkan NOTICE ke user, dan eksekusi hanya sesudah user menyetujui.

### Task 4: Tes regresi Izin Pribadi untuk karyawan lebih dari 1 tahun

**Files:**
- Test: `apps/backend/internal/modules/hris/leave_types_integration_test.go`

- [ ] **Step 1: Tulis tes** `TestPersonalLeaveForLongTenureWithAnnualBalance`: karyawan join 2024-01-02 dengan sisa cuti tahunan 12 mengajukan Izin Pribadi 1 hari kerja → 200; pengajuan ke-4 di bulan yang sama → 400 kuota bulanan (`Kuota Izin Pribadi bulan ... sudah habis`).
- [ ] **Step 2: Jalankan** `go test ./internal/modules/hris/ -run TestPersonalLeaveForLongTenure -p 1`. Expected: PASS, karena aturannya sudah benar sejak EPIC-112. Bila FAIL, berhenti dan laporkan penyebabnya; jangan ubah kode di task ini.
- [ ] **Step 3: Commit** `test(hris): Izin Pribadi tetap bisa untuk karyawan lebih dari 1 tahun (EPIC-119)`

### Task 5: Daftar untuk HR (tanpa perubahan kode)

- [ ] **Step 1:** Di production (read-only), ambil karyawan aktif `schedule_type = 'shift'` tanpa `employee_shift_days` dan tanpa `employee_shifts` yang berlaku dalam periode berjalan (21-10 s.d. 20-11): nama, NIP, departemen, jabatan, penempatan utama. Expected: sekitar 50 baris, termasuk Yudho Bonar Gunarto.
- [ ] **Step 2:** Simpan sebagai CSV mode 600 di `/mnt/hdd1/backup/paddydev/` (data karyawan, jangan di-commit). Kirim ke user untuk HR dengan dua catatan: Yudho perlu roster sebelum clock-in ulang, dan HR perlu tanggal serta jenis cuti Yana untuk mencatatnya di production dari kalender kerja.

### Task 6: Gerbang akhir, dokumen epic, dan acuan

**Files (repo paddydev, `.worktrees/rilis-prod`):**
- Create: `docs/epics/EPIC-119-gaji-hq-cuti-ultah-koreksi-terlambat.md`; Modify: `docs/epics/README.md`
- Modify: `docs/hris/acuan-mom-20261019/pertanyaan-rekomendasi.md` (K-21: jendela 2 bulan, keputusan 19-11), `status-requirement.md` (ES-14; catatan alokasi gaji HQ)

- [ ] **Step 1: Suite penuh** `cd apps/backend && go build ./... && go vet ./internal/modules/payroll/... ./internal/modules/hris/... && go test ./internal/modules/payroll/... ./internal/modules/hris/... ./internal/app/... -p 1`; `cd apps/frontend && pnpm exec tsc --noEmit -p . && pnpm exec eslint src/lib/payroll src/lib/hris && pnpm test`. Expected: PASS. Kegagalan di luar perubahan ini dicatat dengan nama tesnya.
- [ ] **Step 2: Migration** `-apply` lalu `-verify` di `epic119_test`. Expected: 0 drift.
- [ ] **Step 3: Epic doc** status `review`, tiga keputusan user 19-11 di Spec, temuan tanpa kode (Izin Pribadi, Yudho, Yana), daftar commit, Automation Log, tautan rencana ini.
- [ ] **Step 4: Commit dokumen** `docs: EPIC-119 gaji HQ tidak dibagi, cuti ulang tahun 2 bulan, koreksi terlambat Bungsu`.
