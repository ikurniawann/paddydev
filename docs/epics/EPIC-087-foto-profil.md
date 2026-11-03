# EPIC-087: Upload Foto Profil Akun (`MODULE-IDENTITY`)

status: review
environment: dev
retries: 0

## Goal
Permintaan user 03-11-2026: setelah login, user bisa mengunggah foto profil untuk mengganti avatar bawaan berupa huruf inisial (mis. "P").

### Jawaban user 03-11
- **Penyimpanan**: kolom baru `avatar_url` di akun (`configuration.users`), supaya admin/HR tanpa data karyawan juga bisa. Untuk akun yang terhubung ke karyawan, nilainya disalin ke `hris.employees.photo_url`, sehingga muncul juga di roster absensi, bagan organisasi, daftar user, dan shift tim.
- **Tempat tombol**: popup akun di top bar (klik avatar kanan atas → "Ubah foto profil"), berlaku untuk semua role.
- **Proses foto**:
  - JPG/PNG/WebP maksimal 5 MB;
  - otomatis dipotong persegi di tengah, diperkecil ke 512×512, disimpan sebagai JPEG;
  - ada tombol "Hapus foto" untuk kembali ke inisial.
- **Hak**: hanya akun itu sendiri. HR mengubah foto karyawan lain belum termasuk.

## Kondisi kode per 03-11 (main `614d3694`)
- Avatar inisial di-hardcode di tiga tempat, semuanya `full_name.slice(0,1)`:
  - `frontend/src/components/shared/app-sidebar.tsx:276` (top bar);
  - `app-sidebar.tsx:737` (popup akun);
  - `components/shared/phone-nav.tsx:287` (drawer HP).
- Header Beranda ESS (`features/hris/ess/components/beranda-page.tsx`) menampilkan `initials(emp.full_name)`. `photo_url` sudah ada di data, tetapi tidak dirender.
- Komponen ini sudah memakai `photo_url` dengan fallback inisial: roster absensi, bagan organisasi, shift tim, daftar user, header profil karyawan.
- `configuration.users` belum punya kolom avatar. `hris.employees.photo_url` hanya berupa teks bebas di form HR; tidak ada endpoint unggah.
- Sesi:
  - `GET /api/auth/me` di modul Go `identity` (`service.go:153`, `Me{id, full_name, role, brand_id, email}`);
  - layout dashboard memakai `requireUser()` (`frontend/src/lib/auth/require-user.ts`, `PROFILE_SELECT`).
  - Semua `/api/auth/*` sudah diteruskan ke Go (`backend-routes.ts`).
- Penyimpanan:
  - `storage.Store.Upload(bucket, folder, data, mime, name)` mengembalikan `/api/files/<bucket>/…`;
  - bucket yang tidak ada di daftar publik bersifat **staff**, artinya hanya bisa dibaca akun dashboard yang login (route `frontend/src/app/api/files/[bucket]/[...path]`);
  - `imageproc.Decode` (orientasi EXIF dibereskan; jpeg/png/webp/gif/bmp/tiff), `FitInside`, `Flatten`, `EncodeJPEG`.

## Ruling
- **R1 Data**:
  - delta baru `configuration.users.avatar_url text NULL`;
  - bucket `profile-photos` (akses staff);
  - path `profile-photos/<user_id>/<file>.jpg`.
- **R2 API** (modul `identity`, akun yang sedang login saja):
  - `POST /api/auth/me/avatar` dengan body JSON `{ "image": "<data URL>" }` → 200 `{ data: { avatar_url } }`;
  - `DELETE /api/auth/me/avatar` → 200 `{ data: { avatar_url: null } }`;
  - tanpa sesi → 401 `Not authenticated`.
  - Pesan error:
    - format → "Foto harus berupa gambar JPG, PNG, atau WebP";
    - ukuran → "Ukuran foto maksimal 5 MB".
  - `GET /api/auth/me` menambahkan `avatar_url`.
  - File lama dihapus sesudah file baru tersimpan dan baris ter-update (ganti atau hapus).
  - Satu baris audit `account.avatar` (before/after url).
- **R3 Gambar**:
  - hanya format jpeg/png/webp hasil decode (gif/bmp/tiff ditolak dengan pesan format);
  - dipotong ke persegi di tengah (sisi = sisi terpendek);
  - diperkecil ke maksimal 512×512 (tidak diperbesar);
  - latar transparan diratakan putih;
  - JPEG kualitas 85.
- **R4 Salinan ke karyawan**:
  - lewat port `EmployeePhotos` (adapter di `internal/app`, modul tidak saling import): `SetForUser(ctx, userID, newURL, oldURL *string)`;
  - mengubah `hris.employees.photo_url` milik karyawan dengan `user_id` itu **hanya bila** nilainya kosong atau sama dengan `oldURL`, supaya foto yang diisi HR tidak tertimpa.
  - Saat hapus, `photo_url` dikosongkan hanya bila sama dengan avatar lama.
- **R5 Tampilan**:
  - komponen bersama `UserAvatar({ name, src, size })` (inisial bila `src` kosong atau gagal dimuat) dipakai di top bar, popup akun, drawer HP, dan header Beranda ESS (`photo_url`);
  - `AuthUser.avatar_url` dibaca lewat `requireUser`.
- **R6 UI unggah**:
  - popup akun mendapat item "Ubah foto profil" yang membuka dialog;
  - isi dialog: pratinjau lingkaran, tombol "Pilih foto", "Simpan", dan "Hapus foto" (bila ada foto);
  - validasi klien untuk tipe dan 5 MB dengan pesan sama seperti server;
  - setelah berhasil: `router.refresh()` dan toast "Foto profil diperbarui" / "Foto profil dihapus".
- **Di luar cakupan**:
  - HR mengunggah foto karyawan lain;
  - crop manual;
  - foto di aplikasi mobile.

## Catatan deploy
- **Migrasi dulu**: delta `20261108090000_users_avatar_url` harus diterapkan sebelum binary dan frontend baru jalan. `tools/deploy-paddydev.sh` menerapkan migrasi sebelum mengganti container; bila migrasi gagal, berhenti dan jangan lanjut.
- Pengaman bila tetap terjadi:
  - Go `Profile` jatuh ke select tanpa `avatar_url` (error 42703);
  - Next `requireUser` memakai select tanpa avatar yang tetap membaca `can_central_checkout`.
- Bucket `profile-photos` memakai `STORAGE_DIR` yang sama dengan modul upload Go lainnya.

## Tasks
Rencana: `docs/superpowers/plans/2026-11-03-foto-profil.md`.

## Acceptance Criteria
- [x] Semua role bisa mengunggah dan menghapus foto dari popup akun; avatar top bar, popup, dan drawer HP langsung berganti.
- [x] Karyawan terhubung: foto juga tampil di Beranda ESS, roster absensi, bagan organisasi, dan daftar user. Foto yang diisi HR tidak tertimpa.
- [x] Format/ukuran salah ditolak dengan pesan R2; gambar disimpan 512×512 JPEG persegi.
- [x] Gates hijau; QA staging (desktop + HP).

## Automation Log
- 2026-11-03 — Pemetaan + 4 jawaban user; epic + rencana ditulis. Worktree `.worktrees/epic087`, branch `feat/epic087-foto-profil` dari main `614d3694`.
- 2026-11-03 — Implementasi native, 3 task. Gates: `go vet` + `gofmt` bersih, `go test -p 1 ./...` hijau, vitest 6474 lulus, `tsc` + eslint bersih.
- 2026-11-03 — Review akhir (reviewer terpisah): 0 Critical, 4 Important, 8 Minor. Semua Important diperbaiki dengan tes:
  - batas piksel 40 MP sebelum decode (bom dekompresi);
  - update avatar + audit dalam satu transaksi, langkah sesudah commit tidak ikut batal;
  - file lama tidak dihapus bila salinan ke karyawan gagal;
  - fallback pra-migrasi di Go dan Next, plus catatan deploy.

  Minor diperbaiki:
  - pesan body > 8 MB;
  - hapus tanpa foto tidak menulis audit;
  - pratinjau basi;
  - Beranda dimuat ulang;
  - gaya fallback Beranda.

  Ditunda:
  - avatar di Paddy OS desktop (popup akun, layar kunci) masih inisial;
  - `users-table` / `employee-profile-header` tanpa fallback `onError`;
  - tautan ulang karyawan ke user lain;
  - cache browser 5 menit setelah hapus.
- 2026-11-03 — **Staging** `local-20261103-163801`.
  - Backup `staging-db-before-epic087-20261103-*.dump`. Migrasi: 1 delta, verify 10763 objek, 0 berbeda.
  - QA menemukan layout dashboard tidak meneruskan `avatar_url` ke sidebar; diperbaiki (`2a2d301d`).
  - QA ulang lulus (akun karyawan di desktop, akun HR tanpa karyawan di HP):
    - GIF ditolak "Foto harus berupa gambar JPG, PNG, atau WebP";
    - 6 MB ditolak "Ukuran foto maksimal 5 MB";
    - simpan → foto tampil di top bar, popup, dan Beranda; `hris.employees.photo_url` ikut terisi;
    - file tersimpan 512×512 JPEG, satu file per akun;
    - PNG transparan → putih;
    - hapus foto → kembali inisial;
    - 0 error API dan console.
  - Data demo qa087 dan berkasnya dihapus.

