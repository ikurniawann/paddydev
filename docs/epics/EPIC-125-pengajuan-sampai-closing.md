# EPIC-125: Pengajuan Karyawan Terbuka sampai Closing Periode (`MODULE-HRIS`)

status: ready-for-qa
environment: dev
retries: 0

## Goal
Karyawan bisa mengajukan koreksi absen, cuti/izin, dan lembur untuk tanggal di periode yang sudah lewat tanggal akhirnya, selama periode itu belum di-closing. Aturan ini mengganti kunci cut-off EPIC-084 R3.

Rencana: [2026-11-21-pengajuan-sampai-closing.md](../superpowers/plans/2026-11-21-pengajuan-sampai-closing.md).

Kasus acuan: 21-11-2026 seorang karyawan mengajukan koreksi absen untuk 06-11-2026 dan ditolak "Periode 21 Okt – 20 Nov 2026 sudah lewat cut off; pengajuan untuk tanggal ini ditutup". Saat itu periode November (21 Okt–20 Nov) masih **Terbuka** dengan closing otomatis 22 Nov 2026.

## Keputusan user (21-11-2026)
| No | Pertanyaan | Keputusan |
| --- | --- | --- |
| K-20 (revisi) | Sampai kapan karyawan boleh mengajukan untuk periode sebelumnya? | Sampai periode di-closing (manual atau otomatis). Kutipan: "Open until closing". Mengganti jawaban K-20 03-11-2026 |

## Tasks
- [x] Task 1: `ensureSubmissionOpen` = `ensureAttendanceOpen` (hanya periode closed yang menolak); tes `TestSubmissionOpenUntilClosing`, `TestLeaveAcrossPeriodsBeforeClosing`
- [x] Task 2: dokumen acuan dan epic

## Acceptance Criteria
- [x] Koreksi absen, cuti/izin, dan lembur milik sendiri untuk tanggal di periode yang lewat `end_date` tapi belum closed → diterima.
- [x] Periode closed (termasuk otomatis sejak hari sesudah tanggal closing) → 409 "Periode absensi {label} sudah di-closing. …".
- [x] Aturan lain tetap: Cuti Tahunan H-3, Cuti Ibadah H-30, Sakit hari H, lembur milik sendiri H+1, HR untuk karyawan lain bebas H-n.
- [ ] QA HR di production setelah rilis.

## Catatan rilis untuk HR
- Karyawan sekarang bisa mengajukan koreksi absen, cuti/izin, dan lembur untuk periode lalu sampai **akhir hari closing** (contoh periode 21 Okt–20 Nov: sampai 22 Nov).
- Pengajuan yang masuk di hari closing perlu diputus **sebelum hari closing berakhir**:
  - Pengajuan absensi yang masih pending saat periode tertutup tidak bisa disetujui (409 "sudah di-closing"); hari itu tetap kosong untuk payroll kecuali HR menolaknya atau membuka ulang periode.
  - Cuti/lembur yang disetujui sesudah closing tidak ikut ke run payroll yang sudah diproses. Run hanya bisa dihitung ulang selama masih **draft**; kalau sudah tidak draft, hapus dan buat ulang run sebelum ditandai dibayar, atau sesuaikan di periode berikutnya.

## Follow-up
- Approve cuti/lembur untuk tanggal di periode yang sudah closed belum dijaga (sudah begitu sebelum EPIC-125). Karena pengajuan kini bisa masuk sampai hari closing, kasus ini lebih mungkin terjadi. Pilihan perbaikan berikutnya: tolak approve di periode closed, atau ingatkan HR (notifikasi/ringkasan pengajuan pending) sebelum closing otomatis.

## Automation Log
- 21-11-2026: plan ditulis atas permintaan user; eksekusi native ("Native, fast"). Kode `erp-paddy` branch `fix/epic125-pengajuan-sampai-closing`, commit 3a29b47. Tes paket `hris` di DB tes sementara: baseline lulus, `TestSubmissionOpenUntilClosing` RED (409 "sudah lewat cut off") → GREEN, paket lulus. Tanpa migrasi.
- Review akhir (opus): siap merge dengan perbaikan dokumen; 0 Critical, 1 Important (catatan follow-up salah soal hitung ulang payroll + perlu catatan rilis HR) diperbaiki di dokumen ini; komentar usang `leaves.go` diperbaiki (b822882); teks aturan lama di K-19, EPIC-084 R6 dan judul ES-13 diperbarui. Minor ditunda: tes cuti di hari closing dan closing manual belum ada.
- Langkah rilis: PR ke `development`, deploy staging, gladi di salinan production, rilis production atas izin user. Setelah rilis tidak ada langkah data.
- 21-11-2026 09.20 WIB: PR [erp-paddy#40](https://github.com/wit-id-indonesia/erp-paddy/pull/40) di-merge ke `development` (bdd86d7). Backup `/mnt/hdd1/backup/paddydev/staging-db-before-epic125-20261121-091958.dump` (mode 600), staging image `local-20261121-092008`, `/api/ready` 200, tanpa migrasi. Production belum.
- 21-11-2026 09.39 WIB: gladi atas izin user ("Rehearse + release") di salinan production `/mnt/hdd1/backup/paddydev/prod-copy-gladi-epic125-20261121-093902.dump` (mode 600; di production hanya `pg_dump`): 0 migrasi tertunda, verify 12.395 objek 0 selisih; koreksi absen 06-11 ditolak kode lama ("sudah lewat cut off") dan diterima kode baru (201, pending); image baru 092008 dan rollback ke image lama 223604 sehat. Verdict: go.
- **21-11-2026 09.43–09.46 WIB: rilis production ke-6 (EPIC-125) SUKSES.** Backup `~/backups/prod-db-before-rilis-epic125-20261121-0943.dump` (mode 600) dan salinan `~/paddydev-src.before-rilis-epic125-20261121-0943`. PR #41 development→production di-merge (6462b08, tree = bdd86d7). rsync: 5 berkas berubah, 0 dihapus. Deploy `local-20261121-094459`, tanpa migrasi, verify 0 selisih, `/api/ready` dan `/login` 200, kedua container healthy, 0 error di log API. Periode November (closing 22-11) kini menerima pengajuan karyawan sampai akhir hari closing.
