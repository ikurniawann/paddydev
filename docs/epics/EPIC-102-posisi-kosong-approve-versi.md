# EPIC-102: Posisi Kosong & Persetujuan Versi Bagan — Fase 2a (`MODULE-HRIS`)

status: backlog
environment: dev
retries: 0

## Goal
Lanjutan EPIC-101. Pilihan user 10-11-2026: dari sisa fase 2, kerjakan dulu **dashboard posisi kosong (ORG-08)** dan **persetujuan versi bagan**.
- **Posisi Kosong:** HR melihat slot bagan yang belum ada pemegangnya, per divisi dan region, lalu mengunduhnya sebagai Excel.
- **Persetujuan versi:** HR menyetujui satu versi bagan sebagai versi resmi, dengan tanggal berlaku.

Masih ditunda (bergantung pada jawaban klien): importer Draft 7 (ORG-06, Q12) dan approver berbasis posisi (ORG-04, Q6).

## Ruling
- **R1 — Tidak ada data lain yang berubah.** Karyawan, jabatan, departemen, toko, `reporting_to`, dan alur approval tetap seperti EPIC-101 R1. Persetujuan versi hanya mengubah `hris.org_chart_versions`.
- **R2 — Keadaan setiap posisi dihitung, tidak disimpan.** Keadaan dihitung dari pencocokan R9 EPIC-101. Kolom `org_chart_nodes.status` tidak ditulis. Urutan penilaian:
  1. node grup → tidak dihitung;
  2. `frozen` → **Dibekukan**;
  3. punya pemegang → **Terisi**;
  4. `planned` → **Rencana**;
  5. ada karyawan ambigu yang salah satu kandidatnya node ini → **Mungkin terisi**;
  6. selain itu (`active` atau `vacant`) → **Kosong**.
- **R3 — Karyawan ambigu tidak membuat posisi tampak kosong.** Kode kandidat disimpan di hasil pencocokan (`candidate_codes`). Nama karyawannya ditampilkan di posisi yang **Mungkin terisi**, supaya HR tidak membuka lowongan untuk slot yang sebenarnya sudah ada orangnya.
- **R4 — Tanpa target headcount** sampai klien menjawab Q10. Satu node = satu slot. Node dengan lebih dari satu pemegang ditampilkan "Terisi N orang" tanpa dinilai kelebihan.
- **R5 — Satu versi disetujui pada satu waktu.**
  - Hanya versi `draft` yang bisa disetujui. Versi `approved` sebelumnya menjadi `archived` dalam transaksi yang sama.
  - Indeks unik parsial menjaga hanya ada satu `approved`. Persetujuan bersamaan menghasilkan 409.
  - Versi yang masih punya **error** validasi BR ditolak (400, daftar error). Peringatan tidak menghalangi.
- **R6 — Tanggal berlaku** (`valid_from`) wajib berupa tanggal, bawaannya hari ini (WIB). Alasan opsional dan ikut ke audit.
- **R7 — Versi bawaan** untuk pohon, selisih, dan posisi kosong adalah versi yang disetujui. Bila belum ada, dipakai impor terbaru (perilaku EPIC-101). Versi lain tetap bisa dipilih lewat `?version=`.
- **R8 — Akses.**
  - Membaca tetap memakai menu `hris.organization`.
  - Menyetujui butuh aksi `approve` pada menu itu (`withAction`). Tidak ada menu baru.
- **R9 — Audit.** `audit.audit_log` mendapat action `hris.org_chart.version.approve`, entity `hris.org_chart_versions`, dengan isi:
  - before: status lama;
  - after: status baru, `valid_from`, label versi yang diarsipkan.

## Tasks
Rencana: `docs/superpowers/plans/2026-11-10-posisi-kosong-approve-versi.md`.
- [ ] T1 Domain: kandidat karyawan ambigu + keadaan posisi + ringkasan per divisi/region
- [ ] T2 API Posisi Kosong (JSON + Excel)
- [ ] T3 Migrasi + API persetujuan versi + versi bawaan
- [ ] T4 Frontend: pemilih versi, dialog Setujui, tab Posisi Kosong
- [ ] T5 Gate, QA staging, catatan epic

## Automation Log
- 2026-11-10: Epic dibuat dari sisa EPIC-101 R2. User memilih "Vacancy + approve versi". Rencana ditulis, menunggu review user.
