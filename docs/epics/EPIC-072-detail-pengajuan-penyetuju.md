# EPIC-072: Detail Pengajuan untuk Penyetuju — Alasan & Dokumentasi Terlihat Sebelum Memutus (`MODULE-HRIS`)

status: ready-for-qa
environment: production
retries: 0

## Goal
Permintaan user 30-10-2026: untuk **semua pengajuan** yang punya dokumentasi dan alasan, **atasan dan peran lain yang berhak menyetujui/menolak** harus bisa **melihat detailnya**. Saat ini mereka hanya bisa menekan Setujui atau Tolak.

Pemetaan kode per 30-10 (main `b2358de1`):

| Pengajuan | Alasan / dokumentasi tersimpan | Yang dilihat penyetuju sekarang |
|---|---|---|
| Cuti/Izin/Sakit (`hris.leaves`) | `reason`, `attachment_url` (foto) | ESS Persetujuan Tim: alasan terpotong satu baris, lampiran dibuka di tab baru. HR Cuti: alasan `line-clamp-2`, **lampiran tidak tampil**. Panel Hari Ini: putus tanpa alasan, tautan "Buka" ke halaman yang tidak ada. |
| Lembur (`hris.overtime_requests`) | `reason`, `attachments[]` (1–3 PDF/foto) | ESS: alasan terpotong, dialog Setujui tanpa alasan, dialog Tolak tanpa detail. HR Lembur: alasan terpotong 220 px. |
| Pengajuan Absensi (`hris.attendance_requests`) | `reason`, `attachment_path` (foto) | HR: alasan lengkap, foto di tab baru. |
| Perjalanan Dinas (`hris.business_trips`) | `destination`, `purpose` | ESS: terpotong satu baris. HR: lengkap. |
| Kasbon/Pinjaman (`hris.loans`) | `purpose`, `notes` | ESS Kasbon Tim: keperluan inline, **tolak tanpa alasan**. HR: keperluan terpotong 180 px, `notes` tidak tampil. |

Server sudah mengirim alasan dan path dokumentasi di semua antrean; celahnya ada di tampilan. Satu celah server: daftar HR Cuti/Lembur dan file lampiran cuti/lembur hanya terbuka untuk peran aplikasi HR (`IsHR`), padahal keputusan HR memakai hak `approve` per menu daun (EPIC-067 R11). Akun yang diberi `approve` lewat peran IAM saja melihat daftar kosong dan 403 pada lampiran (tindak lanjut tercatat di EPIC-067).

## Ruling (dibuat tanpa tanya; user minta langsung dikerjakan)
- **R1** Satu komponen bersama `RequestDetailDialog` dipakai di semua layar penyetuju. Isinya: identitas karyawan, jenis, tanggal/jam/nominal, **alasan lengkap** (tidak dipotong, baris baru dipertahankan), **dokumentasi dengan pratinjau di dalam aplikasi** (gambar langsung, PDF dalam bingkai, tombol Unduh), status, dan tombol Setujui/Tolak di kaki dialog bila penonton berhak memutus.
- **R2** Setiap baris antrean mendapat tombol **Detail**. Tombol Setujui/Tolak di baris tetap ada (tidak menambah langkah untuk yang sudah terbiasa). Alasan di baris dipendekkan maksimal 2 baris; detail lengkap ada di dialog.
- **R3** Dialog **Tolak** di ESS (cuti, lembur) dan dialog **Setujui lembur** ikut menampilkan ringkasan pengajuan + alasan, supaya keputusan tidak buta walau tombol baris yang ditekan.
- **R4** Kasbon tim di ESS: Tolak kini **wajib alasan** (server sudah menerima `rejection_reason`); `notes` kasbon ditampilkan bila ada.
- **R5** Pratinjau dokumentasi lembur (`OvertimeAttachmentButtons`) digeneralisasi menjadi `AttachmentPreviewButtons` dengan `hrefFor`. Lampiran cuti dan foto bukti absensi memakai pratinjau yang sama, tidak lagi tab baru.
- **R6** Panel **Hari Ini** (desktop): cuti tidak lagi diputus langsung dari panel (sebelumnya tolak dengan alasan tetap "Ditolak dari panel Hari Ini" tanpa melihat alasan/lampiran). "Buka" menuju `/dashboard/hris/leaves` (tautan lama `/dashboard/hris/workforce/leaves` tidak ada). *Direvisi saat review:* panel **tidak** menampilkan alasan, karena panel terlihat oleh semua menu `hris.*` (lebih luas dari pembaca Cuti, R7) dan alasan cuti sering medis.
- **R7** Server: daftar HR Cuti dan Lembur, detail cuti, serta file lampiran cuti/lembur terbuka untuk `IsHR` **atau** `CanApprove(leaf)` daun masing-masing (Cuti `hris.workforce.leaves`, Lembur `hris.kepegawaian.overtime`). Atasan langsung tetap seperti sekarang (bawahan langsung saja). Tidak ada perubahan siapa yang boleh memutus.
- **R8** Antrean kasbon manager ikut mengirim `notes` (kolom non-gaji, aman untuk R8 EPIC-060).
- **R9** Tanpa migrasi DB. Tidak ada perubahan aturan approval (matriks tetap).

### Ruling saat implementasi & review (R10+)
- **R10** Halaman ESS "milik saya" (Izin & Cuti, Lembur) meminta `employee_id=me`; `GET /api/hris/leaves` kini menerima `employee_id=me`. Tanpa ini, pemegang approve (R7) melihat daftar seluruh karyawan di ESS seolah miliknya.
- **R11** Pengajuan milik penonton sendiri tidak menampilkan Setujui/Tolak di Detail maupun baris HR Cuti/Lembur (server juga menolak).
- **R12** Detail lembur HR menampilkan jenis hari yang dipilih di baris (yang akan dikirim Setujui).
- **R13** Dialog Detail di halaman HR mengikuti baris per id, sehingga status/aksi ikut data terbaru setelah keputusan.

## Tasks
Rencana: `docs/superpowers/plans/2026-10-30-detail-pengajuan-penyetuju.md`.

## Acceptance Criteria
- Atasan langsung di ESS Persetujuan Tim bisa membuka Detail cuti, lembur, dan Perjalanan Dinas bawahan: alasan lengkap terbaca, lampiran/dokumentasi tampil di dalam aplikasi, lalu bisa Setujui/Tolak dari dialog.
- Manager kasbon di ESS Pinjaman bisa membuka Detail kasbon tim (jenis, nominal, tenor, keperluan, catatan) dan menolak dengan alasan wajib.
- HR di Cuti, Lembur, Absensi (tab Pengajuan Absensi dan Perjalanan Dinas), dan Pinjaman bisa membuka Detail tiap pengajuan; HR Cuti kini menampilkan lampiran.
- Akun dengan `approve` di daun Cuti/Lembur tanpa peran aplikasi HR melihat daftar HR dan bisa membuka lampiran (tes integrasi Go).
- Dialog dan daftar rapi di laptop (1280) dan HP (390): tidak ada elemen keluar dari dialog.
- go test, pnpm test, lint, tsc hijau; CI 4/4; QA staging.

## Automation Log
- 2026-10-30: Epic dibuat dari permintaan user; pemetaan layar penyetuju dan endpoint (read-only) selesai.
- 2026-10-30: **Implementasi** (eksekusi inline, plan T1–T5):
  - T1 `446f67dd`, `ee909115`: `Actor.hrReads` (IsHR atau approve daun) untuk daftar/detail cuti, daftar lembur, lampiran cuti/lembur; `notes` di antrean kasbon manager; Hari Ini tanpa putus langsung. Tes `TestApproverReadsRequestDetails`.
  - T2 `12bf831f`: `RequestDetailDialog` + `AttachmentPreviewButtons` (dengan `hrefFor`), `leaveAttachmentHref`, URL foto absensi di-encode.
  - T3 `32837601`: Detail cuti/lembur/Perjalanan Dinas di ESS Persetujuan Tim; alasan di dialog Setujui lembur dan dialog Tolak.
  - T4 `7ffe2308`: Detail kasbon tim; tolak wajib alasan.
  - T5 `e4b52722`: Detail di HR Cuti (lampiran tampil), Lembur, Pengajuan Absensi, Perjalanan Dinas, Pinjaman.
- 2026-10-30: **Review akhir seluruh branch:** With fixes, 0 kritis. Diperbaiki di `6e82a83f`: R10 (ESS milik sendiri), R6 direvisi (alasan tidak di Hari Ini), R11, R12, baris kasbon tim di HP bisa turun baris. Ditunda (minor): nama aksesibel tombol "Lampiran 1" sama antarbaris; fallback Next `/api/desktop/inbox` masih actionable (hanya tanpa `BACKEND_URL`); Detail Persetujuan Tim bisa tetap terbuka bila penyetuju lain memutus lebih dulu (server menolak).
- 2026-10-30: **Gerbang lokal:** `go vet` bersih; `go test -p 1` payroll/hris/desktop ok; frontend `tsc` 0, `lint` 0, `pnpm test` 865 file / 6286 tes lulus.
- 2026-10-30 13:10: **Deploy staging** branch `783bd26b` (PR #52), image `local-20261030-131040`, tanpa migrasi (verify 0 beda). Backup `~/Desktop/paddydev-backups/staging-db-before-epic072-20261030-131033.dump`.
- 2026-10-30: **QA staging** dengan data demo (akun `qa072-hr` peran hrd + `hris_manager`, karyawan fiktif "QA072 Atasan" dan "QA072 Pemohon…", 1 cuti sakit berlampiran, 1 lembur berdokumentasi PDF + gambar). Probe Playwright laptop 1280 + HP 390: ESS Persetujuan Tim (Detail cuti, Detail lembur gambar + PDF), HR Cuti, HR Lembur, 10/10 lulus. 0 elemen keluar dari dialog, alasan utuh tampil, lampiran 200 dan termuat, `employee_id=me` tidak memuat pengajuan orang lain, 0 error konsol, 0 respons API ≥ 400, log API 0 error. Data demo, sesi, dan file lampiran dihapus (0 tersisa). Keputusan (Setujui/Tolak) tidak diuji di staging agar tidak mengirim WhatsApp ke nomor fiktif; logika keputusan tidak berubah dan tercakup tes unit.
- 2026-10-30: **PR #52 merged** (`28721f40`), CI 4/4 hijau.
- 2026-10-30 13:26 WIB: **Deploy production** atas izin eksplisit user ("Merge + deploy prod"). Image `local-20261030-132645` = main `28721f40`, tanpa migrasi (565, verify 0 beda), container healthy, log API 0 error, absensi tetap masuk setelah deploy, bundle memuat dialog Detail dan `employee_id=me`.
  - Backup `~/backups/prod-db-before-epic072-20261030-132458.dump`; source lama `~/paddydev-src.before-epic072-20261030-132458` (= `405eddc6`). Rollback: kembalikan folder source itu lalu jalankan `tools/deploy-paddydev.sh`.
  - Status tetap `ready-for-qa`: HR/atasan mencoba tombol Detail di production (tanpa sesi uji di production).
- 2026-10-30: **Catatan tertunda dikerjakan** (branch `fix/epic072-075-sisa`):
  - `fff9b477`: rute cadangan Next `/api/desktop/inbox` sama dengan Go (cuti tanpa putus langsung, href `/dashboard/hris/leaves`); kode Setujui/Tolak panel Hari Ini yang sudah mati dihapus.
  - `57e0d6a1`: tombol lampiran/dokumentasi di baris daftar punya nama aksesibel per karyawan ("Lampiran 1 cuti Budi").
  - `79bcf5d9`: Detail Persetujuan Tim mengikuti antrean terbaru dan tertutup bila penyetuju lain sudah memutus.
