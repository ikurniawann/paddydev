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
- Trigger DB `public.set_employee_nip` juga mengisi `generate_nip()` bila NIP kosong.
- Form Tambah/Ubah Karyawan (`frontend/src/features/users/components/user-form-page.tsx` + `user-form/*`) **tidak punya kolom NIP**.
- Import Excel sudah memakai NIP dari file.
- Data staging: 314 dari 332 karyawan aktif memakai NIP angka perusahaan (mis. `202509397`); 8 memakai `EMP-…`.

## Ruling
- **R1** Membuat karyawan lewat `POST /api/users` (dengan data karyawan) atau `POST /api/hris/employees` tanpa NIP → **400 "NIP wajib diisi"**. NIP dipangkas spasinya, panjang maksimal 50, unik (pesan bentrok yang sudah ada tetap dipakai).
- **R2** Form Tambah Karyawan memiliki kolom **NIP** wajib di bagian Kepegawaian/Employment, dengan placeholder contoh "mis. 202511001". Form Ubah juga menampilkan NIP dan bisa mengubahnya; NIP baru dicek unik. Mengosongkan NIP saat ubah ditolak.
- **R3** Fungsi pembuat nomor otomatis di Go (`generateNIP`, `NextAutoNip` beserta `AutoNipsTaken`) dihapus bila tidak dipakai lagi. Trigger DB `set_employee_nip` dibiarkan sebagai jaring pengaman untuk penulis lain (seed/skrip); aplikasi tidak lagi bergantung padanya. My call: tidak perlu migrasi.
- **R4** NIP karyawan lama (termasuk `EMP-…`) tidak diubah.
- **R5** Akun tanpa data karyawan (akun sistem) tidak terpengaruh, bila jalur itu ada.

## Acceptance Criteria
- [x] Tambah karyawan tanpa NIP: tombol simpan menampilkan error di kolom NIP; API mengembalikan 400 "NIP wajib diisi".
- [x] Tambah dengan NIP yang sudah dipakai → pesan bentrok yang ada.
- [x] Ubah NIP karyawan → tersimpan, unik; dikosongkan → ditolak.
- [ ] HP 360 px tanpa scroll horizontal. (kolom memakai grid satu kolom + input lebar penuh; belum dicek visual di browser, QA)

## Automation Log
- 2026-11-02 — Analisis jalur NIP; keputusan user dicatat.
- 2026-11-02 — Implementasi (branch `feat/epic081-nip-manual`, dari main `c180650a`), TDD:
  - Go: `POST /api/users` dan `POST /api/hris/employees` menolak NIP kosong/spasi/null dengan 400 "NIP wajib diisi" (dicek sebelum ada baris ditulis); NIP dipangkas, maksimal 50 (accounts: 400 "NIP maksimal 50 karakter"; hris: validasi bentuk yang sudah ada); bentrok memakai pesan `employeekeys` yang ada. `PUT` hanya mengecek NIP bila dikirim: kosong/null ditolak, nilai baru dipangkas lalu dicek unik; tidak dikirim = NIP lama (termasuk `EMP-…`) tidak berubah (R4).
  - R3: dihapus `generateNIP` + port `NIPTaken` (accounts), `NextAutoNip` + `AutoNipsTaken` (hris). Trigger DB `set_employee_nip` dibiarkan, tanpa migrasi.
  - Frontend: kolom **NIP** (wajib, placeholder "mis. 202511001") di Employment Details form Tambah/Ubah Karyawan; error di kolom (kosong, >50, atau bentrok dari server), payload mengirim NIP dipangkas.
  - Bukti: RED lebih dulu (Go: `TestEmployeeNIPManualRequired` accounts + hris gagal 201/200 vs 400; vitest 7 tes gagal), lalu GREEN; gofmt/vet bersih, tsc + eslint bersih.
  - Di luar lingkup (tidak diubah): import Excel masih membuat `EMP-…` untuk baris tanpa NIP (`hris/employee_excel.go` `nextEmployeeNIP`), promosi kandidat rekrutmen (adapter recruitment) masih membuat `EMP-…`, serta fallback TypeScript (`lib/users/user-service.ts`, `lib/hris/employees-repo.ts`, `PromoteCandidateButton`) yang tidak terjangkau karena `/api/users` dan `/api/hris` dilayani Go.
