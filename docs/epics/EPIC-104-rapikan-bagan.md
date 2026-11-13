# EPIC-104: Rapikan Bagan Organisasi — Minor EPIC-102 & EPIC-103 (`MODULE-HRIS`)

status: ready-for-qa
environment: dev
retries: 0

## Goal
Pilihan user 11-11-2026: "Rapikan minor 102+103". Menyelesaikan 10 temuan minor yang ditunda dari review akhir EPIC-102 dan EPIC-103. Tidak ada fitur baru, dan tidak bergantung pada jawaban klien. ORG-04 (Q6) tetap ditunda.

## Ruling
- **R1 — Tidak ada data lain yang berubah.** Sama seperti EPIC-102/103 R1. Satu-satunya perubahan skema adalah indeks unik `lower(label)` (R3).
- **R2 — Kalah balapan saat menyetujui.** Bila versi lain baru saja disetujui oleh transaksi lain (pelanggaran indeks unik), jawabannya 409 `Versi lain baru saja disetujui — muat ulang lalu coba lagi`. Pesan ini menggantikan "sudah tidak berstatus draft", yang keliru karena versi itu masih draft. Diuji dengan dua transaksi nyata.
- **R3 — Label unik tanpa membedakan huruf besar/kecil di database.** Indeks unik `lower(label)`. Simpan bersamaan "Draft 7" dan "draft 7" hanya menghasilkan satu versi; yang lain mendapat 409 `Label versi sudah dipakai`.
- **R4 — Node disimpan sekaligus.** Satu `INSERT … VALUES` berisi semua node (maks. 1000 × 16 parameter), bukan satu INSERT per node. Perilaku dan urutan sama.
- **R5 — Template berformat Teks.** Semua kolom sheet Bagan diformat Teks (`@`), supaya kode baru yang hanya berisi angka tidak diubah Excel menjadi angka.
- **R6 — Filter "Tanpa region".** Posisi Kosong dan Bagan menawarkan pilihan "Tanpa region" (`region=-`), yaitu node yang region-nya kosong (HQ).
- **R7 — Tombol mengikuti hak.**
  - `GET /api/hris/org-chart/access` mengembalikan `{approve, import}` untuk akun yang sedang masuk.
  - "Setujui versi" hanya tampil bila `approve`. "Impor versi" dan "Hapus draft" hanya tampil bila `import`. "Unduh template" tetap untuk semua pembaca.
  - Server tetap menolak dengan 403; UI hanya menyembunyikan.
- **R8 — Tanggal bawaan WIB.** Tanggal berlaku bawaan di dialog Setujui adalah hari ini menurut WIB, sama dengan bawaan server, bukan menurut zona waktu perangkat.
- **R6a — Region "-" di template = kosong** (temuan review 11-11). Sel Region berisi `-`, `–`, atau `—` dibaca sebagai tanpa region, supaya tidak bentrok dengan filter "Tanpa region".
- **R9 — Tes lebih ketat.**
  - `TestOrgChartImportRoles` dan `TestOrgChartApproveRoles` memastikan baris grant ada.
  - Sisa baris `_ = iam.HrisOrganization` dihapus.

## Tasks
Rencana: `docs/superpowers/plans/2026-11-11-rapikan-bagan.md`.
- [x] T1 Approve: pesan kalah balapan + tes dua transaksi + bersihkan tes
- [x] T2 Label unik lower + simpan node sekaligus + tes role ketat
- [x] T3 Template kolom Teks
- [x] T4 Filter "Tanpa region"
- [x] T5 Hak tombol + tanggal WIB
- [x] T6 Gate, review, QA staging, catatan

## Automation Log
- 2026-11-11: Epic dibuat dari minor yang ditunda di EPIC-102/103. Branch `feat/epic104-rapikan-bagan` di atas PR #98.
- 2026-11-11: Implementasi selesai (executing-plans inline).
  - Commit: cceeead T1, df54c06 T2, f3f2bff T3, a9bf430 T4, ce4afdc T5, bae7295 perbaikan review.
  - Keputusan eksekusi:
    - Tes konkuren menunggu tx2 tertahan di lock (dicek lewat PID backend-nya) sebelum tx1 commit.
    - Tes WIB memaksa zona perangkat UTC, karena host sendiri berzona WIB.
  - Review akhir seluruh branch oleh reviewer terpisah: 0 Critical, 2 Important, 6 Minor.
    - Diperbaiki: probe lock tes konkuren sekarang per PID; Region "-" dibaca kosong.
    - Ditunda (minor):
      - DELETE audit di cleanup tes tidak berefek (audit append-only);
      - tes konkuren belum mencakup jalur yang sudah punya versi disetujui;
      - `wibToday` menduplikasi `todayWib`;
      - endpoint akses membaca grant dua kali;
      - `versionID` diulang per baris saat insert;
      - bila endpoint akses gagal, tombol hilang tanpa pemberitahuan.
  - Gate:
    - backend di DB baru (593 migrasi): gofmt dan vet bersih, 137 paket lulus, 0 gagal;
    - frontend: tsc OK, eslint OK, vitest 907 file / 6593 tes lulus.
    - Flaky yang tidak terkait: `TestPruneDropsExpiredKeys` (ratelimit), lulus saat diulang.
  - Staging 11-11 14.39: backup `/mnt/hdd1/backup/paddydev/staging-before-e104-20261111-143909.dump`, image `local-20261111-143916`, migrasi `20261116090300` diterapkan.
  - QA staging:
    - Endpoint akses: hrd `{approve:true, import:true}`, akun baca saja `{false, false}`.
    - Indeks `uq_org_chart_versions_label_lower` ada; insert "draft 6" lewat SQL ditolak.
    - `region=-` mengembalikan 95 posisi HQ (cocok dengan grup tanpa region); semua 149.
    - Kolom template berformat `@`.
    - Playwright desktop + HP, akun hrd dan baca saja, 28/28 lulus: tombol mengikuti hak, filter "Tanpa region" berfungsi, tanpa scroll horizontal. Bukti di `.share/qa-epic104/`.
  - Data QA dihapus; Draft 6 tetap `draft`.
  - Prod: belum, perlu izin user (bersama EPIC-102/103).
- 2026-11-13: Registry dirapikan, status review → ready-for-qa. Kode sudah ada di erp-paddy `development` (impor monorepo e815cee) dan jalan di staging (image `local-20261112-234046`). Sisa: QA UI oleh HR, lalu rilis production dengan izin user.
