# Kasbon Sesuai Acuan MOM — Implementation Plan (EPIC-060)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. Also load the project skill `paddy-hris-acuan` before starting.

**Goal:** Kasbon mengikuti MOM 19-10-2026: Pinjaman Darurat dihapus, plafon 50% THP (4 komponen), tenor 1 atau 2 (default 2), Probation/Freelance/Intern tidak bisa mengajukan, dan approval dua tahap (Manager Departemen → HR Penggajian).

**Architecture:**
- Aturan kasbon ada di `backend/internal/modules/payroll/domain/kasbon.go` (fungsi murni).
- Service pinjaman (`backend/internal/modules/payroll/loans.go`):
  - Menolak jenis yang dihapus.
  - Menegakkan aturan kasbon.
  - Saat kasbon dibuat, menyimpan **approver manager** (`manager_approver_id`): atasan berlevel Manager ke atas pertama di rantai `reporting_to`, lewat port HRIS baru `LoanApprover`.
- Keputusan pinjaman bercabang. Approver manager memutus tahap 1 (`pending` → `manager_approved`). HR dengan aksi `approve` di menu Pinjaman **dan** menu Penggajian memutus tahap 2.
- Kasbon tanpa approver manager, dan kasbon lama yang dibuat sebelum rilis, langsung ke tahap HR.
- Frontend:
  - Form ESS/HR hanya menawarkan Kasbon/Pinjaman dengan tenor kasbon 1/2.
  - ESS punya antrean "Persetujuan Kasbon Tim" untuk manager.
  - Tabel HR mengenali status `manager_approved`.

**Tech Stack:** Go (pgx), migrasi PostgreSQL, Next.js (React Query) + vitest.

**Spec:** `docs/hris/acuan-mom-20261019/aturan-payroll.md` (baris Pinjaman/Kasbon), `matriks-approval.md` (Kasbon), `pertanyaan-rekomendasi.md` (K-6, Q-KP-1, Q-KP-2, Q-APV-2; keputusan user 20-10-2026: ikuti rekomendasi).

**Prasyarat:** branch `feat/epic060-kasbon-acuan` dari `main`. Tidak bergantung pada EPIC-059, tetapi bila Task 6 EPIC-059 (signature `Active`) sudah masuk, ikuti signature terbaru di fake `Employees`.

## Ruling

- **R1 — Jenis pinjaman yang bisa diajukan** = `kasbon` dan `loan`. `emergency` ditolak 400 `Pinjaman Darurat sudah tidak tersedia`. Jenis lain ditolak dengan pesan `invalidLoan` yang sudah ada. Riwayat `emergency` tetap tampil dengan label lama, dan pengajuan lama yang masih pending tetap bisa diputus HR (Q-KP-1: diputus manual).
- **R2 — THP kasbon** = jumlah `base_salary + transport_allowance + meal_allowance + position_allowance` dari struktur gaji **aktif** (semua entitas). Plafon: pokok kasbon ≤ 50% THP.
- **R3 — Tenor kasbon** ∈ {1, 2}; default form 2 (cicilan 50/50). Jenis `loan` tetap memakai aturan lama (tenor 1–60, cicilan ≤ `loan_max_installment_percent` gaji pokok), sambil menunggu TnC (KP-03).
- **R4 — Status yang diblokir** untuk kasbon: `probation`, `freelance`, `intern`, `internship` (huruf besar/kecil diabaikan). Pesan: `Karyawan berstatus Probation/Freelance/Magang tidak dapat mengajukan kasbon`.
- **R5 — Approver manager (Q-APV-2)** = karyawan aktif pertama di rantai `reporting_to` pemohon (bukan pemohon sendiri) yang level jabatannya `Manager`, `Senior Manager`, `General Manager`, `Director`, atau `C-Level`. Assistant Manager tidak termasuk ("Manager Departemen"). Penelusuran maksimal 10 tingkat dan berhenti pada siklus atau `reporting_to` diri sendiri (data live: 1 kasus). Nilai disimpan saat kasbon dibuat (`manager_approver_id`). Kalau tidak ditemukan: NULL, tahap 1 dilewati.
- **R6 — Tahap HR** butuh aksi `approve` di menu Pinjaman (`iam.HrisLoans`) **dan** menu Penggajian (`hris.compensation`, `actor.Payroll`) untuk jenis `kasbon` (MOM AP-05). **Ini mengubah keputusan EPIC-057** untuk kasbon: akun Non-Payroll (paddy.peopledevelopment) tidak lagi bisa menyetujui kasbon, tetapi tetap bisa menyetujui jenis `loan`. Cost if wrong: HR Non-Payroll perlu diberi menu Penggajian untuk kasbon.
- **R7 — Kasbon lama** (dibuat sebelum migrasi; `manager_approver_id` NULL) langsung ke tahap HR. Tidak ada backfill.
- **R8 — Manager tidak melihat plafon.** Tahap manager tidak menghitung plafon (tahap HR yang menghitung ulang) dan respons tidak memuat angka gaji.

## Global Constraints

- Uji integrasi hanya di DB test `postgres://postgres@127.0.0.1:55499/paddy_test?sslmode=disable`; jangan pakai port 55432 dan jangan menyentuh DB live.
- Go: `$HOME/.local/go/bin/go`; `gofmt -l` bersih sebelum commit.
- Angka gaji/plafon hanya untuk pemegang menu Penggajian atau pemohon sendiri (aturan EPIC-057 `showCap`).
- Migrasi baru di `backend/database/migrations/deltas/` dengan timestamp lebih besar dari migrasi terakhir di `main`.
- Baca `frontend/AGENTS.md` sebelum mengubah kode Next.js.
- Commit per task dengan trailer `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`. Jangan commit ke `main` dan jangan push tanpa izin user.

## Review Focus

1. **Rantai `reporting_to` yang siklus atau menunjuk diri sendiri:** penelusuran approver harus berhenti dan tidak hang. Diuji di Task 3.
2. **Manager yang juga atasan langsung kedua** (A → SPV → Manager): approver = Manager, bukan SPV. Diuji di Task 3.
3. **Approver manager mencoba menyetujui tahap HR**, atau HR mencoba menyetujui sebelum manager: harus 400/403, bukan lolos. Diuji di Task 3.
4. **Kasbon dengan struktur gaji di dua entitas:** THP dijumlah dari semua struktur aktif. Diuji di Task 2.
5. **Respons tahap manager tidak membocorkan gaji**, termasuk pesan error plafon. Diuji di Task 3.

---

### Task 1: Aturan kasbon (domain murni)

**Files:**
- Create: `backend/internal/modules/payroll/domain/kasbon.go`
- Test: `backend/internal/modules/payroll/domain/kasbon_test.go`

**Interfaces:**
- Consumes: `FormatRupiah` (sudah ada di `domain`).
- Produces:
  - `func KasbonBlockedStatus(status string) bool`
  - `func IsManagerLevel(level string) bool`
  - `func ValidateKasbon(principal float64, tenor int, status string, thp *float64, activeCount, maxActive float64, showCap bool) string` (`""` bila lolos)
  - `const KasbonMaxThpPercent = 50`

- [ ] **Step 1: Tulis test yang gagal**

```go
package domain

import "testing"

func TestKasbonBlockedStatus(t *testing.T) {
	for s, want := range map[string]bool{
		"probation": true, "Freelance": true, "intern": true, " internship ": true,
		"contract": false, "permanent": false, "daily_worker": false, "": false,
	} {
		if got := KasbonBlockedStatus(s); got != want {
			t.Errorf("KasbonBlockedStatus(%q) = %v, want %v", s, got, want)
		}
	}
}

func TestIsManagerLevel(t *testing.T) {
	for l, want := range map[string]bool{
		"Manager": true, "manager": true, "Senior Manager": true, "General Manager": true, "Director": true, "C-Level": true,
		"Assistant Manager": false, "Supervisor": false, "Staff": false, "": false,
	} {
		if got := IsManagerLevel(l); got != want {
			t.Errorf("IsManagerLevel(%q) = %v, want %v", l, got, want)
		}
	}
}

func TestValidateKasbon(t *testing.T) {
	thp := 5_000_000.0
	ok := func(principal float64, tenor int, status string) string {
		return ValidateKasbon(principal, tenor, status, &thp, 0, 1, true)
	}
	if msg := ok(2_500_000, 2, "contract"); msg != "" {
		t.Fatalf("50%% THP tenor 2 harus lolos: %q", msg)
	}
	if msg := ok(2_500_001, 1, "contract"); msg != "Kasbon maksimal 50% THP (maks Rp 2.500.000)" {
		t.Fatalf("melebihi plafon: %q", msg)
	}
	if msg := ValidateKasbon(2_500_001, 1, "contract", &thp, 0, 1, false); msg != "Kasbon maksimal 50% THP" {
		t.Fatalf("tanpa akses Penggajian tidak boleh menyebut angka: %q", msg)
	}
	if msg := ok(1_000_000, 3, "contract"); msg != "Tenor kasbon hanya 1 atau 2 bulan" {
		t.Fatalf("tenor 3: %q", msg)
	}
	if msg := ok(1_000_000, 2, "probation"); msg != "Karyawan berstatus Probation/Freelance/Magang tidak dapat mengajukan kasbon" {
		t.Fatalf("probation: %q", msg)
	}
	if msg := ValidateKasbon(1_000_000, 2, "contract", nil, 0, 1, true); msg != "Karyawan belum punya struktur gaji aktif — atur gaji dulu sebelum mengajukan pinjaman" {
		t.Fatalf("tanpa struktur gaji: %q", msg)
	}
	if msg := ValidateKasbon(1_000_000, 2, "contract", &thp, 1, 1, true); msg != "Karyawan sudah punya 1 pinjaman aktif (maks 1)" {
		t.Fatalf("pinjaman aktif: %q", msg)
	}
}
```

Cek format `FormatRupiah(2500000)` di `domain` (`grep -n "func FormatRupiah" -A8 backend/internal/modules/payroll/domain/*.go`). Kalau hasilnya bukan `Rp 2.500.000` (mis. tanpa spasi), sesuaikan string ekspektasi di test dengan format yang ada. Jangan mengubah `FormatRupiah`.

- [ ] **Step 2: Jalankan test, pastikan gagal**

Run: `cd backend && $HOME/.local/go/bin/go test ./internal/modules/payroll/domain -run 'TestKasbonBlockedStatus|TestIsManagerLevel|TestValidateKasbon' -v`
Expected: FAIL (build error `undefined: KasbonBlockedStatus`).

- [ ] **Step 3: Implementasi**

`backend/internal/modules/payroll/domain/kasbon.go`:

```go
package domain

import (
	"fmt"
	"slices"
	"strings"
)

// Kasbon rules (MOM 19-10-2026 KP-02, tabel payslip Paddy; EPIC-060).
const KasbonMaxThpPercent = 50

var (
	kasbonTenors          = []int{1, 2}
	kasbonBlockedStatuses = []string{"probation", "freelance", "intern", "internship"}
	// managerLevels: "Manager Departemen" and above approve kasbon step 1
	// (Assistant Manager excluded).
	managerLevels = []string{"manager", "senior manager", "general manager", "director", "c-level"}
)

func normLevel(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

// KasbonBlockedStatus: Probation, Freelance and Intern cannot take a kasbon.
func KasbonBlockedStatus(status string) bool { return slices.Contains(kasbonBlockedStatuses, normLevel(status)) }

// IsManagerLevel: a position level of Manager or above.
func IsManagerLevel(level string) bool { return slices.Contains(managerLevels, normLevel(level)) }

// ValidateKasbon is "" when the kasbon fits: allowed status, tenor 1 or 2,
// active-loan count, and principal ≤ 50% of THP (4 components). showCap false
// leaves the cap amount out (it reveals salary; EPIC-057).
func ValidateKasbon(principal float64, tenor int, status string, thp *float64, activeCount, maxActive float64, showCap bool) string {
	if KasbonBlockedStatus(status) {
		return "Karyawan berstatus Probation/Freelance/Magang tidak dapat mengajukan kasbon"
	}
	if !slices.Contains(kasbonTenors, tenor) {
		return "Tenor kasbon hanya 1 atau 2 bulan"
	}
	if activeCount >= maxActive {
		return fmt.Sprintf("Karyawan sudah punya %v pinjaman aktif (maks %v)", activeCount, maxActive)
	}
	if thp == nil || *thp <= 0 {
		return "Karyawan belum punya struktur gaji aktif — atur gaji dulu sebelum mengajukan pinjaman"
	}
	limit := *thp * KasbonMaxThpPercent / 100
	if principal <= limit {
		return ""
	}
	if !showCap {
		return "Kasbon maksimal 50% THP"
	}
	return fmt.Sprintf("Kasbon maksimal 50%% THP (maks %s)", FormatRupiah(limit))
}
```

- [ ] **Step 4: Jalankan test, pastikan lulus**

Run: perintah Step 2. Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/modules/payroll/domain/kasbon.go backend/internal/modules/payroll/domain/kasbon_test.go
git commit -m "feat(payroll): aturan kasbon — plafon 50% THP, tenor 1/2, status diblokir, EPIC-060"
```

---

### Task 2: Pengajuan pinjaman — tolak Pinjaman Darurat, tegakkan aturan kasbon

**Files:**
- Modify: `backend/internal/modules/payroll/loans.go` (`createLoan` service + `decideLoan` validasi saat approve; tambah `kasbonTHP`)
- Test: `backend/internal/modules/payroll/kasbon_integration_test.go` (baru)

**Interfaces:**
- Consumes: `domain.ValidateKasbon` (Task 1); `s.loanLimits` (lama, untuk `loan`); `s.brief(ctx, id)` (`EmploymentStatus`).
- Produces:
  - `func (s *service) kasbonTHP(ctx context.Context, employeeID string) (*float64, error)`.
  - Pesan: `Pinjaman Darurat sudah tidak tersedia`.

- [ ] **Step 1: Tulis test yang gagal**

`backend/internal/modules/payroll/kasbon_integration_test.go` (paket `payroll`, memakai `setup`, `hrStaff`, `e.employee`, `e.exec`, `e.call`, `wantErr` dari test yang sudah ada):

```go
package payroll

import "testing"

// THP kasbon = pokok + transport + makan + jabatan dari semua struktur aktif.
func kasbonWorker(e *env, status string) string {
	e.t.Helper()
	worker := e.employee("Peminjam Kasbon", e.staff.UserID, true)
	e.exec(`UPDATE hris.employees SET employment_status = $2 WHERE id = $1`, worker, status)
	// Fake HRIS port: brief status ikut berubah (createLoan membaca status lewat port).
	b := e.emps.briefs[worker]
	b.EmploymentStatus = status
	e.emps.briefs[worker] = b
	e.exec(`INSERT INTO hris.employee_salary (employee_id, company_id, base_salary, transport_allowance, meal_allowance,
		position_allowance, ptkp_status, effective_date) VALUES ($1, $2, 3000000, 500000, 500000, 1000000, 'TK/0', '2026-01-01')`,
		worker, e.co)
	return worker
}

func TestKasbonCreateRules(t *testing.T) {
	e := setup(t)
	kasbonWorker(e, "contract") // THP 5.000.000 → plafon 2.500.000
	post := func(body map[string]any, status int) map[string]any {
		return e.call(&e.staff, "POST", "/api/hris/loans", body, status)
	}
	wantErr(t, post(map[string]any{"loan_type": "emergency", "principal_amount": 500000, "tenor_months": 1}, 400),
		"Pinjaman Darurat sudah tidak tersedia")
	wantErr(t, post(map[string]any{"loan_type": "kasbon", "principal_amount": 1000000, "tenor_months": 3}, 400),
		"Tenor kasbon hanya 1 atau 2 bulan")
	wantErr(t, post(map[string]any{"loan_type": "kasbon", "principal_amount": 2600000, "tenor_months": 2}, 400),
		"Kasbon maksimal 50% THP (maks Rp 2.500.000)")
	created := post(map[string]any{"loan_type": "kasbon", "principal_amount": 2500000, "tenor_months": 2}, 200)
	if d := created["data"].(map[string]any); d["monthly_installment"] != "1250000.00" || d["status"] != "pending" {
		t.Fatalf("kasbon 2 x 50%%: %v", d)
	}
}

func TestKasbonBlockedForProbation(t *testing.T) {
	e := setup(t)
	kasbonWorker(e, "probation")
	wantErr(t, e.call(&e.staff, "POST", "/api/hris/loans", map[string]any{"loan_type": "kasbon",
		"principal_amount": 500000, "tenor_months": 1}, 400),
		"Karyawan berstatus Probation/Freelance/Magang tidak dapat mengajukan kasbon")
}
```

Pesan plafon memakai format `FormatRupiah` yang dicek di Task 1. Samakan string ekspektasinya.

- [ ] **Step 2: Jalankan test, pastikan gagal**

Run: `cd backend && TEST_DATABASE_URL='postgres://postgres@127.0.0.1:55499/paddy_test?sslmode=disable' $HOME/.local/go/bin/go test ./internal/modules/payroll -run 'TestKasbonCreateRules|TestKasbonBlockedForProbation' -count=1 -v`
Expected: FAIL. `emergency` masih diterima (status 200, want 400).

- [ ] **Step 3: Implementasi**

`loans.go`, tambahkan:

```go
// kasbonTHP is the kasbon THP (EPIC-060 R2): base + transport + meal +
// position allowance over every active salary row; nil without one.
func (s *service) kasbonTHP(ctx context.Context, employeeID string) (*float64, error) {
	var thp *float64
	err := s.db.QueryRow(ctx, `SELECT sum(base_salary + coalesce(transport_allowance, 0) + coalesce(meal_allowance, 0)
		+ coalesce(position_allowance, 0))::float8 FROM hris.employee_salary WHERE employee_id = $1 AND is_active`,
		employeeID).Scan(&thp)
	return thp, err
}

// loanTypes are the requestable loan types (EPIC-060 R1; emergency removed).
var loanTypes = []string{"kasbon", "loan"}
```

Di `createLoan` service, tepat setelah pengecekan `!emp.IsActive`:

```go
	if in.LoanType == "emergency" {
		return nil, httpx.BadRequest("Pinjaman Darurat sudah tidak tersedia")
	}
	if !slices.Contains(loanTypes, in.LoanType) {
		return nil, httpx.BadRequest(invalidLoan)
	}
```

Ganti pengecekan batas (`config, base, count, err := s.loanLimits(...)` sampai `ValidateLoanLimitsFor(...)`) dengan:

```go
	config, base, count, err := s.loanLimits(ctx, target, true)
	if err != nil {
		return nil, err
	}
	showCap := a.Payroll || (a.EmployeeID != nil && *a.EmployeeID == target)
	var msg string
	if in.LoanType == "kasbon" {
		thp, err := s.kasbonTHP(ctx, target)
		if err != nil {
			return nil, err
		}
		msg = domain.ValidateKasbon(in.Principal, in.Tenor, emp.EmploymentStatus, thp, count, config.LoanMaxActivePerPerson, showCap)
	} else {
		msg = domain.ValidateLoanLimitsFor(raw, base, config.LoanMaxInstallmentPct, count, config.LoanMaxActivePerPerson, showCap)
	}
	if msg != "" {
		return nil, httpx.BadRequest(msg)
	}
```

Tambahkan import `"slices"`. Di `decideLoan` (blok `if approve {`), ganti validasi batas dengan cabang yang sama. Butuh `loan_type`, `principal_amount`, dan `tenor_months` pinjaman: perluas `SELECT` awal menjadi `SELECT employee_id::text, COALESCE(status, ''), monthly_installment::text, loan_type, principal_amount::float8, tenor_months FROM hris.loans WHERE id::text = $1`, scan ke variabel baru. Untuk kasbon, ambil status karyawan via `s.brief`, lalu panggil `domain.ValidateKasbon(principal, tenor, brief.EmploymentStatus, thp, count, config.LoanMaxActivePerPerson, showCap)`. Struktur fungsi ini akan diubah lagi di Task 3; cukup pastikan validasinya benar di sini.

- [ ] **Step 4: Jalankan test, pastikan lulus, lalu paket payroll**

Run: perintah Step 2. Expected: PASS.

Run: `cd backend && TEST_DATABASE_URL=... $HOME/.local/go/bin/go test ./internal/modules/payroll/... -count=1`
Expected: `ok`. Test lama yang membuat kasbon tenor 3 (mis. `TestLoanApprovalHidesCapWithoutPayrollMenu`, tenor 3) gagal karena aturan baru. Ubah tenor menjadi 2 dan sesuaikan nominal, atau ganti `loan_type` ke `"loan"` bila test itu menguji aturan cicilan lama. Catat ruling per test di ledger.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/modules/payroll/loans.go backend/internal/modules/payroll/kasbon_integration_test.go backend/internal/modules/payroll/*_test.go
git commit -m "feat(payroll): tolak Pinjaman Darurat dan tegakkan aturan kasbon saat pengajuan & approve, EPIC-060"
```

---

### Task 3: Approval kasbon dua tahap (Manager Departemen → HR Penggajian)

**Files:**
- Create: `backend/database/migrations/deltas/20261021100000_loans_manager_approval.sql`
- Modify: `backend/internal/modules/payroll/ports.go` (port `Employees.LoanApprover`)
- Modify: `backend/internal/app/adapters_payroll.go` (implementasi SQL)
- Modify: `backend/internal/modules/payroll/loans.go` (`createLoan` simpan approver; `decideLoan` dipecah; `listLoans` scope approvals; handler `decideLoan`)
- Modify: `backend/internal/modules/payroll/payroll_integration_test.go` (fake `LoanApprover`, map level)
- Test: `backend/internal/modules/payroll/kasbon_integration_test.go`, `backend/internal/app/loan_approver_test.go` (baru)

**Interfaces:**
- Consumes: `domain.IsManagerLevel` (Task 1), `domain.ValidateKasbon` (Task 2), `actor.Payroll`, `h.auth.RequireMenuAction`.
- Produces:
  - Kolom `hris.loans.manager_approver_id`, `manager_approved_by`, `manager_approved_at`.
  - Status `manager_approved`.
  - `Employees.LoanApprover(ctx, q, employeeID string) (*string, error)`.
  - `GET /api/hris/loans?scope=approvals`.

- [ ] **Step 1: Tulis test yang gagal**

Tambahkan ke `kasbon_integration_test.go`:

```go
// Rantai: pemohon → SPV → Manager. Approver tahap 1 = Manager (bukan SPV).
func TestKasbonTwoStepApproval(t *testing.T) {
	e := setup(t)
	// Akun manager tanpa menu HRIS: hanya bisa memutus sebagai approver manager.
	mgrStaff := hrStaff(e, map[string][]string{"go_test_none": nil})
	mgr := e.employee("Manager Dept", mgrStaff.UserID, true)
	spv := e.employee("Supervisor", "", true)
	worker := kasbonWorker(e, "contract")
	e.exec(`UPDATE hris.employees SET reporting_to = $2 WHERE id = $1`, worker, spv)
	e.exec(`UPDATE hris.employees SET reporting_to = $2 WHERE id = $1`, spv, mgr)
	e.setLevel(mgr, "Manager")
	e.setLevel(spv, "Supervisor")

	loan := e.call(&e.staff, "POST", "/api/hris/loans", map[string]any{"loan_type": "kasbon",
		"principal_amount": 1000000, "tenor_months": 2}, 200)["data"].(map[string]any)
	id := loan["id"].(string)
	if loan["manager_approver_id"] != mgr {
		t.Fatalf("manager_approver_id = %v, want %s", loan["manager_approver_id"], mgr)
	}

	// HR sebelum manager → ditolak.
	wantErr(t, e.call(&e.hr, "POST", "/api/hris/loans/"+id+"/approve", map[string]any{"approved": true}, 400),
		"Kasbon menunggu persetujuan Manager Departemen")
	// Antrean manager memuat kasbon ini, tanpa angka gaji.
	queue := e.call(&mgrStaff, "GET", "/api/hris/loans?scope=approvals", nil, 200)["data"].([]any)
	if len(queue) != 1 || queue[0].(map[string]any)["id"] != id {
		t.Fatalf("antrean manager = %v", queue)
	}
	// Manager menyetujui tahap 1.
	step1 := e.call(&mgrStaff, "POST", "/api/hris/loans/"+id+"/approve", map[string]any{"approved": true}, 200)
	if d := step1["data"].(map[string]any); d["status"] != "manager_approved" || d["manager_approved_by"] != mgr {
		t.Fatalf("tahap 1: %v", d)
	}
	// Manager tidak bisa memutus tahap HR.
	e.call(&mgrStaff, "POST", "/api/hris/loans/"+id+"/approve", map[string]any{"approved": true}, 403)
	// HR Penggajian menyetujui tahap 2.
	step2 := e.call(&e.hr, "POST", "/api/hris/loans/"+id+"/approve", map[string]any{"approved": true}, 200)
	if step2["data"].(map[string]any)["status"] != "approved" {
		t.Fatalf("tahap 2: %v", step2)
	}
}

// Tanpa Manager di rantai (atau reporting_to diri sendiri): langsung tahap HR.
func TestKasbonWithoutManagerGoesToHR(t *testing.T) {
	e := setup(t)
	worker := kasbonWorker(e, "contract")
	e.exec(`UPDATE hris.employees SET reporting_to = id WHERE id = $1`, worker) // siklus diri sendiri
	loan := e.call(&e.staff, "POST", "/api/hris/loans", map[string]any{"loan_type": "kasbon",
		"principal_amount": 1000000, "tenor_months": 1}, 200)["data"].(map[string]any)
	if loan["manager_approver_id"] != nil {
		t.Fatalf("approver = %v, want nil", loan["manager_approver_id"])
	}
	e.call(&e.hr, "POST", "/api/hris/loans/"+loan["id"].(string)+"/approve", map[string]any{"approved": true}, 200)
}

// Tahap HR kasbon butuh menu Penggajian (MOM AP-05; R6).
func TestKasbonHRStepNeedsPayrollMenu(t *testing.T) {
	e := setup(t)
	loansOnly := hrStaff(e, map[string][]string{"hris.kepegawaian.loans": {"read", "create", "update", "approve"}})
	kasbonWorker(e, "contract")
	loan := e.call(&e.staff, "POST", "/api/hris/loans", map[string]any{"loan_type": "kasbon",
		"principal_amount": 1000000, "tenor_months": 1}, 200)["data"].(map[string]any)
	wantErr(t, e.call(&loansOnly, "POST", "/api/hris/loans/"+loan["id"].(string)+"/approve", map[string]any{"approved": true}, 403),
		"Persetujuan kasbon oleh HR butuh akses Penggajian")
}
```

Tambahkan helper ke `payroll_integration_test.go` (dekat `func (e *env) salary`):

```go
// setLevel gives an employee a position with the given level. The fake
// LoanApprover reads the chain from the test DB (same SQL as the adapter).
func (e *env) setLevel(employeeID, level string) {
	e.t.Helper()
	var pos string
	e.scalar(&pos, `INSERT INTO hris.positions (title, level) VALUES ('Posisi '||$1||' '||md5(random()::text), $1) RETURNING id::text`, level)
	e.exec(`UPDATE hris.employees SET job_title_id = $2 WHERE id = $1`, employeeID, pos)
}
```

Fake `LoanApprover` (Step 3) memakai SQL yang sama dengan adapter dan membaca rantai `reporting_to` dari DB uji, jadi `reporting_to` yang di-update lewat `e.exec` langsung terbaca. Fake HRIS disimpan di field `e.emps` (`*fakeEmployees`).

`backend/internal/app/loan_approver_test.go`:

```go
package app

import (
	"context"
	"testing"

	"nuhabit/backend/internal/platform/testutil"
)

func TestLoanApproverSQL(t *testing.T) {
	tx := testutil.Tx(t)
	ctx := context.Background()
	ins := func(name, level string, reportsTo *string) string {
		var pos, id string
		if err := tx.QueryRow(ctx, `INSERT INTO hris.positions (title, level) VALUES ($1||md5(random()::text), $2) RETURNING id::text`, name, level).Scan(&pos); err != nil {
			t.Fatal(err)
		}
		if err := tx.QueryRow(ctx, `INSERT INTO hris.employees (full_name, nip, email, phone, join_date, is_active, job_title_id, reporting_to)
			VALUES ($1, 'LA-'||md5(random()::text), md5(random()::text)||'@la.test', '08', '2024-01-01', true, $2, $3) RETURNING id::text`,
			name, pos, reportsTo).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	gm := ins("GM", "General Manager", nil)
	mgr := ins("Manager", "manager", &gm)
	spv := ins("SPV", "Supervisor", &mgr)
	staff := ins("Staff", "Staff", &spv)
	got, err := payrollEmployees{}.LoanApprover(ctx, tx, staff)
	if err != nil || got == nil || *got != mgr {
		t.Fatalf("approver staff = %v (%v), want manager", got, err)
	}
	// Manager sendiri: approver = GM.
	if got, _ := (payrollEmployees{}).LoanApprover(ctx, tx, mgr); got == nil || *got != gm {
		t.Fatalf("approver manager = %v, want GM", got)
	}
	// Siklus diri sendiri: nil, tidak hang.
	self := ins("Self", "Staff", nil)
	if _, err := tx.Exec(ctx, `UPDATE hris.employees SET reporting_to = id WHERE id = $1`, self); err != nil {
		t.Fatal(err)
	}
	if got, err := (payrollEmployees{}).LoanApprover(ctx, tx, self); err != nil || got != nil {
		t.Fatalf("approver self-loop = %v (%v), want nil", got, err)
	}
}
```

- [ ] **Step 2: Jalankan test, pastikan gagal**

Run: `cd backend && TEST_DATABASE_URL='postgres://postgres@127.0.0.1:55499/paddy_test?sslmode=disable' $HOME/.local/go/bin/go test ./internal/modules/payroll -run 'TestKasbonTwoStep|TestKasbonWithoutManager|TestKasbonHRStep' -count=1 -v`
Expected: FAIL (build error `e.setLevel undefined`, lalu perilaku).

- [ ] **Step 3: Implementasi**

Migrasi `20261021100000_loans_manager_approval.sql`:

```sql
-- Approval kasbon dua tahap (EPIC-060, MOM AP-05): approver manager disimpan
-- saat pengajuan; status antara 'manager_approved' sebelum HR Penggajian.
ALTER TABLE hris.loans
  ADD COLUMN IF NOT EXISTS manager_approver_id uuid REFERENCES hris.employees(id) ON DELETE SET NULL,
  ADD COLUMN IF NOT EXISTS manager_approved_by uuid REFERENCES hris.employees(id) ON DELETE SET NULL,
  ADD COLUMN IF NOT EXISTS manager_approved_at timestamptz;
CREATE INDEX IF NOT EXISTS idx_loans_manager_approver ON hris.loans (manager_approver_id) WHERE status = 'pending';
```

Terapkan ke DB test: `cd backend && MIGRATE_DATABASE_URL='postgres://postgres@127.0.0.1:55499/paddy_test?sslmode=disable' $HOME/.local/go/bin/go run ./cmd/migrate -apply`.

`ports.go`, di interface `Employees`:

```go
	// LoanApprover is the first active employee up the reporting_to chain
	// (not the employee) with a Manager-or-above position level; nil without
	// one. Cycles and self-references stop the walk (EPIC-060).
	LoanApprover(ctx context.Context, q database.Querier, employeeID string) (*string, error)
```

SQL bersama (dipakai adapter dan fake): tambahkan di `ports.go`:

```go
// LoanApproverSQL walks reporting_to up to 10 levels; $1 = employee id.
const LoanApproverSQL = `WITH RECURSIVE chain AS (
	SELECT e.reporting_to AS id, 1 AS depth, ARRAY[e.id] AS seen FROM hris.employees e WHERE e.id = $1::uuid
	UNION ALL
	SELECT e.reporting_to, c.depth + 1, c.seen || e.id FROM chain c JOIN hris.employees e ON e.id = c.id
	WHERE c.depth < 10 AND e.reporting_to IS NOT NULL AND NOT e.reporting_to = ANY(c.seen || e.id))
SELECT m.id::text FROM chain c
JOIN hris.employees m ON m.id = c.id AND m.is_active
JOIN hris.positions p ON p.id = m.job_title_id
WHERE c.id <> $1::uuid
  AND lower(trim(p.level)) IN ('manager', 'senior manager', 'general manager', 'director', 'c-level')
ORDER BY c.depth LIMIT 1`
```

Daftar level di SQL harus sama dengan `managerLevels` di `domain/kasbon.go`. Tambahkan komentar "Mirror: domain.managerLevels" di keduanya.

`adapters_payroll.go`:

```go
func (payrollEmployees) LoanApprover(ctx context.Context, q database.Querier, employeeID string) (*string, error) {
	var id string
	err := q.QueryRow(ctx, payroll.LoanApproverSQL, employeeID).Scan(&id)
	if database.IsNoRows(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &id, nil
}
```

Fake di `payroll_integration_test.go` memakai SQL yang sama (fake ini memang menyimpan data di DB uji):

```go
func (f *fakeEmployees) LoanApprover(ctx context.Context, q database.Querier, employeeID string) (*string, error) {
	var id string
	err := q.QueryRow(ctx, LoanApproverSQL, employeeID).Scan(&id)
	if database.IsNoRows(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &id, nil
}
```

`loans.go`, `createLoan` service: sebelum `INSERT`, untuk kasbon:

```go
	var approver *string
	if in.LoanType == "kasbon" {
		if approver, err = s.ports.Employees.LoanApprover(ctx, s.db, target); err != nil {
			return nil, err
		}
	}
```

Tambahkan kolom `manager_approver_id` ke `INSERT` (nilai `approver`).

Ganti service `decideLoan` dengan dua fungsi:

```go
// loanForDecision is the loan columns a decision reads.
type loanForDecision struct {
	EmployeeID, Status, LoanType string
	Installment                  *string
	Principal                    float64
	Tenor                        int
	ManagerApprover              *string
}

func (s *service) loanForDecision(ctx context.Context, loanID string) (*loanForDecision, error) {
	var l loanForDecision
	err := s.db.QueryRow(ctx, `SELECT employee_id::text, COALESCE(status, ''), COALESCE(loan_type, ''),
		monthly_installment::text, principal_amount::float8, tenor_months, manager_approver_id::text
		FROM hris.loans WHERE id::text = $1`, loanID).Scan(&l.EmployeeID, &l.Status, &l.LoanType, &l.Installment,
		&l.Principal, &l.Tenor, &l.ManagerApprover)
	if database.IsNoRows(err) {
		return nil, httpx.NotFound("Pinjaman tidak ditemukan")
	}
	return &l, err
}

// decideLoanManager is step 1 (EPIC-060): the stored manager approver moves a
// pending kasbon to manager_approved, or rejects it. No salary figures.
func (s *service) decideLoanManager(ctx context.Context, approverEmployeeID, loanID string, approve bool, reason *string) (*messageData, error) {
	now := s.clock()
	var row *obj
	var err error
	if approve {
		row, err = queryObj(ctx, s.db, `UPDATE hris.loans SET status = 'manager_approved', manager_approved_by = $2,
			manager_approved_at = $3, updated_at = $3 WHERE id::text = $1 AND status = 'pending' AND manager_approver_id = $2
			RETURNING id, employee_id, loan_type, status, tenor_months, manager_approver_id, manager_approved_by, manager_approved_at`,
			loanID, approverEmployeeID, now)
	} else {
		why := "Tidak disetujui"
		if reason != nil && *reason != "" {
			why = *reason
		}
		row, err = queryObj(ctx, s.db, `UPDATE hris.loans SET status = 'rejected', rejected_by = $2, rejected_at = $3,
			rejection_reason = $4, is_active = false, updated_at = $3 WHERE id::text = $1 AND status = 'pending'
			AND manager_approver_id = $2 RETURNING id, employee_id, loan_type, status, rejection_reason`,
			loanID, approverEmployeeID, now, why)
	}
	if err != nil {
		return nil, opaque(err)
	}
	if row == nil {
		return nil, httpx.Conflict("Pinjaman sudah diproses oleh orang lain")
	}
	msg := "Kasbon ditolak"
	if approve {
		msg = "Kasbon disetujui Manager — menunggu persetujuan HR"
	}
	return &messageData{Data: row, Message: msg}, nil
}
```

Fungsi `decideLoan` yang lama menjadi tahap HR: rename menjadi `decideLoanHR`. Ganti `SELECT` awalnya dengan `s.loanForDecision`, lalu ganti pengecekan status:

```go
	l, err := s.loanForDecision(ctx, loanID)
	if err != nil {
		return nil, err
	}
	waitingManager := l.LoanType == "kasbon" && l.ManagerApprover != nil && l.Status == "pending"
	if waitingManager {
		return nil, httpx.BadRequest("Kasbon menunggu persetujuan Manager Departemen")
	}
	if l.Status != "pending" && l.Status != "manager_approved" {
		return nil, httpx.BadRequest("Pinjaman sudah diproses")
	}
```

Di `UPDATE` akhir, ganti `AND status = 'pending'` dengan `AND status = $<n>` berisi `l.Status`, agar tetap atomik terhadap status yang dibaca.

Handler `decideLoan`:

```go
// POST /api/hris/loans/{id}/approve { approved, rejection_reason? }: the stored
// manager approver decides kasbon step 1; HR with "approve" on Pinjaman (and,
// for kasbon, the Penggajian menu) decides the rest (EPIC-060).
func (h *handler) decideLoan(w http.ResponseWriter, r *http.Request) error {
	a, err := h.requireActor(r)
	if err != nil {
		return err
	}
	f, err := readJSON(r, "")
	if err != nil {
		return err
	}
	approved := f.Bool("approved", optional)
	reason := f.Str("rejection_reason", nullish, validate.StrOpts{})
	if err := issuesErr(f, ""); err != nil {
		return err
	}
	loanID := r.PathValue("id")
	l, err := h.svc.loanForDecision(r.Context(), loanID)
	if err != nil {
		return err
	}
	if l.Status == "pending" && l.ManagerApprover != nil && a.EmployeeID != nil && *a.EmployeeID == *l.ManagerApprover {
		out, err := h.svc.decideLoanManager(r.Context(), *a.EmployeeID, loanID, approved != nil && *approved, reason)
		if err != nil {
			return err
		}
		return writeJSON(w, out)
	}
	u, err := h.auth.RequireMenuAction(r, "approve", iam.HrisLoans...)
	if err != nil {
		return err
	}
	if l.LoanType == "kasbon" && !a.Payroll {
		return httpx.Forbidden("Persetujuan kasbon oleh HR butuh akses Penggajian")
	}
	showCap, err := h.auth.HasMenuPrefix(r.Context(), u, iam.HrisCompensation...)
	if err != nil {
		return err
	}
	out, err := h.svc.decideLoanHR(r.Context(), u.ID, loanID, approved != nil && *approved, reason, showCap)
	if err != nil {
		return err
	}
	return writeJSON(w, out)
}
```

Manager yang mencoba lagi setelah `manager_approved` jatuh ke cabang HR, lalu ditolak 403 oleh `RequireMenuAction` (tidak punya menu). Ini sesuai test.

`listLoans` handler: baca `scope := q.Str("scope", optional, ...)`. Bila `scope == "approvals"`, panggil service baru:

```go
// listLoanApprovals is the manager queue: pending kasbon whose stored
// approver is the actor. Salary-free columns only (R8).
func (s *service) listLoanApprovals(ctx context.Context, a *actor) ([]*obj, error) {
	if a.EmployeeID == nil {
		return []*obj{}, nil
	}
	rows, err := queryObjs(ctx, s.db, `SELECT id, employee_id, loan_type, principal_amount, tenor_months, purpose,
		status, created_at FROM hris.loans WHERE status = 'pending' AND manager_approver_id = $1 ORDER BY created_at`,
		*a.EmployeeID)
	if err != nil {
		return nil, opaque(err)
	}
	return rows, s.stitch(ctx, s.db, rows, loanListPeople[0])
}
```

`principal_amount` boleh tampil ke manager: itu nominal pengajuan, bukan gaji pemohon. Yang tidak boleh tampil adalah plafon/gaji.

- [ ] **Step 4: Jalankan test, pastikan lulus**

Run: `cd backend && $HOME/.local/go/bin/go build ./... && TEST_DATABASE_URL=... $HOME/.local/go/bin/go test ./internal/modules/payroll/... ./internal/app -count=1 -run 'TestKasbon|TestLoan|TestLoanApproverSQL' -v`
Expected: PASS. `TestLoanApprovalFollowsLoansMenu` (EPIC-057) membuat kasbon dengan HR tanpa Penggajian, sehingga kini 403 (R6). Ganti `loan_type` di test itu ke `"loan"` supaya maksud aslinya (approve lewat menu Pinjaman) tetap teruji, dan catat ruling. Lakukan hal sama untuk `TestLoanApprovalHidesCapWithoutPayrollMenu`.

Run: `cd backend && TEST_DATABASE_URL=... $HOME/.local/go/bin/go test ./internal/modules/payroll/... ./internal/app -count=1`
Expected: `ok` (app sama dengan baseline).

- [ ] **Step 5: Commit**

```bash
git add backend/database/migrations/deltas/20261021100000_loans_manager_approval.sql backend/internal/modules/payroll backend/internal/app/adapters_payroll.go backend/internal/app/loan_approver_test.go
git commit -m "feat(payroll): approval kasbon dua tahap — Manager Departemen lalu HR Penggajian, EPIC-060"
```

---

### Task 4: Frontend — form kasbon, antrean manager ESS, status HR

**Files:**
- Modify: `frontend/src/lib/hris/loans-view.ts` (opsi jenis, label riwayat, tenor default, validasi)
- Modify: `frontend/src/lib/hris/loans-view.test.ts`
- Modify: `frontend/src/features/hris/ess/components/pinjaman-page.tsx` (opsi dari `loans-view`, tenor 1/2, badge, antrean tim)
- Modify: `frontend/src/features/hris/ess/api.ts`, `ess/queries.ts`, `ess/mutations.ts` (antrean tim + keputusan)
- Modify: `frontend/src/features/hris/loans/components/loans-table.tsx` (badge `manager_approved`, tombol HR)
- Modify: `frontend/src/features/hris/loans/components/loan-form-dialog.tsx` (tenor kasbon)

**Interfaces:**
- Consumes: `GET /api/hris/loans?scope=approvals`, `POST /api/hris/loans/{id}/approve` (Task 3).
- Produces:
  - `LOAN_TYPE_OPTIONS` (bisa diajukan: kasbon, loan) dan `loanTypeLabel` (termasuk `emergency` untuk riwayat).
  - `LOAN_STATUS_LABELS`.
  - `kasbonTenorOptions`.
  - Hook `useTeamLoanApprovals`, `useDecideTeamLoan`.

- [ ] **Step 1: Tulis test yang gagal**

Tambahkan ke `frontend/src/lib/hris/loans-view.test.ts`:

```ts
import { EMPTY_LOAN_FORM, LOAN_STATUS_LABELS, LOAN_TYPE_OPTIONS, kasbonTenorOptions, loanTypeLabel, validateLoanForm } from "./loans-view";

describe("kasbon EPIC-060", () => {
  it("Pinjaman Darurat tidak bisa diajukan tapi labelnya tetap untuk riwayat", () => {
    expect(LOAN_TYPE_OPTIONS.map((o) => o.value)).toEqual(["kasbon", "loan"]);
    expect(loanTypeLabel("emergency")).toBe("Pinjaman Darurat");
  });
  it("tenor kasbon 1 atau 2, default 2", () => {
    expect(kasbonTenorOptions.map((o) => o.value)).toEqual(["1", "2"]);
    expect(EMPTY_LOAN_FORM.tenor_months).toBe("2");
    expect(validateLoanForm({ ...EMPTY_LOAN_FORM, employee_id: "e", principal_amount: "100000", tenor_months: "3" }))
      .toBe("Tenor kasbon hanya 1 atau 2 bulan");
  });
  it("status antara manager", () => {
    expect(LOAN_STATUS_LABELS.manager_approved).toBe("Disetujui Manager");
  });
});
```

Sebelum menambah test, baca `validateLoanForm` di `loans-view.ts`. Pastikan pesan yang sudah ada untuk pokok kosong tidak bentrok, dan tempatkan cek tenor kasbon setelah cek yang sudah ada.

- [ ] **Step 2: Jalankan test, pastikan gagal**

Run: `cd frontend && npx vitest run src/lib/hris/loans-view.test.ts`
Expected: FAIL (`kasbonTenorOptions`/`LOAN_STATUS_LABELS` belum ada; opsi masih memuat `emergency`).

- [ ] **Step 3: Implementasi**

`loans-view.ts`:

```ts
/** Jenis yang bisa diajukan (EPIC-060: Pinjaman Darurat dihapus). */
export const LOAN_TYPE_OPTIONS = [
  { value: "kasbon", label: "Kasbon (Salary Advance)" },
  { value: "loan", label: "Pinjaman" },
];

/** Label semua jenis, termasuk yang sudah tidak bisa diajukan (riwayat). */
const LOAN_TYPE_LABELS: Record<string, string> = {
  ...Object.fromEntries(LOAN_TYPE_OPTIONS.map((o) => [o.value, o.label])),
  emergency: "Pinjaman Darurat",
};

export function loanTypeLabel(value: string): string {
  return LOAN_TYPE_LABELS[value] ?? value;
}

/** Tenor kasbon (MOM KP-02): 1 atau 2 bulan; default 2 (50/50). */
export const kasbonTenorOptions = [
  { value: "1", label: "1 bulan" },
  { value: "2", label: "2 bulan (50% / 50%)" },
];

export const LOAN_STATUS_LABELS: Record<string, string> = {
  pending: "Menunggu",
  manager_approved: "Disetujui Manager",
  approved: "Berjalan",
  rejected: "Ditolak",
  paid_off: "Lunas",
};
```

Ganti `tenor_months: "3"` di `EMPTY_LOAN_FORM` menjadi `"2"`. Di `validateLoanForm`, setelah cek yang ada:

```ts
  if (form.loan_type === "kasbon" && !["1", "2"].includes(form.tenor_months)) return "Tenor kasbon hanya 1 atau 2 bulan";
```

`pinjaman-page.tsx`:
- Hapus `LOAN_TYPE_OPTIONS` dan `loanTypeLabel` lokal; impor `LOAN_TYPE_OPTIONS`, `loanTypeLabel`, dan `kasbonTenorOptions` dari `@/lib/hris/loans-view`.
- `EMPTY_FORM.tenor_months` = `"2"`.
- Untuk `loan_type === "kasbon"`, ganti input tenor bebas dengan `Combobox options={kasbonTenorOptions}`. Jenis `loan` tetap memakai input angka.
- Tambahkan ke `STATUS_BADGES`: `manager_approved: { label: "Disetujui Manager", className: "bg-indigo-100 text-indigo-700" }`.
- Tambahkan bagian "Persetujuan Kasbon Tim" di bawah daftar pinjaman, yang hanya dirender bila `useTeamLoanApprovals().data` tidak kosong. Setiap baris berisi nama pemohon, jenis, nominal, tenor, dan dua tombol (Setujui / Tolak) yang memanggil `useDecideTeamLoan`.

`ess/api.ts`:

```ts
export const fetchTeamLoanApprovals = () => softGet<EssLoanRow[]>("/api/hris/loans?scope=approvals", []);

export const decideTeamLoan = (id: string, approved: boolean) =>
  apiPost<{ message?: string }>(`/api/hris/loans/${id}/approve`, { approved });
```

`ess/queries.ts`:

```ts
export const useTeamLoanApprovals = () =>
  useQuery({ queryKey: [...essQueryKeys.loans(), "approvals"], queryFn: fetchTeamLoanApprovals });
```

`ess/mutations.ts`:

```ts
export function useDecideTeamLoan() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, approved }: { id: string; approved: boolean }) => decideTeamLoan(id, approved),
    retry: false,
    onSuccess: () => qc.invalidateQueries({ queryKey: essQueryKeys.loans() }),
  });
}
```

`EssLoanRow` harus punya `employee?: { full_name: string } | null` untuk menampilkan nama pemohon (cek tipe di `ess/types.ts` dan tambahkan bila belum ada).

`loans-table.tsx`:
- Tambahkan `manager_approved: { label: "Disetujui Manager", className: "bg-indigo-100 text-indigo-700" }` ke `STATUS_BADGES`.
- Ubah kondisi tombol menjadi `(row.status === "pending" || row.status === "manager_approved") && (...)`. Server tetap menolak HR yang mendahului manager, dan pesannya tampil lewat toast error yang sudah ada.

`loan-form-dialog.tsx`: untuk kasbon, pakai `kasbonTenorOptions` (Combobox) alih-alih input bebas.

- [ ] **Step 4: Jalankan test, pastikan lulus, lalu tsc dan eslint**

Run: `cd frontend && npx vitest run src/lib/hris src/features/hris/loans src/features/hris/ess`
Expected: PASS. Perbarui test komponen yang mengunci opsi `emergency` atau tenor `"3"`.

Run: `cd frontend && npx tsc --noEmit -p . && npx eslint src/lib/hris/loans-view.ts src/features/hris/ess src/features/hris/loans`
Expected: bersih.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/lib/hris/loans-view.ts frontend/src/lib/hris/loans-view.test.ts frontend/src/features/hris/ess frontend/src/features/hris/loans
git commit -m "feat(hris-ui): form kasbon tenor 1/2 tanpa Pinjaman Darurat, antrean persetujuan kasbon manager di ESS, EPIC-060"
```

---

### Task 5: Verifikasi menyeluruh & dokumentasi

**Files:**
- Modify: `docs/epics/EPIC-060-kasbon-acuan-mom.md`
- Modify: `docs/hris/acuan-mom-20261019/status-requirement.md` (KP-01, KP-02, AP-05)
- Modify: `docs/hris/acuan-mom-20261019/aturan-payroll.md` (baris Pinjaman/Kasbon)
- Modify: `docs/epics/EPIC-057-hris-akses-per-akun.md` (catatan perubahan R6 untuk kasbon)

- [ ] **Step 1: Full suite**

Run: `cd backend && TEST_DATABASE_URL='postgres://postgres@127.0.0.1:55499/paddy_test?sslmode=disable' $HOME/.local/go/bin/go test ./... -count=1 > /tmp/epic060-go.txt 2>&1; grep -- '--- FAIL' /tmp/epic060-go.txt | sed 's/ (.*//' | sort -u`
Expected: nama yang gagal sama dengan baseline `main`.

Run: `cd frontend && npx vitest run 2>&1 | tail -5 && npx tsc --noEmit -p .`
Expected: 0 gagal; tsc tanpa output.

- [ ] **Step 2: Dokumentasi**

Perbarui `status-requirement.md`: KP-01 → ✅, KP-02 → ✅, AP-05 → ✅. Di `aturan-payroll.md`, baris Pinjaman/Kasbon → ✅.

Di Automation Log EPIC-060, catat:
- Commit Task 1–4.
- Ruling R1–R8.
- Bukti test.
- Langkah rilis: migrasi `20261021100000`; HR memutus manual 1 Pinjaman Darurat pending di live **sebelum** rilis (Q-KP-1); kasbon pending lama langsung ke tahap HR (R7).

Di Automation Log EPIC-057, tambahkan catatan bahwa approve **kasbon** kini butuh menu Penggajian (EPIC-060 R6), sedangkan jenis `loan` tidak berubah. Set EPIC-060 `status: review`.

- [ ] **Step 3: Commit**

```bash
git add docs/epics docs/hris/acuan-mom-20261019
git commit -m "docs: EPIC-060 kasbon acuan selesai diimplementasikan, menunggu review"
```

## Follow-up (di luar rencana ini)

- Route fallback Next untuk pinjaman (`frontend/src/lib/payroll/loan-requests.ts`) belum menegakkan aturan kasbon. Go yang melayani `/api/hris`, jadi risikonya hanya bila backend Go mati.
- Audit log (LA-01) untuk keputusan pinjaman tahap manager & HR.
- Aturan jenis `loan` (Pinjaman) menunggu TnC (KP-03).
- Notifikasi WhatsApp ke approver manager saat kasbon diajukan.
