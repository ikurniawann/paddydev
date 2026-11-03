# EPIC-084: Ketentuan Cuti & Izin + SLA Pengajuan terhadap Payroll (`MODULE-HRIS`)

status: ready-for-qa
environment: dev
retries: 0

## Goal
Permintaan user 03-11-2026: menerapkan TnC "KETENTUAN CUTI DAN IZIN TIDAK MASUK KERJA". Ditambah:
- Birthday Leave dengan upload attachment;
- Izin Leader 1x/bulan untuk SPV ke atas, hangus bila tidak dipakai, tanpa potong THP;
- SLA pengajuan terhadap penggajian: lewat tanggal 21 tidak bisa mengajukan untuk periode sebelumnya. Lembur H+1, Cuti H-3, Cuti Ibadah H-30, Sakit hari H.

Acuan:
- MOM 29-10-2026 (`docs/hris/mom-20261029/`):
  - §3.1 SLA;
  - §3.2 jenis cuti;
  - ES-12 s.d. ES-16, K-19, K-20, K-21, K-27.
- MOM 19-10-2026 (`docs/hris/acuan-mom-20261019/`). Aturan yang tidak disebut di TnC tetap berlaku, misalnya kuota Izin Pribadi 3/bulan.
- EPIC-058 (katalog jenis cuti), EPIC-059 (PIP/PIS), EPIC-068 (saldo cuti tahunan).

### TnC (ringkas)
- **Cuti Tahunan**: 12 hari kerja setelah masa kerja minimal 1 tahun. Prorata (berdasarkan sisa bulan efektif, dengan memperhitungkan cuti bersama) bagi yang genap 1 tahun di luar Januari.
- **Birthday Leave**: 1 hari, bagi karyawan yang sudah melewati masa kontrak awal saat ulang tahun.
- **Cuti Khusus**:
  - Menikah 3 hari.
  - Istri Melahirkan 2 hari.
  - Istri Keguguran 2 hari.
  - Menikahkan Anak 2 hari.
  - Khitanan/Baptis Anak 2 hari.
  - Cuti Leaders 1 hari/bulan, SPV ke atas, keperluan mendesak.
- **Cuti Melahirkan**: 1,5 bulan sebelum + 1,5 bulan sesudah.
- **Cuti Keguguran**: 1,5 bulan sesuai surat dokter.
- Melahirkan dan keguguran dibayar penuh hanya bila masa kerja aktif ≥ 1 tahun.
- **Cuti Ibadah Haji**: paling lama 40 hari. Kelebihan di atas 14 hari kalender dibayar penuh bila masa kerja ≥ 1 tahun; bila < 1 tahun, kelebihannya tidak dibayar.
- **Izin Pribadi (unpaid)**: bagi yang belum punya hak cuti atau hak cutinya habis, dengan persetujuan atasan. Memotong gaji pokok, makan, transport, dan jabatan sesuai hari.
- **Cuti Berkabung**: orang tua, istri, anak, saudara kandung 1 rumah — masing-masing 2 hari.
- **Mangkir**: 5 hari kerja berturut-turut tanpa keterangan → dipanggil 2x tertulis → dianggap mengundurkan diri.

### Jawaban user 03-11
- **Prorata**: hak tahun genap 1 tahun = jumlah bulan **sesudah** bulan genap 1 tahun, dikurangi cuti bersama yang jatuh di bulan-bulan itu. Contoh: masuk 10 Mei 2026 → 2027 = 7 hari. Berlaku **mulai saldo 2027**; saldo ≤ 2026 tidak diubah.
- **Masa kerja < 1 tahun** (melahirkan/keguguran, dan haji di atas 14 hari): hari itu **tidak dibayar**, dipotong seperti Izin Pribadi (pokok + makan + transport + jabatan). ≥ 1 tahun: upah penuh.
- **Birthday Leave**: 1 hari kapan saja **dalam bulan ulang tahun**, sekali per tahun, upah penuh, attachment wajib.
- **Tanggal 21**: memakai periode 21–20. Aktivasi `cutoff_day = 20` menjadi langkah ops yang digerbang (lihat R9).

## Kondisi kode per 03-11 (main `57aa4cf4`)
- Katalog jenis cuti ada di 3 tempat + SQL, dan harus diubah bersama:
  - `backend/internal/modules/hris/domain/leave_types.go`;
  - `frontend/src/lib/hris/leave-types.ts`;
  - `backend/internal/modules/desktop/domain/board.go:118`;
  - enum `public.leave_type` dan label di `fn_notify_leave_changes` (delta `20261020150100`).
- `CreateLeave` (`hris/leaves.go:207`):
  - sudah mengecek: jenis selectable, satu hari, LeaderOnly, saldo tahunan, tabrakan, kuota bulanan;
  - **belum ada**: batas waktu, kunci periode, maksimum hari, attachment wajib, syarat gender, masa kerja, Birthday.
- Saldo tahunan: `hris.default_annual_leave` (SQL, IMMUTABLE) kembar dengan `domain.DefaultAnnualLeave` (Go, hanya dipakai tes paritas). Hak dimulai setelah 12 bulan, lalu prorata **hari** dibulatkan 0,5.
- Payroll (`payroll/domain/inputs.go:262-300`, `calculator.go:425-438`):
  - `personal` → PIP;
  - `sick` dan `maternity` → PIS (transport + makan), padahal MOM 29 ES-16 menyatakan melahirkan full paid;
  - `leader` dan jenis lain full paid; ES-15 sudah terpenuhi.
- Lembur milik sendiri diinput setelah selesai (EPIC-066) dan ditolak bila periodenya tertutup. **Belum ada batas H+1.**
- Periode absensi: `hris.attendance_settings` cutoff 0 / offset 3.
  - Staging = prod: periode bulan kalender; Okt ditutup otomatis setelah 03-11.
  - Periode lewat tanggal closing ditutup otomatis.
  - `ensureAttendanceOpen` dipakai pengajuan absensi dan lembur.
- Data staging (salinan prod): 332 karyawan aktif.
  - `birth_date` hanya terisi 51.
  - `gender` terisi 51 (22 male, 29 female).
  - 44 karyawan tanpa kontrak non-draft (34 di antaranya probation).

## Ruling
- **R1 Katalog.** Kolom baru di `LeaveType`:
  - `MaxDays` (hari kerja per pengajuan);
  - `MaxSpanDays` (hari kalender);
  - `LeadDays` (H-n);
  - `SameDayOnly`;
  - `AttachmentRequired`;
  - `Gender` (`"female"`/`"male"`/`""`).

  | code | Label | MaxDays | MaxSpanDays | LeadDays | Lain |
  |---|---|---|---|---|---|
  | annual | Cuti Tahunan | – | – | 3 | saldo |
  | sick | Sakit | – | – | – | SameDayOnly |
  | personal | Izin Pribadi | – | – | – | 3/bln, 1 hari, PIP, hanya bila saldo tahunan < 1 |
  | leader | Izin Leader | – | – | – | 1/bln, 1 hari, SPV+ |
  | **birthday** (baru) | Birthday Leave (Cuti Ulang Tahun) | – | – | 3 | 1 hari, attachment wajib, R4 |
  | maternity | Cuti Melahirkan | – | 90 | 3 | female |
  | **miscarriage** (baru) | Cuti Keguguran | – | 45 | – | female, attachment wajib |
  | paternity | Cuti Istri Melahirkan *(label baru)* | 2 | – | – | male |
  | **wife_miscarriage** (baru) | Cuti Istri Keguguran | 2 | – | – | male |
  | marriage | Cuti Menikah | 3 | – | 3 | |
  | **child_marriage** (baru) | Cuti Menikahkan Anak | 2 | – | 3 | |
  | **child_ceremony** (baru) | Cuti Khitanan/Baptis Anak | 2 | – | 3 | |
  | pilgrimage | Cuti Ibadah Haji/Umrah | – | 40 | 30 | |
  | bereavement | Cuti Berkabung *(label baru)* | 2 | – | – | relasi ditulis di alasan |

  - Export label lama dipertahankan karena dibaca di luar aplikasi. Jenis baru memakai label yang sama untuk export.
  - Syarat gender hanya menolak bila gender karyawan **terisi** dan berbeda, karena 281 karyawan belum terisi.
- **R2 Batas waktu (ES-12).** Berlaku hanya untuk pengajuan **milik sendiri**. HR yang mengajukan untuk karyawan lain dikecualikan, sama seperti lembur HR (EPIC-063 R3).
  - Pesan per jenis:
    - LeadDays n: `start − hari ini (WIB) ≥ n` → "{Label} diajukan paling lambat H-{n} (mulai {tanggal} atau sesudahnya)".
    - SameDayOnly: `start = hari ini` → "Sakit diajukan pada hari sakit (tanggal mulai = hari ini)".
  - Jenis mendadak (berkabung, istri melahirkan/keguguran, keguguran, Izin Leader, Izin Pribadi) tidak punya H-n. Mundurnya hanya dibatasi R3.
- **R3 Kunci periode (ES-13).** Pengajuan milik sendiri untuk tanggal yang periodenya **sudah lewat tanggal akhirnya** (hari ini > `end_date` periode) ditolak dengan 409 "Periode {label} sudah lewat cut off; pengajuan untuk tanggal ini ditutup".
  - Berlaku untuk cuti/izin (tanggal mulai), lembur, dan pengajuan absensi (koreksi absen; K-20 = "semua pengajuan").
  - Dasarnya `end_date`, bukan tanggal closing. Dengan begitu jendela closing HR (offset 3 hari) tetap ada untuk koreksi oleh HR, sementara karyawan sudah terkunci sejak hari sesudah cut off.
  - Kunci periode tertutup yang lama tetap berlaku.
- **R4 Birthday Leave.** Syarat:
  - `birth_date` terisi; bila kosong → "Tanggal lahir belum diisi di data karyawan — hubungi HR".
  - Bulan tanggal cuti = bulan lahir.
  - Maksimal 1 hari per tahun kalender (pending + approved).
  - Sudah melewati masa kontrak awal pada tanggal ulang tahun di tahun itu (29 Feb → 28 Feb di tahun bukan kabisat). "Kontrak awal" = kontrak non-draft paling awal (start_date, lalu sequence):
    - punya `end_date` → `end_date` < ulang tahun;
    - tanpa `end_date` (PKWTT) → `probation_end_date` kosong atau < ulang tahun;
    - tanpa kontrak → lolos hanya bila `employment_status = 'permanent'`.
- **R5 Izin Pribadi.** Ditolak bila sisa cuti tahunan yang tersedia pada tanggal mulai ≥ 1 hari: "Izin Pribadi hanya untuk yang belum punya hak cuti atau cutinya habis — sisa Cuti Tahunan Anda {n} hari". Kuota 3/bulan tetap.
- **R6 Lembur H+1 (K-19).** Lembur milik sendiri ditolak bila hari ini (WIB) > tanggal lembur + 1: "Lembur diajukan paling lambat H+1 ({tanggal})". Aturan EPIC-066 (input setelah selesai) tetap. Bila H+1 jatuh sesudah cut off, kunci R3 menang (payroll final); HR tetap bisa menginput lewat jalur HR.
- **R7 Prorata cuti tahunan (tahun ≥ 2027).** A = join + 12 bulan.
  - A.tahun > Y → 0.
  - A.tahun < Y → 12.
  - A.tahun = Y:
    - A di Januari → 12;
    - selain itu `max(0, 12 − A.bulan − CB)`, dengan CB = jumlah cuti bersama aktif (`type='cuti_bersama'`, `status='aktif'`, tidak dihapus, `is_day_off` dan `deducts_leave`) di bulan > A.bulan tahun Y.
  - Tahun ≤ 2026 memakai rumus EPIC-068 apa adanya.
  - SQL `hris.default_annual_leave` menjadi STABLE. Kembar Go: `DefaultAnnualLeave(join, year, cutiBersama []string)`.
  - Baris saldo dibuat saat pertama dibutuhkan (job 1 Jan / pengajuan). Cuti bersama yang diinput **sesudahnya** tidak mengubah baris yang sudah ada; HR menyesuaikan lewat Saldo Cuti.
- **R8 Payroll (ES-16 + TnC).** Per hari kerja cuti yang disetujui:
  - `personal` → PIP;
  - `sick` → PIS;
  - `maternity`/`miscarriage`: masa kerja ≥ 1 tahun pada tanggal mulai cuti → full paid; < 1 tahun → PIP;
  - `pilgrimage`: hari ke-1 s.d. 14 kalender sejak mulai → full paid; sesudahnya → PIP bila < 1 tahun;
  - jenis lain → full paid.

  "≥ 1 tahun" berarti join + 1 tahun ≤ tanggal mulai cuti. Join kosong atau tak terbaca dianggap ≥ 1 tahun (tidak memotong). Maternity keluar dari PIS.

  **Tanggal berlaku (review akhir):** aturan < 1 tahun hanya untuk cuti yang **mulai ≥ 03-11-2026** (`LeaveTnCEffective`). Cuti yang mulai lebih awal tetap aturan lama: melahirkan < 1 tahun → PIS, haji → full paid. Dengan begitu cuti yang disetujui dengan syarat lama tidak dipotong mundur. Join 29 Feb genap 1 tahun pada 28 Feb.
- **R9 Aktivasi periode 21–20 (ops, digerbang).** Kode R3 generik. Aktivasi = `PUT attendance-settings {cutoff_day: 20, closing_offset_days: 3}` setelah periode Oktober tertutup. Akibatnya: Nov menjadi periode transisi 1–20 Nov, Des = 21 Nov–20 Des.
  - Dampak transisi pada `monthly_prorated` (simulasi 21-10: bisa ±65% pokok) harus diputuskan HR sebelum aktivasi di production.
  - Di staging diaktifkan untuk QA.
- **Di luar cakupan** (tindak lanjut):
  - ES-11 pengecekan attachment oleh role HRIS;
  - otomatisasi mangkir (tetap proses manual HR);
  - AP-07 batas 72 jam lembur (K-23);
  - kuota per kejadian cuti khusus lintas pengajuan.

## Tasks
Rencana: `docs/superpowers/plans/2026-11-03-ketentuan-cuti-izin.md` (Task 1–7).

## Acceptance Criteria
- [x] Form ESS & HR menampilkan 14 jenis aktif (13 untuk non-SPV: Izin Leader tersembunyi) sesuai R1; jenis baru tersimpan dan terbaca di daftar, detail, notifikasi, export, dan board desktop.
- [x] Pengajuan milik sendiri ditolak sesuai R2/R3/R5/R6 dengan pesan di atas; HR untuk karyawan lain tidak terkena R2/R5/R6.
- [x] Birthday Leave hanya lolos sesuai R4 dan wajib attachment; Cuti Keguguran wajib attachment.
- [x] Saldo 2027 karyawan join 10-05-2026 = 7 (tanpa cuti bersama); saldo ≤ 2026 tidak berubah (checksum sebelum/sesudah migrasi sama).
- [x] Payroll: melahirkan ≥ 1 tahun tanpa potongan; < 1 tahun dipotong PIP; haji < 1 tahun dipotong PIP mulai hari ke-15.
- [x] Gates: `go test ./...`, vitest, typecheck, build hijau; migrasi apply + verify di staging.

## Automation Log
- 2026-11-03 — Epic + rencana ditulis (writing-plans) setelah 4 jawaban user. Worktree `.worktrees/epic084`, branch `feat/epic084-ketentuan-cuti`, dari main `57aa4cf4`.
- 2026-11-03 — Implementasi native (executing-plans), 6 task + perbaikan review akhir, branch `feat/epic084-ketentuan-cuti`:
  - Commit:
    - `ae3195fa` katalog + enum + label;
    - aturan domain;
    - SLA + kunci cut off;
    - prorata 2027;
    - payroll;
    - form;
    - `e7f58eda` perbaikan review.
  - Gates:
    - `go vet` + `gofmt` bersih;
    - `go test -p 1 ./...` hijau;
    - vitest 6459 lulus;
    - `tsc` + eslint bersih.
  - Tes fixture lama yang terkena aturan baru disesuaikan tanpa melonggarkan aturan:
    - lembur memakai jam H+1;
    - kuota Izin Pribadi memakai saldo tahunan 0.
- 2026-11-03 — Review akhir (reviewer terpisah): 0 Critical, 1 Important, 10 Minor.
  - **Important (R8 berlaku mundur)**: diputuskan aturan < 1 tahun hanya untuk cuti mulai ≥ 03-11-2026 (`LeaveTnCEffective`).
  - Diperbaiki:
    - label slip "Potongan Sakit";
    - cuti bersama ganda dihitung sekali (`count(DISTINCT holiday_date)`);
    - hint form HR tanpa H-n;
    - angka desimal pakai koma;
    - join 29 Feb genap 1 tahun pada 28 Feb.
  - Ditunda (minor):
    - Izin Pribadi belum memperhitungkan cuti tahunan yang masih pending;
    - kepemilikan path lampiran belum dicek (ES-11);
    - route fallback Next tanpa R2–R6 (tidak dipakai production);
    - tes integrasi untuk syarat gender belum ada.
- 2026-11-03 — **Staging** `local-20261103-062547` (commit `e7f58eda`):
  - Backup `~/Desktop/paddydev-backups/staging-db-before-epic084-20261103-062529.dump`.
  - Migrasi: 2 diterapkan; verify 10762 objek, 0 berbeda.
  - `hris.leave_balances` md5 sebelum = sesudah (`264767ee…`, 386 baris).
  - QA browser (akun demo qa084, sudah dihapus; baris audit append-only tetap ada):
    - form ESS menampilkan 13 jenis (Izin Leader tersembunyi untuk non-SPV), hint "Diajukan paling lambat H-3", dan tanggal minimal 06-11;
    - API menolak pengajuan yang melanggar aturan:
      - H-1 tahunan;
      - sakit kemarin;
      - Birthday tanpa lampiran;
      - Birthday di luar bulan lahir;
      - Izin Pribadi saat saldo 11,5;
      - Menikah 4 hari;
      - cuti 30-10 → 409 cut off;
      - lembur 01-11 → H+1;
      - koreksi absen 30-10 → 409;
    - sakit hari ini dan Birthday dengan lampiran (lewat UI) tersimpan;
    - form HR tanpa H-3 dan menampilkan "Maksimal 3 hari kerja".
  - **Cutoff 20 diaktifkan di staging saja** (R9). Periode staging sekarang:
    - Okt transisi 1–20 Okt (closing 23-10, tertutup otomatis saat dibaca);
    - Nov 21 Okt–20 Nov;
    - Des 21 Nov–20 Des.
  - Production tetap cutoff 0 sampai HR memutuskan periode transisi.
- 2026-11-03 11.40 WIB — **LIVE production** (izin user 03-11: "semua yang belum merged main production lakukan implementasi CI deploy sampai hijau" + "tapi nanti backup").
  - PR #69, #70, #71 di-merge (merge commit, karena bertumpuk). main `a39feb81`, CI 4/4 hijau. Staging = main (`local-20261103-113500`).
  - Gladi di salinan prod 11.34:
    - 2 migrasi lancar;
    - `leave_balances` md5 sama (`04963f81…`, 387 baris);
    - default 2027 untuk join 2026-05-10 = 7;
    - production **sudah memakai cutoff 20** (offset 4, diatur HR), jadi kunci tanggal 21 langsung berlaku;
    - 1 run payroll draft 2026-11, tanpa cuti melahirkan/haji aktif yang terdampak;
    - 80 lembur lama > 4 jam tetap ada dan bisa dikoreksi turun;
    - 2 pengajuan lembur pending milik level SPV+ tetap bisa diputuskan;
    - `birth_date` terisi 60 dari 314.
  - Deploy prod:
    - backup `~/backups/prod-db-before-main-e086-20261103-113943.dump` (3918 entri);
    - image `local-20261103-114048` (rollback ke image `local-20261103-004840` + `~/paddydev-src.before-main-e086-20261103-113943`);
    - 576 migrasi, verify 0 berbeda;
    - container healthy, 0 error API;
    - md5 saldo sama;
    - `/login` dan `/api/ready` 200.
  - Runner: `~/Desktop/paddydev-backups/prod084/run.sh`.
