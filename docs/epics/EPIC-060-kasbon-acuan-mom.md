# EPIC-060: Kasbon Sesuai Acuan MOM 19-10-2026

status: review
environment: dev
retries: 0

## Goal
Kasbon dan pinjaman mengikuti MOM 19-10-2026 dan tabel payslip Paddy, diputuskan user 20-10-2026. Cakupan: Pinjaman Darurat dihapus, plafon kasbon 50% THP (4 komponen), tenor 1 atau 2 (default 2, 50/50), Probation/Freelance/Intern tidak bisa mengajukan, approval dua tahap (Manager Departemen lalu HR Penggajian). Requirement: KP-01, KP-02, AP-05.

Acuan: `docs/hris/acuan-mom-20261019/` (aturan-payroll.md, matriks-approval.md, pertanyaan-rekomendasi.md).

## Tasks
Lihat `docs/superpowers/plans/2026-10-20-kasbon-acuan-mom.md`.

## Acceptance Criteria
- Pengajuan baru `emergency` ditolak 400 dan tidak muncul di form ESS/HR. Riwayat tetap terbaca dengan label lama.
- Kasbon: pokok ≤ 50% THP (Gaji Pokok + Transport + Makan + Jabatan dari struktur gaji aktif); tenor hanya 1 atau 2; default 2.
- Kasbon ditolak untuk status kepegawaian probation, freelance, intern/internship.
- Approval kasbon dua tahap: (1) atasan berlevel Manager ke atas pertama di rantai `reporting_to`; (2) HR dengan aksi `approve` di menu Pinjaman **dan** menu Penggajian. Bila rantai tidak punya Manager, tahap 1 dilewati.
- Manager punya antrean persetujuan kasbon tim di ESS.
- Pinjaman jenis `loan` tidak berubah (aturan menunggu TnC, KP-03).

## Automation Log
- 2026-10-20: epic dibuat. Keputusan user: lihat EPIC-059 (keputusan yang sama). Q-KP-1: 1 Pinjaman Darurat pending di live diputus HR secara manual sebelum rilis.
- 2026-10-20: Implementasi selesai di branch `feat/epic060-kasbon-acuan` (belum push, belum deploy). Commit:
  - `24c8671` Task 1: aturan kasbon (plafon 50% THP, tenor 1/2, status diblokir).
  - `659b1c3` Task 2: tolak Pinjaman Darurat; aturan kasbon ditegakkan saat pengajuan & approve.
  - `9529fcd` Task 3: approval kasbon dua tahap (Manager Departemen lalu HR Penggajian), migrasi `20261021100000`.
  - `0de5a7f` Task 4a: penghitung pending pinjaman mengikuti tahap HR kasbon.
  - `57bd3c9` Task 4b: form kasbon tenor 1/2 tanpa Pinjaman Darurat; antrean persetujuan kasbon manager di ESS.
- Ruling rencana: R1 jenis yang bisa diajukan hanya `kasbon` dan `loan` (`emergency` ditolak 400; riwayat tetap terbaca). R2 THP kasbon = Gaji Pokok + Transport + Makan + Jabatan dari struktur gaji aktif, plafon pokok ≤ 50%. R3 tenor kasbon 1 atau 2 (default 2, 50/50); `loan` tetap aturan lama (KP-03). R4 status diblokir: probation, freelance, intern/internship. R5 approver manager = atasan aktif pertama di rantai `reporting_to` berlevel Manager/Senior Manager/General Manager/Director/C-Level (maks. 10 tingkat, tahan siklus), disimpan di `manager_approver_id`; tidak ada = tahap 1 dilewati. R6 tahap HR butuh `approve` di menu Pinjaman **dan** Penggajian untuk kasbon (mengubah keputusan EPIC-057 untuk kasbon saja). R7 kasbon lama (`manager_approver_id` NULL) langsung ke tahap HR, tanpa backfill. R8 tahap manager tidak melihat plafon/angka gaji.
- Ruling controller: (Task 3) status `manager_approved` dihitung sebagai kasbon aktif, mencegah kasbon kedua saat yang pertama menunggu HR. (Task 3/4) tombol HR hanya untuk `manager_approved` atau `pending` tanpa `manager_approver_id`; penghitung "Menunggu HR" = `manager_approved` + `pending` tanpa manager; filter status HR memuat `manager_approved`. (Task 3) route fallback Next `/api/hris/loans/[id]/approve` tidak diselaraskan: Go melayani `/api/hris` selama `BACKEND_URL` ada (konfigurasi live); bila Go mati, tahap 1 dan R6 bisa terlewati lewat fallback (follow-up). (Task 4) tenor di luar 1/2 di-reset ke 2 saat jenis berganti ke kasbon.
- Bukti test (DB test 55499): Go full suite gagal 27 nama, sama persis dengan baseline `main` (`diff` kosong; semua kegagalan sudah ada di `main`, bukan dari EPIC-060). Vitest 761 file lolos / 2 dilewati, 5531 tes lolos / 10 dilewati. `tsc --noEmit` bersih.
- Langkah rilis: (1) HR memutus manual 1 Pinjaman Darurat pending di live **sebelum** rilis (Q-KP-1); (2) terapkan migrasi `20261021100000`; (3) build ulang API dan web; (4) kasbon pending lama langsung ke tahap HR (R7). Backup DB sebelum migrasi. Tidak ada perubahan ke DB live pada tahap implementasi.
- Minor yang ditunda: ValidateKasbon mencetak activeCount/maxActive sebagai float64 (`%v`); principal <= 0/NaN tidak divalidasi di ValidateKasbon (ditangani validasi input handler); test iterasi map membuat urutan kegagalan nondeterministik; belum ada test status berubah (contract ke probation) antara pengajuan dan approve; `loanLimits` menghitung sum(base_salary) yang tak dipakai kasbon; `decideLoan` cabang kasbon tidak cek `emp.IsActive` (sama dengan perilaku lama `loan`); 404 vs 403 membocorkan keberadaan id pinjaman (UUID, dampak rendah); kasbon bisa macet `pending` bila manager nonaktif/keluar (HR tidak bisa override; follow-up override HR atau re-resolve approver); satu orang bisa menyetujui tahap 1 dan 2 bila manager juga HR Penggajian (separation of duties, tanyakan user); SQL `trim()` hanya spasi vs Go `TrimSpace` semua whitespace; belum ada test langsung `isAwaitingHr` dan render "Menunggu Manager"; `LOAN_STATUS_LABELS` terduplikasi di dua `STATUS_BADGES`; keputusan manager tidak meng-invalidate query HR/badge (refetch berikutnya).
- Follow-up di luar epic: route fallback Next untuk pinjaman (`frontend/src/lib/payroll/loan-requests.ts`) belum menegakkan aturan kasbon; audit log (LA-01) untuk keputusan pinjaman tahap manager & HR; aturan jenis `loan` menunggu TnC (KP-03); notifikasi WhatsApp ke approver manager saat kasbon diajukan.
- 2026-10-20: Gelombang perbaikan review akhir: (1) HR boleh menolak kasbon yang menunggu manager (hanya approve diblok), approver tahap 1 wajib punya akun login (`m.user_id IS NOT NULL`, penelusuran lanjut melewati manager tanpa akun); test `TestKasbonHRCanRejectWhileWaitingManager`, `TestLoanApproverSQL` (kasus manager tanpa akun). (2) Test kunci `TestKasbonTHPSumsAllActiveSalaryRows` (THP dari dua baris gaji aktif; langsung lolos, perilaku sudah benar). (3) Badge ESS `manager_approved` "Disetujui Manager" (`ess-view.test.ts`). (4) Penghitung pinjaman menunggu HR di `overview.ts` (test SQL di `overview.test.ts`) dan `notifications-watcher.ts` (tanpa test, query internal) memakai definisi yang sama. (5) Dokumen acuan `matriks-approval`, `status-requirement` KP-03, `aturan-payroll` diselaraskan. Mengurangi minor "kasbon macet bila manager nonaktif": HR kini bisa menolak (approve tetap menunggu manager).
- 2026-10-21 07:14 WIB: **Deploy live** main `5035ae5` (PR #4–#8), image `local-20261021-071158`. Backup sebelum deploy: `~/Desktop/paddydev-backups/paddydev-db-before-epic059-062-deploy-20261021-071142.dump`. 9 migrasi diterapkan (20261020150000 s.d. 20261023090100), cek skema 0 berbeda; `SELECT iam.sync_hris_staff_roles()` dijalankan (grant aktif admin_hris 57, hris_manager 57, hris_non_payroll 52, hris_non_rekrutmen_kpi 40; hak hapus daun HRIS hanya hris_manager = 18). Container sehat tanpa restart; /login, /api/ready 200. Pinjaman Darurat pending satu-satunya (keperluan "test", dibuat 20-10 lewat aplikasi, bukan data Odoo) ditolak langsung di DB atas instruksi user sebelum deploy.
