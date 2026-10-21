# Aktifkan Kembali Karyawan & Pesan Duplikat Jelas — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** HR bisa mengaktifkan kembali karyawan yang sudah resign pada **data yang sama**, tanpa membuat baris karyawan baru. Pesan bentrok data juga harus menyebut kolom dan pemiliknya, bukan hanya "Data sudah ada di sistem".

**Kasus pemicu (live, 21-10-2026):** Alya Nur Hikmah punya dua data nonaktif:
- `202504229`: staff PPBA dari Odoo, keluar 5 Jul 2026, punya KTP dan email asli.
- `202610085`: freelance, di-resign 20 Okt 2026, email placeholder.

HR mencoba "Tambah karyawan" lagi pada 21 Okt dan gagal karena `employees_ktp_key`/`employees_nip_key`/`employees_npwp_key` juga berlaku untuk baris nonaktif. Pesan yang muncul hanya "Data sudah ada di sistem". Tanggal resign maupun tanggal tambah tidak berpengaruh.

**Architecture:**
- **Endpoint baru `POST /api/hris/employees/{id}/rehire`** di modul Go `hris`, kebalikan dari `ResignEmployee` (`backend/internal/modules/hris/employee_resign.go`). Semua langkah berjalan dalam satu transaksi:
  - set `is_active=true`, `end_date=NULL`, status kepegawaian baru, dan `join_date` baru;
  - buat penempatan utama baru;
  - pasang jadwal tetap bila karyawan Non-Shift;
  - aktifkan kembali login bila diminta;
  - catat `employment_history` (`change_type='rehire'`) dan audit `employee.rehire`.
- **Pesan duplikat:**
  - Sebelum insert/update karyawan, cari pemilik KTP/NPWP/NIP di **semua** karyawan, termasuk yang nonaktif. KTP/NPWP dibandingkan per digit.
  - Bila ada bentrok, kembalikan 400 dengan nama, NIP, dan status pemiliknya. Bila pemiliknya nonaktif, pesan mengarahkan HR ke tombol "Aktifkan kembali".
  - Pemeriksaannya ditaruh di paket platform baru (`backend/internal/platform/employeekeys`), karena modul `hris` dan `configuration/accounts` tidak boleh saling import.
  - Untuk balapan (race), `httpx.WriteError` memetakan nama constraint ke pesan per kolom.
- **UI:** tombol "Aktifkan kembali" di header detail karyawan nonaktif, beserta dialognya.

**Tech Stack:** Go (`backend/internal/modules/hris`, `configuration/accounts`, `platform/httpx`), PostgreSQL 16, Next.js versi repo (baca `frontend/node_modules/next/dist/docs/` sebelum menyentuh halaman), TanStack Query, vitest + testing-library.

**Spec:** permintaan user 21-10-2026 (sesi ini): "aktifkan kembali karyawan & pesan duplikat jelas", berdasarkan kasus Alya di atas. Ruling R1–R9 di bawah adalah keputusan rencana; user menyuruh langsung dikerjakan.

## Ruling

- **R1 — Data yang sama diaktifkan kembali, tidak ada baris baru.** KTP/NPWP/NIP tidak bentrok dengan dirinya sendiri, dan riwayat (kontrak, absensi, slip) tetap di satu NIP.
- **R2 — Hak akses sama dengan resign:** `withAction("update", employeeRecordManagers, …)` di `employeeSubroutes()`, ditempatkan tepat di bawah `resign`.
- **R3 — Tanggal efektif (`effective_date`):**
  - Wajib diisi, format `YYYY-MM-DD`.
  - Harus **setelah** `end_date` lama. Penempatan memakai daterange inklusif, jadi tanggal yang sama akan bentrok dengan penempatan lama.
  - Tidak boleh setelah hari ini (WIB), sama seperti resign: karyawan langsung aktif.
- **R4 — `join_date` diganti dengan tanggal efektif** (masa kerja baru). Nilai lama dicatat di audit `before` dan di `employment_history.notes`. Ini memengaruhi prorata cuti tahunan dan masa kerja tunjangan kesehatan, yang memang dihitung dari tanggal masuk.
- **R5 — Penempatan dan jadwal:**
  - Cabang penempatan utama wajib diisi. Penempatan baru: `is_primary=true`, `start_date=effective_date`, notes `Aktif kembali`.
  - Karyawan Non-Shift otomatis dipasang jadwal tetap NS lewat `applyFixedSchedule` mulai `max(effective_date, hari ini)`.
  - Karyawan Shift tidak dibuatkan pola; HR mengatur lewat Jadwal Kerja. Respons memberi tahu hal ini.
- **R6 — Login:**
  - `reactivate_login` (bool, default `true`) hanya berlaku bila karyawan punya `user_id`. Efeknya: `auth.users.banned_until=NULL`, `configuration.users.status='active'`, `is_access_app=true`.
  - Akun yang memegang role sistem (`permaudit.HoldsSystemRole`) hanya boleh diaktifkan kembali oleh IAM `super_admin`: 403 `Akun Super Admin/Admin hanya bisa diaktifkan kembali oleh Super Admin`.
  - Karyawan tanpa login tetap tanpa login. Akunnya dibuat lewat jalur yang sudah ada.
- **R7 — Email:**
  - Field `email` opsional dan hanya boleh diisi bila karyawan **tidak** punya `user_id`. Bila punya login, email dikelola di Manajemen User: 400 `Email akun login diubah lewat Manajemen User`.
  - Email baru harus belum dipakai karyawan lain mana pun (`EmployeeExists` email) maupun akun login (`auth.users`): 400 `Email sudah digunakan`.
- **R8 — Status kepegawaian:**
  - Wajib diisi, dan harus ada di daftar status yang sama dengan import (`loadEmployeeLists`: daftar bawaan `EmploymentStatusLabels` + master aktif).
  - `resigned` dan `terminated` ditolak: 400 `Status kepegawaian tidak valid untuk karyawan aktif`.
- **R9 — Pesan bentrok (persis):**
  - Format: `<Label> sudah dipakai karyawan <Nama> (NIP <nip>, <aktif|nonaktif>)`.
  - Bila pemiliknya nonaktif, tambahkan: ` — aktifkan kembali data tersebut dari halaman detail karyawan`.
  - Label: `No. KTP`, `NPWP`, `NIP`.
  - Pesan fallback dari constraint (race): `No. KTP sudah dipakai karyawan lain`, `NPWP sudah dipakai karyawan lain`, `NIP sudah dipakai karyawan lain`.

## Global Constraints

- **Pesan error lain (persis):**
  - `Karyawan masih aktif`
  - `Tanggal efektif wajib diisi`
  - `Tanggal efektif tidak boleh setelah hari ini`
  - `Tanggal efektif harus setelah tanggal keluar (<dd/mm/yyyy>)` (pakai `domain.FormatDateID`)
  - `Cabang penempatan wajib diisi`
  - `Cabang tidak ditemukan`
- **Pesan sukses:** `Karyawan diaktifkan kembali`.
- **Test integrasi Go:**
  - DB test `TEST_DATABASE_URL='postgres://postgres@127.0.0.1:55499/paddy_test?sslmode=disable'`, Go `$HOME/.local/go/bin/go`, jalankan dengan `-p 1`.
  - Kegagalan baseline main ada 27 nama (lihat ledger). Nama test dibandingkan tanpa durasi.
  - Test yang butuh data IAM seed harus memanggil `testutil.RequireSeededIAM`.
- **Route Go baru:** jalankan `PATH=$HOME/.local/go/bin:$PATH make -C backend generate`, lalu tambahkan path-nya ke `goNativePrefixes` di `frontend/src/lib/backend-routes.test.ts`, karena route ini lahir di Go.
- **Worktree & commit:**
  - Kerja di `/home/wit/Desktop/paddydev/.worktrees/epic064-rehire`, branch `feat/epic064-aktifkan-kembali` (dari main `5035ae5`).
  - Satu commit per task, diakhiri `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`.
  - Jangan push. Jangan menyentuh DB live.

## Review Focus

1. **Reaktivasi dua kali / karyawan masih aktif** → 400 `Karyawan masih aktif`, tanpa efek samping. Test: `TestRehireEmployee` (Task 2).
2. **Tanggal efektif sama dengan `end_date` lama** → 400. Kalau tidak dicegah, penempatan bentrok dengan exclusion constraint. Test: `TestRehireEmployee`.
3. **Akun login dengan role sistem** diaktifkan kembali oleh HR biasa → 403, dan login tetap terblokir. Test: `TestRehireSystemAccountNeedsSuperAdmin` (Task 2).
4. **KTP dengan format berbeda** (`3273 0101 9900 0001` vs `3273010199000001`) tetap terdeteksi sebagai duplikat, dengan nama pemilik nonaktif. Test: `TestEmployeeKeyConflictMessages` (Task 1).
5. **Update karyawan memakai KTP miliknya sendiri** tidak dianggap bentrok (`excludeID`). Test: `TestEmployeeKeyConflictMessages`.

---

### Task 1: Pemeriksaan bentrok KTP/NPWP/NIP dengan pesan jelas (Go)

**Files:**
- Create: `backend/internal/platform/employeekeys/employeekeys.go`
- Modify: `backend/internal/platform/httpx/errors.go` + `respond.go` (peta constraint 23505)
- Modify: `backend/internal/modules/hris/employees.go` (`CreateEmployee`, `UpdateEmployee`)
- Modify: `backend/internal/modules/configuration/accounts/employees.go` (`createUser`, `updateUser`) — pemanggilan lewat port/adapter yang sudah ada (`backend/internal/app/adapters_configuration_accounts.go`) bila akun tidak memegang `database.Querier`
- Modify: `backend/internal/modules/hris/domain/employees.go` (`EmployeeUniqueMessage`) — tidak lagi dipakai untuk ktp/npwp/nip bila pre-check menggantikannya; hapus bila tak terpakai
- Test: `backend/internal/platform/employeekeys/employeekeys_test.go` (unit: normalisasi & format pesan), `backend/internal/modules/hris/employee_keys_integration_test.go`, tambahan di `backend/internal/modules/configuration/accounts/accounts_integration_test.go`, `backend/internal/platform/httpx` test untuk fallback

**Interfaces (Produces):**

```go
package employeekeys

// Keys are the unique identity columns of hris.employees.
type Keys struct{ KTP, NPWP, NIP *string }

// Conflict describes the employee already holding one of the keys.
type Conflict struct {
	Field    string // "ktp" | "npwp" | "nip"
	Name     string
	NIP      string
	Active   bool
}

// Message is the user-facing text (ruling R9).
func (c Conflict) Message() string

// Find returns the first conflict (order: ktp, npwp, nip) with any employee other than excludeID ("" = none),
// active or not. KTP/NPWP compare digits only; NIP compares trimmed text. Empty/NULL keys are skipped.
func Find(ctx context.Context, q database.Querier, k Keys, excludeID string) (*Conflict, error)

// ConstraintMessage maps employees_{ktp,npwp,nip}_key to the R9 fallback text; ok=false otherwise.
func ConstraintMessage(constraint string) (msg string, ok bool)
```

- [ ] **Step 1: Test unit gagal** — `employeekeys_test.go`:

```go
func TestConflictMessage(t *testing.T) {
	c := Conflict{Field: "ktp", Name: "Alya Nur Hikmah", NIP: "202504229", Active: false}
	want := "No. KTP sudah dipakai karyawan Alya Nur Hikmah (NIP 202504229, nonaktif) — aktifkan kembali data tersebut dari halaman detail karyawan"
	if c.Message() != want { t.Fatalf("got %q", c.Message()) }
	c = Conflict{Field: "nip", Name: "Budi", NIP: "1", Active: true}
	if c.Message() != "NIP sudah dipakai karyawan Budi (NIP 1, aktif)" { t.Fatal(c.Message()) }
	for constraint, want := range map[string]string{
		"employees_ktp_key": "No. KTP sudah dipakai karyawan lain", "employees_npwp_key": "NPWP sudah dipakai karyawan lain",
		"employees_nip_key": "NIP sudah dipakai karyawan lain"} {
		if got, ok := ConstraintMessage(constraint); !ok || got != want { t.Fatalf("%s: %q", constraint, got) }
	}
	if _, ok := ConstraintMessage("other_key"); ok { t.Fatal("unknown constraint must not map") }
}
```

- [ ] **Step 2: Test integrasi gagal** — `employee_keys_integration_test.go` (paket `hris`, harness yang ada):
  - Buat karyawan nonaktif A (`h.employee(emp{name:"Alya Uji", inactive:true})`), lalu set `ktp='3273010199000001'` dan `npwp='12.345.678.9-012.000'` lewat `h.exec`.
  - `POST /api/hris/employees` dengan `ktp:"3273 0101 9900 0001"` → 400, pesan R9 lengkap dengan "nonaktif — aktifkan kembali…".
  - `POST` dengan npwp `123456789012000` → 400 dimulai `NPWP sudah dipakai karyawan Alya Uji`.
  - `PUT /api/hris/employees/{A}` dengan ktp miliknya sendiri → 200, tidak dianggap bentrok.
  - `PUT` karyawan B dengan ktp A → 400 pesan R9.
  - Accounts: `POST /api/users` dengan ktp A → 400 pesan R9. Tambahkan ke test akun yang sudah ada, pola `accounts_integration_test.go`.
  - Fallback httpx: `WriteError` untuk `*pgconn.PgError{Code:"23505", ConstraintName:"employees_npwp_key"}` → 409 `NPWP sudah dipakai karyawan lain`, sedangkan constraint lain tetap `Data sudah ada di sistem`.

- [ ] **Step 3: Jalankan → FAIL.**

- [ ] **Step 4: Implementasi.**
  - `Find` memakai satu query per key, dengan `regexp_replace(ktp,'\D','','g') = $1` untuk KTP/NPWP (input dinormalisasi ke digit; lewati bila hasilnya kosong) dan `trim(nip) = trim($1)`, plus `AND id <> $2` (pakai `NULLIF($2,'')::uuid`). Urutan pemeriksaan: ktp, npwp, nip.
  - Panggil `Find` sebelum insert/update di keempat jalur, dengan `excludeID` = id karyawan untuk update. Kembalikan `httpx.BadRequest(c.Message())`.
  - Pre-check NIP/email yang sudah ada tetap dipertahankan. Bila NIP bentrok, `Find` memberi pesan yang lebih jelas, jadi pre-check NIP lama boleh diganti asal test lama tetap lulus. Sesuaikan ekspektasi test lama yang mengecek "NIP sudah digunakan" / "NIK/KTP sudah terdaftar", dan catat di laporan.
  - `WriteError`: untuk 23505, cek `employeekeys.ConstraintMessage(pgErr.ConstraintName)` lebih dulu dan kembalikan 409 dengan pesan itu. Hati-hati dengan import cycle: `httpx` tidak boleh mengimpor paket yang mengimpor `httpx`. Bila `employeekeys` memakai `httpx`, letakkan peta constraint sebagai map sederhana di `httpx` dan biarkan `employeekeys.ConstraintMessage` membaca dari sana, atau sebaliknya. Pilih satu arah dan jelaskan di laporan.

- [ ] **Step 5: Jalankan → PASS; paket `hris`, `configuration/accounts`, `platform/...` = baseline.**

- [ ] **Step 6: Commit** `fix(hris): bentrok KTP/NPWP/NIP menyebut pemilik & arahkan aktifkan kembali (EPIC-064 Task 1)`.

**Exit criteria:** keempat jalur tulis karyawan memberi pesan R9 untuk bentrok KTP/NPWP/NIP (termasuk dengan karyawan nonaktif dan format KTP berbeda); karyawan yang memakai KTP sendiri tidak dianggap bentrok; balapan tetap memberi pesan per kolom.

---

### Task 2: Endpoint aktifkan kembali (Go)

**Files:**
- Create: `backend/internal/modules/hris/employee_rehire.go` (service + handler, pola `employee_resign.go`)
- Modify: `backend/internal/modules/hris/routes.go` (`"rehire": {POST: h.withAction("update", employeeRecordManagers, h.rehireEmployee)}` di `employeeSubroutes()`)
- Regenerate: `frontend/src/lib/go-routes.generated.json`; Modify: `frontend/src/lib/backend-routes.test.ts` (`goNativePrefixes` + komentar `// Aktifkan kembali karyawan (EPIC-064).`)
- Test: `backend/internal/modules/hris/employee_rehire_integration_test.go`

**Interfaces (Produces):** `POST /api/hris/employees/{id}/rehire` body `{effective_date, employment_status, branch_id, reactivate_login?: bool (default true), email?: string, reason?: string (≤500)}` → 200 `{success, message:"Karyawan diaktifkan kembali", data:{id, employment_status, join_date, schedule_note?}}`. `schedule_note` = `"Atur jadwal shift karyawan ini di Jadwal Kerja"` untuk karyawan Shift; kosong untuk Non-Shift.

- [ ] **Step 1: Test gagal** — `TestRehireEmployee`:
  - Fixture seperti `TestResignEmployee`.
  - Resign karyawan (login + penempatan + pola).
  - Klock harness: periksa `newHarness`/`s.clock` untuk "hari ini". Test resign memperlakukan `2026-10-15` sebagai masa depan; pilih tanggal relatif terhadap jam harness, dan jelaskan pilihan itu di laporan.
  - Pemeriksaan berurutan:
    1. Rehire karyawan aktif lain → 400 `Karyawan masih aktif`.
    2. `effective_date` = `end_date` lama → 400 `Tanggal efektif harus setelah tanggal keluar (dd/mm/yyyy)`.
    3. Tanggal setelah hari ini → 400.
    4. Status `resigned` → 400.
    5. Tanpa `branch_id` → 400 `Cabang penempatan wajib diisi`.
    6. Sukses: `is_active=true`, `end_date IS NULL`, status baru, `join_date`=tanggal efektif; penempatan utama baru dengan `start_date`=tanggal efektif (penempatan lama tetap tertutup); `auth.users.banned_until IS NULL`; `configuration.users.status='active'`; `is_access_app=true`; history `rehire` dengan prev/new status; audit `employee.rehire` dengan `before.join_date` lama.
    7. `reactivate_login:false` pada karyawan lain → login tetap terblokir, karyawan aktif.
    8. Karyawan tanpa login + `email` baru → email tersimpan; email yang dipakai karyawan lain → 400 `Email sudah digunakan`; karyawan dengan login + `email` → 400 `Email akun login diubah lewat Manajemen User`.
    9. Non-Shift: setelah rehire ada pola NS mulai `max(eff, today)`.
    10. Tanpa menu `update` → 403.
  - `TestRehireSystemAccountNeedsSuperAdmin`:
    - Karyawan resign yang login-nya role `admin` (lihat `employee_import_system_account_integration_test.go` untuk fixture), di-rehire oleh HR → 403 dengan pesan R6, dan login tetap terblokir.
    - Oleh `super_admin` (IAM) → 200.

- [ ] **Step 2: Jalankan → FAIL (404).**

- [ ] **Step 3: Implementasi** `RehireEmployee(ctx, actor *auth.User, id string, in rehireInput, superAdmin func(context.Context) (bool, error))`, dalam `s.repo.InTx`:
  1. `SELECT full_name, employment_status, join_date::text, end_date::text, is_active, user_id::text, email, schedule_type FROM hris.employees WHERE id=$1 FOR UPDATE`. Tidak ada → 404 `Karyawan tidak ditemukan`; aktif → 400.
  2. Validasi R3/R7/R8, plus cabang ada (`BranchExists`).
  3. Bila login akan diaktifkan dan `permaudit.HoldsSystemRole` → wajib `superAdmin`.
  4. `UPDATE hris.employees SET is_active=true, end_date=NULL, employment_status=$2, join_date=$3::date, is_access_app=$4, email=COALESCE($5,email), updated_at=now()`.
  5. `INSERT hris.employee_placements (employee_id, branch_id, is_primary, start_date, notes, created_by)`; error dipetakan lewat `placementError`.
  6. Non-Shift → `applyFixedSchedule(ctx, r, id, from, actor, clearGuard{})`. Periksa signature-nya; pemeriksaan periode absensi tertutup ikut berlaku. Shift → `schedule_note`.
  7. Login: `UPDATE auth.users SET banned_until=NULL`, `UPDATE configuration.users SET status='active', updated_at=now()`.
  8. `INSERT hris.employment_history (… change_type 'rehire', effective_date, prev_employment_status, new_employment_status, reason, notes 'Tanggal masuk sebelumnya: <old join>', recorded_by …)`.
  9. `writeEmployeeAudit(..., "employee.rehire", ...)` dengan before `{is_active:false, employment_status, join_date, end_date}` dan after `{is_active:true, employment_status, join_date, branch_id, reactivate_login, reason}`.
  - Handler: parse dengan pola `resignEmployee`. Status divalidasi terhadap daftar dari `loadEmployeeLists` atau setara (sebutkan sumbernya). `superAdmin` = `h.auth.HasRoleCode(ctx, u, "super_admin")`.

- [ ] **Step 4: generate route + goNativePrefixes; jalankan test → PASS; paket `hris` = baseline; `npx vitest run src/lib/backend-routes.test.ts` PASS.**

- [ ] **Step 5: Commit** `feat(hris): aktifkan kembali karyawan resign pada data yang sama (EPIC-064 Task 2)`.

**Exit criteria:** karyawan resign bisa diaktifkan kembali dalam satu transaksi dengan status, tanggal masuk, penempatan, jadwal NS, login (opsional) dan email (opsional); semua penolakan R3/R6/R7/R8 dengan pesan persis; audit dan history tercatat.

---

### Task 3: UI aktifkan kembali + label + paritas pesan fallback Next

**Files:**
- Create: `frontend/src/features/users/components/rehire-dialog.tsx` + `rehire-dialog.test.tsx`
- Modify: `frontend/src/features/users/api.ts` (`rehireEmployee(id, body)` → `POST /api/hris/employees/${id}/rehire`)
- Modify: `frontend/src/features/users/components/detail/employee-profile-header.tsx` (tombol `Aktifkan kembali` bila `onRehire && !employee.is_active`, di samping badge Resign)
- Modify: `frontend/src/features/users/components/user-detail-page.tsx` (state + mount dialog, gate `isHR` seperti resign, `onDone` → refetch)
- Modify: `frontend/src/features/audit/components/audit-labels.ts` (`"employee.rehire": "Karyawan diaktifkan kembali"`, bukan destructive), `frontend/src/lib/hris/employee-audit.ts` (`ACTIONS["employee.rehire"]="Diaktifkan kembali"`, FIELDS untuk key after baru: `join_date`, `branch_id`, `reactivate_login`), label history `rehire` (dan `resign`/`terminated`/`end_of_contract`/`contract_activated` yang selama ini mentah) di `employee-lifecycle-tab.tsx` dan `detail/employment-history-tab.tsx`
- Modify: `frontend/src/lib/api/handler.ts` (+ test) — 23505 dengan `error.constraint` `employees_{ktp,npwp,nip}_key` → pesan fallback R9 (paritas `WriteError`)

**Dialog (pola `resign-dialog.tsx`):**
- Tanggal efektif: `min` = `end_date`+1 hari, `max` = hari ini WIB.
- Status kepegawaian: dari master status (`features/users/api.ts` lookups) tanpa `resigned`/`terminated`.
- Cabang penempatan: pola `Select` + `useStoreBranches` dari `placement-form-dialog.tsx`.
- Checkbox "Aktifkan kembali akun login" (default tercentang), hanya bila `employee.user_id`.
- Input email hanya bila tidak ada `user_id`. Diisi otomatis kosong bila email saat ini berakhiran `@tanpa-email.invalid` atau `@resign.invalid`; selain itu email saat ini.
- Alasan (opsional, ≤500).
- Submit → toast `res.message` (+ `schedule_note` bila ada) → `onDone`. Error server → `toast.error`.

- [ ] **Step 1: Test gagal** (`rehire-dialog.test.tsx`, mock api & queries):
  - field tampil sesuai ada/tidaknya `user_id`;
  - status `resigned` tidak ditawarkan;
  - submit mengirim body yang benar;
  - error server muncul di toast.
  - Header: tombol tampil hanya untuk karyawan nonaktif + `onRehire`.
  - `handler.ts`: constraint → pesan.
- [ ] **Step 2: Jalankan → FAIL.**
- [ ] **Step 3: Implementasi.**
- [ ] **Step 4: `npx vitest run` penuh, `npx tsc --noEmit -p .`, eslint path tersentuh → bersih.**
- [ ] **Step 5: Commit** `feat(hris): tombol & dialog Aktifkan kembali karyawan, label audit/riwayat (EPIC-064 Task 3)`.

**Exit criteria:** HR membuka karyawan nonaktif → "Aktifkan kembali" → mengisi tanggal, status, cabang (dan email bila perlu) → karyawan aktif lagi; audit/riwayat berlabel Indonesia.

---

### Task 4: Verifikasi menyeluruh & dokumentasi

**Files:** `docs/epics/EPIC-064-hris-aktifkan-kembali.md` (Automation Log, status), `docs/epics/README.md`.

- [ ] Suite penuh Go (`-p 1 -count=1`) = baseline; vitest penuh; tsc.
- [ ] Smoke API di DB test (API Go lokal, bukan live): resign → rehire → login terbuka; tambah karyawan dengan KTP milik karyawan nonaktif → pesan R9.
- [ ] Automation Log: commit per task, ruling, bukti test, langkah rilis (tanpa migrasi; build ulang API & web), dan langkah data live untuk Alya (dikerjakan terpisah atas persetujuan user). Status `review`.
- [ ] Commit `docs: EPIC-064 aktifkan kembali karyawan selesai, menunggu review`.

## Follow-up (di luar rencana ini)
- Jalur "Tambah karyawan" di Manajemen User untuk orang yang pernah resign: pertimbangkan saran otomatis "aktifkan kembali" sebelum form dikirim (lookup pemilik KTP saat input).
- Membuat akun login untuk karyawan aktif yang belum punya login dari halaman detail.
