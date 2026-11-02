# EPIC-081: NIP Karyawan Diisi Manual, Wajib (`MODULE-HRIS`)

status: review
environment: dev
retries: 0

## Goal
Pertanyaan user 02-11-2026: "untuk NIK (nomor induk karyawan) bisa ga tidak autoincrement?" Jawaban user: **NIP diisi manual dan wajib**, tanpa nomor otomatis.

## Kondisi kode per 02-11 (main `c180650a`)
- Backend sudah menerima NIP manual dan mengecek keunikannya. Nomor otomatis `EMP-YYYY-NNNNN` hanya dibuat bila NIP kosong:
  - `POST /api/users` → `configuration/accounts/employees.go:274-284` (`generateNIP`);
  - `POST /api/hris/employees` → `hris/employees.go:57-77` (`NextAutoNip`).
- Fungsi DB `public.set_employee_nip` (di DB baru tidak terpasang sebagai trigger) mengisi `generate_nip()` bila NIP kosong.
- Form Tambah/Ubah Karyawan (`frontend/src/features/users/components/user-form-page.tsx` + `user-form/*`) **tidak punya kolom NIP**.
- Import Excel sudah memakai NIP dari file.
- Data staging: 314 dari 332 karyawan aktif memakai NIP angka perusahaan (mis. `202509397`); 8 memakai `EMP-…`.

## Ruling
- **R1** Membuat karyawan lewat `POST /api/users` (dengan data karyawan), `POST /api/hris/employees`, **promosi kandidat (`POST /api/hris/promote`)**, atau **baris karyawan baru di import Excel** tanpa NIP → **400 "NIP wajib diisi"** (import: error per baris "NIP wajib diisi", file tidak disimpan selama masih ada error). NIP dipangkas spasinya, panjang maksimal 50, unik (pesan bentrok yang sudah ada tetap dipakai).
- **R2** Form Tambah Karyawan memiliki kolom **NIP** wajib di bagian Kepegawaian/Employment, dengan placeholder contoh "mis. 202511001". Form Ubah juga menampilkan NIP dan bisa mengubahnya; NIP baru dicek unik. Mengosongkan NIP saat ubah ditolak.
- **R3** Fungsi pembuat nomor otomatis di Go (`generateNIP`, `NextAutoNip` beserta `AutoNipsTaken`, `nextEmployeeNIP` import) dihapus. Fungsi DB `set_employee_nip`/`generate_nip` ada di skema tetapi pada DB baru tidak terpasang sebagai trigger; aplikasi tidak bergantung padanya dan tidak diubah. Promosi kandidat memakai fungsi DB `promote_candidate_to_employee` yang membuat nomor sendiri, jadi ada **satu migrasi**: `20261105090000_promote_candidate_manual_nip.sql` menambah overload 6 argumen dengan `p_nip`; versi 5 argumen sengaja dibiarkan (rollback aplikasi dan fallback TS tetap jalan, tidak dipakai Go lagi).
- **R4** NIP karyawan lama (termasuk `EMP-…`) tidak diubah.
- **R5** Akun tanpa data karyawan (akun sistem) tidak terpengaruh, bila jalur itu ada. Dialog **Promote Kandidat Jadi Karyawan** punya kolom **NIP** wajib (placeholder "mis. 202511001"); template import Excel menandai kolom NIP wajib (`*`).

## Acceptance Criteria
- [x] Tambah karyawan tanpa NIP: tombol simpan menampilkan error di kolom NIP; API mengembalikan 400 "NIP wajib diisi".
- [x] Tambah dengan NIP yang sudah dipakai → pesan bentrok yang ada.
- [x] Ubah NIP karyawan → tersimpan, unik; dikosongkan → ditolak.
- [ ] HP 360 px tanpa scroll horizontal. (kolom memakai grid satu kolom + input lebar penuh; belum dicek visual di browser, QA)

## Automation Log
- 2026-11-02 — Analisis jalur NIP; keputusan user dicatat.
- 2026-11-02 — Implementasi (branch `feat/epic081-nip-manual`, dari main `c180650a`), TDD:
  - Go: `POST /api/users` dan `POST /api/hris/employees` menolak NIP kosong/spasi/null dengan 400 "NIP wajib diisi" (dicek sebelum ada baris ditulis); NIP dipangkas, maksimal 50 (accounts: 400 "NIP maksimal 50 karakter"; hris: validasi bentuk yang sudah ada); bentrok memakai pesan `employeekeys` yang ada. `PUT` hanya mengecek NIP bila dikirim: kosong/null ditolak, nilai baru dipangkas lalu dicek unik; tidak dikirim = NIP lama (termasuk `EMP-…`) tidak berubah (R4).
  - R3: dihapus `generateNIP` + port `NIPTaken` (accounts), `NextAutoNip` + `AutoNipsTaken` (hris). Fungsi DB `set_employee_nip` dibiarkan (pada DB baru tidak terpasang sebagai trigger).
  - Frontend: kolom **NIP** (wajib, placeholder "mis. 202511001") di Employment Details form Tambah/Ubah Karyawan; error di kolom (kosong, >50, atau bentrok dari server), payload mengirim NIP dipangkas.
  - Bukti: RED lebih dulu (Go: `TestEmployeeNIPManualRequired` accounts + hris gagal 201/200 vs 400; vitest 7 tes gagal), lalu GREEN; gofmt/vet bersih, tsc + eslint bersih.
  - Fallback TypeScript (`lib/users/user-service.ts`, `lib/hris/employees-repo.ts`, `lib/hris/promote-candidate.ts`) masih punya generator tetapi tidak terjangkau karena `/api/users`, `/api/hris` dilayani Go (rute POST terdaftar di `go-routes.generated.json`); dibiarkan.
- 2026-11-02 — Perluasan atas keputusan user ("tidak autoincrement"): import Excel dan promosi kandidat ikut manual.
  - Import Excel (`hris/domain/employee_import.go`, `hris/employee_excel.go`): baris karyawan baru tanpa NIP (mode Tambah, juga Timpa) menjadi error baris "NIP wajib diisi" (mode Timpa memakai pesan lama "wajib diisi di mode Timpa"); `nextEmployeeNIP` dihapus, penulisan baris menolak NIP kosong sebagai pengaman. Kolom NIP ditandai wajib di template dan catatannya tidak lagi menyebut "dibuat otomatis".
  - Promosi kandidat: `POST /api/hris/promote` menerima `nip` (dipangkas, wajib, maks 50, cek unik lewat pesan bentrok yang ada) setelah pengecekan kandidat/status; migrasi `20261105090000_promote_candidate_manual_nip.sql` (overload `p_nip`, versi lama dipertahankan). `PromoteCandidateButton`: kolom NIP wajib, error di kolom (kosong/bentrok server), teks "NIP akan digenerate otomatis" dihapus.
  - Bukti RED lebih dulu: domain `TestValidateEmployeeImportAdd` (error NIP baris 8 dan 9 tidak ada), `TestPromote` (200 menghasilkan `EMP-2026-00001` padahal diharapkan 400), vitest `PromoteCandidateButton.test.tsx` 4 gagal; lalu GREEN (hris, recruitment, accounts, app; vitest).
  - **Rilis**: migrasi `20261105090000` harus berjalan (`migrate -apply`) sebelum/bersama deploy Go, kalau tidak promosi gagal karena fungsi 6 argumen belum ada.
- 2026-11-02 — Review: satu pembersih NIP bersama (`validate.CleanNIP` murni + `employeekeys.CleanNIP`: pangkas, kosong → "NIP wajib diisi", > 50 unit UTF-16 setelah pangkas → "NIP maksimal 50 karakter") dipakai accounts, hris buat/ubah, promosi kandidat, dan validasi import Excel (sebelumnya import tanpa batas 50); batas panjang hris kini dicek setelah pangkas. Dialog promosi mereset NIP/galat saat ditutup. Tes: PUT dengan NIP milik sendiri sukses di kedua endpoint; NIP 50 + spasi diterima, 51 ditolak; import NIP 52 karakter ditolak.
- 2026-11-02 — **Review independen** (Opus): siap merge dengan perbaikan.
  - Berkas biner `backend/.swarm` dikeluarkan dari commit yang belum dipush; `.swarm/` dan `.claude-flow/` masuk `.gitignore`.
  - Pembersih NIP bersama `employeekeys.CleanNIP` dipakai di akun, HRIS, promosi, dan import. Import kini juga dibatasi 50 karakter.
  - Dialog promosi direset saat ditutup.
  - Tes PUT dengan NIP milik sendiri.
- Gates:
  - Go: gofmt 0, vet, seluruh `./internal/...` lulus 2× berturut-turut.
  - Frontend: tsc 0, eslint, vitest 6429 tes.
- Data staging: tidak ada NIP ganda setelah trim, dan tidak ada NIP kosong.
- **Urutan deploy**: migrasi `20261105090000` harus jalan bersama Go (deploy script menjalankan migrasi sebelum container baru).

