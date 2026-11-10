# Format NIP YYYYMM + Nomor Urut Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Karyawan baru hanya bisa dibuat dengan NIP `YYYYMM` (tahun-bulan join date) + nomor urut berikutnya di deretnya (utama atau harian). Format juga dicek saat NIP lama diganti.

**Architecture:**
- Aturannya ada di satu paket murni baru `platform/nipformat`: parse, deret, nomor terakhir, dan pesan.
- `platform/employeekeys` memuat semua NIP dari DB lalu memanggil paket itu. Hasilnya dipakai jalur buat dan ubah karyawan di `accounts`, `hris`, dan `recruitment`.
- Validasi impor (`domain.ValidateEmployeeImport`) memakai paket murni yang sama, dengan NIP dari `lk.ExistingNIP`.
- Tidak ada perubahan skema.

**Tech Stack:** Go (`backend/internal/platform`, `modules/{hris,configuration/accounts,recruitment}`), Next.js (teks placeholder), Postgres.

**Spec:** `docs/epics/EPIC-099-format-nip-urut.md` (ruling R1–R6).

## Global Constraints
- NIP tetap diketik HR; tidak ada nomor yang dibuat otomatis (EPIC-081, R1).
- Deret harian = status awal `daily_worker`, `freelance`, `intern`; selain itu deret utama (R2).
- Nomor ≥ 1000 ditulis 4 digit; nomor tidak pernah mulai ulang (R3).
- Ubah karyawan hanya mengecek format dan prefix, dan hanya bila NIP diganti (R4).
- Semua pesan NIP diawali `NIP ` (R6), karena form memetakan `/^NIP /` ke kolom NIP.
- Tidak ada perubahan skema DB.
- Branch `feat/nip-format-urut` (worktree `.worktrees/nip-format`) ditumpuk di atas PR #93.
- Commit diakhiri baris `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`.

## Review Focus
1. Join date di **tahun lalu** untuk karyawan baru deret utama (mis. `2025-12-15`): yang diterima `202512677`, karena nomornya mengikuti deret utama global, bukan tahun 2025 (Task 1).
2. NIP lama tidak berformat (`2026-05-594`, `EMP-2026-00001`, `000000`) **tidak ikut** dihitung sebagai nomor terakhir, dan tidak membuat error (Task 1).
3. Tahun baru belum punya nomor harian: deret harian 2027 mulai `001`, deret utama tetap lanjut (Task 1).
4. Mode Timpa memperbarui karyawan lama tanpa mengganti NIP → tidak ada cek format, walau NIP-nya `EMP-…` (Task 3).
5. Join date atau status di baris impor salah, atau sel kosong → hanya error kolom itu. Cek NIP dilewati, supaya tidak ada pesan ganda yang menyesatkan (Task 3).

---

### Task 1: Paket aturan `nipformat`

**Files:**
- Create: `backend/internal/platform/nipformat/nipformat.go`
- Test: `backend/internal/platform/nipformat/nipformat_test.go`

**Interfaces:**
- Produces:
  - `type Series string` dengan konstanta `Main Series = "main"` dan `Harian Series = "harian"`.
  - `func SeriesFor(status string) Series`.
  - `func Parse(nip string) (ym string, seq int, ok bool)`.
  - `type Last struct{ Main, Harian, HarianCap int }`. `HarianCap` 0 berarti tanpa batas.
  - `func LastFor(nips []string, year int) Last`.
  - `func Next(joinDate string, s Series, l Last) string`, dengan `joinDate` `YYYY-MM-DD`.
  - `func CheckNew(nip, joinDate, status string, l Last) string`.
  - `func CheckChanged(nip, joinDate string) string`.

  Kedua fungsi Check mengembalikan `""` bila NIP diterima.

- [ ] **Step 1: Tulis tes yang gagal**

`data := []string{"202512507","202601002","202611095","202611676","202610655","2026-05-594","EMP-2026-00001","000000","201601001"}`

- `TestParse`:
  - `202611677` → `("202611",677,true)`;
  - `2028011000` → `("202801",1000,true)`;
  - semua ini → `ok=false`: `202611000`, `20261167`, `202613001`, `2026110677` (4 digit dengan nol di depan), `2026-11-677`, `EMP-1`, ` 202611677`.
- `TestSeriesFor`: `daily_worker`, `freelance`, `intern` → `Harian`; `probation`, `contract`, `permanent`, `pkwt`, `""` → `Main`.
- `TestLastFor`:
  - `LastFor(data, 2026)` == `Last{Main:676, Harian:95, HarianCap:507}`;
  - `LastFor(data, 2027)` == `Last{Main:676, Harian:0, HarianCap:676}`;
  - `LastFor([]string{"202611005"}, 2026)` == `Last{Main:5, Harian:5, HarianCap:0}`.
- `TestNext`:
  - `Next("2026-11-10", Main, l2026)` == `"202611677"`;
  - `Next("2026-11-10", Harian, l2026)` == `"202611096"`;
  - `Next("2028-01-03", Main, Last{Main:999})` == `"2028011000"`.
- `TestCheckNew` (l = `LastFor(data, 2026)`). Yang diterima (`""`):
  - `("202611677","2026-11-10","probation")`;
  - `("202611700","2026-11-10","contract")`;
  - `("202611096","2026-11-10","daily_worker")`;
  - `("202512677","2025-12-15","probation")` dengan `LastFor(data,2025)`.

  Yang ditolak, dengan pesan persis:
  - `("2026-11-677","2026-11-10","probation")` → `NIP harus angka: 6 digit tahun-bulan join date + 3 digit nomor urut, mis. 202611677`
  - `("202610677","2026-11-10","probation")` → `NIP 202610677: 6 digit depan harus 202611 (tahun-bulan join date)`
  - `("202611650","2026-11-10","probation")` → `NIP 202611650: nomor urut terakhir karyawan 676 — pakai nomor berikutnya, mis. 202611677`
  - `("202611090","2026-11-10","freelance")` → `NIP 202611090: nomor urut terakhir Daily Worker/Freelance/Magang 2026 adalah 095 — pakai nomor berikutnya, mis. 202611096`
  - `("202611700","2026-11-10","intern")` → `NIP 202611700: nomor urut Daily Worker/Freelance/Magang 2026 maksimal 507 — mis. 202611096`
- `TestCheckChanged`:
  - `("202603518","2026-03-09")` → `""`;
  - `("202603518","2026-07-21")` → pesan prefix `…harus 202607…`;
  - `("EMP-9","2026-07-21")` → pesan format;
  - `("202603001","2026-03-09")` → `""` (nomor tidak dicek, R4).

- [ ] **Step 2: Jalankan dan pastikan gagal**

Run: `cd backend && go test ./internal/platform/nipformat/ -count=1`
Expected: FAIL (paket belum ada / compile error).

- [ ] **Step 3: Implementasi**

- `Parse` memakai regex `^(\d{4})(0[1-9]|1[0-2])(\d{3}|[1-9]\d{3})$` dan menolak nomor 0.
- `LastFor` menghitung dari NIP yang lolos `Parse` saja:
  - `Main` = nomor terbesar dari semua NIP;
  - `HarianCap` = nomor terbesar dari NIP bertahun < `year`;
  - `Harian` = nomor terbesar dari NIP bertahun `year` dengan nomor ≤ `HarianCap`, atau semua NIP tahun itu bila `HarianCap == 0`.
- `Next` = `YYYYMM` join date + `%03d` dari nomor terakhir + 1.
- `CheckNew`:
  1. cek format;
  2. cek prefix;
  3. cek nomor > nomor terakhir deret;
  4. untuk deret harian, cek juga nomor ≤ `HarianCap` bila cap > 0.

  Nomor terakhir di pesan ditulis `%03d`.
- `CheckChanged` hanya menjalankan langkah 1 dan 2.

- [ ] **Step 4: Jalankan dan pastikan lulus**

Run: `cd backend && go test ./internal/platform/nipformat/ -count=1 && go vet ./internal/platform/nipformat/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/platform/nipformat
git commit -m "feat(hris): aturan format NIP YYYYMM + nomor urut dua deret (EPIC-099)"
```

### Task 2: Terapkan di jalur API buat/ubah karyawan

**Files:**
- Modify: `backend/internal/platform/employeekeys/employeekeys.go`
- Modify: `backend/internal/modules/configuration/accounts/employees.go`: `createUser` (~baris 240) dan cabang ganti NIP saat update (~baris 494)
- Modify: `backend/internal/modules/hris/employees.go`: `CreateEmployee` (~baris 57) dan `UpdateEmployee` (~baris 155)
- Modify: `backend/internal/modules/recruitment/service_hris.go`: `Promote` (~baris 83)
- Test: `employeekeys_test.go` (integration bila paket sudah punya harness DB; bila tidak, cukup tes per modul di bawah), `accounts_integration_test.go`, `hris/employees_integration_test.go`, `recruitment/recruitment_integration_test.go`
- Modify (tes lama): setiap tes integration yang membuat karyawan lewat 4 endpoint dengan NIP sembarang (`grep -rn '"nip"' --include=*_test.go internal`)

**Interfaces:**
- Consumes: semua fungsi Task 1.
- Produces:
  - `func employeekeys.CheckNewNIP(ctx, q database.Querier, nip, joinDate, status string) error`, yang mengembalikan `httpx.BadRequest(pesan)` atau nil;
  - `func employeekeys.NextNIP(ctx, q database.Querier, joinDate, status string) (string, error)`, dipakai helper tes di semua modul;
  - `func employeekeys.CheckChangedNIP(nip, joinDate string) error`.

  Data dimuat dengan `SELECT nip FROM hris.employees` (semua status), lalu `nipformat.LastFor(nips, tahun joinDate)`.

- [ ] **Step 1: Tulis tes yang gagal**

Ketiga modul punya tes `TestEmployeeNIPFormat`. Setiap tes memakai `next, _ := employeekeys.NextNIP(ctx, db, "2026-11-10", "probation")`. Untuk recruitment, join date dikirim di body.

Yang ditolak (400):
- NIP `next` dengan join `2026-10-10` → pesan memuat `6 digit depan harus 202610`;
- `"2026-11-001"` → `NIP harus angka`;
- nomor terakhir deret (`next` dikurangi 1 dalam bentuk NIP) → `nomor urut terakhir karyawan`.

Yang diterima:
- `next` → 201 (accounts/hris) atau 200 (promote);
- status `daily_worker` dengan `NextNIP(..., "daily_worker")` → diterima.

Tes ubah (accounts dan hris):
- PUT `nip: "EMP-X"` → 400 pesan format;
- PUT `phone` saja pada karyawan ber-NIP `EMP-…`, yang dibuat langsung lewat SQL di tes → 200.

- [ ] **Step 2: Jalankan dan pastikan gagal**

Run: `cd backend && go test ./internal/modules/configuration/accounts/ ./internal/modules/hris/ ./internal/modules/recruitment/ -run TestEmployeeNIPFormat -count=1` (DB uji `paddydev-sdd-testdb` :55499)
Expected: FAIL (fungsi belum ada atau NIP salah format diterima).

- [ ] **Step 3: Implementasi**

Pembuatan karyawan:
- `CheckNewNIP` dipanggil sesudah `CleanNIP` dan sebelum cek bentrok, dengan join date dan status yang akan disimpan.
- Promote memakai `joinDate` dan `status` setelah default diterapkan.

Ubah karyawan:
- Bila NIP dikirim **dan** berbeda dari NIP sekarang, panggil `CheckChangedNIP` dengan join date baru bila dikirim, atau join date lama (`existing.JoinDate`, atau `current` di hris).

Tes lama yang gagal karena NIP sembarang diganti ke `NextNIP`, dengan join date dan status yang sama dengan body.
- Tes yang memang menguji NIP kosong, panjang, atau bentrok tetap pada nilainya.
- Untuk tes bentrok, gunakan NIP pertama yang dibuat valid.

- [ ] **Step 4: Jalankan dan pastikan lulus**

Run: `cd backend && go vet ./... && go test ./internal/... -count=1 > /tmp/claude-1000/e099-t2.log 2>&1; tail -40 /tmp/claude-1000/e099-t2.log`
Expected: PASS seluruh `./internal/...`.

- [ ] **Step 5: Commit**

```bash
git add backend/internal
git commit -m "feat(hris): karyawan baru wajib NIP YYYYMM join date + nomor urut berikutnya (EPIC-099)"
```

### Task 3: Terapkan di impor Excel + petunjuk template

**Files:**
- Modify: `backend/internal/modules/hris/domain/employee_import.go` (`ValidateEmployeeImport`; sebelum `out = append(out, row)`, ~baris 418)
- Modify: `backend/internal/modules/hris/employee_excel.go`: teks Petunjuk baris ~248 dan hint `"nip"` baris ~255
- Test: `backend/internal/modules/hris/domain/employee_import_test.go`, `backend/internal/modules/hris/employee_excel_integration_test.go`

**Interfaces:**
- Consumes: `nipformat.LastFor`, `CheckNew`, `SeriesFor`, `Parse` (Task 1).
- Produces: tidak ada interface baru.
  - NIP yang dihitung adalah kunci `lk.ExistingNIP`. `LastFor` dihitung sekali per tahun join date (cache map).

- [ ] **Step 1: Tulis tes yang gagal**

Tes domain memakai `lk()` dengan tambahan `ExistingNIP` `202611676` (probation) dan `202611095`.

`TestValidateEmployeeImportNIPFormat`, mode `add`, join `10/11/2026`:
- baris `202611677` probation → tanpa error;
- `202611650` → error kolom NIP `nomor urut terakhir karyawan 676`;
- `202610678` → `6 digit depan harus 202611`;
- `202611096` daily_worker → tanpa error;
- `202612677` probation di baris berikutnya → `NIP 202612677: nomor urut 677 sudah dipakai baris N di file` (R5). Join baris ini `01/12/2026`, supaya prefix-nya benar.

Tes tambahan:
- `TestValidateEmployeeImportNIPFormatSkipsBadDate`: join `31/02/2026` → hanya error kolom Tanggal Masuk, tanpa error NIP.
- `TestValidateEmployeeImportReplaceKeepsOldNIP`: mode `replace` untuk NIP lama `EMP-2026-00001` yang ada di `ExistingNIP` → tanpa error NIP.
- Tes impor lama yang memakai NIP baru sembarang diganti ke NIP valid sesuai lookup tesnya.
- Integration `TestEmployeeImportManagerInSameFile` (EPIC-098) diganti ke `employeekeys.NextNIP` dan `NextNIP` + 1 untuk dua baris barunya.

- [ ] **Step 2: Jalankan dan pastikan gagal**

Run: `cd backend && go test ./internal/modules/hris/domain/ -run 'TestValidateEmployeeImport' -count=1`
Expected: FAIL (`202611650` dan `202610678` diterima).

- [ ] **Step 3: Implementasi**

Cek dijalankan hanya untuk baris dengan:
- `row.Action == "create"`;
- NIP bersih tidak kosong;
- `Set["join_date"]` dan `Set["employment_status"]` sudah terisi (keduanya lolos parse).

Error ditulis di kolom NIP.

Untuk R5, simpan `map[Series+tahun-deret]nomor → baris` untuk baris baru yang lolos. Kunci deret harian = `harian:YYYY`, deret utama = `main`.

Teks template:
- Petunjuk menjadi `Mode Tambah: hanya karyawan baru; NIP yang sudah ada ditolak. NIP wajib diisi: 6 digit tahun-bulan Tanggal Masuk + nomor urut berikutnya (mis. 202611677).`
- Hint kolom NIP menjadi `Teks. YYYYMM Tanggal Masuk + nomor urut berikutnya`.

- [ ] **Step 4: Jalankan dan pastikan lulus**

Run: `cd backend && go test ./internal/modules/hris/... -count=1 > /tmp/claude-1000/e099-t3.log 2>&1; tail -30 /tmp/claude-1000/e099-t3.log`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/modules/hris
git commit -m "feat(hris): impor karyawan mengecek format dan nomor urut NIP (EPIC-099)"
```

### Task 4: Placeholder form + verifikasi staging + catatan epic

**Files:**
- Modify: `frontend/src/features/users/components/user-form/employment-section.tsx:49`
- Modify: `frontend/src/components/hris/PromoteCandidateButton.tsx:163`
- Test: `frontend/src/features/users/components/user-form-page.test.tsx:95`, `frontend/src/components/hris/PromoteCandidateButton.test.tsx:37`
- Modify: `docs/epics/EPIC-099-format-nip-urut.md`, `docs/epics/README.md`

- [ ] **Step 1:** Ubah ekspektasi placeholder di kedua tes menjadi `"mis. 202611677 (tahun-bulan join date + nomor urut)"`. Run `cd frontend && npx vitest run src/features/users/components/user-form-page.test.tsx src/components/hris/PromoteCandidateButton.test.tsx`. Expected: FAIL 2 tes.
- [ ] **Step 2:** Ubah kedua placeholder ke teks yang sama. Jalankan perintah yang sama, lalu `npx tsc --noEmit`. Expected: PASS, tsc 0.
- [ ] **Step 3:** Gate: `cd backend && gofmt -l ./internal && go vet ./... && go test ./internal/... -count=1`. Expected: gofmt tanpa output, PASS.
- [ ] **Step 4:** Deploy ke staging:
  - backup DB staging ke `/mnt/hdd1/backup/paddydev/`;
  - jalankan `umask 022; bash tools/deploy-paddydev.sh` dari worktree.

  Uji lewat API (sesi HR uji) untuk setiap butir Acceptance Criteria EPIC-099: `202611677` diterima, lalu `202611650`, `202610677`, `2026-11-677` ditolak; DW `202611096` diterima, `202611700` ditolak; satu file impor dengan nomor ganda.

  Expected: sesuai AC. Sesudahnya hapus data dan sesi uji, dengan `set local search_path = hris, public`.
- [ ] **Step 5:** Isi Automation Log EPIC-099 dengan hasil gate dan staging. Set status `review` dan tambahkan baris di `docs/epics/README.md`. Commit `docs: EPIC-099 hasil gate dan QA staging`.
