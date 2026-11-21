# EPIC-125: Pengajuan Karyawan Terbuka sampai Closing Periode (`MODULE-HRIS`)

status: review
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

## Follow-up
- Approve cuti/lembur untuk tanggal di periode yang sudah closed belum dijaga (sudah begitu sebelum EPIC-125). Karena pengajuan kini bisa masuk sampai hari closing, kasus ini lebih mungkin terjadi: periksa pengajuan pending sebelum closing, lalu hitung ulang payroll sebelum ditandai dibayar.

## Automation Log
- 21-11-2026: plan ditulis atas permintaan user; eksekusi native ("Native, fast"). Kode `erp-paddy` branch `fix/epic125-pengajuan-sampai-closing`, commit 3a29b47. Tes paket `hris` di DB tes sementara: baseline lulus, `TestSubmissionOpenUntilClosing` RED (409 "sudah lewat cut off") → GREEN, paket lulus. Tanpa migrasi.
- Langkah rilis: PR ke `development`, deploy staging, gladi di salinan production, rilis production atas izin user. Setelah rilis tidak ada langkah data.
