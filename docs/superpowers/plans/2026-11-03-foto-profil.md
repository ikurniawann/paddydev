# Upload Foto Profil (EPIC-087) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Setiap akun yang login bisa mengunggah dan menghapus foto profilnya dari popup akun. Foto itu menggantikan avatar inisial di seluruh aplikasi.

**Architecture:**
- Modul Go `identity`:
  - pemrosesan gambar murni di `identity/domain/avatar.go`;
  - endpoint `POST`/`DELETE /api/auth/me/avatar` menyimpan file ke bucket staff `profile-photos` dan menulis `configuration.users.avatar_url`;
  - salinan ke `hris.employees.photo_url` lewat port `EmployeePhotos` yang diadaptasi di `internal/app`.
- Frontend:
  - komponen bersama `UserAvatar`;
  - dialog unggah di popup akun.

**Tech Stack:** Go (identity, platform/imageproc, platform/storage), PostgreSQL delta, Next.js/TS (vitest).

**Spec:** `docs/epics/EPIC-087-foto-profil.md` (R1–R6).

## Global Constraints
- Bucket `profile-photos`; folder = user id; output JPEG 512×512 maksimal, kualitas 85.
- Batas file 5 MB (`5 << 20` byte data hasil decode base64).
- Pesan error persis:
  - "Foto harus berupa gambar JPG, PNG, atau WebP";
  - "Ukuran foto maksimal 5 MB";
  - "Not authenticated".
- Toast: "Foto profil diperbarui" / "Foto profil dihapus". Item menu: "Ubah foto profil".
- Delta: `backend/database/migrations/deltas/20261108090000_users_avatar_url.sql`.
- Tes integrasi: `TEST_DATABASE_URL=…/paddy_test_e084`, `go test -p 1`. Setelah menambah route jalankan `make -C backend generate` (CI memeriksa `frontend/src/lib/go-routes.generated.json`).
- Trailer commit `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`. Jangan commit `.swarm`, `.claude-flow`, atau symlink `node_modules`.

## Review Focus
1. **Foto HP berorientasi EXIF** (diambil tegak): hasil tetap tegak. `imageproc.Decode` sudah memutar; uji dengan JPEG ber-EXIF orientation 6 di Task 1.
2. **Gambar kecil** (< 512 px): tidak diperbesar, tetap persegi. Uji di Task 1.
3. **PNG transparan**: latar putih, bukan hitam. Uji di Task 1.
4. **Ganti foto dua kali**: file lama terhapus dari storage dan tidak menumpuk. Uji di Task 2.
5. **Karyawan yang `photo_url`-nya sudah diisi HR**: tidak tertimpa saat unggah, tidak terhapus saat hapus. Uji di Task 2.

---

### Task 1: Pemrosesan gambar avatar (domain murni)

**Files:**
- Create: `backend/internal/modules/identity/domain/avatar.go`
- Test: `backend/internal/modules/identity/domain/avatar_test.go`

**Interfaces:**
- Produces:
  - `const AvatarMaxBytes = 5 << 20`, `AvatarSize = 512`;
  - `var ErrAvatarFormat`, `ErrAvatarTooLarge` (pesan Global Constraints);
  - `func DecodeDataURL(s string) ([]byte, error)`: menerima `data:image/...;base64,…`; selain itu `ErrAvatarFormat`; lebih dari `AvatarMaxBytes` → `ErrAvatarTooLarge`;
  - `func SquareAvatar(data []byte) ([]byte, error)`: `imageproc.Decode`; format selain jpeg/png/webp → `ErrAvatarFormat`; potong persegi di tengah; `FitInside(512,512)`; `Flatten` putih; `EncodeJPEG(85)`.

- [ ] **Step 1: Failing tests** (gambar dibuat di tes dengan `image/png`/`image/jpeg` encoder):
  - 1200×800 PNG → hasil JPEG 512×512; piksel tengah hasil sama warnanya dengan piksel tengah sumber (sumber: kiri merah, tengah hijau, kanan biru → tengah hijau);
  - 300×200 → 200×200 (tidak diperbesar);
  - PNG transparan penuh → piksel (0,0) putih (R,G,B ≥ 250);
  - GIF → `ErrAvatarFormat`;
  - bukan gambar → `ErrAvatarFormat`;
  - `DecodeDataURL` dengan data > 5 MB → `ErrAvatarTooLarge`;
  - `DecodeDataURL("abc")` → `ErrAvatarFormat`;
  - JPEG 400×200 dengan EXIF orientation 6 (fixture byte di tes, atau pakai helper EXIF yang sudah ada di tes `imageproc` bila ada) → hasil 200×200 dari gambar tegak.
- [ ] **Step 2: Run** `go test ./internal/modules/identity/domain/ -run Avatar`. Expect FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run.** Expect PASS.
- [ ] **Step 5: Commit** `feat(identity): proses foto profil — potong persegi 512 JPEG (EPIC-087)`.

### Task 2: Kolom, endpoint, dan salinan ke karyawan

**Files:**
- Create: `backend/database/migrations/deltas/20261108090000_users_avatar_url.sql` (`ALTER TABLE configuration.users ADD COLUMN IF NOT EXISTS avatar_url text`)
- Modify:
  - `backend/internal/modules/identity/{service.go,postgres.go,http.go,module.go}`
  - `backend/internal/app/module_identity.go` + adapter baru `backend/internal/app/adapters_identity_photos.go`
- Test:
  - `backend/internal/modules/identity/avatar_integration_test.go`
  - `backend/internal/app/adapters_identity_photos_test.go`

**Interfaces:**
- Consumes: Task 1.
- Produces:
  - `Me.AvatarURL *string` (json `avatar_url`), dibaca dari `Profile`;
  - port `type EmployeePhotos interface { SetForUser(ctx context.Context, userID string, newURL, oldURL *string) error }`;
  - `identity.New(deps, stalls, photos)`. Tes yang ada memakai no-op;
  - service `func (s *Service) SetAvatar(r *http.Request, dataURL string) (*string, error)` dan `ClearAvatar(r *http.Request) error`;
  - file disimpan dengan `storage.FromEnv()` di `New`. Tes mengganti handler `files` dengan `storage.New(t.TempDir())`.
  - Adapter SQL: `UPDATE hris.employees SET photo_url = $2, updated_at = now() WHERE user_id = $1 AND (photo_url IS NULL OR photo_url = '' OR photo_url = $3)`.

- [ ] **Step 1: Failing tests** `TestAvatarUploadAndClear`:
  - tanpa sesi → 401;
  - data URL GIF → 400 pesan format;
  - PNG valid → 200, `avatar_url` diawali `/api/files/profile-photos/<userID>/`, file ada di temp storage, `GET /api/auth/me` memuat `avatar_url` yang sama;
  - unggah kedua → file pertama tidak ada lagi (Review Focus 4);
  - `DELETE` → `avatar_url` null di `/me`, file terhapus;
  - satu baris audit `account.avatar` per perubahan;
  - port palsu mencatat panggilan `(userID, new, old)`.

  `TestEmployeePhotosAdapter` (paket app, DB uji):
  - karyawan ber-`user_id` dengan `photo_url` NULL → terisi;
  - `photo_url` = "hr.jpg" → tidak berubah (Review Focus 5);
  - hapus dengan old = avatar sebelumnya → NULL;
  - hapus saat `photo_url` milik HR → tetap.
- [ ] **Step 2: Run** `go test -p 1 ./internal/modules/identity/ ./internal/app/ -run 'Avatar|EmployeePhotos'`. Expect FAIL.
- [ ] **Step 3: Implement.** Terapkan delta ke DB uji (`psql -f`). Lalu jalankan `make -C backend generate`.
- [ ] **Step 4: Run** the tests, then `go test -p 1 ./internal/modules/identity/ ./internal/app/`. Perbarui ekspektasi JSON `/api/auth/me` yang ada (tambah `avatar_url`). Expect PASS.
- [ ] **Step 5: Commit** `feat(identity): unggah dan hapus foto profil akun, salin ke karyawan (EPIC-087)`.

### Task 3: Frontend — avatar bersama dan dialog unggah

**Files:**
- Create:
  - `frontend/src/components/shared/user-avatar.tsx`
  - `frontend/src/components/shared/profile-photo-dialog.tsx`
  - `frontend/src/lib/profile-photo.ts`
- Modify:
  - `frontend/src/lib/auth/require-user.ts` (`PROFILE_SELECT` + `PROFILE_SELECT_LEGACY` tambah `avatar_url`; `AuthUser.avatar_url?: string | null`)
  - `frontend/src/components/shared/app-sidebar.tsx` (avatar top bar :276, popup :737, item "Ubah foto profil")
  - `components/shared/phone-nav.tsx` (:287; prop baru `userAvatar`)
  - `features/hris/ess/components/beranda-page.tsx` (header memakai `UserAvatar` dengan `emp.photo_url`)
- Test:
  - `frontend/src/components/shared/user-avatar.test.tsx`
  - `frontend/src/lib/profile-photo.test.ts`
  - `frontend/src/components/shared/profile-photo-dialog.test.tsx`

**Interfaces:**
- Produces:
  - `UserAvatar({ name: string; src?: string | null; size?: number; className?: string })`: `<img>` bulat bila `src`; inisial huruf pertama (huruf besar, "A" bila kosong) bila tidak ada `src` atau `onError`;
  - `validateProfilePhoto(file: { type: string; size: number }): string | null` dengan pesan Global Constraints;
  - `uploadProfilePhoto(dataUrl: string): Promise<string>` dan `deleteProfilePhoto(): Promise<void>` (fetch ke endpoint R2, error memakai pesan server).

- [ ] **Step 1: Failing tests**:
  - `UserAvatar`:
    - tanpa src → teks "P" untuk "Paddy HR";
    - dengan src → `img[src]`;
    - setelah event `error` pada img → kembali "P".
  - `validateProfilePhoto`:
    - gif → pesan format;
    - 6 MB png → pesan ukuran;
    - 1 MB jpeg → null.
  - Dialog (mock `uploadProfilePhoto`, `next/navigation` refresh):
    - pilih file valid → pratinjau;
    - "Simpan" memanggil upload, lalu `router.refresh`, lalu toast "Foto profil diperbarui";
    - "Hapus foto" hanya tampil bila ada foto dan memanggil `deleteProfilePhoto`.
- [ ] **Step 2: Run** `npx vitest run src/components/shared/user-avatar.test.tsx src/lib/profile-photo.test.ts src/components/shared/profile-photo-dialog.test.tsx`. Expect FAIL.
- [ ] **Step 3: Implement.** Ikuti pola dialog dan toast yang sudah ada (`ChangePasswordDialog` di app-sidebar).
- [ ] **Step 4: Run** the tests, then `npx vitest run src/components src/lib src/features/hris/ess`, `npx tsc --noEmit`, and `npx eslint --quiet` on the changed files. Expect PASS.
- [ ] **Step 5: Commit** `feat(frontend): foto profil di top bar, popup akun, drawer HP, dan Beranda (EPIC-087)`.

### Task 4: Review akhir, staging, PR

- [ ] Final review (reviewer terpisah) → perbaikan.
- [ ] Backup staging, lalu deploy branch (migrasi + verify).
- [ ] QA dengan akun demo (dihapus sesudahnya; audit append-only jangan disentuh):
  - akun HR tanpa karyawan dan akun karyawan, desktop + HP;
  - unggah JPG besar dan PNG transparan;
  - avatar berganti di top bar, popup, drawer;
  - Beranda karyawan dan roster absensi menampilkan foto;
  - hapus foto → inisial lagi;
  - GIF dan 6 MB ditolak.
- [ ] Docs: Automation Log epic, `docs/epics/README.md`.
- [ ] PR ke `main`. Merge dan production hanya dengan izin user.
