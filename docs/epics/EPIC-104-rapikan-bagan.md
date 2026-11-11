# EPIC-104: Rapikan Bagan Organisasi — Minor EPIC-102 & EPIC-103 (`MODULE-HRIS`)

status: backlog
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
- **R9 — Tes lebih ketat.**
  - `TestOrgChartImportRoles` dan `TestOrgChartApproveRoles` memastikan baris grant ada.
  - Sisa baris `_ = iam.HrisOrganization` dihapus.

## Tasks
Rencana: `docs/superpowers/plans/2026-11-11-rapikan-bagan.md`.
- [ ] T1 Approve: pesan kalah balapan + tes dua transaksi + bersihkan tes
- [ ] T2 Label unik lower + simpan node sekaligus + tes role ketat
- [ ] T3 Template kolom Teks
- [ ] T4 Filter "Tanpa region"
- [ ] T5 Hak tombol + tanggal WIB
- [ ] T6 Gate, review, QA staging, catatan

## Automation Log
- 2026-11-11: Epic dibuat dari minor yang ditunda di EPIC-102/103. Branch `feat/epic104-rapikan-bagan` di atas PR #98.
