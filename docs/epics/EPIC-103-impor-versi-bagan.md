# EPIC-103: Impor Versi Bagan dari Template Excel — Fase 2b (`MODULE-HRIS`)

status: ready-for-qa
environment: dev
retries: 0

## Goal
Lanjutan EPIC-101/102 (ORG-06). Pilihan user 11-11-2026: "Impor versi bagan".

HR membuat versi bagan baru (misalnya Draft 7) tanpa bantuan developer:
1. unduh template dari versi yang sedang dilihat;
2. ubah di Excel;
3. unggah dan lihat pratinjau: error, peringatan, perubahan dibanding versi dasar, dan perkiraan posisi terisi/kosong;
4. simpan sebagai versi **Draft**, lalu setujui lewat alur EPIC-102.

Q12 (template tabular dari klien) belum dijawab. Template ini jawaban sementara dari kita: file SmartArt klien tidak dibaca otomatis.

Masih ditunda: approver berbasis posisi (ORG-04, Q6).

## Ruling
- **R1 — Tidak ada data lain yang berubah.**
  - Impor hanya menambah baris ke `hris.org_chart_versions` (status `draft`) dan `hris.org_chart_nodes`.
  - Hapus draft hanya menghapus versi draft beserta node-nya (cascade).
  - Karyawan, jabatan, toko, `reporting_to`, alias jabatan, dan approval tidak berubah.
- **R2 — Template = versi yang sedang dilihat.** `GET /api/hris/org-chart/template?version=` membuat workbook berisi:
  - sheet **Bagan** dengan satu baris per node, urut seperti bagan;
  - sheet **Petunjuk** yang menjelaskan nilai yang diterima.

  Kolom sheet Bagan, berurutan: Kode, Kode Atasan, Jabatan, Posisi, Kode Divisi, Divisi, Level, Jenis Kerja, Vendor, Region, Toko, Status, Perlu Konfirmasi.
- **R3 — Nilai yang diterima** (tidak peka huruf besar/kecil; spasi di tepi diabaikan):
  - **Kode:** wajib, `^[A-Z0-9][A-Z0-9-]{1,29}$` setelah diubah ke huruf besar, unik di file.
  - **Kode Atasan:** kosong (puncak), atau kode lain di file yang sama (BR-03).
  - **Jabatan:** wajib, maks. 150 karakter.
  - **Posisi:** `Ya` atau `Tidak` (`Tidak` = node grup).
  - **Kode Divisi:** wajib, `^[A-Z]{2,6}$`.
  - **Divisi:** wajib, maks. 100 karakter.
  - **Level:** kosong atau `L0`–`L8`.
  - **Jenis Kerja:** kosong, `Tetap/Kontrak`, `Magang`, `Freelance`, atau `Outsource`. Kode asli (`PERMANENT_OR_CONTRACT`, `INTERN`, `FREELANCE`, `OUTSOURCE`) juga diterima.
  - **Vendor:** bebas, maks. 100 karakter.
  - **Region:** bebas, maks. 60 karakter.
  - **Toko:** kode toko dipisah koma atau titik koma, masing-masing `^[A-Z0-9]{2,10}$`.
  - **Status:** kosong (= `Aktif`), `Aktif`, `Rencana`, `Kosong`, atau `Dibekukan`.
  - **Perlu Konfirmasi:** kosong, `Ya`, atau `Tidak`.

  Baris yang semua selnya kosong dilewati.
- **R4 — Error memblokir, peringatan perlu dicentang.**
  - **Error:**
    - error per baris (R3);
    - error BR dari `ValidateOrgChart` (puncak ganda, siklus, atasan tidak ada, level, toko frontline, judul intern/freelance, vendor outsource);
    - label kosong atau sudah dipakai;
    - file tanpa baris;
    - lebih dari 1000 baris.
  - **Peringatan:**
    - peringatan BR (level sama dengan atasan);
    - kode toko yang tidak ada sebagai toko aktif di HRIS.
  - Simpan butuh tidak ada error, centang "Saya sudah memeriksa peringatan" bila ada peringatan, dan sidik (fingerprint) yang sama dengan pratinjau.
- **R5 — Label versi:** wajib, maks. 40 karakter, unik (case-insensitive). Contoh: "Draft 7". `source_file` = nama file yang diunggah.
- **R6 — Pratinjau membandingkan dengan versi dasar.** Versi dasar = parameter `base`; bila kosong, versi bawaan (EPIC-102 R7).
  - Ditampilkan: kode yang ditambah, dihapus, dan diubah (jabatan, atasan, level, status, toko, divisi, posisi), serta jumlah keadaan posisi versi baru (Terisi/Kosong/Mungkin terisi/Rencana/Dibekukan) dihitung dengan karyawan aktif saat ini (EPIC-102 R2).
- **R7 — Disimpan sebagai draft, tidak pernah langsung resmi.**
  - Versi baru berstatus `draft`, `imported_at = now()`, dan `sort_order` mengikuti urutan baris.
  - `needs_confirm` diambil dari kolom Perlu Konfirmasi; `raw_title` = Jabatan; `level_confidence` = `manual`; `flags` kosong.
  - Persetujuan tetap lewat EPIC-102.
- **R8 — Hapus versi draft.**
  - Hanya versi `draft` yang bisa dihapus (`DELETE /api/hris/org-chart/versions/{id}`). Versi disetujui atau arsip → 409.
  - Diperlukan supaya impor yang salah tidak menumpuk di pemilih versi.
- **R9 — Akses.**
  - Template: menu `hris.organization` (baca).
  - Impor dan hapus draft: aksi `import` pada menu `hris.organization.org-chart`, untuk role yang sama dengan hak setujui (EPIC-102 R8a: super_admin, admin, hrd, admin_hris, hris_manager).
  - Dua role staf HR tidak mendapat `import` (masker yang sama ditambah `import`). **Keputusan perencanaan, perlu dicek user.**
- **R8a — Versi terakhir tidak bisa dihapus** (temuan review 11-11): 409 "Versi terakhir tidak bisa dihapus — impor versi lain dulu". Dengan begitu template selalu bisa diunduh.
- **R10 — Audit.** Entity `hris.org_chart_versions`:
  - `hris.org_chart.version.import`; after = label, source_file, nodes, positions, base, added, removed, changed;
  - `hris.org_chart.version.delete`; before = label, nodes.

## Tasks
Rencana: `docs/superpowers/plans/2026-11-11-impor-versi-bagan.md`.
- [x] T1 Domain: baca sheet Bagan, validasi baris, selisih dengan versi dasar
- [x] T2 Template Excel + migrasi aksi `import`
- [x] T3 API impor (pratinjau, file error, simpan) + hapus draft
- [x] T4 Frontend: unduh template, dialog impor, hapus draft
- [x] T5 Gate, review, QA staging, catatan epic

## Automation Log
- 2026-11-11: Epic dibuat dari sisa EPIC-101 (ORG-06). Rencana ditulis di branch `feat/epic103-impor-bagan` (bertumpuk di atas PR #97). Menunggu review user.
- 2026-11-11: Implementasi selesai di branch `feat/epic103-impor-bagan` (executing-plans inline, pilihan user "Native").
  - Commit: 9312c4a T1 parser + selisih, f74bf1c T2 template + aksi import, 7733114 T3 API impor + hapus draft, b0ff8a2 T4 frontend, 60d2ac4 perbaikan review.
  - Keputusan eksekusi:
    - Baris yang punya error tidak ikut memicu BR-03 di bawahannya.
    - `level_confidence = manual` hanya diisi untuk posisi.
    - Tombol tetap tampil untuk semua akun; server menolak dengan 403 berbahasa Indonesia.
    - File error langsung terunduh.
  - Review akhir seluruh branch oleh reviewer terpisah: 0 Critical, 1 Important, 7 Minor.
    - Diperbaiki (Important dan tiga Minor yang dinaikkan):
      - versi terakhir tidak bisa dihapus;
      - semua 13 kolom wajib ada;
      - selisih ikut membandingkan Vendor, nama Divisi, dan Perlu Konfirmasi;
      - pratinjau menampilkan Dibekukan.
    - Ditunda (minor):
      - balapan dua label yang hanya beda huruf besar/kecil;
      - node disisipkan satu per satu;
      - tes role belum memastikan baris grant ada;
      - kolom template belum berformat Teks.
  - Gate:
    - backend di DB baru (592 migrasi): gofmt dan vet bersih, 137 paket lulus;
    - frontend: tsc OK, eslint OK, vitest 907 file / 6587 tes lulus.
    - `TestRewardRedeemRateLimit` (memberportal) sempat flaky sekali; tidak terkait.
  - Tes IAM: RED/GREEN di salinan staging; skenario menu tersembunyi tetap tersembunyi; migrasi bisa dijalankan ulang.
  - Staging 11-11 13.46: backup `/mnt/hdd1/backup/paddydev/staging-before-e103-20261111-134601.dump`, image `local-20261111-134609`, migrasi `20261116090200` diterapkan.
  - QA staging:
    - Grant `import`: admin, super_admin, hrd, admin_hris, hris_manager. Role staf HR hanya read.
    - Template Draft 6 berisi 150 baris dan 2 sheet.
    - Diedit dengan openpyxl, lalu pratinjau: 0 error, 3 peringatan BR-04. Selisih: +RTL-900, −RTL-056, RTL-054 Jabatan. Perkiraan: 129 terisi, 9 kosong, 2 mungkin terisi, 9 rencana.
    - Label "draft 6" → ditolak. Simpan tanpa centang → ditolak. Simpan → "Versi QA E103 Draft disimpan sebagai draft (150 node)".
    - Akun baca saja: impor 403, hapus 403, template 200.
    - Setujui QA draft → 200. Hapus versi yang disetujui → 409. Hapus draft → 200. Hapus versi terakhir → 409.
    - Audit import/approve/delete tercatat.
    - Playwright desktop + HP 14/14 lulus, tanpa scroll horizontal. Bukti di `.share/qa-epic103/`.
  - Setelah QA: Draft 6 kembali `draft`; versi, role, user, dan sesi QA dihapus. Baris audit QA tetap ada (append-only).
  - Prod: belum, perlu izin user (bersama EPIC-102, PR #97).
- 2026-11-13: Registry dirapikan, status review → ready-for-qa. Kode sudah ada di erp-paddy `development` (impor monorepo e815cee) dan jalan di staging (image `local-20261112-234046`). Sisa: QA UI oleh HR, lalu rilis production dengan izin user.
