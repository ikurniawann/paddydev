# EPIC-083: Database Cleanup per 20 Oktober 2026 — DM-01 (`MODULE-HRIS`)

status: backlog
environment: dev
retries: 0

## Goal
MOM 29 Oktober 2026 (WIT × Paddy, "Adjustment HRIS PaddyDev (Lanjutan)"):
- §2 No. 6: "Database cleanup per tanggal 20 Oktober 2026 untuk data: pengajuan cuti & izin, lembur, transport, dan pinjaman."
- **DM-01**: "… didahului backup."
- **K-22**: cakupan, hapus permanen/arsip, definisi "transport", dan persetujuan tertulis sebelum eksekusi.

Permintaan user 02-11-2026: "cleanup dump data semua pengajuan, absensi dan lainnya terhitung dari tgl 20 (19, 18, 17, dst.) Oktober ke bawah karena semua sudah ada catatannya".

Jawaban user 02-11 (menjawab K-22):
- **Cakupan**: data bertanggal **≤ 20 Oktober 2026**, yaitu absensi, pengajuan absensi, cuti & izin, lembur, perjalanan dinas ("transport" = perjalanan dinas), dan pinjaman/kasbon.
- **Metode**: **hapus permanen**, didahului backup penuh database dan file lampiran.
- **Pinjaman yang masih berjalan** (disetujui, sisa > 0) **tidak dihapus**.
- **Saldo cuti tetap**: hanya riwayat pengajuan yang dihapus; `leave_balances` tidak diubah.

## Data di staging (salinan struktur production; angka production dilihat saat gladi)
| Data ≤ 20 Okt | Staging |
|---|---|
| `hris.attendance` (`date`) | 56.156 dari 56.972 |
| `hris.attendance_requests` | 11 |
| `hris.leaves` | 660 (approved 480, pending 152, rejected 28); 1 melintasi 20/21 Okt |
| `hris.overtime_requests` (`date`) | 2.694 |
| `hris.business_trips` | 0 |
| `hris.loans` (`requested_date`) | 33: lunas 14, berjalan 15, pending 3, ditolak 1 |

Trigger yang ikut terpicu saat DELETE:
- `trg_notify_attendance_changes` dan `trg_notify_leave_changes` mengirim notifikasi "Data … Dihapus" ke HR **per baris**.
- `attendance_closed_period_guard` menolak penghapusan absensi di periode yang sudah ditutup.

FK `attendance_requests.attendance_id` → `attendance` (SET NULL).

## Ruling
- **R1 Batas tanggal** (inklusif 20 Okt):
  - absensi `date`; pengajuan absensi `date`; lembur `date`;
  - cuti/izin dan perjalanan dinas: `end_date ≤ 20 Okt`. Pengajuan yang melintasi 21 Okt tetap ada (my call: sebagian harinya masuk periode berjalan).
  - pinjaman `requested_date`.
- **R2 Pinjaman**: dihapus bila status `rejected`/`cancelled`/`pending`, atau `approved` dengan `remaining_balance ≤ 0` (lunas). **Disetujui dan sisa > 0 tetap ada.**
- **R3 Saldo cuti**: `hris.leave_balances`, `leave_carry_over_*` dan setting tidak disentuh. Penghapusan langsung SQL (bukan lewat aplikasi), jadi hook penghitung ulang EPIC-068 tidak berjalan.
- **R4 Tanpa notifikasi**: kedua trigger notifikasi dinonaktifkan di dalam transaksi (`ALTER TABLE … DISABLE TRIGGER`), lalu diaktifkan lagi di transaksi yang sama. `SET LOCAL hris.allow_closed_period_write = 'on'` untuk periode tertutup. Tidak memakai `session_replication_role = replica`, supaya FK dan cascade tetap berjalan.
- **R5 Satu transaksi**: semua DELETE berjalan dalam satu transaksi; gagal → tidak ada yang terhapus. Satu baris audit `data_cleanup.dm01` (aktor "Sistem (cleanup DM-01)") berisi jumlah per tabel.
- **R6 File lampiran**: lampiran cuti, lembur, pinjaman, pengajuan absensi, dan foto absensi milik baris yang dihapus ikut dihapus dari volume storage. Sebelumnya file di-tar ke backup. My call: data pribadi ikut hilang bersama datanya.
- **R7 Yang tidak disentuh**:
  - payroll run/detail dan periode absensi;
  - `audit.audit_log` (append-only);
  - notifikasi lama;
  - data master (karyawan, jadwal, saldo).
- **R8 Production** hanya dieksekusi setelah:
  - gladi di salinan production (angka per tabel dan daftar file);
  - **persetujuan tertulis HR** (K-22, Mba Risky/Manager HR) atas angka gladi;
  - izin eksplisit user untuk jam eksekusi (jam sepi).

## Acceptance Criteria
- [ ] Pratinjau baca-saja menampilkan jumlah per tabel, rincian status, dan daftar file yang akan dihapus.
- [ ] Setelah eksekusi:
  - 0 baris ≤ 20 Okt di semua tabel cakupan, kecuali pinjaman berjalan dan cuti yang melintasi 21 Okt;
  - data > 20 Okt utuh;
  - `leave_balances` identik dengan sebelum eksekusi (checksum);
  - 0 notifikasi baru.
- [ ] Backup DB (`pg_dump -Fc`) dan tar file lampiran tersimpan di server dan disalin ke lokal, dan uji restore ke DB sementara berhasil.
- [ ] Payroll run November dan slip lama tetap terbuka tanpa error.

## Automation Log
- 2026-11-02 — MOM 29 Okt dibaca (DM-01, K-22). Data staging diinventaris. Jawaban user dicatat. Rencana: `docs/superpowers/plans/2026-11-02-cleanup-data-20-oktober.md`.
