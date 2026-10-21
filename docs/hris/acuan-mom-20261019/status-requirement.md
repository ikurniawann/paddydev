# Status Requirement HRIS — MOM 19-10-2026 (ES / AP / PY / KP / AB / LA)

Dicek terhadap kode `main` (`4b7056e` + EPIC-058 `97e4c8f`) dan data live (hanya baca) pada 20-10-2026. Diperbarui 20-10-2026 untuk EPIC-059 (PY-01, PY-03, PY-04, ES-06: terkode di branch `feat/epic059-payroll-acuan`, menunggu review & rilis; setting dan data live belum diubah). Diperbarui 20-10-2026 untuk EPIC-062 (AB-01, AB-02, AB-03: terkode di branch `docs/epic062-geofence-absensi`, menunggu review & rilis; koordinat toko live belum diisi, menunggu K-14). Diperbarui 21-10-2026: AP-01..AP-04 direncanakan di EPIC-063 (`feat/epic063-approval-atasan`, belum dikode). Status:

| Tanda | Arti |
|---|---|
| ✅ | Sudah sesuai requirement |
| 🟡 | Sebagian: sebagian aturan ada, atau ada tapi harus dinyalakan lewat setting |
| ⚠️ | Ada, tetapi aturannya **berbeda** dengan requirement |
| ❌ | Belum ada |
| ⏳ | Menunggu konfirmasi (K-n) atau TnC dari Paddy sebelum bisa dikerjakan tuntas |

Kolom "Status MOM" menyalin status di dokumen MOM (Disepakati / Konfirmasi / Menunggu TnC). Satu requirement bisa ✅ di kode tapi tetap ⏳ di MOM.

## ESS – Cuti/Izin

| ID | Requirement | Status MOM | Kode | Bukti / perilaku sekarang | Langkah berikutnya |
|---|---|---|---|---|---|
| ES-01 | Nonaktifkan Cuti Haid, Izin Darurat, Izin Tanpa Gaji | Konfirmasi K-1 | ✅ (EPIC-058, merged, **belum deploy**) | Katalog `hris/domain/leave_types.go`: `Selectable:false`, pengajuan baru ditolak 400; riwayat tetap terbaca | Deploy EPIC-058 |
| ES-02 | Tambah Izin Leader & Izin Pribadi | Konfirmasi K-2 | ✅ (EPIC-058, belum deploy) | `leader` 1/bulan (level Supervisor ke atas), `personal` 3/bulan, satu hari per pengajuan; level jabatan live sudah diisi (41 jabatan) | Deploy; jawab Q-APV-1 (posisi "Lead") |
| ES-03 | Kuota per jenis (bulan/tahun/kejadian), masa kerja (Cuti Tahunan setelah 12 bulan), syarat jabatan | Disepakati | 🟡 | Kuota bulanan Izin Pribadi/Leader dan syarat jabatan Izin Leader ada, tetapi **di-hard-code** di katalog Go+TS. Belum ada: kuota per tahun/per kejadian, cek masa kerja 12 bulan (Cuti Tahunan sekarang prorata tahun masuk, karyawan baru tetap bisa), maks. 3 hari/bulan Cuti Tahunan | Tabel konfigurasi kuota + cek masa kerja (lihat aturan-cuti di bawah) |
| ES-04 | Kuota Izin Leader hangus bila tidak dipakai bulan itu | Disepakati | ✅ (EPIC-058) | Kuota dihitung per bulan kalender dari `hris.leaves`, tanpa carry-over | — |
| ES-05 | Lampiran surat wajib untuk Sakit > batas hari | Konfirmasi K-3 (≥ 2 hari) | 🟡 | Upload lampiran ada (`/api/hris/leaves/attachment`, gambar ≤ 5 MB) tapi **selalu opsional** | Wajibkan lampiran untuk `sick` ≥ 2 hari (setelah K-3 dikonfirmasi) |
| ES-06 | Potongan ke payroll: Sakit & Cuti Melahirkan potong transport+makan; Izin Pribadi potong 1 hari THP | Konfirmasi K-12 | ✅ (EPIC-059, menunggu review & rilis) | Sakit dan Cuti Melahirkan → PIS (transport+makan; saklar `sick_leave_deduction`, label slip "Potongan Sakit & Cuti Melahirkan"). Izin Pribadi → PIP = 1 hari dari 4 komponen (Gaji Pokok, Transport, Makan, Jabatan) tanpa potongan ganda untuk komponen `per_day`; pembagi = hari kerja terjadwal periode (K-12 diputuskan); intern tidak dipotong. Teruji: `TestPayrollRunAcuanPayslip` | Berlaku penuh di live setelah struktur gaji dipindah ke `monthly_prorated` + `per_day` (skrip ops, menunggu persetujuan user) |
| ES-07 | Antar jenis cuti dapat dikombinasikan | Disepakati | ✅ (EPIC-058) | Jenis berbeda boleh berdampingan; satu tanggal = satu pengajuan aktif | — |
| ES-08 | Internship mengikuti skema bulanan | Disepakati | ✅ (PR #3, **belum merge**) | Intern: kuota Izin Pribadi 3/bulan, tanpa potongan THP (`IsInternStatus`: `intern`/`internship`) | Merge PR #3 sebelum deploy EPIC-058 |
| ES-09 | Aturan Cuti Ibadah | Menunggu TnC | ⏳ | `pilgrimage` ada, tanpa batas hari/sekali seumur kerja, tidak memotong gaji | Tunggu TnC |

**Batas per jenis dari MOM §3.1 yang belum ditegakkan kode:** Cuti Tahunan maks. 3 hari/bulan dan baru setelah 12 bulan (masa kerja < 1 tahun → Izin Pribadi); Sakit maks. 3 hari/bulan; Cuti Melahirkan 3 bulan (1,5 + 1,5); Cuti Ayah 2 hari (per tahun, K-4); Menikah 3 hari; Duka 2 hari per kejadian (K-5).

## Approval

| ID | Requirement | Status MOM | Kode | Bukti / perilaku sekarang | Langkah berikutnya |
|---|---|---|---|---|---|
| AP-01 | Time-off disetujui atasan langsung (SPV ke atasannya) | Disepakati | 🟡 | Satu tahap: HR **atau** `employees.reporting_to` boleh memutus (`hris/leaves.go` DecideLeave). Atasan dapat WA, tetapi **tidak ada antrean approval di ESS** (`ListLeaves` non-HR hanya data sendiri; inbox desktop memakai menu, bukan `reporting_to`). Live: 55 karyawan aktif tanpa atasan; Retail Operation Manager tercatat atasan dirinya sendiri | **EPIC-063** (rencana `docs/superpowers/plans/2026-10-21-approval-atasan.md`): ESS "Persetujuan Tim" (cuti, lembur, Perjalanan Dinas, tautan kasbon); atasan langsung approver utama, HR cadangan/override tercatat (`decided_as`, audit); tolak self-approval termasuk `reporting_to` = diri sendiri. Perbaikan data `reporting_to` live tetap keputusan user |
| AP-02 | Lembur wajib diajukan sebelum dilaksanakan; approval atasan langsung | Disepakati | 🟡 | Alur pengajuan lembur ada (pending → approved/rejected, HR atau atasan). **Tanggal lampau tetap diterima**; API `GET /api/hris/overtime?scope=approvals` ada tapi tanpa UI | **EPIC-063**: pengajuan lembur milik sendiri ditolak bila tanggal + jam mulai (WIB) sudah lewat (penugasan HR untuk karyawan lain boleh lampau); antrean atasan di ESS Persetujuan Tim; WA ke atasan saat pengajuan |
| AP-03 | Jenis lembur Weekday / Weekend | Konfirmasi K-13 | 🟡 | `day_type` hanya `regular`/`rest_day`, diisi HR; form ESS tanpa pilihan jenis; payroll menyatukan libur nasional + rest day (tarif 2×) | **EPIC-063** (K-13 diputuskan 20-10): ESS memilih Weekday (`regular`) / Weekend-Libur (`rest_day`), disarankan & dikunci dari jadwal + hari libur; tarif tetap dari setting, kalkulator payroll tidak berubah |
| AP-04 | Koreksi lembur oleh AdminHRIS | Disepakati | 🟡 | Tidak ada endpoint ubah pengajuan lembur; HR hanya bisa mengoreksi `overtime_hours` di absensi (tercatat di audit) | **EPIC-063**: `PATCH /api/hris/overtime/{id}` + dialog Koreksi di HRIS → Lembur, dijaga aksi `update` menu `hris.kepegawaian.overtime`; audit `overtime.correct` (sebelum/sesudah, alasan); periode closing ditolak |
| AP-05 | Kasbon disetujui Manager Departemen **dan** HR (akses Payroll) | Disepakati | ✅ | Dua tahap untuk kasbon (EPIC-060): Manager Departemen (atasan berlevel Manager ke atas pertama di rantai `reporting_to`, disimpan di `manager_approver_id`; dilewati bila tidak ada) lalu HR dengan aksi `approve` di menu Pinjaman **dan** Penggajian. Antrean persetujuan manager ada di ESS. Jenis `loan` tetap satu tahap | Menunggu review & rilis (migrasi `20261021100000`) |
| AP-06 | Approval CEO dan GM | Konfirmasi K-7 | ❌ ⏳ | Tidak ada rantai multi-tahap/eskalasi di HRIS. Tabel generik `configuration.user_approval_permissions` hanya dipakai accounting/procurement | Tunggu K-7 (untuk pengajuan apa) |

## Payroll & Periode Kerja

| ID | Requirement | Status MOM | Kode | Bukti / perilaku sekarang | Langkah berikutnya |
|---|---|---|---|---|---|
| PY-01 | 4 komponen gaji: Gaji Pokok, Tunj. Makan, Tunj. Transport, Tunj. Jabatan | Disepakati | ✅ (EPIC-059, menunggu review & rilis) | Keempat komponen ada di struktur gaji (plus fixed/variable/housing/health). "1 hari THP" = 4 komponen untuk PIP dan potongan telat tier basis THP (upah harian: 1 hari upah + tunjangan); lembur, bonus, insentif, THR tidak ikut. Gaji Pokok/Jabatan mengikuti hari kerja aktif dibayar, Transport/Makan mengikuti hari hadir | — |
| PY-02 | Cut off tanggal 20 (periode 21 → 20) | Disepakati | 🟡 | Mekanisme ada (`hris.attendance_settings.cutoff_day`), tetapi **live = 0 (bulan kalender)**. `payroll_settings.payroll_day` live = 25. Cicilan pinjaman masih per bulan kalender | Set `cutoff_day = 20` (keputusan data, bukan kode); cek ulang cicilan & THR terhadap periode |
| PY-03 | Skema 5 hari & 6 hari kerja per periode | Konfirmasi K-10 | ✅ (EPIC-059, menunggu review & rilis) | Pembagi gaji bulanan = hari kerja terjadwal karyawan dalam periode dari jadwal shift (K-10/K-12 diputuskan: skema 5/6 hari mengikuti jadwal, bukan field baru). Tanpa jadwal: pembagi struktur gaji, lalu hari kerja hasil hitung. Upah harian tidak berubah | Pembagi struktur live (20/25/26 campur) hanya jadi cadangan; `cutoff_day = 20` (PY-02) tetap keputusan user |
| PY-04 | Prorate untuk karyawan keluar di tengah periode | Konfirmasi K-12 | ✅ (EPIC-059, menunggu review & rilis) | Payroll run mengambil karyawan aktif dan nonaktif yang `end_date`-nya jatuh di dalam periode; tanggal setelah `end_date` di luar cakupan, dibayar prorata sampai `end_date` (mekanisme sama dengan kontrak parsial). **Catatan:** PPh21 Pasal 17 pada masa pajak terakhir karyawan yang keluar di tengah tahun masih follow-up (sekarang hanya Desember) | Follow-up PPh21 masa pajak terakhir |
| PY-05 | Payroll mingguan untuk daily worker | Disepakati | ❌ | Run hanya bulanan; `pay_basis = daily` ada tanpa periode mingguan | Fitur run mingguan (live: 5 daily worker aktif) |

## Kasbon & Pinjaman

| ID | Requirement | Status MOM | Kode | Bukti / perilaku sekarang | Langkah berikutnya |
|---|---|---|---|---|---|
| KP-01 | Hapus fitur Pinjaman Darurat | Disepakati | ✅ | Pengajuan baru `emergency` ditolak 400 di API dan tidak ada di form ESS/HR; riwayat tetap terbaca dengan label lama (EPIC-060). Live: 1 pengajuan darurat **pending** | HR memutus manual 1 pengajuan pending sebelum rilis (Q-KP-1) |
| KP-02 | Kasbon maks. 50% THP, tenor 1 atau 2 | Konfirmasi K-6 | ✅ | Kasbon: pokok ≤ 50% THP (Gaji Pokok + Transport + Makan + Jabatan dari struktur gaji aktif), tenor 1 atau 2 (default 2, 50/50), status Probation/Freelance/Intern/Magang diblokir, maksimal kasbon aktif ditegakkan saat pengajuan & approve (EPIC-060). Jenis `loan` tetap aturan lama (menunggu TnC, KP-03). Live: 1 kasbon lama tenor 3 tidak diubah | Menunggu review & rilis |
| KP-03 | Aturan Pinjaman | Menunggu TnC | ⏳ | Jenis sekarang: `kasbon`, `loan` (`emergency` hanya riwayat); bunga flat (hanya HR) | Tunggu TnC |

## Absensi & Timesheet

| ID | Requirement | Status MOM | Kode | Bukti / perilaku sekarang | Langkah berikutnya |
|---|---|---|---|---|---|
| AB-01 | Geofence radius 100 m; status Valid / Invalid | Konfirmasi K-14 | ✅ (kode; koordinat live menunggu K-14) | Clock-in/out karyawan sendiri dinilai di server: `domain.Geofence` (`backend/internal/modules/hris/domain/geofence.go:81`, radius 100 m, override toko 25–1000 m, toleransi akurasi `min(akurasi, 100 m)`) dipanggil lewat `geofenceFor` (`attendance_geofence.go:51`) dari `attendance_clock.go:128` (masuk) dan `:226` (pulang). Kolom `clock_in/out_geofence` + `*_distance_m` (migrasi `20261023090000_attendance_geofence.sql`). Koordinat kosong, tanpa GPS, atau 0,0 → "Belum dicek", tidak pernah Invalid. HR melihat badge Masuk/Pulang + jarak di Monitoring & Rekap; HR mengisi koordinat/radius lewat dialog Lokasi Toko (`PUT /api/hris/branches/{id}/location`, `placements.go:541`, audit `branch_location.update`) dan form profil cabang, keduanya dengan peta OpenLayers/OSM. Test: `TestDistanceMeters`, `TestGeofence`, `TestGeofenceBoundaries`, `TestValidateBranchLocation`, `TestClockInOutsideAreaNeedsNote`, `TestClockOutOutsideAreaNeedsNote`, `TestClockGeofenceUnchecked`, `TestSelfClockIgnoresBodyDate`, `TestGeofenceInRosterAndList`, `TestBranchLocation`; vitest `geofence.test.ts`, `branch-location-dialog.test.tsx`, `location-picker-field.test.tsx` | Rilis (EPIC-062, status `review`); HR mengisi koordinat toko aktif setelah rilis (K-14, data dari Paddy). Selama kosong, absen tercatat "Belum dicek" |
| AB-02 | Perjalanan Dinas (luar area) | Disepakati | ✅ | Pengajuan Perjalanan Dinas: tabel `hris.business_trips` (migrasi `20261023090100_business_trips.sql`), `GET/POST /api/hris/business-trips` dan `POST /api/hris/business-trips/{id}/decide` (`routes.go:88-90`, `business_trips.go`). Rentang ≤ 31 hari, mulai ≥ hari ini (`domain.ValidateBusinessTripDates`, `business_trip.go:14`), diputus atasan langsung atau HRD, bukan pemohon sendiri (`CanDecideBusinessTrip`, `business_trip.go:48`), audit tiap keputusan. Tanggal dinas yang disetujui membuat absen berstatus "Perjalanan Dinas" tanpa popup; tanggal selalu dari server (`TestBusinessTripDateIsServerSide`). UI: ESS → Perjalanan Dinas (pengajuan + antrean atasan) dan tab HRD di Absensi. Test: `TestBusinessTripFlow`, `TestBusinessTripRejectCancelAndClosedPeriod`, `TestValidateBusinessTripDates`, `TestCanDecideBusinessTrip`; vitest `business-trips-tab`, `perjalanan-dinas-page`, `trip-request-dialog`, `business-trips.api` | Rilis. Batal setelah disetujui tidak mengembalikan absen yang sudah berstatus `dinas` (HR meninjau manual). Persetujuan masuk antrean gabungan menunggu Paket Approval atasan |
| AB-03 | Popup saat absen di luar area + kolom keterangan | Disepakati | ✅ | Di luar area tanpa keterangan ≥ 5 karakter → 422 `geofence_note_required` (`domain.GeofenceNoteMissing`, `geofence.go:107`; tidak ada baris absensi/selfie tersimpan). ESS menampilkan popup (jarak, toko, radius toko dari server) lalu mengirim ulang payload yang sama + keterangan (`geofence-note-dialog.tsx`, `ess-clock-panel.tsx`); absen tercatat Invalid dengan keterangan di `notes`. Test: `TestClockInOutsideAreaNeedsNote`, `TestClockOutOutsideAreaNeedsNote`, `TestGeofenceNoteMissing`; vitest `ess-clock-panel.test.tsx` (422 → popup → kirim ulang), `geofence-note-dialog.test.tsx`, `ess/api.test.ts` | Rilis |
| AB-04 | Absen manual & koreksi absen dengan approval HR | Disepakati | ⚠️ | HR mengedit langsung (alasan wajib, audit, terkunci setelah periode ditutup). **Tidak ada pengajuan dari karyawan** yang lalu disetujui HR | Pengajuan koreksi/absen manual oleh karyawan → antrean HR |
| AB-05 | Keterangan "kurang jam / pulang lebih cepat" bila < 9 jam (HQ saja) | Disepakati | ⚠️ | Trigger DB mengirim notifikasi HR "Jam Kerja Kurang" bila **< 7 jam, semua cabang**; tidak ada catatan di baris absensi; tidak ada penanda cabang HQ | Ambang 9 jam khusus HQ + label di baris absensi |
| AB-06 | Alert absen terlewat (mis. internet mati) + detail request karyawan | Konfirmasi K-9 | ❌ ⏳ | Roster HR menampilkan "Belum Absen"; tanpa alert dan tanpa form karyawan | Tunggu K-9; kemungkinan digabung dengan AB-04 |
| AB-07 | Rules keterlambatan | Menunggu TnC | ✅ (mekanisme) ⏳ | Toleransi per shift; potongan telat mode `tier` menyala di live (4–6 hari = 1 hari THP, 7–9 = 2 hari gaji pokok, 10+ = 3 hari) | Sesuaikan setelah TnC |

## Log Activity HRIS

| ID | Requirement | Status MOM | Kode | Bukti / perilaku sekarang | Langkah berikutnya |
|---|---|---|---|---|---|
| LA-01 | Audit log perubahan data, termasuk import/export, koreksi absen & lembur | Disepakati | 🟡 | `audit.audit_log` append-only + halaman `/dashboard/settings/audit` + `GET /api/hris/employees/{id}/audit`. Tercakup: koreksi absensi, tutup/buka periode, import karyawan, resign, jadwal. **Belum:** export, keputusan/koreksi lembur, edit karyawan biasa, cuti, shift, payroll | Tambah event audit untuk yang belum tercakup |

## Ringkasan

| Status | Jumlah | ID |
|---|---|---|
| ✅ selesai (termasuk yang menunggu deploy/merge) | 16 | ES-01, ES-02, ES-04, ES-06, ES-07, ES-08, AB-07 (mekanisme; aturan final menunggu TnC), AP-05, KP-01, KP-02 (EPIC-060, menunggu review & rilis), PY-01, PY-03, PY-04 (EPIC-059, menunggu review & rilis; PPh21 masa pajak terakhir follow-up), AB-01, AB-02, AB-03 (EPIC-062, menunggu review & rilis; koordinat toko live menunggu K-14) |
| 🟡 sebagian | 8 | ES-03, ES-05, AP-01, AP-02, AP-03, AP-04, PY-02, LA-01 |
| ⚠️ berbeda | 2 | AB-04, AB-05 |
| ❌ belum ada | 3 | AP-06, PY-05, AB-06 |
| ⏳ hanya menunggu TnC | 2 | ES-09, KP-03 |
| **Total** | **31** | |
