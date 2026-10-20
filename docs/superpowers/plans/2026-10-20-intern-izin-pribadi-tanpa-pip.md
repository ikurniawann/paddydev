# Izin Pribadi Intern Tanpa Potongan THP — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Karyawan berstatus Magang (intern) tetap mendapat kuota Izin Pribadi 3 hari per bulan, tetapi Izin Pribadi mereka tidak memotong THP (PIP dikecualikan). Tunjangan per hari tetap mengikuti aturan yang ada.

**Architecture:**
- Pengecualian diputuskan di kalkulator payroll Go (`payroll/domain.Calculate`) berdasarkan `Input.EmploymentStatus`. Field ini sudah diisi dari `hris.employees.employment_status` lewat `BuildInput`.
- Endpoint kuota cuti (`GET /api/hris/leave-quota`, modul hris) mengirim `deducts_thp` per jenis, supaya catatan form ESS/HR tidak lagi menulis "Memotong 1 hari THP" untuk intern.
- Kuota, validasi pengajuan, dan tunjangan per hari tidak berubah.

**Tech Stack:** Go (backend modular monolith, pgx), Next.js + vitest (frontend), PostgreSQL.

**Spec:** Keputusan user 20-10-2026 di sesi induk:
> Untuk intern: Izin Pribadi tetap kuota 3x per bulan tapi tidak memotong THP (PIP dikecualikan untuk status intern). Tunjangan per hari ikut aturan yang ada. Buat rencana perubahannya dulu sebelum deploy EPIC-058.

Rencana ini adalah amandemen EPIC-058 (`docs/epics/EPIC-058-hris-revisi-jenis-cuti.md`; rencana induk `docs/superpowers/plans/2026-10-20-hris-revisi-jenis-cuti.md`). EPIC-058 sudah di-merge ke main (`97e4c8f`) tetapi **belum di-deploy**; deploy menunggu rencana ini selesai.

## Temuan investigasi (dasar rencana)

- **Status intern punya dua kode:**
  - Master live `hris.employment_statuses` memakai kode `intern` ("Magang").
  - Modul rekrutmen memakai `internship`, baik saat mempromosikan kandidat jadi karyawan (`recruitment/domain/promotion.go:19`, `EmploymentStatuses`) maupun di label laporan.
  - Karyawan intern hasil promosi rekrutmen bisa tersimpan sebagai `internship`, jadi **kedua kode** harus dikecualikan.
- **Live (read-only, 20-10-2026):**
  - 8 karyawan aktif berstatus `intern`; 0 `internship`.
  - Setting `hris.payroll_settings.personal_leave_deduction = true`, sehingga PIP aktif untuk semua status.
  - Struktur gaji intern: 5 `monthly` (transport flat, makan per_day), 1 `monthly` (transport & makan per_day), 1 `monthly_prorated`, 1 tanpa struktur gaji.
  - Intern belum punya riwayat cuti (0 baris), sehingga hitung ulang periode lama tidak berubah.
  - Semua intern berlevel jabatan Staff, jadi tidak berhak Izin Leader (tidak berubah).
- **Kode PIP:** `backend/internal/modules/payroll/domain/calculator.go:388-392`. Potongan = `Round(perDay(thp, in.PersonalLeaveDays))`, aktif bila `c.PersonalLeaveDeduction`. Tidak ada cabang per status kepegawaian.
- **Tunjangan per hari:** transport/makan mode `per_day` dan gaji pokok `monthly_prorated`/`daily` dihitung dari hari hadir. Hari Izin Pribadi tidak hadir, jadi tunjangan harinya tidak dibayar. Keputusan user: **tetap begitu**. Artinya intern `monthly_prorated` tetap kehilangan gaji pokok hari itu karena basis bayarnya; yang dikecualikan hanya potongan PIP.
- **Teks form:** `frontend/src/lib/hris/leave-form.ts:10` menulis "Memotong 1 hari THP per hari izin." berdasarkan katalog (`deductsThpDay`), tanpa melihat status karyawan.

## Ruling

- **R1 — Kode intern** = `intern` atau `internship`, case-insensitive dan di-trim. Helper Go `IsInternStatus(status string) bool` ada di **dua** paket: `payroll/domain` dan `hris/domain`. Keduanya bounded context terpisah, jadi tidak saling import. Setiap salinan diberi komentar "Mirror", mengikuti pola katalog cuti EPIC-058.
- **R2 — Status yang dipakai** adalah `employment_status` karyawan **saat payroll dihitung** (sama dengan aturan THR yang sudah ada). Tidak ada riwayat status per tanggal. Intern yang diangkat menjadi kontrak sebelum payroll bulan itu dihitung akan terkena PIP untuk seluruh bulan itu.
- **R3 — Hanya PIP yang dikecualikan.** PIS (izin sakit), potongan unpaid, potongan telat, dan tunjangan per hari tidak berubah untuk intern.
- **R4 — Kuota Izin Pribadi intern tetap 3 hari per bulan**, divalidasi dengan aturan EPIC-058 yang sama (satu hari per pengajuan, cek tabrakan).
- **R5 — Endpoint kuota** menambah `deducts_thp` (bool) per jenis: `true` hanya untuk jenis yang memotong THP (`personal`) **dan** karyawan bukan intern. Tanpa angka rupiah, dan akses tidak berubah.
- **R6 — Frontend** memakai `deducts_thp` dari endpoint bila ada. Bila tidak ada (respons lama), fallback ke katalog (`deductsThpDay`).

## Global Constraints

- Jangan menyentuh DB live. Uji hanya di DB test `postgres://postgres@127.0.0.1:55499/paddy_test?sslmode=disable` (bukan port 55432).
- Go: `$HOME/.local/go/bin/go`; test integrasi butuh `TEST_DATABASE_URL` di atas.
- Tidak ada migrasi skema dan tidak ada perubahan setting payroll.
- Tidak ada angka rupiah di endpoint cuti atau form; teks memakai "Memotong 1 hari THP" / "Tidak memotong THP".
- Baca `frontend/AGENTS.md` sebelum menulis kode Next.js.
- Commit per task dengan trailer `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`. Jangan commit ke main dan jangan push tanpa izin user.

## Review Focus

1. **Kode `internship` dari rekrutmen** (bukan `intern`): harus dikecualikan juga. Diuji di Task 1 (unit) dan Task 2 (endpoint).
2. **Huruf/spasi** (`"Intern"`, `" intern "`): tetap dikecualikan. Diuji di Task 1 (`TestIsInternStatus`).
3. **Intern dengan PIP aktif dan tunjangan makan `per_day`:** satu-satunya selisih dengan karyawan kontrak yang setara adalah potongan PIP. Tunjangan per hari dan net lainnya sama. Diuji di Task 1.
4. **Kuota intern tetap ditegakkan:** pengajuan Izin Pribadi ke-4 di bulan yang sama ditolak 400, sama dengan karyawan lain. Diuji di Task 2.
5. **Catatan form untuk intern** tidak boleh menulis "Memotong 1 hari THP"; karyawan non-intern tetap melihatnya. Diuji di Task 3.

---

### Task 1: Kalkulator payroll — PIP dikecualikan untuk intern

**Files:**
- Modify: `backend/internal/modules/payroll/domain/calculator.go:388-392` (blok PIP)
- Create: `backend/internal/modules/payroll/domain/intern.go`
- Test: `backend/internal/modules/payroll/domain/intern_test.go`

**Interfaces:**
- Consumes: `Input.EmploymentStatus string` (sudah ada, diisi `BuildInput` dari `Employee.EmploymentStatus`), `Config.PersonalLeaveDeduction bool`, `Result.PersonalLeaveDeduction float64`.
- Produces: `func IsInternStatus(status string) bool` di paket `payroll/domain`.

- [ ] **Step 1: Tulis test yang gagal**

`backend/internal/modules/payroll/domain/intern_test.go`:

```go
package domain

import "testing"

func TestIsInternStatus(t *testing.T) {
	for status, want := range map[string]bool{
		"intern": true, "internship": true, "Intern": true, " intern ": true, "INTERNSHIP": true,
		"contract": false, "permanent": false, "probation": false, "": false, "internal": false,
	} {
		if got := IsInternStatus(status); got != want {
			t.Errorf("IsInternStatus(%q) = %v, want %v", status, got, want)
		}
	}
}

// Keputusan user 20-10-2026: Izin Pribadi intern tidak memotong THP; tunjangan
// per hari tetap mengikuti kehadiran. Selisih intern vs kontrak setara hanya PIP.
func TestInternPersonalLeaveSkipsPIP(t *testing.T) {
	c := DefaultConfig()
	c.PersonalLeaveDeduction = true
	// Makan per hari (MealPerDay) dibayar menurut AllowanceDays (hari hadir):
	// 21 hari hadir dari 22 hari kerja, satu hari Izin Pribadi.
	in := func(status string) Input {
		return parityInput(func(in *Input) {
			in.EmploymentStatus = status
			in.BaseSalary, in.TransportAllowance, in.DivisorDays, in.WorkingDays, in.PersonalLeaveDays = 2_600_000, 260_000, 26, 22, 1
			in.MealAllowance, in.MealPerDay, in.AllowanceDays = 520_000, true, 21
		})
	}
	contract := Calculate(in("contract"), c)
	if contract.PersonalLeaveDeduction <= 0 {
		t.Fatalf("kontrak harus tetap kena PIP, got %v", contract.PersonalLeaveDeduction)
	}
	for _, status := range []string{"intern", "internship"} {
		r := Calculate(in(status), c)
		eq(t, status+": PIP", r.PersonalLeaveDeduction, 0)
		eq(t, status+": tunjangan makan per hari sama", r.MealAllowance, contract.MealAllowance)
		eq(t, status+": net hanya beda PIP", r.NetSalary, contract.NetSalary+contract.PersonalLeaveDeduction)
	}
}
```

Field yang dipakai sudah dicek di `calculator.go`: Input punya `MealAllowance`, `MealPerDay`, `AllowanceDays`, dan `EmploymentStatus`; Result punya `MealAllowance`, `NetSalary`, dan `PersonalLeaveDeduction`. Helper `eq` dan `parityInput` ada di `parity_test.go` (paket yang sama).

- [ ] **Step 2: Jalankan test, pastikan gagal**

Run: `cd backend && $HOME/.local/go/bin/go test ./internal/modules/payroll/domain -run 'TestIsInternStatus|TestInternPersonalLeaveSkipsPIP' -v`
Expected: FAIL, `undefined: IsInternStatus` (build error).

- [ ] **Step 3: Implementasi minimal**

`backend/internal/modules/payroll/domain/intern.go`:

```go
package domain

import "strings"

// IsInternStatus: status kepegawaian Magang. Master live memakai "intern",
// promosi rekrutmen menulis "internship". Mirror: hris/domain.IsInternStatus.
func IsInternStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "intern", "internship":
		return true
	}
	return false
}
```

`calculator.go`, blok PIP (baris 388-392) menjadi:

```go
	// G6: potongan izin pribadi (PIP) dan izin sakit (PIS). Intern (Magang)
	// tidak dipotong PIP (keputusan user 20-10-2026); PIS tetap.
	personal, sick := 0.0, 0.0
	if c.PersonalLeaveDeduction && in.PersonalLeaveDays > 0 && !IsInternStatus(in.EmploymentStatus) {
		personal = Round(perDay(thp, in.PersonalLeaveDays))
	}
```

- [ ] **Step 4: Jalankan test, pastikan lulus, lalu seluruh paket payroll**

Run: `cd backend && $HOME/.local/go/bin/go test ./internal/modules/payroll/domain -run 'TestIsInternStatus|TestInternPersonalLeaveSkipsPIP|TestLeaveDeductions|TestLeaderAndPersonalLeavePayroll' -v`
Expected: PASS semua. Test lama tetap lulus karena `parityInput` memakai `contract`/`permanent`.

Run: `cd backend && TEST_DATABASE_URL='postgres://postgres@127.0.0.1:55499/paddy_test?sslmode=disable' $HOME/.local/go/bin/go test ./internal/modules/payroll/... -count=1`
Expected: `ok` untuk semua paket.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/modules/payroll/domain/intern.go backend/internal/modules/payroll/domain/intern_test.go backend/internal/modules/payroll/domain/calculator.go
git commit -m "fix(payroll): Izin Pribadi intern tidak memotong THP (PIP), EPIC-058"
```

---

### Task 2: Endpoint kuota cuti — `deducts_thp` per jenis

**Files:**
- Create: `backend/internal/modules/hris/domain/intern.go`
- Modify: `backend/internal/modules/hris/domain/leave_types.go` (field `DeductsThpDay` di `LeaveType`, isi `true` untuk `personal`)
- Modify: `backend/internal/modules/hris/leaves.go` (interface repo sekitar baris 48; handler kuota sekitar baris 539-556)
- Modify: `backend/internal/modules/hris/postgres_leaves.go` (method repo baru)
- Test: `backend/internal/modules/hris/domain/intern_test.go`, `backend/internal/modules/hris/leave_types_integration_test.go`

**Interfaces:**
- Consumes: `domain.QuotaLeaveTypes() []LeaveType`; repo `EmployeePositionLevel(ctx, employeeID) (level string, found bool, err error)` (sudah ada).
- Produces:
  - `func IsInternStatus(status string) bool` di `hris/domain`.
  - Field `LeaveType.DeductsThpDay bool`.
  - Method repo `EmployeeEmploymentStatus(ctx context.Context, employeeID string) (string, error)`.
  - Respons `GET /api/hris/leave-quota`: `types[].deducts_thp` (bool) dan `employment_status` (string) di level atas.

- [ ] **Step 1: Tulis test yang gagal**

`backend/internal/modules/hris/domain/intern_test.go`:

```go
package domain

import "testing"

func TestIsInternStatus(t *testing.T) {
	for status, want := range map[string]bool{
		"intern": true, "internship": true, " Intern ": true, "contract": false, "": false, "internal": false,
	} {
		if got := IsInternStatus(status); got != want {
			t.Errorf("IsInternStatus(%q) = %v, want %v", status, got, want)
		}
	}
}

func TestPersonalLeaveDeductsThpInCatalog(t *testing.T) {
	p, _ := LookupLeaveType("personal")
	l, _ := LookupLeaveType("leader")
	if !p.DeductsThpDay || l.DeductsThpDay {
		t.Fatalf("DeductsThpDay personal=%v leader=%v", p.DeductsThpDay, l.DeductsThpDay)
	}
}
```

Tambahkan di akhir `backend/internal/modules/hris/leave_types_integration_test.go`:

```go
// Intern: kuota Izin Pribadi tetap 3 hari, tetapi tidak memotong THP
// (keputusan user 20-10-2026). Kode "internship" dari rekrutmen ikut.
func TestLeaveQuotaInternNoThpDeduction(t *testing.T) {
	h := newHarness(t)
	intern, _ := h.staff("pos", nil, true, emp{status: "intern"})
	recruited, _ := h.staff("pos", nil, true, emp{status: "internship"})
	worker, _ := h.staff("pos", nil, true, emp{status: "contract"})

	deducts := func(s *testutil.Staff) bool {
		t.Helper()
		res := h.do(s, "GET", "/api/hris/leave-quota?month=2031-03", nil)
		expect(t, res, 200, "")
		for _, x := range res.data()["types"].([]any) {
			ty := x.(map[string]any)
			if ty["leave_type"] == "personal" {
				return ty["deducts_thp"] == true
			}
		}
		t.Fatalf("personal tidak ada di %s", res.raw)
		return false
	}
	if deducts(&intern) || deducts(&recruited) {
		t.Fatal("intern tidak boleh dipotong THP untuk Izin Pribadi")
	}
	if !deducts(&worker) {
		t.Fatal("karyawan kontrak tetap dipotong THP untuk Izin Pribadi")
	}

	// Kuota intern tetap 3 hari per bulan.
	for _, d := range []string{"2031-03-04", "2031-03-05", "2031-03-06"} {
		expect(t, h.do(&intern, "POST", "/api/hris/leaves", leaveBody("personal", d, d)), 200, "")
	}
	expect(t, h.do(&intern, "POST", "/api/hris/leaves", leaveBody("personal", "2031-03-07", "2031-03-07")), 400,
		"Kuota Izin Pribadi bulan 2031-03 sudah habis (3 dari 3 hari)")
}
```

`h.staff` mengembalikan `(testutil.Staff, string)` dan `h.do` menerima `*testutil.Staff` (`harness_test.go:99,132`). Tambahkan import `"nuhabit/backend/internal/platform/testutil"` ke file test ini (sekarang hanya mengimpor `strings` dan `testing`).

- [ ] **Step 2: Jalankan test, pastikan gagal**

Run: `cd backend && $HOME/.local/go/bin/go test ./internal/modules/hris/domain -run 'TestIsInternStatus|TestPersonalLeaveDeductsThpInCatalog' -v`
Expected: FAIL (build error `undefined: IsInternStatus` / `DeductsThpDay`).

- [ ] **Step 3: Implementasi**

`backend/internal/modules/hris/domain/intern.go`:

```go
package domain

import "strings"

// IsInternStatus: status kepegawaian Magang. Master live memakai "intern",
// promosi rekrutmen menulis "internship". Mirror: payroll/domain.IsInternStatus.
func IsInternStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "intern", "internship":
		return true
	}
	return false
}
```

`leave_types.go`: tambahkan field ke struct `LeaveType`, setelah `LeaderOnly`:

```go
	DeductsThpDay bool // memotong 1 hari THP per hari izin (PIP payroll), kecuali intern
```

Lalu ganti baris katalog `personal` menjadi:

```go
	{Code: "personal", Label: "Izin Pribadi", Short: "Izin Pribadi", Export: "Izin Pribadi", Selectable: true, MonthlyQuota: 3, SingleDay: true, DeductsThpDay: true},
```

`leaves.go`: di interface repo (dekat `EmployeePositionLevel`, baris ~48) tambahkan:

```go
	EmployeeEmploymentStatus(ctx context.Context, employeeID string) (string, error)
```

`postgres_leaves.go`, tepat setelah `EmployeePositionLevel`:

```go
// EmployeeEmploymentStatus is hris.employees.employment_status ("" when null).
func (s *store) EmployeeEmploymentStatus(ctx context.Context, employeeID string) (string, error) {
	var status *string
	if err := s.db.QueryRow(ctx, `SELECT employment_status FROM hris.employees WHERE id = $1`, employeeID).Scan(&status); err != nil {
		return "", err
	}
	if status == nil {
		return "", nil
	}
	return *status, nil
}
```

`leaves.go`, di handler kuota tepat setelah pengecekan `found` (sekitar baris 545), lalu ganti loop `types`:

```go
	status, err := s.repo.EmployeeEmploymentStatus(ctx, *target)
	if err != nil {
		return nil, err
	}
	intern := domain.IsInternStatus(status)
```

```go
	for _, t := range domain.QuotaLeaveTypes() {
		used := use[t.Code]
		types = append(types, obj("leave_type", t.Code, "label", t.Label, "quota", t.MonthlyQuota,
			"used", used, "remaining", max(0, float64(t.MonthlyQuota)-used), "available", !t.LeaderOnly || eligible,
			"deducts_thp", t.DeductsThpDay && !intern))
	}
	return obj("employee_id", *target, "month", month, "position_level", level,
		"employment_status", status, "leader_eligible", eligible, "types", types), nil
```

Tidak ada fake repo hris yang mengimplementasikan interface ini (dicek: `grep -rn "EmployeePositionLevel" backend/internal/modules/hris/*_test.go` kosong); satu-satunya implementasi adalah `*store` di `postgres_leaves.go`.

- [ ] **Step 4: Jalankan test, pastikan lulus, lalu seluruh paket hris**

Run: `cd backend && $HOME/.local/go/bin/go test ./internal/modules/hris/domain -run 'TestIsInternStatus|TestPersonalLeaveDeductsThpInCatalog' -v`
Expected: PASS.

Run: `cd backend && TEST_DATABASE_URL='postgres://postgres@127.0.0.1:55499/paddy_test?sslmode=disable' $HOME/.local/go/bin/go test ./internal/modules/hris/... -count=1 -run 'TestLeaveQuota|TestLeaveMonthlyQuota|TestLeaderLeaveEligibility' -v`
Expected: PASS, termasuk `TestLeaveQuotaInternNoThpDeduction` dan `TestLeaveQuotaEndpoint` yang lama.

Run: `cd backend && TEST_DATABASE_URL='postgres://postgres@127.0.0.1:55499/paddy_test?sslmode=disable' $HOME/.local/go/bin/go test ./internal/modules/hris/... -count=1`
Expected: hanya `TestAttendancePeriodClosing` yang gagal (baseline yang sudah ada).

- [ ] **Step 5: Commit**

```bash
git add backend/internal/modules/hris/domain/intern.go backend/internal/modules/hris/domain/intern_test.go backend/internal/modules/hris/domain/leave_types.go backend/internal/modules/hris/leaves.go backend/internal/modules/hris/postgres_leaves.go backend/internal/modules/hris/leave_types_integration_test.go
git commit -m "feat(hris): kuota cuti menandai Izin Pribadi intern tanpa potongan THP, EPIC-058"
```

---

### Task 3: Frontend — catatan form mengikuti `deducts_thp`

**Files:**
- Modify: `frontend/src/features/hris/leaves/types.ts:2-8` (`LeaveQuota`)
- Modify: `frontend/src/lib/hris/leave-form.ts:10`
- Test: `frontend/src/lib/hris/leave-form.test.ts`

**Interfaces:**
- Consumes: respons `GET /api/hris/leave-quota` dari Task 2 (`types[].deducts_thp`, `employment_status`).
- Produces: `leaveFormNote(code, quota)` dengan signature tetap; isi teks mengikuti `deducts_thp`.

- [ ] **Step 1: Tulis test yang gagal**

Tambahkan di `describe("leaveFormNote", ...)` di `frontend/src/lib/hris/leave-form.test.ts`:

```ts
  it("intern: Izin Pribadi tidak memotong THP (deducts_thp=false), kuota tetap", () => {
    const intern: LeaveQuota = {
      ...quota,
      employment_status: "intern",
      types: [{ ...quota.types[0], deducts_thp: false }, quota.types[1]],
    };
    const note = leaveFormNote("personal", intern)!;
    expect(note).toContain("Sisa kuota Izin Pribadi bulan 2031-03: 2 dari 3 hari");
    expect(note).toContain("Tidak memotong THP");
    expect(note).not.toContain("Memotong 1 hari THP");
  });
  it("non-intern dengan deducts_thp=true tetap memotong 1 hari THP", () => {
    const worker: LeaveQuota = { ...quota, types: [{ ...quota.types[0], deducts_thp: true }, quota.types[1]] };
    expect(leaveFormNote("personal", worker)).toContain("Memotong 1 hari THP");
  });
```

Test lama "Izin Pribadi: ... memotong 1 hari THP" (tanpa `deducts_thp`) harus tetap lulus lewat fallback katalog (R6).

- [ ] **Step 2: Jalankan test, pastikan gagal**

Run: `cd frontend && npx vitest run src/lib/hris/leave-form.test.ts`
Expected: FAIL. Test intern gagal (teks masih "Memotong 1 hari THP"), dan tsc/vitest menolak properti `deducts_thp`/`employment_status` yang belum ada di tipe.

- [ ] **Step 3: Implementasi**

`frontend/src/features/hris/leaves/types.ts`:

```ts
export interface LeaveQuota {
  employee_id: string;
  month: string;
  position_level: string;
  /** Status kepegawaian karyawan (mis. "intern"). Tidak ada di respons lama. */
  employment_status?: string;
  leader_eligible: boolean;
  types: {
    leave_type: string;
    label: string;
    quota: number;
    used: number;
    remaining: number;
    available: boolean;
    /** Memotong 1 hari THP untuk karyawan ini (false untuk intern). */
    deducts_thp?: boolean;
  }[];
}
```

`frontend/src/lib/hris/leave-form.ts` baris 10:

```ts
  const deducts = q.deducts_thp ?? info.deductsThpDay;
  const effect = deducts
    ? "Memotong 1 hari THP per hari izin."
    : info.deductsThpDay
      ? "Tidak memotong THP (status Magang)."
      : "Tidak memotong gaji.";
```

- [ ] **Step 4: Jalankan test, pastikan lulus, lalu tsc dan eslint**

Run: `cd frontend && npx vitest run src/lib/hris src/features/hris/leaves src/features/hris/ess`
Expected: PASS semua.

Run: `cd frontend && npx tsc --noEmit -p . && npx eslint src/lib/hris/leave-form.ts src/lib/hris/leave-form.test.ts src/features/hris/leaves/types.ts`
Expected: tanpa error.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/features/hris/leaves/types.ts frontend/src/lib/hris/leave-form.ts frontend/src/lib/hris/leave-form.test.ts
git commit -m "feat(hris-ui): catatan Izin Pribadi intern tanpa potongan THP, EPIC-058"
```

---

### Task 4: Verifikasi menyeluruh & dokumentasi

**Files:**
- Modify: `docs/epics/EPIC-058-hris-revisi-jenis-cuti.md` (Acceptance Criteria + Automation Log)

**Interfaces:**
- Consumes: hasil Task 1–3.
- Produces: bukti test dan catatan di epic. Tidak ada kode.

- [ ] **Step 1: Full suite Go, bandingkan dengan baseline**

Run: `cd backend && TEST_DATABASE_URL='postgres://postgres@127.0.0.1:55499/paddy_test?sslmode=disable' $HOME/.local/go/bin/go test ./... -count=1 > /tmp/intern-go.txt 2>&1; grep -- '--- FAIL' /tmp/intern-go.txt | sed 's/ (.*//' | sort -u`
Expected: daftar nama sama persis dengan baseline main (27 nama, termasuk `TestAttendancePeriodClosing`; tanpa nama baru). Jalankan juga di `main` bila perlu pembanding segar.

- [ ] **Step 2: Full vitest, tsc, eslint file yang berubah**

Run: `cd frontend && npx vitest run 2>&1 | tail -5 && npx tsc --noEmit -p .`
Expected: 0 gagal; tsc tanpa output.

- [ ] **Step 3: Tambah Acceptance Criteria dan Automation Log di EPIC-058**

Di bagian `## Acceptance Criteria` tambahkan:

```markdown
- Intern (status `intern`/`internship`): Izin Pribadi tetap kuota 3 hari per bulan tetapi tidak memotong THP (PIP dikecualikan); tunjangan per hari tetap mengikuti kehadiran. Form menulis "Tidak memotong THP (status Magang)".
```

Di akhir `## Automation Log` tambahkan entri bertanggal dengan isi:
- Keputusan user (kutip dari bagian Spec rencana ini).
- Commit Task 1–3.
- Bukti test: nama test baru; Go full suite sama dengan baseline; vitest/tsc/eslint bersih.
- Catatan bahwa deploy EPIC-058 menyertakan perubahan ini.

- [ ] **Step 4: Commit**

```bash
git add docs/epics/EPIC-058-hris-revisi-jenis-cuti.md
git commit -m "docs: EPIC-058 amandemen intern tanpa PIP"
```

## Follow-up (di luar rencana ini)

- **Pengingat kontrak salah kode:** `backend/internal/modules/hris/postgres_contracts.go:228` mengecualikan `employment_status <> 'internship'`, padahal kode live `intern`. Akibatnya 8 intern live muncul di daftar "Tanpa kontrak aktif". Perbaikannya kecil (pakai `IsInternStatus` atau `NOT IN ('intern','internship')`), tapi itu perilaku lain dan butuh keputusan user.
- **Satu kode status Magang:** rekrutmen sebaiknya menulis `intern` (kode master live) agar tidak ada dua kode. Perubahan ini butuh migrasi data dan keputusan user.
