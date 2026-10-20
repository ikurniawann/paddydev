# Payroll Sesuai Acuan Payslip & MOM — Implementation Plan (EPIC-059)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. Also load the project skill `paddy-hris-acuan` before starting.

**Goal:** Kalkulator payroll Go menghitung gaji pokok, tunjangan, potongan Izin Pribadi/Sakit/Cuti Melahirkan, lembur, subsidi BPJS Kesehatan, dan prorata karyawan keluar sesuai tabel payslip Paddy dan keputusan MOM 19-10-2026.

**Architecture:**
- Hampir semua perubahan ada di `backend/internal/modules/payroll/domain`.
  - `BuildInputRange` mengklasifikasi tiap tanggal periode menjadi *hari dibayar* dan *hari efektif*, lalu menentukan pembagi dari jadwal.
  - `Calculate` memakai kedua hitungan hari itu, dan "1 hari THP" = 4 komponen tanpa potongan ganda.
- Dua perubahan kecil di luar domain:
  - Validasi minimum lembur di modul hris.
  - Setting subsidi BPJS Kesehatan: migrasi kolom, settings API, dan form setting.
- Prorata karyawan keluar membutuhkan perubahan query karyawan untuk payroll run (adapter `internal/app`).
- Perpindahan data struktur gaji ke `monthly_prorated`/`per_day` **tidak** dijalankan. Rencana hanya menyiapkan skrip ops dengan mode pratinjau.

**Tech Stack:** Go (pgx), PostgreSQL migrations (`backend/database/migrations/deltas`), Next.js + vitest (form setting payroll).

**Spec:** `docs/hris/acuan-mom-20261019/aturan-payroll.md` dan `docs/hris/acuan-mom-20261019/pertanyaan-rekomendasi.md` (keputusan user 20-10-2026: semua rekomendasi berlaku, kecuali K-7). Sumber asli: tabel payslip Paddy dan MOM 19-10-2026 §2.6, §3.

**Prasyarat:** PR ikurniawann/paddydev#3 (Izin Pribadi intern tanpa PIP) sudah di-merge ke `main`. Branch kerja `feat/epic059-payroll-acuan` dibuat dari `main` sesudah merge itu. `calculator.go` harus sudah berisi `IsInternStatus` (`grep -n IsInternStatus backend/internal/modules/payroll/domain/calculator.go`). Kalau belum ada, berhenti: PR #3 belum di-merge.

## Ruling (diturunkan dari keputusan user; berlaku untuk semua task)

- **R1 — Hari efektif** = baris absensi dengan status ≠ `absent` (`present`, `late`, `remote`, `half-day`). Ini sama dengan `AttendanceDays` yang ada sekarang. Dipakai untuk tunjangan Transport & Makan mode `per_day`.
- **R2 — Hari dibayar** (`PaidDays`) = hari efektif + hari cuti approved jenis apa pun **selain `unpaid`**, dihitung pada *hari kerja* yang belum punya baris absensi efektif, dalam cakupan kontrak/karyawan. Batas atasnya hari kerja terjadwal bila jadwal ada. Dipakai untuk Gaji Pokok `monthly_prorated` dan Tunjangan Jabatan `per_day`.
- **R3 — Hari kerja** sebuah tanggal:
  - Bila ada baris jadwal yang berlaku pada tanggal itu: tanggal itu punya shift (`ResolveScheduleRow(...).ShiftID != nil`).
  - Bila tidak ada jadwal yang berlaku: Senin–Jumat dan bukan libur nasional. Ini meniru `DescribeLeaveDays` di modul hris.
  - Semua hitungan hari cuti untuk payroll (`unpaid`, `personal`, `sick`, `maternity`, cuti dibayar) memakai hari kerja, bukan hari kalender.
- **R4 — Pembagi (K-12)** untuk `pay_basis` `monthly`/`monthly_prorated` = hari kerja terjadwal karyawan dalam periode, bila jadwal ada dan > 0. Bila tidak ada, pakai `working_days_divisor` struktur gaji, lalu hari kerja hasil hitung. `pay_basis = daily` tidak berubah.
- **R5 — "1 hari THP" (Q-PY-1)** = Gaji Pokok + Transport + Makan + Jabatan (nominal bulanan struktur gaji). Tidak termasuk lembur, bonus, insentif, THR, atau tunjangan lain.
- **R6 — Izin Pribadi tanpa potongan ganda (Q-PY-3).** Hari Izin Pribadi termasuk hari dibayar (R2), jadi Gaji Pokok dan Jabatan hari itu dibayar. Tetapi hari itu bukan hari efektif, jadi Transport/Makan `per_day` sudah tidak dibayar. Karena itu PIP = (Gaji Pokok [kecuali `daily`] + Jabatan + Transport bila `flat` + Makan bila `flat`) / pembagi × hari. Total yang hilang untuk karyawan non-intern = tepat 1 hari dari 4 komponen. Intern: PIP = 0, dan tunjangan `per_day` hari itu tetap tidak dibayar (keputusan intern EPIC-058).
- **R7 — Sakit & Cuti Melahirkan (ES-06, Q-PY-3).** Kedua jenis termasuk hari dibayar (Gaji Pokok & Jabatan dibayar) tetapi bukan hari efektif. Transport/Makan `per_day` sudah tidak dibayar lewat hari efektif. PIS hanya memotong bagian `flat`: (Transport bila `flat` + Makan bila `flat`) / pembagi × (hari sakit + hari melahirkan). Saklar `sick_leave_deduction` mengatur keduanya. Kolom hasil tetap `SickLeaveDeduction` dengan label slip "Potongan Sakit & Cuti Melahirkan".
- **R8 — Potongan telat tier basis `thp`** memakai THP 4 komponen (R5).
- **R9 — Minimum lembur 1 jam:** pengajuan < 1 jam ditolak. Di payroll, jam lembur terealisasi < 1 jam per pengajuan tidak dibayar.
- **R10 — Subsidi BPJS Kesehatan:** setting global `bpjs_kes_subsidy_amount` (Rp, default 0).
  - Bila > 0: nominal itu menggantikan `health_allowance` struktur, dan syaratnya masa kerja **lebih dari 1 tahun** pada akhir periode (tanggal cutoff), yaitu `join_date + 1 tahun < akhir periode`.
  - Bila 0: perilaku lama (`health_allowance` dan `health_min_service_days` per struktur).
- **R11 — Karyawan keluar (PY-04):** payroll run mengambil karyawan aktif **dan** karyawan nonaktif yang `end_date`-nya jatuh di dalam periode. Tanggal setelah `end_date` di luar cakupan, dengan mekanisme yang sama seperti kontrak parsial. PPh21 Pasal 17 untuk masa pajak terakhir karyawan keluar **tidak** termasuk (follow-up).
- **R12 — Data live tidak disentuh.** Perpindahan struktur gaji ke `monthly_prorated` + `per_day` dan pengaturan `cutoff_day = 20` dilakukan lewat skrip ops (Task 7) atau UI HR, **hanya setelah user menyetujui** hasil pratinjau.

## Global Constraints

- Uji integrasi hanya di DB test `postgres://postgres@127.0.0.1:55499/paddy_test?sslmode=disable`; jangan pakai port 55432 dan jangan menyentuh DB live.
- Go: `$HOME/.local/go/bin/go`; jalankan `gofmt -l` sebelum commit.
- Tidak ada angka rupiah gaji di endpoint non-Penggajian (aturan EPIC-057 tetap berlaku).
- Migrasi baru di `backend/database/migrations/deltas/` dengan prefiks timestamp `2026102X…` lebih besar dari `20261020150100`.
- Baca `frontend/AGENTS.md` sebelum mengubah kode Next.js.
- Commit per task dengan trailer `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`. Jangan commit ke `main` dan jangan push tanpa izin user.

## Review Focus

1. **Karyawan tanpa jadwal dan tanpa absensi** (banyak di live): pembagi jatuh ke struktur gaji/20, `PaidDays` = cuti saja. Untuk `monthly` (bukan prorated), gaji pokok tetap penuh × faktor, tidak boleh jadi 0. Diuji di Task 3.
2. **Cuti yang melewati akhir pekan atau libur** (Sakit Jumat–Senin): hanya hari kerja yang dihitung (R3). Diuji di Task 1.
3. **Hari cuti yang juga punya baris absensi efektif** (karyawan tetap absen masuk saat cuti): tidak dihitung dua kali di `PaidDays`. Diuji di Task 1.
4. **Karyawan resign tanggal 5 dalam periode 21→20:** ikut run, dibayar sampai tanggal 5, dan cicilan pinjamannya tetap terpotong. Diuji di Task 6.
5. **Kombinasi Izin Pribadi + Transport `per_day` + Jabatan `flat`:** total hilang = 1 hari 4 komponen, tidak lebih. Diuji di Task 3.

---

### Task 1: Klasifikasi hari — hari kerja, hari dibayar, hari cuti per jenis

**Files:**
- Modify: `backend/internal/modules/payroll/domain/inputs.go` (`BuildInputRange`, blok hitung cuti sekitar baris 186-202, dan `return Input{...}`)
- Modify: `backend/internal/modules/payroll/domain/calculator.go` (struct `Input`: tambah `PaidDays`, `MaternityLeaveDays`)
- Modify: `backend/internal/modules/payroll/domain/period.go` (tambah `IsWorkDate`)
- Test: `backend/internal/modules/payroll/domain/paid_days_test.go` (baru)

**Interfaces:**
- Consumes: `ResolveScheduleRow(rows []ShiftRow, date string) *ShiftRow`, `ISODayOfWeek(date string) int`, `EachDate(start, end string) []string`, `DateText(*string) *string` (semua sudah ada).
- Produces:
  - `func IsWorkDate(date string, schedule []ShiftRow, holidays map[string]bool) bool`
  - `Input.PaidDays float64`
  - `Input.MaternityLeaveDays float64`
  - Hitungan `UnpaidLeaveDays`, `PersonalLeaveDays`, `SickLeaveDays` kini dalam hari kerja (R3).

- [ ] **Step 1: Tulis test yang gagal**

`backend/internal/modules/payroll/domain/paid_days_test.go`:

```go
package domain

import "testing"

// Jadwal Senin–Jumat berlaku sepanjang 2026 (shift "s").
func weekdaySchedule() []ShiftRow {
	var rows []ShiftRow
	for d := 1; d <= 5; d++ {
		rows = append(rows, ShiftRow{DayOfWeek: d, ShiftID: ptr("s"), EffectiveFrom: "2026-01-01"})
	}
	return rows
}

func TestIsWorkDate(t *testing.T) {
	holidays := map[string]bool{"2026-09-16": true}
	sched := weekdaySchedule()
	for date, want := range map[string]bool{
		"2026-09-14": true,  // Senin
		"2026-09-19": false, // Sabtu tanpa shift
		"2026-09-16": true,  // libur nasional, tapi jadwal punya shift: jadwal yang menentukan
	} {
		if got := IsWorkDate(date, sched, holidays); got != want {
			t.Errorf("IsWorkDate(%s, jadwal) = %v, want %v", date, got, want)
		}
	}
	// Tanpa jadwal: Senin–Jumat dan bukan libur nasional.
	for date, want := range map[string]bool{"2026-09-14": true, "2026-09-16": false, "2026-09-20": false} {
		if got := IsWorkDate(date, nil, holidays); got != want {
			t.Errorf("IsWorkDate(%s, tanpa jadwal) = %v, want %v", date, got, want)
		}
	}
}

// Periode 1–30 Sep 2026, Senin–Jumat = 22 hari kerja. Hadir 18 hari; cuti tahunan
// 2 hari kerja; sakit Jumat 11 – Senin 14 (2 hari kerja); satu hari cuti tahunan juga
// punya absensi efektif (tidak dihitung dua kali); melahirkan 1 hari; unpaid 1 hari.
func TestBuildInputPaidDays(t *testing.T) {
	emp := Employee{ID: "e", JoinDate: "2020-01-01", EmploymentStatus: "permanent"}
	sal := Salary{BaseSalary: "2200000.00"}
	var att []AttendanceRow
	for _, d := range []string{"01", "02", "03", "04", "07", "08", "09", "10", "15", "16", "17", "18", "21", "22", "23", "24", "25", "28"} {
		att = append(att, AttendanceRow{Date: "2026-09-" + d, Status: "present", HasClockOut: true})
	}
	rec := WorkforceRecords{Schedule: weekdaySchedule(), Attendance: att, Leaves: []Leave{
		{ptr("2026-09-28"), ptr("2026-09-29"), "annual"},  // 28 sudah hadir → hanya 29 yang dihitung
		{ptr("2026-09-11"), ptr("2026-09-14"), "sick"},    // Jumat + Senin = 2 hari kerja
		{ptr("2026-09-30"), ptr("2026-09-30"), "maternity"},
		{ptr("2026-09-29"), ptr("2026-09-29"), "unpaid"}, // tidak masuk hari dibayar
	}}
	in := BuildInputRange(emp, sal, rec, nil, nil, 9, 2026, "2026-09-01", "2026-09-30", false)
	if in.AttendanceDays != 18 {
		t.Fatalf("AttendanceDays = %v, want 18", in.AttendanceDays)
	}
	if in.SickLeaveDays != 2 || in.MaternityLeaveDays != 1 || in.UnpaidLeaveDays != 1 {
		t.Fatalf("sick=%v maternity=%v unpaid=%v", in.SickLeaveDays, in.MaternityLeaveDays, in.UnpaidLeaveDays)
	}
	// 18 hadir + sick 11 & 14 + maternity 30 = 21. Tanggal 28 hadir (cuti tidak dihitung
	// dua kali); tanggal 29 tercakup unpaid sehingga tidak dibayar meski ada annual.
	if in.PaidDays != 21 {
		t.Fatalf("PaidDays = %v, want 21", in.PaidDays)
	}
}
```

Tanggal 29 sengaja tercakup `annual` dan `unpaid` sekaligus. Validasi EPIC-058 mencegah tabrakan pengajuan baru, tetapi data lama bisa tumpang tindih. Aturannya: tanggal yang tercakup `unpaid` **tidak pernah** menjadi hari dibayar ("unpaid menang").

- [ ] **Step 2: Jalankan test, pastikan gagal**

Run: `cd backend && $HOME/.local/go/bin/go test ./internal/modules/payroll/domain -run 'TestIsWorkDate|TestBuildInputPaidDays' -v`
Expected: FAIL, build error `undefined: IsWorkDate` / `in.PaidDays undefined`.

- [ ] **Step 3: Implementasi**

`period.go`, tambahkan setelah `ScheduledDays`:

```go
// IsWorkDate: a date with a shift in the schedule in force that day; without
// a schedule in force, Monday–Friday that is not a national holiday (the
// hris DescribeLeaveDays rule). Payroll counts leave on work dates only.
func IsWorkDate(date string, schedule []ShiftRow, holidays map[string]bool) bool {
	for _, r := range schedule {
		if r.EffectiveFrom <= date && (r.EffectiveTo == nil || *r.EffectiveTo >= date) {
			row := ResolveScheduleRow(schedule, date)
			return row != nil && row.ShiftID != nil
		}
	}
	dow := ISODayOfWeek(date)
	return dow >= 1 && dow <= 5 && !holidays[date]
}
```

`calculator.go`, di struct `Input` setelah `SickLeaveDays float64`:

```go
	// EPIC-059: hari dibayar (hadir + cuti dibayar, hari kerja) untuk gaji
	// pokok monthly_prorated & jabatan per hari; hari cuti melahirkan.
	PaidDays           float64
	MaternityLeaveDays float64
```

`inputs.go`, ganti seluruh blok `unpaid, personal, sick := 0, 0, 0` sampai akhir loop `for _, l := range rec.Leaves {...}` dengan:

```go
	// Hari cuti dihitung pada hari kerja (EPIC-059 R3) di dalam cakupan.
	// Hari dibayar = hadir + cuti selain unpaid pada hari kerja tanpa absensi
	// efektif; tanggal unpaid tidak pernah dibayar.
	attended := map[string]bool{}
	for _, a := range rec.Attendance {
		if a.Status != "absent" && (!partial || inCoverage(a.Date)) {
			attended[a.Date] = true
		}
	}
	unpaidDates, paidLeaveDates := map[string]bool{}, map[string]bool{}
	unpaid, personal, sick, maternity := 0, 0, 0, 0
	for _, l := range rec.Leaves {
		s, e := DateText(l.StartDate), DateText(l.EndDate)
		if s == nil || e == nil {
			continue
		}
		from, to := max(*s, start), min(*e, end)
		for _, d := range EachDate(from, to) {
			if !IsWorkDate(d, schedule, holidays) || (partial && !inCoverage(d)) {
				continue
			}
			// Hanya unpaid/personal/sick/maternity yang memotong; leader dan
			// jenis lain full paid (EPIC-058).
			switch l.LeaveType {
			case "unpaid":
				unpaid++
				unpaidDates[d] = true
				continue
			case "personal":
				personal++
			case "sick":
				sick++
			case "maternity":
				maternity++
			}
			if !attended[d] {
				paidLeaveDates[d] = true
			}
		}
	}
	paidLeave := 0
	for d := range paidLeaveDates {
		if !unpaidDates[d] {
			paidLeave++
		}
	}
	paidDays := float64(attendanceDays + paidLeave)
	if hasSchedule && scheduled > 0 && paidDays > workingDays {
		paidDays = workingDays
	}
```

Lalu, di `return Input{...}`, tambahkan setelah `SickLeaveDays: float64(sick),`:

```go
		MaternityLeaveDays: float64(maternity),
		PaidDays:           paidDays,
```

`EachDate(from, to)` mengembalikan slice kosong bila `from > to` (cek `period.go:49`). Kalau ternyata tidak, tambahkan `if from > to { continue }` sebelum loop tanggal.

- [ ] **Step 4: Jalankan test, pastikan lulus, lalu seluruh paket domain**

Run: `cd backend && $HOME/.local/go/bin/go test ./internal/modules/payroll/domain -run 'TestIsWorkDate|TestBuildInputPaidDays' -v`
Expected: PASS.

Run: `cd backend && $HOME/.local/go/bin/go test ./internal/modules/payroll/domain`
Expected: `ok`. Test lama yang memakai cuti satu hari pada hari kerja (`TestLeaderAndPersonalLeavePayroll`, 8–10 Sep 2026 = Selasa–Kamis) tetap lulus, karena tanpa jadwal tanggal itu tetap hari kerja (R3). Kalau ada test lama yang menghitung cuti di akhir pekan, perbarui ekspektasinya dan catat ruling di ledger.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/modules/payroll/domain/period.go backend/internal/modules/payroll/domain/calculator.go backend/internal/modules/payroll/domain/inputs.go backend/internal/modules/payroll/domain/paid_days_test.go
git commit -m "feat(payroll): hari dibayar & hari cuti per jenis dihitung pada hari kerja, EPIC-059"
```

---

### Task 2: Pembagi = hari kerja terjadwal (K-12)

**Files:**
- Modify: `backend/internal/modules/payroll/domain/inputs.go` (blok `divisor := OrZero(salary.WorkingDaysDivisor)`, sekitar baris 204-213)
- Test: `backend/internal/modules/payroll/domain/paid_days_test.go`

**Interfaces:**
- Consumes: `scheduled`, `hasSchedule`, `workingDays` di `BuildInputRange` (sudah ada).
- Produces: `Input.DivisorDays` = hari kerja terjadwal untuk `monthly`/`monthly_prorated` bila jadwal ada.

- [ ] **Step 1: Tulis test yang gagal**

Tambahkan ke `paid_days_test.go`:

```go
// K-12: pembagi = hari kerja terjadwal periode; pembagi struktur hanya tanpa jadwal.
func TestDivisorFollowsSchedule(t *testing.T) {
	emp := Employee{ID: "e", JoinDate: "2020-01-01", EmploymentStatus: "permanent"}
	sal := Salary{BaseSalary: "2200000.00", WorkingDaysDivisor: "26.00"}
	withSchedule := BuildInputRange(emp, sal, WorkforceRecords{Schedule: weekdaySchedule()}, nil, nil, 9, 2026, "2026-09-01", "2026-09-30", false)
	if withSchedule.DivisorDays != 22 {
		t.Fatalf("pembagi dengan jadwal = %v, want 22 (hari kerja terjadwal)", withSchedule.DivisorDays)
	}
	noSchedule := BuildInputRange(emp, sal, WorkforceRecords{}, nil, nil, 9, 2026, "2026-09-01", "2026-09-30", false)
	if noSchedule.DivisorDays != 26 {
		t.Fatalf("pembagi tanpa jadwal = %v, want 26 (struktur)", noSchedule.DivisorDays)
	}
	daily := sal
	daily.PayBasis = ptr("daily")
	daily.WorkingDaysDivisor = nil
	if d := BuildInputRange(emp, daily, WorkforceRecords{Schedule: weekdaySchedule()}, nil, nil, 9, 2026, "2026-09-01", "2026-09-30", false).DivisorDays; d != 0 {
		t.Fatalf("daily tanpa pembagi struktur = %v, want 0 (tidak berubah)", d)
	}
}
```

- [ ] **Step 2: Jalankan test, pastikan gagal**

Run: `cd backend && $HOME/.local/go/bin/go test ./internal/modules/payroll/domain -run TestDivisorFollowsSchedule -v`
Expected: FAIL `pembagi dengan jadwal = 26, want 22`.

- [ ] **Step 3: Implementasi**

Ganti blok pembagi di `inputs.go`:

```go
	// Pembagi (EPIC-059 K-12): hari kerja terjadwal periode untuk gaji bulanan;
	// tanpa jadwal, pembagi struktur lalu hari kerja hasil hitung. Upah harian
	// tidak berubah (struktur kosong = aturan per hari tidak berlaku).
	isDaily := salary.PayBasis != nil && *salary.PayBasis == "daily"
	divisor := 0.0
	if !isDaily && hasSchedule && scheduled > 0 {
		divisor = float64(scheduled)
	}
	if divisor <= 0 {
		divisor = OrZero(salary.WorkingDaysDivisor)
	}
	if divisor <= 0 {
		divisor = workingDays
		if isDaily {
			divisor = 0
		}
	}
```

- [ ] **Step 4: Jalankan test, pastikan lulus, lalu seluruh paket**

Run: `cd backend && $HOME/.local/go/bin/go test ./internal/modules/payroll/domain -run 'TestDivisorFollowsSchedule|TestBuildInputPaidDays' -v && $HOME/.local/go/bin/go test ./internal/modules/payroll/domain`
Expected: PASS dan `ok`. Kalau test parity lama mengunci pembagi struktur pada input dengan jadwal, perbarui ekspektasinya ke hari terjadwal dan catat ruling (perubahan perilaku yang disengaja, K-12).

- [ ] **Step 5: Commit**

```bash
git add backend/internal/modules/payroll/domain/inputs.go backend/internal/modules/payroll/domain/paid_days_test.go
git commit -m "feat(payroll): pembagi harian = hari kerja terjadwal periode (K-12), EPIC-059"
```

---

### Task 3: Kalkulator — hari dibayar, PIP 4 komponen tanpa potongan ganda, Sakit & Cuti Melahirkan, telat tier

**Files:**
- Modify: `backend/internal/modules/payroll/domain/calculator.go` (`Calculate`: basis `monthly_prorated`, jabatan `per_day`, PIP, PIS, telat tier)
- Modify: `frontend/src/lib/payroll/ui-settings-form.ts:108-112` (teks bantuan saklar sakit)
- Modify: `frontend/src/features/hris/payroll/components/payslip-page.tsx` & `frontend/src/features/hris/ess/components/payslip-detail-dialog.tsx` (label baris potongan sakit)
- Modify: `backend/internal/modules/payroll/payslip_pdf.go` (label `{"Potongan Izin Sakit", "sick_leave_deduction"}`)
- Test: `backend/internal/modules/payroll/domain/thp_day_test.go` (baru)

**Interfaces:**
- Consumes: `Input.PaidDays`, `Input.MaternityLeaveDays` (Task 1); `Input.DivisorDays` (Task 2); `IsInternStatus` (PR #3).
- Produces: `func ThpDayBasis(in Input) float64` (4 komponen nominal, R5); `Result.SickLeaveDeduction` kini mencakup cuti melahirkan.

- [ ] **Step 1: Tulis test yang gagal**

`backend/internal/modules/payroll/domain/thp_day_test.go`:

```go
package domain

import "testing"

func base4(mod func(*Input)) Input {
	return parityInput(func(in *Input) {
		in.EmploymentStatus = "contract"
		in.BaseSalary, in.TransportAllowance, in.MealAllowance, in.PositionAllowance = 2_200_000, 220_000, 220_000, 440_000
		in.DivisorDays, in.WorkingDays, in.PayBasis = 22, 22, "monthly_prorated"
		in.AttendanceDays, in.AllowanceDays, in.PaidDays = 21, 21, 22
		if mod != nil {
			mod(in)
		}
	})
}

func TestThpDayBasisIsFourComponents(t *testing.T) {
	in := base4(func(in *Input) { in.Bonus, in.OvertimeHours = 1_000_000, 5 })
	if got := ThpDayBasis(in); got != 3_080_000 {
		t.Fatalf("ThpDayBasis = %v, want 3.080.000 (pokok+transport+makan+jabatan, tanpa bonus/lembur)", got)
	}
}

// monthly_prorated: gaji pokok & jabatan per hari dibayar (cuti dibayar ikut);
// transport & makan per hari efektif.
func TestProratedUsesPaidDays(t *testing.T) {
	r := Calculate(base4(func(in *Input) { in.TransportPerDay, in.MealPerDay, in.PositionPerDay = true, true, true }), DefaultConfig())
	eq(t, "pokok 22/22", r.BaseSalary, 2_200_000)
	eq(t, "jabatan 22/22", r.PositionAllowance, 440_000)
	eq(t, "transport 21/22", r.TransportAllowance, 210_000)
	eq(t, "makan 21/22", r.MealAllowance, 210_000)
}

// Izin Pribadi 1 hari, transport & makan per_day (sudah tidak dibayar hari itu):
// PIP hanya pokok + jabatan → total hilang = tepat 1 hari 4 komponen (140.000).
func TestPersonalLeaveNoDoubleDeduction(t *testing.T) {
	c := DefaultConfig()
	c.PersonalLeaveDeduction = true
	in := base4(func(in *Input) {
		in.TransportPerDay, in.MealPerDay, in.PositionPerDay = true, true, true
		in.PersonalLeaveDays = 1
	})
	r := Calculate(in, c)
	eq(t, "PIP = (pokok + jabatan)/22", r.PersonalLeaveDeduction, 120_000)
	full := Calculate(base4(func(in *Input) {
		in.TransportPerDay, in.MealPerDay, in.PositionPerDay = true, true, true
		in.AttendanceDays, in.AllowanceDays = 22, 22
	}), c)
	eq(t, "total hilang = 1 hari 4 komponen", full.NetSalary-r.NetSalary, 140_000)

	flat := Calculate(base4(func(in *Input) { in.PersonalLeaveDays = 1 }), c)
	eq(t, "semua flat: PIP = 4 komponen/22", flat.PersonalLeaveDeduction, 140_000)

	intern := Calculate(base4(func(in *Input) { in.EmploymentStatus, in.PersonalLeaveDays = "intern", 1 }), c)
	eq(t, "intern tanpa PIP", intern.PersonalLeaveDeduction, 0)
}

// Sakit & Cuti Melahirkan: potong transport + makan hanya bagian flat.
func TestSickAndMaternityDeductFlatAllowancesOnly(t *testing.T) {
	c := DefaultConfig()
	c.SickLeaveDeduction = true
	flat := Calculate(base4(func(in *Input) { in.SickLeaveDays, in.MaternityLeaveDays = 1, 2 }), c)
	eq(t, "flat: (220k+220k)/22 × 3", flat.SickLeaveDeduction, 60_000)
	perDay := Calculate(base4(func(in *Input) {
		in.TransportPerDay, in.MealPerDay = true, true
		in.SickLeaveDays, in.MaternityLeaveDays = 1, 2
	}), c)
	eq(t, "per_day: sudah tidak dibayar lewat hari efektif", perDay.SickLeaveDeduction, 0)
}

// Telat tier basis thp memakai THP 4 komponen, bukan seluruh slip.
func TestLateTierUsesFourComponentThp(t *testing.T) {
	c := DefaultConfig()
	c.LateMode, c.LateMinMinutes = "tier", 1
	c.LateTiers = []LateTier{{Min: 1, Days: 1, Basis: "thp"}}
	r := Calculate(base4(func(in *Input) {
		in.Bonus = 2_200_000
		in.LateDayMinutes = []float64{30}
	}), c)
	eq(t, "1 hari THP 4 komponen", r.LateDeduction, 140_000)
}
```

- [ ] **Step 2: Jalankan test, pastikan gagal**

Run: `cd backend && $HOME/.local/go/bin/go test ./internal/modules/payroll/domain -run 'TestThpDayBasis|TestProratedUsesPaidDays|TestPersonalLeaveNoDoubleDeduction|TestSickAndMaternity|TestLateTierUsesFour' -v`
Expected: FAIL. `undefined: ThpDayBasis`, yang membuat seluruh paket gagal build.

- [ ] **Step 3: Implementasi**

`calculator.go`, tambahkan sebelum `func Calculate`:

```go
// ThpDayBasis is "THP" for a one-day deduction (EPIC-059 R5): the monthly
// base salary + transport + meal + position allowance of the structure.
func ThpDayBasis(in Input) float64 {
	return in.BaseSalary + in.TransportAllowance + in.MealAllowance + in.PositionAllowance
}
```

Di `Calculate`, ganti kasus `monthly_prorated`:

```go
	case "monthly_prorated":
		// Hari kerja aktif yang dibayar (EPIC-059): hadir + cuti dibayar.
		paid := in.PaidDays
		if paid == 0 {
			paid = in.AttendanceDays
		}
		base = Round(perDay(fullBase, paid))
		dayWage = perDay(fullBase, 1)
```

Ganti baris jabatan:

```go
	// Jabatan per hari mengikuti hari dibayar (EPIC-059); transport & makan
	// mengikuti hari efektif (AllowanceDays).
	positionDays := in.PaidDays
	if positionDays == 0 {
		positionDays = in.AllowanceDays
	}
	position := Round(in.PositionAllowance * factor)
	if in.PositionPerDay {
		position = Round(perDay(in.PositionAllowance, positionDays))
	}
```

Ganti cabang telat tier `if basis == "thp" { late = Round(perDay(thp, days)) }` dengan:

```go
		if basis == "thp" {
			late = Round(perDay(ThpDayBasis(in), days))
		}
```

Ganti blok PIP/PIS (blok setelah komentar `// G6: potongan izin pribadi (PIP) dan izin sakit (PIS).`, termasuk pengecekan intern dari PR #3):

```go
	// G6: PIP dan PIS (EPIC-059 R6/R7). Komponen per_day sudah tidak dibayar
	// pada hari tidak hadir, jadi yang dipotong hanya bagian yang tetap dibayar:
	// PIP = pokok (bukan upah harian) + jabatan + transport/makan flat;
	// PIS (sakit + melahirkan) = transport/makan flat. Intern tidak dipotong PIP.
	flatTransport, flatMeal := in.TransportAllowance, in.MealAllowance
	if in.TransportPerDay {
		flatTransport = 0
	}
	if in.MealPerDay {
		flatMeal = 0
	}
	personal, sick := 0.0, 0.0
	if c.PersonalLeaveDeduction && in.PersonalLeaveDays > 0 && !IsInternStatus(in.EmploymentStatus) {
		pipBasis := in.PositionAllowance + flatTransport + flatMeal
		if in.PayBasis != "daily" {
			pipBasis += fullBase
		}
		personal = Round(perDay(pipBasis, in.PersonalLeaveDays))
	}
	if sickDays := in.SickLeaveDays + in.MaternityLeaveDays; c.SickLeaveDeduction && sickDays > 0 {
		sick = Round(perDay(flatTransport+flatMeal, sickDays))
	}
```

Hapus variabel `thp` bila tidak dipakai lagi. `thp := gross + nontax` masih dipakai untuk `net`? `net` dihitung dari `gross + nontax - total`, jadi `thp` boleh dihapus. Pastikan `go vet` bersih.

Label slip: di `payslip_pdf.go` ganti `{"Potongan Izin Sakit", "sick_leave_deduction"}` menjadi `{"Potongan Sakit & Cuti Melahirkan", "sick_leave_deduction"}`. Lakukan pengganti teks yang sama di `payslip-page.tsx` dan `payslip-detail-dialog.tsx` bila label "Potongan Izin Sakit" ada di sana (`grep -rn "Potongan Izin Sakit" frontend/src`). Di `ui-settings-form.ts`, saklar `sick_leave_deduction`:

```ts
  {
    key: "sick_leave_deduction",
    label: "Potong transport & makan saat Sakit / Cuti Melahirkan",
    help: "Hanya tunjangan flat: (transport + makan) ÷ pembagi hari × hari sakit & melahirkan. Tunjangan per hari sudah tidak dibayar saat tidak hadir.",
  },
```

Saklar `personal_leave_deduction`, ganti `help` menjadi: `"1 hari THP (gaji pokok + transport + makan + jabatan) ÷ hari kerja terjadwal × hari izin. Tunjangan per hari tidak dipotong dua kali; intern tidak dipotong."`

- [ ] **Step 4: Jalankan test, pastikan lulus, lalu seluruh paket payroll + frontend terkait**

Run: `cd backend && $HOME/.local/go/bin/go test ./internal/modules/payroll/domain -v -run 'TestThpDayBasis|TestProratedUsesPaidDays|TestPersonalLeaveNoDoubleDeduction|TestSickAndMaternity|TestLateTierUsesFour|TestLeaveDeductions|TestLeaderAndPersonalLeavePayroll|TestInternPersonalLeaveSkipsPIP'`
Expected: PASS semua. `TestLeaveDeductions` (semua tunjangan flat: PIP 240.000, PIS 60.000) dan `TestInternPersonalLeaveSkipsPIP` tetap lulus. Kalau test parity lama tentang telat tier `thp` dengan bonus/lembur di THP gagal, ekspektasinya berubah sesuai R8. Perbarui dengan komentar "EPIC-059 R8" dan catat ruling.

Run: `cd backend && TEST_DATABASE_URL='postgres://postgres@127.0.0.1:55499/paddy_test?sslmode=disable' $HOME/.local/go/bin/go test ./internal/modules/payroll/... ./internal/app -count=1 && $HOME/.local/go/bin/go vet ./internal/modules/payroll/...`
Expected: `ok`, vet bersih.

Run: `cd frontend && npx vitest run src/lib/payroll src/features/hris/payroll src/features/hris/ess`
Expected: PASS. Perbarui snapshot/teks test yang mengunci label lama "Potongan Izin Sakit".

- [ ] **Step 5: Commit**

```bash
git add backend/internal/modules/payroll frontend/src/lib/payroll/ui-settings-form.ts frontend/src/features/hris/payroll frontend/src/features/hris/ess
git commit -m "feat(payroll): 1 hari THP = 4 komponen tanpa potongan ganda; sakit & cuti melahirkan potong tunjangan flat, EPIC-059"
```

---

### Task 4: Minimum lembur 1 jam

**Files:**
- Modify: `backend/internal/modules/hris/overtime.go` (setelah `hours := domain.OvertimeHoursFromTimes(...)`, sekitar baris 126)
- Modify: `backend/internal/modules/payroll/domain/period.go` (`SplitOvertimeWith`, loop pengajuan)
- Test: `backend/internal/modules/hris/overtime_min_integration_test.go` (baru), `backend/internal/modules/payroll/domain/paid_days_test.go`

**Interfaces:**
- Consumes: harness hris (`newHarness`, `h.staff`, `h.do`, `expect`), `SplitOvertimeWith(requests, attendance, holidays, schedule, requireClockout)`.
- Produces: pesan error `Durasi lembur minimal 1 jam`; jam terealisasi < 1 tidak dibayar.

- [ ] **Step 1: Tulis test yang gagal**

`backend/internal/modules/hris/overtime_min_integration_test.go`:

```go
package hris

import "testing"

// Tabel payslip Paddy: lembur minimal 1 jam (EPIC-059 R9).
func TestOvertimeMinimumOneHour(t *testing.T) {
	h := newHarness(t)
	staff, _ := h.staff("pos", nil, true)
	expect(t, h.do(&staff, "POST", "/api/hris/overtime", map[string]any{
		"date": "2031-03-04", "start_time": "17:00", "end_time": "17:45", "reason": "Rapat"}), 400,
		"Durasi lembur minimal 1 jam")
	expect(t, h.do(&staff, "POST", "/api/hris/overtime", map[string]any{
		"date": "2031-03-04", "start_time": "17:00", "end_time": "18:00", "reason": "Rapat"}), 200, "")
}
```

Tambahkan ke `paid_days_test.go`:

```go
func TestOvertimeUnderOneHourNotPaid(t *testing.T) {
	att := []AttendanceRow{
		{Date: "2026-09-07", Status: "present", HasClockOut: true, OvertimeHours: "0.50"},
		{Date: "2026-09-08", Status: "present", HasClockOut: true, OvertimeHours: "2.00"},
	}
	req := []OvertimeRequest{{Date: "2026-09-07", Hours: 0.5}, {Date: "2026-09-08", Hours: 2}}
	got := SplitOvertimeWith(req, att, nil, nil, true)
	if got.Regular != 2 {
		t.Fatalf("Regular = %v, want 2 (0,5 jam tidak dibayar)", got.Regular)
	}
}
```

Cek nama field `OvertimeRequest` (`grep -n "type OvertimeRequest" -A6 backend/internal/modules/payroll/domain/*.go`), terutama `Date`, `Hours`, dan `DayType`. Sesuaikan literal bila nama atau tipe `Hours` berbeda, tanpa mengubah maksud test.

- [ ] **Step 2: Jalankan test, pastikan gagal**

Run: `cd backend && $HOME/.local/go/bin/go test ./internal/modules/payroll/domain -run TestOvertimeUnderOneHourNotPaid -v; TEST_DATABASE_URL='postgres://postgres@127.0.0.1:55499/paddy_test?sslmode=disable' $HOME/.local/go/bin/go test ./internal/modules/hris -run TestOvertimeMinimumOneHour -count=1 -v`
Expected: domain FAIL `Regular = 2.5`; hris FAIL `status 200, want 400`.

- [ ] **Step 3: Implementasi**

`hris/overtime.go`, setelah cek maksimal 12 jam:

```go
	if hours < 1 {
		return nil, httpx.BadRequest("Durasi lembur minimal 1 jam")
	}
```

`payroll/domain/period.go`, di `SplitOvertimeWith` tepat setelah blok `if ok { ... hours = math.Min(...) }`:

```go
		// Tabel payslip Paddy: lembur dibayar minimal 1 jam (EPIC-059 R9).
		if hours < 1 {
			continue
		}
```

- [ ] **Step 4: Jalankan test, pastikan lulus**

Run: perintah Step 2.
Expected: PASS keduanya. Lalu `TEST_DATABASE_URL=... $HOME/.local/go/bin/go test ./internal/modules/hris/... ./internal/modules/payroll/... -count=1`. Expected: hanya baseline `TestAttendancePeriodClosing` yang gagal. Kalau test lama membuat lembur < 1 jam, ubah durasinya menjadi ≥ 1 jam dan catat ruling.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/modules/hris/overtime.go backend/internal/modules/hris/overtime_min_integration_test.go backend/internal/modules/payroll/domain/period.go backend/internal/modules/payroll/domain/paid_days_test.go
git commit -m "feat(hris,payroll): lembur minimal 1 jam, EPIC-059"
```

---

### Task 5: Subsidi BPJS Kesehatan global (Rp100.000, masa kerja > 1 tahun pada cutoff)

**Files:**
- Create: `backend/database/migrations/deltas/20261021090000_payroll_bpjs_kes_subsidy.sql`
- Modify: `backend/internal/modules/payroll/settings.go` (daftar field setting, dekat `loan_max_installment_percent`)
- Modify: `backend/internal/modules/payroll/domain/config.go` (`Config` + `BuildConfig`)
- Modify: `backend/internal/modules/payroll/domain/inputs.go` (`BuildOptions`, `BuildInputRange`)
- Modify: `backend/internal/modules/payroll/domain/period.go` (`ServiceOverOneYear`)
- Modify: `backend/internal/modules/payroll/service_runs.go:431` (isi `BuildOptions`)
- Modify: `frontend/src/lib/payroll/ui-settings-form.ts` (`BPJS_KES_FIELDS`)
- Test: `backend/internal/modules/payroll/domain/paid_days_test.go`

**Interfaces:**
- Consumes: `HealthEligible` (lama, tetap dipakai bila subsidi 0).
- Produces:
  - Kolom `hris.payroll_settings.bpjs_kes_subsidy_amount numeric NOT NULL DEFAULT 0`.
  - `Config.BpjsKesSubsidyAmount float64`.
  - `BuildOptions.HealthSubsidyAmount float64`.
  - `func ServiceOverOneYear(joinDate, periodEnd string) bool`.

- [ ] **Step 1: Tulis test yang gagal**

Tambahkan ke `paid_days_test.go`:

```go
func TestServiceOverOneYear(t *testing.T) {
	for _, c := range []struct {
		join, end string
		want      bool
	}{
		{"2025-10-20", "2026-10-20", false}, // tepat 1 tahun: belum lebih
		{"2025-10-19", "2026-10-20", true},
		{"2026-01-01", "2026-10-20", false},
		{"bukan-tanggal", "2026-10-20", false},
	} {
		if got := ServiceOverOneYear(c.join, c.end); got != c.want {
			t.Errorf("ServiceOverOneYear(%s, %s) = %v, want %v", c.join, c.end, got, c.want)
		}
	}
}

func TestHealthSubsidyOverridesStructure(t *testing.T) {
	sal := Salary{BaseSalary: "2200000.00", HealthAllowance: "50000.00"}
	opts := DefaultBuildOptions()
	opts.HealthSubsidyAmount = 100_000
	senior := BuildInputRange(Employee{ID: "a", JoinDate: "2024-01-01"}, sal, WorkforceRecords{}, nil, nil, 10, 2026, "2026-09-21", "2026-10-20", false, opts)
	if senior.HealthAllowance != 100_000 || !senior.HealthEligible {
		t.Fatalf("senior: allowance=%v eligible=%v", senior.HealthAllowance, senior.HealthEligible)
	}
	junior := BuildInputRange(Employee{ID: "b", JoinDate: "2026-01-01"}, sal, WorkforceRecords{}, nil, nil, 10, 2026, "2026-09-21", "2026-10-20", false, opts)
	if junior.HealthEligible {
		t.Fatal("junior (< 1 tahun) tidak berhak subsidi")
	}
	// Setting 0: perilaku lama (nominal struktur).
	old := BuildInputRange(Employee{ID: "a", JoinDate: "2024-01-01"}, sal, WorkforceRecords{}, nil, nil, 10, 2026, "2026-09-21", "2026-10-20", false)
	if old.HealthAllowance != 50_000 {
		t.Fatalf("tanpa setting: allowance=%v, want 50.000", old.HealthAllowance)
	}
}
```

- [ ] **Step 2: Jalankan test, pastikan gagal**

Run: `cd backend && $HOME/.local/go/bin/go test ./internal/modules/payroll/domain -run 'TestServiceOverOneYear|TestHealthSubsidy' -v`
Expected: FAIL (build error `undefined: ServiceOverOneYear`, `opts.HealthSubsidyAmount undefined`).

- [ ] **Step 3: Implementasi**

Migrasi `20261021090000_payroll_bpjs_kes_subsidy.sql`:

```sql
-- Subsidi BPJS Kesehatan global (EPIC-059, tabel payslip Paddy): Rp100.000
-- untuk masa kerja > 1 tahun pada tanggal cutoff. 0 = nominal per struktur gaji.
ALTER TABLE hris.payroll_settings
  ADD COLUMN IF NOT EXISTS bpjs_kes_subsidy_amount numeric NOT NULL DEFAULT 0;
```

`period.go`:

```go
// ServiceOverOneYear: more than one year of service at periodEnd (the
// cut-off date): join + 1 year falls strictly before periodEnd (EPIC-059).
func ServiceOverOneYear(joinDate, periodEnd string) bool {
	join, err1 := time.Parse("2006-01-02", joinDate)
	end, err2 := time.Parse("2006-01-02", periodEnd)
	if err1 != nil || err2 != nil {
		return false
	}
	return join.AddDate(1, 0, 0).Before(end)
}
```

`inputs.go`, `BuildOptions`, tambahkan:

```go
	// HealthSubsidyAmount > 0 replaces the structure's health allowance with a
	// global BPJS Kesehatan subsidy for more than one year of service (EPIC-059).
	HealthSubsidyAmount float64
```

Di `BuildInputRange`, sebelum `return Input{`:

```go
	health := OrZero(salary.HealthAllowance)
	healthEligible := HealthEligible(emp.JoinDate, end, minService)
	if opt.HealthSubsidyAmount > 0 {
		health, healthEligible = opt.HealthSubsidyAmount, ServiceOverOneYear(emp.JoinDate, end)
	}
```

Lalu ganti kedua field di literal `Input` menjadi `HealthAllowance: health,` dan `HealthEligible: healthEligible,`.

`config.go`: tambahkan field `BpjsKesSubsidyAmount float64` ke `Config` (dekat `LoanMaxActivePerPerson`), dan di `BuildConfig` sebelum `return c`:

```go
	c.BpjsKesSubsidyAmount = settings.num("bpjs_kes_subsidy_amount", d.BpjsKesSubsidyAmount)
```

`settings.go`: tambahkan `nums("num", rupiah(), "bpjs_kes_subsidy_amount"),` tepat sebelum `nums("num", pct(), "loan_max_installment_percent"),`.

`service_runs.go:431`:

```go
	opts := domain.BuildOptions{OvertimeRequireClockout: config.OvertimeRequireClockout,
		OvertimeRestFromSchedule: config.OvertimeRestFromSchedule, HealthSubsidyAmount: config.BpjsKesSubsidyAmount}
```

`ui-settings-form.ts`, di `BPJS_KES_FIELDS` tambahkan:

```ts
  { key: "bpjs_kes_subsidy_amount", label: "Subsidi BPJS Kesehatan (masa kerja > 1 tahun)", suffix: "Rp" },
```

Terapkan migrasi ke DB test: `cd backend && MIGRATE_DATABASE_URL='postgres://postgres@127.0.0.1:55499/paddy_test?sslmode=disable' $HOME/.local/go/bin/go run ./cmd/migrate -apply`. Expected: `[ok] 20261021090000_payroll_bpjs_kes_subsidy.sql`.

- [ ] **Step 4: Jalankan test, pastikan lulus**

Run: `cd backend && $HOME/.local/go/bin/go test ./internal/modules/payroll/domain -run 'TestServiceOverOneYear|TestHealthSubsidy' -v && TEST_DATABASE_URL='postgres://postgres@127.0.0.1:55499/paddy_test?sslmode=disable' $HOME/.local/go/bin/go test ./internal/modules/payroll/... -count=1`
Expected: PASS dan `ok`. Kalau ada test settings API yang mengunci daftar key, tambahkan `bpjs_kes_subsidy_amount` ke ekspektasinya.

Run: `cd frontend && npx vitest run src/lib/payroll src/features/hris/payroll && npx tsc --noEmit -p .`
Expected: PASS, tsc tanpa output.

- [ ] **Step 5: Commit**

```bash
git add backend/database/migrations/deltas/20261021090000_payroll_bpjs_kes_subsidy.sql backend/internal/modules/payroll frontend/src/lib/payroll/ui-settings-form.ts
git commit -m "feat(payroll): setting subsidi BPJS Kesehatan global untuk masa kerja > 1 tahun, EPIC-059"
```

---

### Task 6: Prorata karyawan keluar di tengah periode (PY-04)

**Files:**
- Modify: `backend/internal/modules/payroll/ports.go` (`EmployeeBrief.EndDate`, signature `Active`)
- Modify: `backend/internal/app/adapters_payroll.go` (`payrollEmployees.Active`)
- Modify: `backend/internal/modules/payroll/service_runs.go` (pemanggil `Active`, `domain.Employee{...}` di `payrollInput`)
- Modify: `backend/internal/modules/payroll/domain/inputs.go` (`Employee.EndDate`, batas cakupan)
- Test: `backend/internal/modules/payroll/domain/paid_days_test.go`, `backend/internal/app/payroll_leaver_test.go` (baru)

**Interfaces:**
- Consumes: mekanisme cakupan kontrak di `BuildInputRange` (`merged`, `hasContract`, `partial`, `inCoverage`).
- Produces:
  - `domain.Employee.EndDate *string`.
  - `payroll.EmployeeBrief.EndDate *string`.
  - `Active(ctx, q, start, end string)`: aktif ATAU `end_date` di dalam [start, end].

- [ ] **Step 1: Tulis test yang gagal**

Tambahkan ke `paid_days_test.go`:

```go
// Resign 9 Sep (Rabu) dalam periode 1–30 Sep: hari terjadwal 1–9 Sep = 7 dari 22.
func TestLeaverProratedToEndDate(t *testing.T) {
	emp := Employee{ID: "e", JoinDate: "2020-01-01", EmploymentStatus: "resigned", EndDate: ptr("2026-09-09")}
	sal := Salary{BaseSalary: "2200000.00"}
	att := []AttendanceRow{
		{Date: "2026-09-01", Status: "present", HasClockOut: true},
		{Date: "2026-09-15", Status: "present", HasClockOut: true}, // setelah end_date: diabaikan
	}
	in := BuildInputRange(emp, sal, WorkforceRecords{Schedule: weekdaySchedule(), Attendance: att}, nil, nil, 9, 2026, "2026-09-01", "2026-09-30", false)
	if in.ProrateFactor == nil || Round(*in.ProrateFactor*1000) != Round(7.0/22*1000) {
		t.Fatalf("factor = %v, want 7/22", in.ProrateFactor)
	}
	if in.AttendanceDays != 1 {
		t.Fatalf("AttendanceDays = %v, want 1 (absen setelah end_date diabaikan)", in.AttendanceDays)
	}
	r := Calculate(in, DefaultConfig())
	eq(t, "pokok monthly × 7/22", r.BaseSalary, 700_000)
}
```

`backend/internal/app/payroll_leaver_test.go` (paket `app`; ikuti pola `payrollFilesSetup` di `adapters_payroll_files_test.go`):

```go
package app

import (
	"context"
	"testing"

	"nuhabit/backend/internal/platform/testutil"
)

// PY-04: karyawan nonaktif yang end_date-nya di dalam periode tetap ikut run.
func TestActiveIncludesLeaversInPeriod(t *testing.T) {
	tx := testutil.Tx(t)
	ctx := context.Background()
	var leaver, gone string
	if err := tx.QueryRow(ctx, `INSERT INTO hris.employees (full_name, nip, email, phone, join_date, is_active, end_date, employment_status)
		VALUES ('Keluar Periode', 'LV-'||md5(random()::text), md5(random()::text)||'@lv.test', '08', '2024-01-01', false, '2031-03-09', 'resigned')
		RETURNING id::text`).Scan(&leaver); err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRow(ctx, `INSERT INTO hris.employees (full_name, nip, email, phone, join_date, is_active, end_date, employment_status)
		VALUES ('Keluar Lama', 'LV-'||md5(random()::text), md5(random()::text)||'@lv.test', '08', '2024-01-01', false, '2031-01-31', 'resigned')
		RETURNING id::text`).Scan(&gone); err != nil {
		t.Fatal(err)
	}
	list, err := payrollEmployees{}.Active(ctx, tx, "2031-02-21", "2031-03-20")
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]bool{}
	for _, e := range list {
		found[e.ID] = true
	}
	if !found[leaver] || found[gone] {
		t.Fatalf("leaver in=%v, gone in=%v", found[leaver], found[gone])
	}
}
```

Sebelum menulis test ini, cek kolom wajib `hris.employees` di DB test (`\d hris.employees`). Bila insert gagal karena kolom NOT NULL lain, tambahkan kolom itu dengan nilai uji. Pastikan juga `payrollEmployees{}` bisa dibuat tanpa field (lihat definisinya di `adapters_payroll.go`). Kalau butuh dependency, buat dengan cara yang sama seperti `payrollPorts()`.

- [ ] **Step 2: Jalankan test, pastikan gagal**

Run: `cd backend && $HOME/.local/go/bin/go test ./internal/modules/payroll/domain -run TestLeaverProratedToEndDate -v`
Expected: FAIL (build error `unknown field EndDate in struct literal`).

- [ ] **Step 3: Implementasi**

`domain/inputs.go`, struct `Employee`:

```go
	EndDate          *string // YYYY-MM-DD; resign/akhir kerja (EPIC-059 PY-04)
```

Di `BuildInputRange`, tepat setelah `merged := MergeRanges(coverages)`:

```go
	// PY-04: tanggal setelah end_date karyawan berada di luar cakupan.
	if ed := DateText(emp.EndDate); ed != nil && *ed < end && *ed >= start {
		if len(merged) == 0 {
			merged = []DateRange{{Start: start, End: *ed}}
		} else {
			var clipped []DateRange
			for _, r := range merged {
				if r.Start <= *ed {
					clipped = append(clipped, DateRange{Start: r.Start, End: min(r.End, *ed)})
				}
			}
			merged = clipped
		}
	}
```

Cek nama field `DateRange` (`grep -n "type DateRange" -A3 backend/internal/modules/payroll/domain/*.go`) dan sesuaikan `Start`/`End` bila berbeda. `hasContract` dan `partial` dihitung dari `merged` di baris sesudahnya, jadi mekanisme kontrak parsial berlaku otomatis.

`ports.go`: tambahkan `EndDate *string` ke `EmployeeBrief`, dan ubah signature `Active` di interface `Employees`:

```go
	Active(ctx context.Context, q database.Querier, start, end string) ([]EmployeeBrief, error)
```

`adapters_payroll.go`, `Active`:

```go
func (payrollEmployees) Active(ctx context.Context, q database.Querier, start, end string) ([]payroll.EmployeeBrief, error) {
	rows, err := q.Query(ctx, `SELECT id::text, full_name, nip, is_active, employment_status, join_date::text,
		department_id::text, end_date::text FROM hris.employees
		WHERE is_active = true OR (end_date BETWEEN $1::date AND $2::date)`, start, end)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (payroll.EmployeeBrief, error) {
		var b payroll.EmployeeBrief
		err := r.Scan(&b.ID, &b.FullName, &b.NIP, &b.IsActive, &b.EmploymentStatus, &b.JoinDate, &b.DepartmentID, &b.EndDate)
		return b, err
	})
}
```

`service_runs.go`: setiap pemanggil `s.ports.Employees.Active(ctx, s.db)` (`grep -n "Employees.Active" backend/internal/modules/payroll/*.go`) diberi rentang periode yang sama dengan yang dipakai `payrollInput` (`start`, `end` periode absensi). Di `payrollInput`:

```go
	emp := domain.Employee{ID: e.ID, EmploymentStatus: e.EmploymentStatus, JoinDate: e.JoinDate, EndDate: e.EndDate}
```

Fake `Employees` di test payroll (bila ada; `grep -rn "func (.*) Active(ctx" backend/internal --include=*_test.go`) diubah ke signature baru.

- [ ] **Step 4: Jalankan test, pastikan lulus**

Run: `cd backend && $HOME/.local/go/bin/go build ./... && $HOME/.local/go/bin/go test ./internal/modules/payroll/domain -run TestLeaverProratedToEndDate -v && TEST_DATABASE_URL='postgres://postgres@127.0.0.1:55499/paddy_test?sslmode=disable' $HOME/.local/go/bin/go test ./internal/app -run TestActiveIncludesLeaversInPeriod -count=1 -v`
Expected: PASS keduanya.

Run: `cd backend && TEST_DATABASE_URL=... $HOME/.local/go/bin/go test ./internal/modules/payroll/... ./internal/app -count=1`
Expected: `ok` (`internal/app` sama dengan baseline; `TestKPISnapshotRoute` sudah diperbaiki di EPIC-057).

- [ ] **Step 5: Commit**

```bash
git add backend/internal/modules/payroll backend/internal/app/adapters_payroll.go backend/internal/app/payroll_leaver_test.go
git commit -m "feat(payroll): karyawan keluar di tengah periode ikut run dan dibayar prorata sampai end_date, EPIC-059"
```

---

### Task 7: Skrip ops pratinjau — struktur gaji ke `monthly_prorated` + tunjangan `per_day`

**Files:**
- Create: `backend/database/sql/fixes/2026-10-21_payroll_struktur_gaji_acuan.sql`

**Interfaces:**
- Consumes: tabel `hris.employee_salary`, `hris.employees`, `hris.attendance`.
- Produces: skrip ops dengan mode `-v preview=1` (read-only + ROLLBACK), pola yang sama dengan `backend/database/sql/fixes/2026-10-20_positions_level_dari_judul.sql`.

- [ ] **Step 1: Tulis skrip**

```sql
-- =============================================================================
-- EPIC-059: struktur gaji aktif karyawan bulanan ke acuan payslip Paddy:
-- pay_basis monthly → monthly_prorated; transport/makan/jabatan → per_day.
-- Upah harian (daily) tidak disentuh. Jalankan HANYA setelah user menyetujui
-- pratinjau dan simulasi payroll.
-- Pratinjau (transaksi read-only + ROLLBACK):
--   psql "$U" -v ON_ERROR_STOP=1 -v preview=1 -f backend/database/sql/fixes/2026-10-21_payroll_struktur_gaji_acuan.sql
-- Terapkan (setelah backup):
--   psql "$U" -v ON_ERROR_STOP=1 -f backend/database/sql/fixes/2026-10-21_payroll_struktur_gaji_acuan.sql
-- =============================================================================
BEGIN;
CREATE TEMP VIEW struktur_usulan AS
SELECT s.id, e.full_name, s.pay_basis, s.transport_mode, s.meal_mode, s.position_mode,
  (SELECT count(*) FROM hris.attendance a
    WHERE a.employee_id = e.id AND a.date >= (current_date - 30) AND a.status <> 'absent') AS absensi_30_hari
FROM hris.employee_salary s
JOIN hris.employees e ON e.id = s.employee_id AND e.is_active
WHERE s.is_active AND coalesce(s.pay_basis, 'monthly') IN ('monthly', 'monthly_prorated')
  AND (coalesce(s.pay_basis, 'monthly') <> 'monthly_prorated'
       OR coalesce(s.transport_mode, 'flat') <> 'per_day'
       OR coalesce(s.meal_mode, 'flat') <> 'per_day'
       OR coalesce(s.position_mode, 'flat') <> 'per_day');

\if :{?preview}
  SET TRANSACTION READ ONLY;
\endif

\echo 'Struktur yang akan diubah:'
SELECT count(*) AS struktur, count(*) FILTER (WHERE absensi_30_hari = 0) AS tanpa_absensi_30_hari FROM struktur_usulan;
\echo 'PERINGATAN — karyawan tanpa absensi 30 hari terakhir (gaji pokok prorated akan 0 tanpa absensi/cuti):'
SELECT full_name, pay_basis, absensi_30_hari FROM struktur_usulan WHERE absensi_30_hari = 0 ORDER BY full_name;

\if :{?preview}
\else
  UPDATE hris.employee_salary s SET pay_basis = 'monthly_prorated', transport_mode = 'per_day',
    meal_mode = 'per_day', position_mode = 'per_day', updated_at = now()
  FROM struktur_usulan u WHERE u.id = s.id;
\endif

\if :{?preview}
  ROLLBACK;
\else
  COMMIT;
\endif
```

Cek dulu kolom `hris.attendance` (`date`, `employee_id`, `status`) dan `hris.employee_salary.updated_at` di DB test (`\d`). Sesuaikan nama kolom bila berbeda.

- [ ] **Step 2: Uji di DB test**

Run: `psql 'postgres://postgres@127.0.0.1:55499/paddy_test?sslmode=disable' -v ON_ERROR_STOP=1 -v preview=1 -f backend/database/sql/fixes/2026-10-21_payroll_struktur_gaji_acuan.sql`
Expected: output `Struktur yang akan diubah` dan daftar peringatan, diakhiri `ROLLBACK`, tanpa error.

Lalu jalankan mode terapkan di dalam transaksi pembungkus yang di-rollback, supaya UPDATE teruji tanpa mengubah DB test secara permanen. Salin isi skrip tanpa baris `BEGIN;` dan blok `COMMIT;` ke file sementara di scratchpad, lalu bungkus dengan `BEGIN; ... ROLLBACK;`. Expected: `UPDATE n` tanpa error.

- [ ] **Step 3: Commit (tidak dijalankan di live)**

```bash
git add backend/database/sql/fixes/2026-10-21_payroll_struktur_gaji_acuan.sql
git commit -m "chore(payroll): skrip ops pratinjau struktur gaji ke acuan payslip (belum dijalankan), EPIC-059"
```

---

### Task 8: Verifikasi menyeluruh & dokumentasi

**Files:**
- Modify: `docs/epics/EPIC-059-payroll-acuan-mom.md` (status + Automation Log)
- Modify: `docs/hris/acuan-mom-20261019/status-requirement.md` (PY-01, PY-03, PY-04, ES-06)
- Modify: `docs/hris/acuan-mom-20261019/aturan-payroll.md` (kolom "Kode sekarang" baris terkait)

- [ ] **Step 1: Full suite Go & frontend**

Run: `cd backend && TEST_DATABASE_URL='postgres://postgres@127.0.0.1:55499/paddy_test?sslmode=disable' $HOME/.local/go/bin/go test ./... -count=1 > /tmp/epic059-go.txt 2>&1; grep -- '--- FAIL' /tmp/epic059-go.txt | sed 's/ (.*//' | sort -u`
Expected: daftar nama sama dengan baseline `main` sebelum branch ini. Jalankan suite yang sama di `main` untuk pembanding segar dan bandingkan dengan `diff`.

Run: `cd frontend && npx vitest run 2>&1 | tail -5 && npx tsc --noEmit -p .`
Expected: 0 gagal; tsc tanpa output.

- [ ] **Step 2: Simulasi angka di DB test**

Buat satu karyawan uji di DB test dengan struktur `monthly_prorated` + `per_day`: pokok 2.200.000, transport/makan 220.000, jabatan 440.000, jadwal Senin–Jumat. Isi absensi 20 hari, 1 Izin Pribadi, dan 1 Sakit. Jalankan kalkulasi run lewat API, dengan alur yang sama seperti test integrasi payroll yang ada (`grep -rn "calculate" backend/internal/modules/payroll/*_integration_test.go`). Hasil yang diharapkan: pokok = 2.200.000/22×22, transport & makan ×20/22, PIP = (2.200.000+440.000)/22, PIS = 0. Catat angkanya di Automation Log. Test ini boleh ditulis sebagai test integrasi permanen `TestPayrollRunAcuanPayslip` di paket payroll bila harness memungkinkan.

- [ ] **Step 3: Dokumentasi**

Perbarui `status-requirement.md`:
- PY-01 → ✅ (basis 4 komponen).
- PY-03 → ✅ pembagi terjadwal (K-10/K-12 diputuskan).
- PY-04 → ✅ (catatan: PPh21 masa pajak terakhir masih follow-up).
- ES-06 → ✅ (Cuti Melahirkan masuk potongan).

Perbarui baris terkait di `aturan-payroll.md` ("Kode sekarang" dan status). Di Automation Log EPIC-059 tambahkan:
- Commit Task 1–7.
- Ruling R1–R12.
- Bukti test.
- Catatan bahwa skrip Task 7 dan `cutoff_day = 20` **belum** dijalankan di live dan menunggu persetujuan user.
Set `status: review`.

- [ ] **Step 4: Commit**

```bash
git add docs/epics/EPIC-059-payroll-acuan-mom.md docs/hris/acuan-mom-20261019
git commit -m "docs: EPIC-059 payroll acuan selesai diimplementasikan, menunggu review"
```

## Follow-up (di luar rencana ini)

- PPh21 Pasal 17 tahunan pada masa pajak terakhir karyawan yang keluar di tengah tahun (sekarang hanya Desember).
- Baris slip terpisah untuk Insentif dan Denda (sekarang tergabung di "Penghasilan Lain" / "Potongan Lain").
- Payroll mingguan daily worker (PY-05): periode dan run baru.
- Menjalankan skrip Task 7 dan mengubah `cutoff_day = 20` di live: keputusan user setelah pratinjau dan simulasi.
