# Slip Izin Sakit & Izin Pribadi Sesuai Hitungan HR (EPIC-124) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Slip menampilkan Tunjangan Transport & Makan penuh, lalu memotong Izin Sakit = (transport + makan)/hari × hari sakit dan Izin Pribadi = 1 hari THP 4 komponen, persis seperti hitungan manual HR, tanpa mengubah PPh21 dan dengan THP yang sama (selisih pembulatan ≤ Rp2).

**Architecture:** Hari sakit dan hari Izin Pribadi yang memang dipotong dihitung ikut sebagai hari tunjangan untuk komponen `per_day`, lalu potongannya mengambil bagian per hari komponen itu secara penuh. Uang yang dulu "hilang lewat hari hadir" kini muncul sebagai baris potongan. Dasar PPh21 memakai bruto lama (bruto baru dikurangi pergeseran tunjangan), jadi pajak dan BPJS tidak berubah.

**Tech Stack:** Go (`apps/backend/internal/modules/payroll/domain` di repo `erp-paddy`, branch `development`). Tidak ada kalkulator di Next; slip web dan PDF membaca kolom yang sama lewat `slip_lines.go`.

**Spec:** [EPIC-124](../../epics/EPIC-124-slip-izin-sakit-pribadi-tunjangan-penuh.md) (dibuat di Task 3). Permintaan user 20-11-2026: "izin pribadi potongan dari THP tp kalo untuk sakit dipotong tunjangan saja tapi di sistem utk potongan izin pribadi dan sakitnya beda, dan tapi perhitungannya 78.261 itu dari mana ya? yg manual 130rb an" dan "sakit motong tunjangan transport dan makan aja". Kasus acuan: Shafa Triya Auliya, November 2026. Gaji pokok 1.800.000, makan 600.000, transport 600.000 (keduanya `per_day`), 23 hari terjadwal, hadir 20, sakit 2, Izin Pribadi 1. Hitungan HR: Izin Sakit 104.348, Izin Pribadi 130.435, THP 2.765.217. Slip sistem sekarang: transport dan makan masing-masing 521.739, Potongan Izin Pribadi 78.261 (= 1.800.000/23), THP 2.765.217.

## Global Constraints

- THP setiap karyawan boleh berubah hanya karena pembulatan: |net baru − net lama| ≤ 2 (kecuali K1).
- PPh21, `TaxableIncome`, DPP TER, dan semua BPJS harus sama persis dengan sebelum perubahan.
- Mode `flat` tidak berubah sama sekali (pergeseran = 0).
- Izin Sakit hanya memotong Tunjangan Transport + Tunjangan Makan.
- Izin Pribadi memotong 1 hari THP = Gaji Pokok + Jabatan + Transport + Makan (`ThpDayBasis`). Untuk `pay_basis = daily`, pokok tidak ikut (perilaku sekarang).
- Pergeseran hanya berlaku untuk hari yang benar-benar dipotong: hari sakit bila `sick_leave_deduction` on; hari Izin Pribadi bila `personal_leave_deduction` on (intern: lihat K1).
- Subsidi BPJS (`HealthAllowance`, R-0x "Subsidi BPJS") tidak dipotong Izin Pribadi maupun Izin Sakit dan tidak ikut hari tunjangan; nilainya tetap penuh (permintaan user 20-11-2026: "jika ada izin pribadi yang dipotong dari THP tp untuk subsidi BPJS tidak dipotong ya").
- Cuti Melahirkan tetap full paid (EPIC-084) dan tidak ikut pergeseran.
- Pembagi per hari tetap `DivisorDays` (hari terjadwal).
- Label slip tetap: R-02 "Tunjangan Transportasi", R-03 "Tunjangan Makan", R-20 "Potongan Izin Pribadi", R-21 "Potongan Izin Sakit".

## Keputusan

- **K1 (diputuskan user 20-11-2026: "Yes, pay in full" — intern tetap dibayar transport + makan hari Izin Pribadi): Intern yang Izin Pribadi.** Aturan acuan: intern tidak dipotong Izin Pribadi. Sekarang intern dengan transport/makan `per_day` tetap kehilangan transport + makan hari itu, karena hari itu tidak dihitung hadir. Rekomendasi: hari Izin Pribadi intern ikut dihitung sebagai hari tunjangan dan tidak dipotong, jadi intern menerima transport + makan hari itu. Ini satu-satunya perubahan uang di plan ini, dan hanya menyangkut intern `per_day` yang Izin Pribadi. Bila user memilih mempertahankan perilaku sekarang, hari Izin Pribadi intern tidak masuk pergeseran, `TestInternPersonalLeaveKeepsAllowances` mengharapkan transport 521.739, dan asersi net di `TestInternPersonalLeaveSkipsPIP` disesuaikan.

## Review Focus

- Karyawan kena PPh (TER dan annualized) yang sakit/Izin Pribadi dengan tunjangan `per_day`: pajak harus sama dengan sebelumnya, bukan naik karena bruto naik.
- Setting `sick_leave_deduction` atau `personal_leave_deduction` off: hari itu tidak boleh menjadi hari tunjangan berbayar (THP tidak boleh naik).
- Struktur campuran (transport `per_day`, makan `flat`): potongan mengambil bagian per hari kedua komponen penuh, dan tunjangan `flat` tidak dihitung ganda.
- `pay_basis = daily` dengan Izin Pribadi: dasar PIP tetap tanpa pokok, transport/makan ikut pergeseran.
- Rincian per toko (`allocation.go`) dan total run: jumlah per kolom antar toko tetap sama dengan slip.

---

## Peta file

| File (repo `erp-paddy`) | Perubahan |
|---|---|
| `apps/backend/internal/modules/payroll/domain/calculator.go` | pergeseran hari tunjangan, dasar PIP/PIS penuh, bruto pajak |
| `apps/backend/internal/modules/payroll/domain/leave_slip_split_test.go` | test baru (kasus acuan + invarian) |
| `domain/thp_day_test.go`, `leave_deduction_rules_test.go`, `parity_test.go`, `intern_test.go` | asersi yang mengunci pembagian lama diperbarui |
| `domain/testdata/calc_golden.json` | ditulis ulang, dicek dengan skrip invarian |

| File (repo paddydev, branch `docs/rencana-rilis-prod-erp-paddy`) | Perubahan |
|---|---|
| `docs/epics/EPIC-124-slip-izin-sakit-pribadi-tunjangan-penuh.md`, `docs/epics/README.md` | epic baru + baris registry |
| `docs/hris/acuan-mom-20261019/aturan-payroll.md` baris 19, 20, 28, 30 | rumus baru |

---

### Task 1: Kalkulator memindahkan hari sakit & Izin Pribadi ke baris potongan

**Files:**
- Modify: `apps/backend/internal/modules/payroll/domain/calculator.go` (blok G2 tunjangan ±L392-402; blok G6 PIP/PIS ±L464-487; blok PPh ±L493-523)
- Create: `apps/backend/internal/modules/payroll/domain/leave_slip_split_test.go`
- Modify tests: `thp_day_test.go`, `leave_deduction_rules_test.go`, `parity_test.go`, `intern_test.go`

**Interfaces:**
- Consumes: `Input`, `Config`, `Calculate`, `ThpDayBasis`, `IsInternStatus`, helper test `parityInput`, `eq` (sudah ada di paket).
- Produces: tidak ada fungsi publik baru. `Result.TransportAllowance`, `MealAllowance`, `GrossSalary`, `PersonalLeaveDeduction`, `SickLeaveDeduction`, `TotalDeductions` berubah nilai; `NetSalary` ±2; `Pph21Deduction`, `TaxableIncome`, BPJS tetap.

- [ ] **Step 1: Tulis test yang gagal** di `leave_slip_split_test.go`:

```go
// shafa: kasus acuan EPIC-124 (slip November 2026).
func shafa(mod func(*Input)) Input {
	return parityInput(func(in *Input) {
		in.EmploymentStatus, in.PayBasis = "contract", "monthly_prorated"
		in.BaseSalary, in.TransportAllowance, in.MealAllowance, in.PositionAllowance = 1_800_000, 600_000, 600_000, 0
		in.TransportPerDay, in.MealPerDay = true, true
		in.DivisorDays, in.WorkingDays = 23, 23
		in.AttendanceDays, in.AllowanceDays, in.PaidDays = 20, 20, 23
		in.SickLeaveDays, in.PersonalLeaveDays = 2, 1
		in.BpjsTkEnrolled, in.BpjsKesEnrolled, in.TaperaEnrolled, in.IsTaxable = false, false, false, false
		if mod != nil {
			mod(in)
		}
	})
}

func leaveOn() Config {
	c := DefaultConfig()
	c.PersonalLeaveDeduction, c.SickLeaveDeduction = true, true
	return c
}

func TestSlipMatchesHrManualShafa(t *testing.T) {
	r := Calculate(shafa(nil), leaveOn())
	eq(t, "transport penuh", r.TransportAllowance, 600_000)
	eq(t, "makan penuh", r.MealAllowance, 600_000)
	eq(t, "Izin Sakit = (600k+600k)/23 × 2", r.SickLeaveDeduction, 104_348)
	eq(t, "Izin Pribadi = 3.000.000/23", r.PersonalLeaveDeduction, 130_435)
	eq(t, "bruto", r.GrossSalary, 3_000_000)
	eq(t, "THP", r.NetSalary, 2_765_217)
}

// Pajak sama dengan karyawan yang sama tanpa izin dan hanya dibayar 20 hari tunjangan
// (= bruto lama), untuk kedua metode PPh.
func TestLeaveShiftKeepsTax(t *testing.T) {
	for _, method := range []string{"ter", "annualized"} {
		c := leaveOn()
		c.Pph21Method = method
		taxed := func(in *Input) {
			in.IsTaxable, in.BpjsTkEnrolled, in.BpjsKesEnrolled = true, true, true
			in.BaseSalary = 9_000_000
		}
		got := Calculate(shafa(taxed), c)
		ref := Calculate(shafa(func(in *Input) {
			taxed(in)
			in.SickLeaveDays, in.PersonalLeaveDays = 0, 0
		}), c)
		eq(t, method+": PPh", got.Pph21Deduction, ref.Pph21Deduction)
		eq(t, method+": taxable", got.TaxableIncome, ref.TaxableIncome)
	}
}

func TestLeaveShiftOnlyWhenDeductionOn(t *testing.T) {
	r := Calculate(shafa(nil), DefaultConfig()) // kedua potongan off
	eq(t, "transport tetap 20/23", r.TransportAllowance, 521_739)
	eq(t, "tanpa PIS", r.SickLeaveDeduction, 0)
	eq(t, "tanpa PIP", r.PersonalLeaveDeduction, 0)
}

func TestLeaveShiftMixedModes(t *testing.T) {
	r := Calculate(shafa(func(in *Input) { in.MealPerDay = false }), leaveOn())
	eq(t, "transport per_day penuh", r.TransportAllowance, 600_000)
	eq(t, "makan flat penuh", r.MealAllowance, 600_000)
	eq(t, "PIS tetap (600k+600k)/23 × 2", r.SickLeaveDeduction, 104_348)
}

// Subsidi BPJS tidak ikut dipotong Izin Pribadi / Izin Sakit.
func TestLeaveKeepsHealthSubsidy(t *testing.T) {
	r := Calculate(shafa(func(in *Input) { in.HealthAllowance, in.HealthEligible = 100_000, true }), leaveOn())
	eq(t, "subsidi BPJS penuh", r.HealthAllowance, 100_000)
	eq(t, "Izin Pribadi tanpa subsidi", r.PersonalLeaveDeduction, 130_435)
	eq(t, "Izin Sakit tanpa subsidi", r.SickLeaveDeduction, 104_348)
}

// K1: intern tidak dipotong Izin Pribadi dan tetap dapat transport + makan hari itu.
func TestInternPersonalLeaveKeepsAllowances(t *testing.T) {
	r := Calculate(shafa(func(in *Input) { in.EmploymentStatus, in.SickLeaveDays = "intern", 0 }), leaveOn())
	eq(t, "intern PIP", r.PersonalLeaveDeduction, 0)
	eq(t, "transport 21/23", r.TransportAllowance, 547_826)
}
```

Tambahkan juga `TestLeaveShiftDailyPayBasis`: `PayBasis = "daily"`, `BaseSalary = 80_000`, `AttendanceDays = 20`, `PersonalLeaveDays = 1`, sakit 0. Asersinya: `TransportAllowance == 547_826` dan `PersonalLeaveDeduction == 52_174`, yaitu `Round(1_200_000.0/23)` (PIP daily = transport + makan per hari, tanpa pokok).

- [ ] **Step 2: Jalankan dan pastikan gagal**

Run: `cd apps/backend && go test ./internal/modules/payroll/domain/ -run 'TestSlipMatchesHrManualShafa|TestLeaveShift|TestLeaveKeepsHealthSubsidy|TestInternPersonalLeaveKeepsAllowances' -count=1`
Expected: FAIL. `transport penuh` mendapat 521739, dan `Izin Pribadi` mendapat 78261. `subsidi BPJS penuh` sudah lulus sekarang: test ini menjaga perilaku yang sudah benar.

- [ ] **Step 3: Ubah `Calculate` di `calculator.go`**

Keputusan yang mengikat:
1. Hitung `shiftDays` sebelum blok tunjangan:
   - hari sakit bila `c.SickLeaveDeduction`;
   - hari Izin Pribadi bila `c.PersonalLeaveDeduction`, untuk siapa pun termasuk intern (K1).
2. Tunjangan `per_day` memakai `perDay(amount, in.AllowanceDays + shiftDays)`.
3. Hitung juga nilai lama, `perDay(amount, in.AllowanceDays)` untuk transport dan makan. Setelah itu `allowanceShift := (transport + meal) − (transportLama + mealLama)`.
4. PIP:
   - `pipBasis = in.PositionAllowance + in.TransportAllowance + in.MealAllowance`, ditambah `fullBase` bila bukan `daily`;
   - hapus `flatTransport`/`flatMeal`;
   - intern tetap 0.
5. PIS: `Round(perDay(in.TransportAllowance + in.MealAllowance, in.SickLeaveDays))`, hanya transport + makan.
6. PPh: di kedua cabang (`ter` dan annualized) pakai `taxGross := gross − allowanceShift` di tempat `gross`. `gross` di `Result` tetap bruto baru.
7. Perbarui komentar blok G6 supaya menjelaskan pergeseran ini (sekarang komentarnya bilang "komponen per_day sudah tidak dibayar").

- [ ] **Step 4: Jalankan test baru sampai lulus**

Run: perintah Step 2. Expected: PASS.

- [ ] **Step 5: Perbarui test lama yang mengunci pembagian lama**

Hanya asersi pembagian yang diubah; asersi THP harus tetap lulus tanpa diubah.
- `thp_day_test.go`, `TestPersonalLeaveNoDoubleDeduction`: PIP per_day `120_000` → `140_000`. Asersi "total hilang = 1 hari 4 komponen" (140.000) tetap.
- `thp_day_test.go`, `TestSickAndMaternityDeductFlatAllowancesOnly`: kasus `per_day` sakit 1 hari → `SickLeaveDeduction == 20_000`. Ganti nama test menjadi `TestSickDeductsFullDayAllowances`. Cuti Melahirkan tetap 0.
- `leave_deduction_rules_test.go`, `parity_test.go`, `intern_test.go`: jalankan paket. Setiap test yang gagal hanya boleh diubah bila yang berubah adalah transport/makan/bruto/PIP/PIS/total potongan, sementara THP selisih ≤ 2 dan PPh sama. Catat nama test dan nilai lama → baru di laporan.

Run: `cd apps/backend && go test ./internal/modules/payroll/... -count=1`
Expected: semua lulus, kecuali `TestCalculateGoldenUnchanged` (Task 2).

- [ ] **Step 6: Commit**

```bash
git add apps/backend/internal/modules/payroll/domain
git commit -m "fix(payroll): slip Izin Sakit & Izin Pribadi memotong 1 hari penuh, tunjangan tampil penuh (EPIC-124)"
```

### Task 2: Golden kalkulator dan bukti invarian

**Files:**
- Modify: `apps/backend/internal/modules/payroll/domain/testdata/calc_golden.json`

**Interfaces:**
- Consumes: Task 1.
- Produces: golden baru plus bukti di laporan bahwa hanya kolom yang diizinkan yang berubah.

- [ ] **Step 1: Tulis ulang golden**

Run: `cd apps/backend && go test ./internal/modules/payroll/domain/ -run TestCalculateGoldenUnchanged -update -count=1`

- [ ] **Step 2: Bandingkan dengan golden lama**

Tulis skrip Python sekali pakai di scratchpad, bukan di repo. Skrip membaca `git show HEAD:apps/backend/internal/modules/payroll/domain/testdata/calc_golden.json` dan file baru, lalu mencetak untuk tiap kasus:
- kunci yang berubah;
- |Δ net_salary| (atau nama field THP di golden);
- apakah PPh, taxable, dan BPJS sama.

Expected:
- kunci yang berubah hanya transport, makan, bruto, Izin Pribadi, Izin Sakit, total potongan, dan net;
- |Δnet| ≤ 2;
- PPh, taxable, dan BPJS identik.

Kasus dengan Izin Pribadi intern `per_day` boleh mendapat Δnet sebesar transport + makan sehari (K1). Sebutkan kasus itu di laporan.

- [ ] **Step 3: Seluruh paket payroll + slip**

Run: `cd apps/backend && go test ./internal/modules/payroll/... -count=1 && gofmt -l internal/modules/payroll`
Expected: lulus. `gofmt` tanpa output. Test integrasi yang butuh DB mengikuti baseline `development`.

- [ ] **Step 4: Commit**

```bash
git add apps/backend/internal/modules/payroll/domain/testdata/calc_golden.json
git commit -m "test(payroll): golden kalkulator EPIC-124 (net tetap, PPh tetap)"
```

### Task 3: Dokumen acuan dan epic

**Files (repo paddydev, branch `docs/rencana-rilis-prod-erp-paddy`):**
- Create: `docs/epics/EPIC-124-slip-izin-sakit-pribadi-tunjangan-penuh.md` (format epic seperti EPIC-122: status, Goal, Tasks, Acceptance Criteria, Automation Log)
- Modify: `docs/epics/README.md` (baris EPIC-124, `MODULE-PAYROLL`)
- Modify: `docs/hris/acuan-mom-20261019/aturan-payroll.md` baris 19, 20, 28, 30

- [ ] **Step 1: Ubah `aturan-payroll.md`**
  - Baris 19–20: transport/makan `per_day` dibayar untuk hari hadir + hari sakit + hari Izin Pribadi yang dipotong.
  - Baris 28: PIP = Gaji Pokok + Jabatan + Transport + Makan per hari, untuk semua mode.
  - Baris 30: PIS = (Transport + Makan) per hari untuk semua mode. Cuti Melahirkan full paid. Label "Potongan Izin Sakit".
  - Kolom status: "EPIC-124, menunggu review & rilis". Catat bahwa dasar PPh21 tidak berubah.
- [ ] **Step 2: Epic.** Isi Automation Log dengan:
  - commit Task 1–2;
  - ringkasan invarian golden;
  - keputusan K1;
  - langkah rilis:
    1. merge PR ke `development`;
    2. gladi di salinan prod;
    3. setelah rilis, HR menghitung ulang run payroll November yang masih draft supaya slip menampilkan pembagian baru (run final tidak diubah);
    4. QA HR membandingkan slip Shafa Triya Auliya dengan sheet manual.
- [ ] **Step 3: Commit**

```bash
git add docs/epics docs/hris/acuan-mom-20261019/aturan-payroll.md
git commit -m "docs: EPIC-124 slip Izin Sakit & Izin Pribadi sesuai hitungan HR"
```

## Self-review

- **Cakupan:** baris tunjangan penuh → Task 1 (`TestSlipMatchesHrManualShafa`); Izin Pribadi 1 hari THP penuh → Task 1; Izin Sakit = transport + makan per hari → Task 1; THP sama → Task 1 (asersi THP) dan Task 2 (golden); pertanyaan "78.261 dari mana" → dijawab di Spec; dokumen → Task 3.
- **Review Focus:** pajak → `TestLeaveShiftKeepsTax`; setting off → `TestLeaveShiftOnlyWhenDeductionOn`; mode campuran → `TestLeaveShiftMixedModes`; `daily` → `TestLeaveShiftDailyPayBasis`; rincian per toko → Task 2 Step 3 (paket payroll termasuk `allocation_test.go` dan `slip_split_test.go`).
- **Konsistensi nama:** `shiftDays`, `allowanceShift`, `taxGross` hanya dipakai di Task 1. Test helper `shafa`/`leaveOn` didefinisikan di file test baru.
