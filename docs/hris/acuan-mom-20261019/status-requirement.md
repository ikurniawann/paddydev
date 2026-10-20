# Status Requirement HRIS — MOM 19-10-2026 (ES / AP / PY / KP / AB / LA)

Dicek terhadap kode `main` (`4b7056e` + EPIC-058 `97e4c8f`) dan data live (hanya baca) pada 20-10-2026. Status:

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
| ES-06 | Potongan ke payroll: Sakit & Cuti Melahirkan potong transport+makan; Izin Pribadi potong 1 hari THP | Konfirmasi K-12 | 🟡 / ⚠️ | Sakit → PIS ✅ (setting `sick_leave_deduction` menyala di live). Izin Pribadi → PIP ✅ tapi **THP = seluruh slip** (termasuk lembur/bonus), bukan 4 komponen, dan pembaginya hari kerja terjadwal, bukan "total hari periode berjalan" ⚠️. **Cuti Melahirkan tidak memotong apa pun** ❌ | Basis PIP = 4 komponen; tambah `maternity` ke potongan transport+makan; pembagi menunggu K-12 |
| ES-07 | Antar jenis cuti dapat dikombinasikan | Disepakati | ✅ (EPIC-058) | Jenis berbeda boleh berdampingan; satu tanggal = satu pengajuan aktif | — |
| ES-08 | Internship mengikuti skema bulanan | Disepakati | ✅ (PR #3, **belum merge**) | Intern: kuota Izin Pribadi 3/bulan, tanpa potongan THP (`IsInternStatus`: `intern`/`internship`) | Merge PR #3 sebelum deploy EPIC-058 |
| ES-09 | Aturan Cuti Ibadah | Menunggu TnC | ⏳ | `pilgrimage` ada, tanpa batas hari/sekali seumur kerja, tidak memotong gaji | Tunggu TnC |

**Batas per jenis dari MOM §3.1 yang belum ditegakkan kode:** Cuti Tahunan maks. 3 hari/bulan dan baru setelah 12 bulan (masa kerja < 1 tahun → Izin Pribadi); Sakit maks. 3 hari/bulan; Cuti Melahirkan 3 bulan (1,5 + 1,5); Cuti Ayah 2 hari (per tahun, K-4); Menikah 3 hari; Duka 2 hari per kejadian (K-5).

## Approval

| ID | Requirement | Status MOM | Kode | Bukti / perilaku sekarang | Langkah berikutnya |
|---|---|---|---|---|---|
| AP-01 | Time-off disetujui atasan langsung (SPV ke atasannya) | Disepakati | 🟡 | Satu tahap: HR **atau** `employees.reporting_to` boleh memutus (`hris/leaves.go` DecideLeave). Atasan dapat WA, tetapi **tidak ada antrean approval di ESS** (`ListLeaves` non-HR hanya data sendiri; inbox desktop memakai menu, bukan `reporting_to`). Live: 55 karyawan aktif tanpa atasan; Retail Operation Manager tercatat atasan dirinya sendiri | Halaman "Persetujuan Tim" di ESS untuk cuti & lembur; bereskan data `reporting_to` |
| AP-02 | Lembur wajib diajukan sebelum dilaksanakan; approval atasan langsung | Disepakati | 🟡 | Alur pengajuan lembur ada (pending → approved/rejected, HR atau atasan). **Tanggal lampau tetap diterima**; API `GET /api/hris/overtime?scope=approvals` ada tapi tanpa UI | Tolak pengajuan untuk waktu yang sudah lewat; UI antrean atasan |
| AP-03 | Jenis lembur Weekday / Weekend | Konfirmasi K-13 | 🟡 | `day_type` hanya `regular`/`rest_day`, diisi HR; form ESS tanpa pilihan jenis; payroll menyatukan libur nasional + rest day (tarif 2×) | Pilihan Weekday/Weekend di form; tarif menunggu SOP Lembur (K-13) |
| AP-04 | Koreksi lembur oleh AdminHRIS | Disepakati | 🟡 | Tidak ada endpoint ubah pengajuan lembur; HR hanya bisa mengoreksi `overtime_hours` di absensi (tercatat di audit) | Endpoint + UI koreksi lembur (HR) dengan audit log |
| AP-05 | Kasbon disetujui Manager Departemen **dan** HR (akses Payroll) | Disepakati | ⚠️ | Satu tahap: siapa pun dengan aksi `approve` di menu Pinjaman (`payroll/loans.go`). Tidak ada langkah manager; departemen tidak punya kolom kepala | Approval 2 tahap (manager departemen → HR Penggajian); tentukan sumber "manager departemen" (Q-APV-2) |
| AP-06 | Approval CEO dan GM | Konfirmasi K-7 | ❌ ⏳ | Tidak ada rantai multi-tahap/eskalasi di HRIS. Tabel generik `configuration.user_approval_permissions` hanya dipakai accounting/procurement | Tunggu K-7 (untuk pengajuan apa) |

## Payroll & Periode Kerja

| ID | Requirement | Status MOM | Kode | Bukti / perilaku sekarang | Langkah berikutnya |
|---|---|---|---|---|---|
| PY-01 | 4 komponen gaji: Gaji Pokok, Tunj. Makan, Tunj. Transport, Tunj. Jabatan | Disepakati | 🟡 / ⚠️ | Keempat komponen ada di struktur gaji (plus fixed/variable/housing/health). "1 hari THP" dihitung dari **seluruh slip**, bukan 4 komponen ⚠️ | Basis "1 hari THP" = 4 komponen (PIP & potongan telat tier) |
| PY-02 | Cut off tanggal 20 (periode 21 → 20) | Disepakati | 🟡 | Mekanisme ada (`hris.attendance_settings.cutoff_day`), tetapi **live = 0 (bulan kalender)**. `payroll_settings.payroll_day` live = 25. Cicilan pinjaman masih per bulan kalender | Set `cutoff_day = 20` (keputusan data, bukan kode); cek ulang cicilan & THR terhadap periode |
| PY-03 | Skema 5 hari & 6 hari kerja per periode | Konfirmasi K-10 | 🟡 | Hari kerja dari jadwal shift karyawan → absensi → fallback 20; pembagi per struktur gaji (live: 20/25/26 campur). Tidak ada skema 5/6 hari eksplisit (23/24) | Tunggu K-10/K-12; lalu field skema kerja per karyawan |
| PY-04 | Prorate untuk karyawan keluar di tengah periode | Konfirmasi K-12 | 🟡 | Prorata hanya dari tanggal kontrak; payroll run hanya ambil `is_active`, jadi karyawan yang sudah dinonaktifkan **tidak dapat slip**; `end_date` karyawan tidak dibaca | Sertakan karyawan keluar dalam periode + prorata dari `end_date` |
| PY-05 | Payroll mingguan untuk daily worker | Disepakati | ❌ | Run hanya bulanan; `pay_basis = daily` ada tanpa periode mingguan | Fitur run mingguan (live: 5 daily worker aktif) |

## Kasbon & Pinjaman

| ID | Requirement | Status MOM | Kode | Bukti / perilaku sekarang | Langkah berikutnya |
|---|---|---|---|---|---|
| KP-01 | Hapus fitur Pinjaman Darurat | Disepakati | ❌ | `emergency` masih bisa dipilih (`frontend/src/lib/hris/loans-view.ts`); live: 1 pengajuan darurat **pending** | Sembunyikan dari form & tolak di API; putuskan nasib 1 pengajuan pending (Q-KP-1) |
| KP-02 | Kasbon maks. 50% THP, tenor 1 atau 2 | Konfirmasi K-6 | ⚠️ | Batas = cicilan ≤ `loan_max_installment_percent` (live **30%**) dari **gaji pokok**; tenor 1–60 (default form 3). Tidak ada blokir status Probation/Freelance/Intern (tabel payslip). Live: 1 kasbon tenor 3 | Plafon 50% THP (4 komponen), tenor {1,2}, blokir status; default 50/50 |
| KP-03 | Aturan Pinjaman | Menunggu TnC | ⏳ | Jenis sekarang: `kasbon`, `loan`, `emergency`; bunga flat (hanya HR) | Tunggu TnC |

## Absensi & Timesheet

| ID | Requirement | Status MOM | Kode | Bukti / perilaku sekarang | Langkah berikutnya |
|---|---|---|---|---|---|
| AB-01 | Geofence radius 100 m; status Valid / Invalid | Konfirmasi K-14 | ❌ | `configuration.branches.lat/lng` ada tapi **live 0 dari 52 cabang terisi**; GPS clock-in opsional, disimpan di `clock_in_location`; tidak ada cek radius/status | Isi koordinat cabang (K-14) → cek jarak di server + kolom valid/invalid |
| AB-02 | Perjalanan Dinas (luar area) | Disepakati | 🟡 | Status absensi `remote` ("Remote / dinas luar") hanya bisa diset HR; tanpa pengajuan/approval/rentang tanggal | Pengajuan Perjalanan Dinas + pengecualian geofence |
| AB-03 | Popup saat absen di luar area + kolom keterangan | Disepakati | ❌ | Backend menerima `notes` saat clock-in; ESS tidak mengirimnya dan tidak ada cek area | Bergantung AB-01 |
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
| ✅ selesai (termasuk yang menunggu deploy/merge) | 6 | ES-01, ES-02, ES-04, ES-07, ES-08, AB-07 (mekanisme; aturan final menunggu TnC) |
| 🟡 sebagian | 13 | ES-03, ES-05, ES-06, AP-01, AP-02, AP-03, AP-04, PY-01, PY-02, PY-03, PY-04, AB-02, LA-01 |
| ⚠️ berbeda | 4 | AP-05, KP-02, AB-04, AB-05 |
| ❌ belum ada | 6 | AP-06, PY-05, KP-01, AB-01, AB-03, AB-06 |
| ⏳ hanya menunggu TnC | 2 | ES-09, KP-03 |
| **Total** | **31** | |
