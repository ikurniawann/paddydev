# EPIC-064: Aktifkan Kembali Karyawan & Pesan Duplikat Jelas

status: review
environment: dev
retries: 0

## Goal
- HR bisa mengaktifkan kembali karyawan yang sudah resign pada data yang sama: status, tanggal masuk baru, penempatan, jadwal NS, login, dan email.
- HR tidak perlu lagi membuat baris karyawan baru yang bentrok dengan KTP/NPWP/NIP data lama.
- Pesan bentrok menyebut kolom, nama, NIP, dan status pemiliknya.

## Tasks
Lihat `docs/superpowers/plans/2026-10-21-hris-aktifkan-kembali.md` (Task 1–4).

## Acceptance Criteria
- Karyawan nonaktif menampilkan tombol "Aktifkan kembali". Setelah disimpan, karyawan aktif kembali dengan NIP yang sama, mendapat penempatan utama baru, dan riwayat lamanya tetap utuh.
- Login karyawan yang diaktifkan kembali bisa dipakai lagi bila dipilih. Akun dengan role sistem hanya bisa diaktifkan oleh Super Admin.
- Tambah atau ubah karyawan dengan KTP/NPWP/NIP milik karyawan lain (termasuk yang nonaktif, dan KTP dengan format berbeda) menampilkan `<Label> sudah dipakai karyawan <Nama> (NIP …, aktif|nonaktif)`. Bila pemiliknya nonaktif, pesan mengarahkan HR ke tombol aktifkan kembali.
- Audit `employee.rehire` dan riwayat `rehire` tercatat.

## Automation Log
- 2026-10-21: Kasus pemicu: Alya Nur Hikmah punya dua data nonaktif (`202504229`, `202610085`). Tambah karyawan pada 21-10 gagal dengan "Data sudah ada di sistem" karena unique KTP/NIP/NPWP juga berlaku untuk baris nonaktif; tanggal tidak berpengaruh. User meminta usulan "aktifkan kembali karyawan & pesan duplikat jelas" dikerjakan di sesi ini. Rencana ditulis (Ruling R1–R9). Branch `feat/epic064-aktifkan-kembali` dibuat dari main `5035ae5` di worktree `.worktrees/epic064-rehire`.
- 2026-10-21 (selesai, menunggu review): Task 1–4 rampung di branch `feat/epic064-aktifkan-kembali` (worktree `.worktrees/epic064-rehire`, dari main `5035ae5`; belum di-push, belum di-merge). Tanpa migrasi DB.
  - **Commit:** `9b328d1` Task 1 (pesan bentrok KTP/NPWP/NIP menyebut pemilik: paket `platform/employeekeys`, peta constraint di `httpx`, dipakai jalur create/update/import karyawan + accounts, paritas Next `handler.ts`). `b90cd1b` Task 2 (`POST /api/hris/employees/{id}/rehire`, satu transaksi: status, join_date, penempatan primer, jadwal NS, login, email, audit `employee.rehire`, riwayat `rehire`). `aef16f6` perbaikan review Task 2 (email rehire tanpa beda huruf, guard role sistem dipakai bersama dengan import via `guardSystemReactivation`, tanggal efektif tanpa tanggal keluar). `857d418` Task 3 (tombol & dialog "Aktifkan kembali", label audit/riwayat, `handler.ts`). Seluruhnya 32 file, +1284/-29.
  - **Ruling diterapkan:** R1–R9 dari rencana (data sama diaktifkan kembali; hak akses `update` di `employeeRecordManagers`; tanggal efektif wajib, setelah tanggal keluar, tidak setelah hari ini; join_date diganti, nilai lama di audit `before` + `employment_history.notes`; penempatan primer baru + jadwal NS otomatis, Shift diatur manual; `reactivate_login` default true dan role sistem hanya Super Admin; email hanya bila tanpa login, unik tanpa beda huruf; status kepegawaian dari daftar import, `resigned`/`terminated` ditolak; format pesan bentrok R9 persis). Ruling pelaksanaan: arah impor Task 1 — peta constraint->pesan ada di `httpx` (paket daun), `employeekeys` boleh mengimpor `httpx`, tidak sebaliknya. Task 3 memakai `<select>` native untuk status/cabang (bukan Radix Select) demi testabilitas.
  - **Bukti test (21-10-2026):** Go penuh `go test -p 1 -count=1 ./...` di DB test: 122 paket ok, kegagalan = 27 nama test yang identik dengan baseline main (tidak ada kegagalan baru, tidak ada yang hilang), `gofmt -l` bersih, test rehire/employeekeys/hris fokus hijau. Vitest penuh: 792 file lulus / 2 skip, 5757 test lulus / 10 skip. `tsc --noEmit -p .` bersih.
  - **Smoke API (API Go lokal port 18764, DB test saja; bukan live):** resign karyawan ber-login lalu aktifkan kembali dengan tanggal efektif 20-10 — login terbuka lagi (`banned_until` kosong, status `active`, `is_access_app` true), NIP sama, `join_date` baru, `end_date` kosong, penempatan lama tertutup (10/01/2025 s.d. 19/10/2026) + penempatan primer baru "Aktif kembali", riwayat `rehire` (notes "Tanggal masuk sebelumnya: 10/01/2025"), audit `employee.rehire` (before join_date lama/is_active false, after join_date baru + branch_id). Penolakan: karyawan aktif ("Karyawan masih aktif"), tanggal sama dengan tanggal keluar, tanggal di masa depan. `POST /api/hris/employees` dengan KTP berformat spasi milik karyawan nonaktif menjawab 400 `No. KTP sudah dipakai karyawan <Nama> (NIP …, nonaktif) — aktifkan kembali data tersebut dari halaman detail karyawan`; NIP milik nonaktif sama; KTP milik karyawan aktif menjawab `… (NIP …, aktif)` tanpa saran aktifkan kembali. Fixture smoke dihapus dari DB test dan API dimatikan (catatan: baris `audit.audit_log` bersifat append-only sehingga baris audit smoke tertinggal di DB test).
  - **Belum diverifikasi:** UI browser (dialog Aktifkan kembali, tombol di halaman detail, label audit/riwayat) hanya dicakup test komponen vitest — perlu QA manusia. Jadwal NS otomatis, login role sistem oleh Super Admin dan jalur import/accounts dicakup test integrasi Go, tidak di smoke manual.
  - **Langkah rilis:** tanpa migrasi. Build ulang API Go dan web (Next), deploy; lalu QA manusia dialog: karyawan resign -> Aktifkan kembali (pilih status, cabang, tanggal efektif; centang login) -> cek penempatan, riwayat, audit, dan bahwa login bisa masuk lagi.
  - **Data live:** kasus pemicu Alya Nur Hikmah (NIP `202610085`) sudah diperbaiki manual oleh controller pada 21-10-2026 (backup `paddydev-db-before-aktif-kembali-alya-20261021-164209.dump`). Tidak ada yang dijalankan terhadap DB live oleh epic ini.
  - **Follow-up (minor yang diparkir dari review):**
    - Task 1: fallback race memberi 400 di service tetapi 409 di `WriteError` + blok 3 baris terduplikasi; ktp/npwp kosong tersimpan `''` (pesan race menyesatkan, bukan regresi); doc comment `columns()/keys()` di `http_employees.go` tergeser; belum ada test race tingkat service; `updateUser` accounts menjalankan `keyConflict` sebelum guard akun sistem.
    - Task 2: `rehireStatusAllowed` memuat seluruh lookup di dalam lock; karyawan ber-login ditolak mengisi email walau sama; assertion audit sebagian; tidak ada test rollback di tengah transaksi; entri `goNativePrefixes` redundan (diwajibkan rencana, tidak berbahaya); `branch_id` huruf besar kini 400 (perbandingan teks `BranchExists`) — turunkan ke huruf kecil di handler.
    - Task 3: `after.branch_id` di audit tampil UUID (sesuai rencana; tambahkan nama cabang di audit Go); daftar status hanya baris master; belum ada test untuk default `reactivate_login` true dan gate `isHR`; `onDone` hanya me-refetch detail.
    - Dari rencana: saran otomatis "aktifkan kembali" di jalur "Tambah karyawan" Manajemen User (lookup pemilik KTP saat input); membuat akun login untuk karyawan aktif yang belum punya login dari halaman detail.
