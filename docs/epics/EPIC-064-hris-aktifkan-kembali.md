# EPIC-064: Aktifkan Kembali Karyawan & Pesan Duplikat Jelas

status: on-progress
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
