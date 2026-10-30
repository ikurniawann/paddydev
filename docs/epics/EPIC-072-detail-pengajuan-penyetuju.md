# EPIC-072: Detail Pengajuan untuk Penyetuju — Alasan & Dokumentasi Terlihat Sebelum Memutus (`MODULE-HRIS`)

status: coding
environment: dev
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
- **R6** Panel **Hari Ini** (desktop): cuti tidak lagi diputus langsung dari panel (sebelumnya tolak dengan alasan tetap "Ditolak dari panel Hari Ini" tanpa melihat alasan/lampiran). Panel menampilkan alasan singkat; "Buka" menuju `/dashboard/hris/leaves` (tautan lama `/dashboard/hris/workforce/leaves` tidak ada).
- **R7** Server: daftar HR Cuti dan Lembur, detail cuti, serta file lampiran cuti/lembur terbuka untuk `IsHR` **atau** `CanApprove(leaf)` daun masing-masing (Cuti `hris.workforce.leaves`, Lembur `hris.kepegawaian.overtime`). Atasan langsung tetap seperti sekarang (bawahan langsung saja). Tidak ada perubahan siapa yang boleh memutus.
- **R8** Antrean kasbon manager ikut mengirim `notes` (kolom non-gaji, aman untuk R8 EPIC-060).
- **R9** Tanpa migrasi DB. Tidak ada perubahan aturan approval (matriks tetap).

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
