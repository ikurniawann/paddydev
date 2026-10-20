# Aturan Payroll Paddy — Acuan Payslip vs Kode

**Sumber acuan:** tabel "Payslip Line / Category / Formula / Notes" dari Paddy (dikirim 20-10-2026), ditambah MOM 19-10-2026 §2.6, §2.7, dan §3. Kode dicek di `backend/internal/modules/payroll/**` (Go melayani semua `/api/hris/*`; kalkulator TS `frontend/src/lib/payroll/calculator.ts` hanya fallback lama tanpa aturan EPIC-055).

**Aturan main:** bila acuan dan kode berbeda, **acuan yang menang**. Pengecualiannya poin yang di MOM masih "Konfirmasi (K-n)" atau "Menunggu TnC": di sana jangan menebak, tanyakan dulu.

## Istilah

- **Hari Kerja Terjadwal**: jumlah hari kerja menurut jadwal karyawan dalam periode 21 → 20 (skema 5 hari ≈ 23, 6 hari ≈ 24; angka pasti menunggu K-10/K-12).
- **Hari Kerja Aktif yang Dibayar**: hari hadir ditambah hari cuti yang *Full Paid* (Cuti Tahunan, Menikah, Duka, Izin Leader, dsb.).
- **Hari Kerja Efektif**: hari benar-benar hadir.
- **THP untuk "1 hari THP"** = 4 komponen: Gaji Pokok + Tunj. Makan + Tunj. Transport + Tunj. Jabatan (MOM §2.6.1 dan Daftar Istilah).

## Baris payslip

| Baris | Kategori | Formula acuan | Kode sekarang | Status |
|---|---|---|---|---|
| Gaji Pokok | Basic | Nominal bulanan / Total Hari Kerja Terjadwal × Hari Kerja Aktif yang Dibayar | `pay_basis = monthly_prorated`: base / pembagi × hari hadir (status ≠ absent). Default `monthly` = base × faktor kontrak. Pembagi = `working_days_divisor` struktur gaji (live: 20/25/26 campur), atau hari terjadwal bila kosong | 🟡 harus `monthly_prorated` + pembagi = hari terjadwal periode |
| Tunj. Transportasi | Allowance | Nominal / Total Hari Kerja Terjadwal × Hari Kerja **Efektif** | `transport_mode = per_day`: nominal / pembagi × AllowanceDays (= hari hadir). Default `flat` | 🟡 harus `per_day` |
| Tunj. Makan | Allowance | sama dengan transport | `meal_mode = per_day`, sama | 🟡 |
| Tunj. Jabatan | Allowance | Nominal / Total Hari Kerja Terjadwal × Hari Kerja **Aktif yang Dibayar** | `position_mode = per_day` memakai hari hadir (bukan hari aktif dibayar) | ⚠️ hari cuti dibayar ikut dihitung di acuan |
| Subsidi BPJS Kesehatan | Allowance | Rp100.000 jika masa kerja > 1 tahun pada tanggal cutoff; selain itu Rp0 | `health_allowance` per karyawan (nominal bebas), syarat `health_min_service_days` (default 0 = semua dapat), dikali faktor kontrak | 🟡 set nominal 100.000 + 365 hari, ukur pada tanggal cutoff |
| Lembur Hari Kerja Biasa | Allowance | 1,5 × Gaji Pokok Bulanan / 173 × Jam Lembur Dibayar; **min. 1 jam** | 1,5 × base / 173 × jam (setting `overtime_multiplier`, `overtime_hourly_divisor`); jam dibatasi `overtime_hours` absensi; butuh clock-out. **Tanpa minimum 1 jam** | 🟡 tambah minimum 1 jam |
| Lembur Libur Nasional / Istirahat Mingguan | Allowance | 2 × Gaji Pokok Bulanan / 173 × Jam; min. 1 jam ("formula awal") | 2 × base / 173 × jam (`overtime_multiplier_holiday`) untuk libur nasional, `day_type = rest_day`, atau hari tanpa shift bila `overtime_rest_from_schedule` | 🟡 minimum 1 jam; Weekday/Weekend menunggu K-13 |
| Insentif | Allowance | (nominal input) | Kode manual `INCENTIVE`, tetapi di slip digabung ke "Penghasilan Lain" | 🟡 butuh baris sendiri di slip |
| Other Allowance | Allowance | (nominal input) | `OTHER_EARNING` / `OTHER_EARNING_NONTAX` | ✅ |
| Total Pendapatan | Computation | Gaji Pokok + Tunjangan + Lembur + Subsidi BPJS Kes + Insentif | gross = base + fixed + variable + transport + meal + housing + jabatan + health + lembur + THR + bonus + earning lain | ✅ (lebih luas) |
| Potongan Izin Pribadi | Deduction | THP / **Total Hari Periode Berjalan** × Hari Izin Pribadi | PIP = **seluruh slip** (gross + nontax) / pembagi hari kerja × hari; setting `personal_leave_deduction` (live: on). Intern dikecualikan (PR #3) | ⚠️ basis harus 4 komponen; pembagi menunggu K-12 |
| Pinjaman Karyawan / Kasbon | Deduction | Sesuai jadwal cicilan & sisa pinjaman; **maks. ½ THP**; default maks. 2 cicilan 50%/50%; **Probation, Freelance, Intern tidak bisa mengajukan** | Cicilan rata mulai bulan setelah approve. Kasbon (EPIC-060): pokok ≤ 50% THP (Gaji Pokok + Transport + Makan + Jabatan), tenor 1 atau 2 (default 2, 50/50), Probation/Freelance/Intern/Magang diblokir. Jenis `loan` tetap batas lama (cicilan ≤ `loan_max_installment_percent` × gaji pokok, tenor 1–60) menunggu TnC | ✅ |
| Potongan Izin Sakit | Deduction | (Tunj. Transport + Tunj. Makan) / Total Hari Kerja Bulan Berjalan × Hari Sakit | PIS = (transport + makan) / pembagi × hari sakit; setting `sick_leave_deduction` (live: on). **Risiko potong ganda**: dengan mode `per_day`, hari sakit sudah tidak dibayar lewat tunjangan harian | ⚠️ cek interaksi PIS × per_day |
| Denda | Deduction | Nominal input HR manual; default Rp0 | Kode manual `PENALTY`, digabung ke "Potongan Lain" | 🟡 butuh baris sendiri |
| PPh Pasal 21 | Deduction | TER per masa pajak; Pasal 17 tahunan di masa pajak terakhir | `pph21_method = ter` (live: ter); Pasal 17 tahunan **hanya bulan Desember** (hard-code), tidak untuk karyawan yang keluar di tengah tahun | 🟡 masa pajak terakhir karyawan keluar |
| Other Deduction | Deduction | (nominal input) | `OTHER_DEDUCTION`, `ABSENCE`, `UNION` | ✅ |
| Total Potongan | Computation | Pinjaman + Izin Pribadi + Izin Sakit + Denda (+ PPh, Other) | BPJS karyawan + PPh + unpaid + telat + pinjaman + PIP + PIS + potongan manual | ✅ (lebih luas; BPJS & telat ikut) |
| Take Home Pay | Computation | Total Pendapatan − Total Potongan | net = gross + nontax − total | ✅ |

Catatan acuan: rumus "Total Potongan" di tabel Paddy tertulis "Pinjaman Karyawan − Potongan izin pribadi − potongan izin sakit − denda". Tanda minus di sana dibaca sebagai daftar item yang dijumlahkan, bukan pengurangan (kalau dibaca sebagai pengurangan, potongan malah menambah THP).

## Perbedaan yang paling berdampak

1. **"1 hari THP" memakai seluruh slip, bukan 4 komponen.** Ini berlaku untuk PIP dan potongan telat tier ber-basis `thp`. Karyawan yang lembur atau dapat bonus jadi terpotong lebih besar dari aturan.
2. ~~**Kasbon**: plafon 30% gaji pokok, tenor bebas 1–60, tanpa blokir status, Pinjaman Darurat masih bisa diajukan, approval satu tahap.~~ **Selesai di EPIC-060** (plafon 50% THP, tenor 1/2, status diblokir, Pinjaman Darurat dihapus, approval dua tahap); menunggu review & rilis.
3. **Cuti Melahirkan** belum memotong transport + makan (MOM §3.1, ES-06).
4. **Minimum lembur 1 jam** belum ada.
5. **Subsidi BPJS Kesehatan** masih nominal bebas, syarat masa kerja default 0 hari.
6. **Prorate karyawan keluar** (PY-04): karyawan nonaktif tidak ikut payroll run; `end_date` tidak dibaca.
7. **Payroll mingguan daily worker** (PY-05) belum ada.

## Setting & data yang harus benar di live (bukan perubahan kode)

| Setting | Live 20-10-2026 | Acuan |
|---|---|---|
| `hris.attendance_settings.cutoff_day` | 0 (bulan kalender) | 20 |
| `hris.payroll_settings.payroll_day` | 25 | konfirmasi tanggal bayar |
| `personal_leave_deduction` | on | on |
| `sick_leave_deduction` | on | on |
| `pph21_method` | ter | ter |
| `loan_max_installment_percent` | 30 | hanya berlaku untuk jenis `loan`; kasbon memakai plafon 50% THP |
| `overtime_multiplier` / `_holiday` / `_hourly_divisor` | 1,5 / 2 / 173 | 1,5 / 2 / 173 ✅ |
| Struktur gaji aktif `pay_basis` (karyawan aktif) | 240 `monthly`, 32 `monthly_prorated`, 18 `daily` | `monthly_prorated` untuk karyawan bulanan |
| Struktur gaji pembagi `working_days_divisor` | 26 (135), 25 (103), kosong (36), 20 (16) | hari kerja terjadwal periode (K-10/K-12) |
| `transport_mode` / `meal_mode` / `position_mode` | campuran flat & per_day | per_day |
