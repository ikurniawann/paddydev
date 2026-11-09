# EPIC-090: Riwayat Persetujuan Tim untuk Atasan (`MODULE-HRIS`)

status: review
environment: dev
retries: 0

## Goal
Permintaan user 05-11-2026: "pastikan atasan dan HR bisa melihat cuti dan lembur karyawan ini dengan melihat detail".

QA staging menunjukkan HR sudah bisa melihat semua status, tetapi atasan hanya bisa membuka Detail selama pengajuan **menunggu keputusannya**. Setelah atasan memutus, atau setelah cuti berlampiran pindah ke tahap verifikasi HR (EPIC-088), pengajuan itu hilang dari Persetujuan Tim dan atasan tidak bisa melihatnya lagi. Jawaban user: "Ya, buat Riwayat".

## Ruling
- **R1 API.** `GET /api/hris/me/team-history` mengembalikan `{ period: { from, to, label }, leaves, overtime }`:
  - isinya cuti/izin dan lembur **bawahan langsung** (`reporting_to` = akun; atasan dirinya sendiri tidak dihitung, seperti EPIC-063 R2);
  - semua status dan sumber;
  - cuti yang `end_date`-nya ≥ **awal periode absensi sebelumnya** (`periodContaining`), dan lembur yang `date`-nya ≥ tanggal itu. **Tanpa batas atas** (review akhir): cuti yang diajukan jauh hari untuk tanggal setelah periode berjalan, dan cuti panjang yang masih berjalan, tetap tampil setelah atasan memutusnya. `period.to` hanya informasi;
  - urut tanggal terbaru dulu, maksimal 300 per jenis.

  Akun tanpa data karyawan mendapat daftar kosong. Kolom:
  - cuti: id, leave_type, start/end, total_days, reason, attachment_url, status, attachment_review, attachment_return_reason, rejection_reason, decided_by_name, manager_approved_name, attachment_reviewed_name, created_at, employee;
  - lembur: id, date, start/end, hours, break_minutes, attachments, day_type, reason, source, status, rejection_reason, decided_by_name, created_at, employee.
- **R2 Tampilan.** Persetujuan Tim mendapat pilihan **"Menunggu" | "Riwayat"**. Isi "Menunggu" tidak berubah.
  - "Riwayat" menampilkan "sejak {awal periode lalu}" lalu daftar Cuti & Izin dan Lembur dengan lencana status:
    - Menunggu atasan / Menunggu verifikasi HR / Lampiran dikembalikan;
    - Disetujui / Ditolak / Dibatalkan.
  - Tombol **Detail** membuka dialog baca-saja: data pengajuan, alasan, lampiran/dokumentasi, dan catatan keputusan ("Disetujui oleh …", "Disetujui atasan …", "Diverifikasi HR …", "Alasan penolakan …").
  - Keputusan di tab Menunggu menyegarkan Riwayat.
- **Di luar cakupan**: bawahan tidak langsung, filter/pencarian, rentang lebih dari 2 periode, ekspor.

## Acceptance Criteria
- [x] Atasan melihat cuti & lembur bawahan langsung semua status (2 periode) dan bisa membuka Detail beserta lampirannya.
- [x] Bukan bawahan dan di luar rentang tidak tampil.
- [x] Gates hijau; QA staging (HP + desktop).

## Automation Log
- 2026-11-05 — Spesifikasi ditulis setelah QA detail (26/26 untuk pengajuan menunggu). Worktree `.worktrees/epic090`, branch `feat/epic090-riwayat-tim`, ditumpuk di atas EPIC-089 (#77).
- 2026-11-05 — Implementasi (`b1a30196` API, `d7def126` tab Riwayat).
  - Gates: `go test -p 1 ./internal/modules/hris/... ./internal/app/` ok, vet bersih, vitest 4521 lulus, tsc + eslint bersih.
- 2026-11-05 — Review akhir (reviewer terpisah): 0 Critical, 2 Important, 5 Minor.
  - **Important 1**: rentang ber-batas atas menyembunyikan cuti yang diajukan jauh hari (tanggal setelah periode berjalan) setelah diputus. Diperbaiki: tanpa batas atas, cuti dihitung dari `end_date` (`9850d1a2`), R1 diperbarui.
  - **Important 2**: tes cutoff 20 dan atasan dirinya sendiri ditambah (`TestTeamHistoryCutoff20`).
  - Ditunda:
    - akun tanpa karyawan tetap menghitung periode sebelum dikembalikan kosong;
    - batas 300 baris tanpa penanda terpotong;
    - `fetchTeamHistory` tanpa fallback data kosong;
    - "Disetujui atasan" tidak tampil bila HR kemudian menolak cuti yang menunggu verifikasi.
- 2026-11-05 — **Staging** `local-20261105-145019` (gabungan lokal EPIC-087 + EPIC-090, yang memuat #76 dan #77; merge `bf7999aa` tidak di-push).
  - Backup `staging-db-before-epic090b-20261105-145018.dump`.
  - QA lulus 19/19 (atasan desktop + HP):
    - setelah atasan menyetujui, tab Riwayat menampilkan cuti "Menunggu verifikasi HR", cuti 25 Nov (setelah periode berjalan), dan lembur "Disetujui";
    - Detail baca-saja memuat alasan, catatan keputusan, dan lampiran (200);
    - setelah HR verifikasi, HP menampilkan "Disetujui oleh … / Lampiran diverifikasi HR: …";
    - 0 error API dan console.
  - Data demo dan berkas dihapus.

