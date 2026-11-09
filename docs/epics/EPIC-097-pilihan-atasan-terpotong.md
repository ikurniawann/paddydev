# EPIC-097: Pilihan Manager (Atasan Langsung) Terpotong 200 Karyawan (`MODULE-HRIS`)

status: review
environment: dev
retries: 0

## Goal
Laporan bug user (09-11-2026):

> beberapa ga bisa di set sbg manager (atasan langsung) … imam nugraha atasannya susmitha, susmintha atasannya teguh arif laksono … ga bisa di tambahin manager-nya, tapi di master data sudah ada jabatannya

**Penyebab.** Pilihan Manager di form karyawan (`fetchUserFormLookups`) mengambil `GET /api/hris/employees?is_active=true&limit=200&sort_by=full_name`. Production punya 317 karyawan aktif dan staging 332, jadi karyawan setelah urutan ke-200 menurut abjad tidak pernah muncul. Di production, urutan ke-200 adalah "Nerzalina…", sehingga Susmitha (S) dan Teguh (T) tidak bisa dipilih. Struktur organisasi memakai `limit=200` yang sama. Tiga combobox lain memakai `limit=500`, batas maksimal API, yang akan terpotong bila karyawan aktif melewati 500.

## Ruling
- **R1.** Helper `fetchAllEmployees` (`frontend/src/lib/hris/fetch-all-employees.ts`) mengambil halaman demi halaman (500 per halaman) sampai `total` tercapai.
- **R2.** Helper ini dipakai di lima tempat:
  - pilihan Manager di form karyawan;
  - struktur organisasi;
  - karyawan aktif di Absensi;
  - pengajuan Cuti;
  - combobox karyawan di form pinjaman/lembur.
- **R3.** Backend tidak diubah. Batas `limit` 1–500 di `ParseDirectoryParams` tetap.

## Acceptance Criteria
- [ ] Di form edit Imam Nugraha, Manager "Susmitha Fauziah" bisa dipilih; di form Susmitha, "Teguh Arif Laksono" bisa dipilih (QA staging).
- [ ] Struktur organisasi memuat semua karyawan aktif.
- [ ] CI hijau.

## Automation Log
- 2026-11-09: Laporan bug.
  - Penyebab dipetakan: `limit=200` di `frontend/src/features/users/api.ts` dan `frontend/src/features/hris/org-chart/api.ts`.
  - Helper + 4 tes; tes terkait 22 file / 86 tes lulus lokal.
  - Suite penuh lokal terhenti karena OOM: host sedang sangat sibuk (load average ±340, build proyek lain). Suite penuh diserahkan ke CI.
