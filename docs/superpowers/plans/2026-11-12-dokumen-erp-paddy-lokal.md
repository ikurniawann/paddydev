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

1. **Skrip dijalankan dari branch paddydev selain `main`:** skrip menolak berjalan, supaya dokumen yang belum di-merge tidak tersalin. Test: T1 langkah 2.
2. **`.gitignore` erp-paddy berubah** (misalnya `/docs/` dihapus): skrip berhenti sebelum menyalin, supaya ratusan file tidak muncul sebagai perubahan di erp-paddy. Test: T1 langkah 2.
3. **File dokumen yang dihapus di paddydev** ikut terhapus dari salinan (`rsync --delete`), tetapi hanya di dua folder target. Test: T1 langkah 4.

---

### Task 1: Skrip sinkron dokumen

**Files:**
- Create: `tools/sync-docs-erp-paddy.sh` (di repo paddydev)

**Interfaces:**
- `tools/sync-docs-erp-paddy.sh [ERP_DIR]`. Bawaan `ERP_DIR=$HOME/WORK/erp-paddy`.
- Exit 0 bila sukses dan mencetak `docs: <n> file, seeders: <m> file → <ERP_DIR>`.
- Exit 1 dengan pesan bila:
  - branch paddydev saat ini bukan `main` atau worktree tidak bersih: `Jalankan dari paddydev main yang bersih`;
  - `git -C ERP_DIR check-ignore docs/x.md` atau `apps/backend/database/seeders/x.sql` gagal: `Jalur tujuan tidak diabaikan git erp-paddy — batal`.
- Menyalin `docs/` → `ERP_DIR/docs/` dan `backend/database/seeders/` → `ERP_DIR/apps/backend/database/seeders/`, dengan `rsync -a --delete`.

- [ ] **Step 1:** Tulis skrip dengan `set -euo pipefail`.
- [ ] **Step 2: Uji penolakan.**
  - Jalankan dari worktree branch `docs/erp-paddy-lokal`. Hasilnya harus exit 1 dengan pesan pertama.
  - Jalankan dengan `ERP_DIR` sebuah repo git kosong sementara yang tidak mengabaikan `docs/`. Hasilnya harus exit 1 dengan pesan kedua.
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
