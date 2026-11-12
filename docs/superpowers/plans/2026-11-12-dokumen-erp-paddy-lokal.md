# Dokumen untuk Kerja di erp-paddy Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Sesi yang bekerja di erp-paddy bisa membaca epic, rencana, dan acuan HRIS, tanpa melanggar aturan repo erp-paddy yang tidak menyimpan `docs/` dan seeder di git.

**Architecture:**
- Dokumen tetap tersimpan dan ber-versi di repo **paddydev** (arsip dokumen, branch `main`).
- Sebuah skrip sinkron di paddydev menyalin `docs/` dan seeder ke `~/WORK/erp-paddy`, di jalur yang sudah diabaikan `.gitignore` erp-paddy.
- Kode baru tetap lewat PR ke erp-paddy `development`.

**Tech Stack:** bash + rsync, git.

**Spec:** keputusan user 12-11-2026:
- "mulai skg remote ganti ke erp-paddy (branch development, oleh agussugiman)";
- "Arsip paddydev + salinan lokal";
- aturan `.gitignore` erp-paddy: `/docs/` ("dokumentasi internal tidak disimpan di repo") dan `apps/backend/database/seeders/` ("Seeder hanya untuk lokal, tidak ikut di-push").

## Global Constraints

- Tidak ada commit, push, atau PR ke erp-paddy dalam rencana ini. `git status` erp-paddy harus tetap bersih.
- Sumber dokumen = paddydev `main`. Salinan di erp-paddy hanya untuk dibaca. Dokumen baru ditulis di paddydev, lalu disinkron.
- Sinkron tidak boleh menghapus file erp-paddy di luar `docs/` dan `apps/backend/database/seeders/`.
- Repo: paddydev = `ikurniawann/paddydev` (remote `paddydev`); erp-paddy = `wit-id-indonesia/erp-paddy`, lokal di `~/WORK/erp-paddy`.

## Review Focus

1. **Skrip dijalankan dari branch paddydev selain `main`:** hanya isi yang sudah di-merge yang tersalin, karena skrip membaca ref `paddydev/main` lewat `git archive`, bukan working tree. `SYNC_REF` bisa sengaja menunjuk ref lain. Test: simulasi dari branch fitur menyalin isi 9c4bb841.
2. **`.gitignore` erp-paddy berubah** (`/docs/` dihapus, ada file ter-track di target, atau aturan negasi): skrip berhenti **sebelum** menyalin. Test: `tools/sync-docs-erp-paddy.test.sh` kasus A dan B.
3. **File dokumen yang dihapus di paddydev** ikut terhapus dari salinan (`rsync --delete`), tetapi hanya di dua folder target. Test: T1 langkah 4.

---

### Task 1: Skrip sinkron dokumen

**Files:**
- Create: `tools/sync-docs-erp-paddy.sh` (di repo paddydev)

**Interfaces:**
- `tools/sync-docs-erp-paddy.sh [ERP_DIR]`. Bawaan `ERP_DIR=$HOME/WORK/erp-paddy`.
- Exit 0 bila sukses dan mencetak `docs: <n> file, seeders: <m> file → <ERP_DIR>`.
- Sumber selalu ref `paddydev/main` (`git fetch` lalu `git archive`), bukan working tree, jadi skrip boleh dijalankan dari branch mana pun.
- Exit 1 dengan pesan, semuanya sebelum ada file yang disalin, bila:
  - `ERP_DIR` bukan akar repo git (subfolder repo lain ikut ditolak): `Bukan akar repo git`;
  - `docs/x.md` atau `apps/backend/database/seeders/x.sql` tidak diabaikan: `Jalur tujuan tidak diabaikan git erp-paddy — batal`;
  - ada file ter-track, atau tak-diabaikan, di kedua target: `Ada file erp-paddy yang dilacak…`;
  - ada file masuk yang tidak diabaikan (aturan negasi): `Sebagian file tujuan tidak diabaikan…`.
- Menyalin `docs/` → `ERP_DIR/docs/` dan `backend/database/seeders/` → `ERP_DIR/apps/backend/database/seeders/`, dengan `rsync -a --delete`.

- [ ] **Step 1:** Tulis skrip dengan `set -euo pipefail`.
- [ ] **Step 2: Uji penolakan dan salin.** `SYNC_REF=refs/remotes/paddydev/main bash tools/sync-docs-erp-paddy.test.sh` → `PASS` untuk A (file ter-track), B (negasi), C (subfolder repo lain), dan D (salin bersih). Exit 0.
- [ ] **Step 3: Commit** `tools: sinkron dokumen paddydev ke salinan lokal erp-paddy`

### Task 2: Catatan arah kerja di registry epic

**Files:**
- Modify: `docs/epics/README.md`. Tambahkan paragraf di bawah judul:
  - sejak 12-11-2026, kode dikerjakan di `wit-id-indonesia/erp-paddy` (PR ke `development`, otomatis ke staging);
  - dokumen tetap di repo ini dan disinkron dengan `tools/sync-docs-erp-paddy.sh`;
  - lokasi epic baru: tetap `docs/epics/` di repo ini.

- [ ] **Step 1:** Edit README, lalu commit `docs: arah kerja pindah ke erp-paddy, dokumen tetap di sini`.

### Task 3: PR, merge, sinkron pertama

- [ ] **Step 1:** Push `docs/erp-paddy-lokal`, buka PR ke paddydev `main`, dan tunggu CI hijau.
- [ ] **Step 2:** Merge, lalu di checkout paddydev yang berada di `main` jalankan `bash tools/sync-docs-erp-paddy.sh`.
  - Output harus menyebut jumlah file `docs` sama dengan `git ls-tree -r --name-only HEAD docs | wc -l`, dan jumlah file seeder sama dengan isi folder seeder.
- [ ] **Step 3: Verifikasi erp-paddy.** `git -C ~/WORK/erp-paddy status --short` harus kosong. `ls ~/WORK/erp-paddy/docs/epics | wc -l` harus sama dengan paddydev.
- [ ] **Step 4: Uji `--delete`.** Buat file uji `docs/qa/_hapus.md` di salinan erp-paddy, lalu jalankan sinkron lagi. File itu harus hilang, dan `git status` erp-paddy tetap kosong.
- [ ] **Step 5: Memori.** Perbarui `remote-erp-paddy`: dokumen dibaca di `~/WORK/erp-paddy/docs`, ditulis di paddydev, lalu jalankan skrip sinkron.
