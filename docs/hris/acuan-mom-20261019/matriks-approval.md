# Matriks Approval & Struktur Organisasi Paddy

**Sumber acuan:** MOM 19-10-2026 §4.2 dan file `Org_Chart_Paddy_2026_-_Draft_5.xlsx` (SmartArt, 122 node, dikirim 20-10-2026).

## Matriks approval (MOM §4.2)

| Pengajuan | Approver | Ketentuan | Kode sekarang |
|---|---|---|---|
| Time-off (cuti/izin) | Atasan langsung | SPV mengajukan ke atasannya masing-masing | EPIC-063: atasan langsung approver utama (`decided_as = manager`), HR cadangan/override (`decided_as = hr`, audit `leave.approve\|reject`), tanpa self-approval; antrean ESS Persetujuan Tim. HR = `super_admin`/`admin`/`hrd`; `hiring_manager` bukan cadangan HR EPIC-067: jalur HR = aksi `approve` di menu daun Cuti `hris.workforce.leaves` (bukan peran aplikasi `IsHRD`); diberikan ke super_admin, admin, hrd, admin_hris + role HRIS turunan. |
| Overtime | Atasan langsung | Diinput setelah lembur selesai dengan dokumentasi (EPIC-066); atasan langsung menyetujui/mengoreksi jam; jenis Weekday / Weekend | EPIC-066 (mengganti aturan "sebelum jam mulai" EPIC-063): input sendiri baru bisa setelah jam selesai (WIB) selama periode absensi terbuka, istirahat menit, 1–3 dokumentasi wajib; penugasan HR tidak berubah; jenis Weekday/Weekend dari ESS dicek ke jadwal; atasan langsung + HR cadangan (`super_admin`/`admin`/`hrd`), audit `overtime.approve\|reject`; WA atasan saat pengajuan EPIC-067: jalur HR = aksi `approve` di menu daun Lembur `hris.kepegawaian.overtime` (bukan peran aplikasi `IsHRD`); diberikan ke super_admin, admin, hrd, admin_hris + role HRIS turunan. |
| Koreksi Overtime | AdminHRIS | — | EPIC-063: `PATCH /api/hris/overtime/{id}`, aksi `update` menu `hris.kepegawaian.overtime`, audit `overtime.correct`. EPIC-066: AdminHRIS hanya untuk karyawan tanpa atasan langsung yang valid (semua field; HR tetap boleh menetapkan jenis hari saat menyetujui); untuk karyawan beratasan, atasan langsung mengoreksi jam mulai/selesai/istirahat saat menyetujui |
| Kasbon | Manager Departemen **dan** HR (akses Payroll) | Maks. 50% THP; tenor 1 dan 2 | Dua tahap (EPIC-060): Manager Departemen tersimpan (`manager_approver_id`) memutus tahap 1, lalu HR Penggajian (aksi `approve` menu Pinjaman **dan** akses Penggajian) memutus tahap 2. HR tetap boleh menolak kasbon yang masih menunggu manager |
| Absen Manual / Koreksi Absen | HR | Termasuk request absen terlewat (internet mati) dengan keterangan karyawan | EPIC-065: pengajuan karyawan dari ESS → antrean HR (Absensi → Pengajuan Absensi), HRD + menu Absensi, tanpa self-approval, persetujuan menulis lewat penyesuaian HR, audit `attendance_request.*` EPIC-067: jalur HR = aksi `approve` di menu daun Absensi `hris.workforce.attendance` (bukan peran aplikasi `IsHRD`); diberikan ke super_admin, admin, hrd, admin_hris + role HRIS turunan. |
| Perjalanan Dinas | Atasan langsung (HRD cadangan & override) | Rentang ≤ 31 hari, diajukan sebelum berangkat | EPIC-062: satu tahap, `reporting_to` atau HRD; pemohon tidak boleh memutus miliknya sendiri; kuasa HRD (lihat semua, memutus cadangan, mencatat dinas karyawan langsung disetujui, membatalkan) hanya `super_admin`/`admin`/`hrd`, bukan `hiring_manager`; audit tiap keputusan EPIC-067: jalur HR = aksi `approve` di menu daun Absensi `hris.workforce.attendance` (bukan peran aplikasi `IsHRD`); diberikan ke super_admin, admin, hrd, admin_hris + role HRIS turunan. |
| Approval CEO dan GM | CEO, GM | Jenis pengajuan belum jelas (K-7) | Belum ada |

**Sumber "atasan langsung"** di sistem adalah `hris.employees.reporting_to`. Kolom ini bisa diisi lewat import Excel (kolom "NIP Atasan" / `manager_nip`), form karyawan, atau promosi rekrutmen. Org chart di `/dashboard/hris/org-chart` dibangun dari kolom ini, tetapi hanya memuat 200 karyawan pertama.

**"Manager Departemen"**: tabel `hris.departments` tidak punya kolom kepala departemen. Diputuskan lewat Q-APV-2: karyawan aktif berlevel Manager ke atas pertama **yang punya akun login** di rantai `reporting_to` pemohon (penelusuran tetap naik melewati manager tanpa akun), disimpan di `hris.loans.manager_approver_id` saat pengajuan. Tidak ada manager berakun di rantai: tahap 1 dilewati, kasbon langsung ke HR.

## Hierarki org chart (ringkas)

```
CEO
└─ General Manager
   ├─ Operations Manager ─ SCM Supervisor; Warehouse Supervisor ─ Inbound/Outbound/Packing/Logistic/Production Coordinator ─ Staff
   ├─ Finance & Accounting Manager ─ Finance / Accounting / Tax Supervisor ─ Senior Staff ─ Staff ─ Intern
   ├─ HRGA Manager ─ HR Supervisor (Talent Acquisition, HR Ops & Com, People Ops & Development); GA Supervisor
   ├─ Support ─ Corporate Secretary; Internal Audit Supervisor; IT Developer
   ├─ Product R&D Manager ─ Product Designer; Product Development Supervisor ─ Sample Maker Lead ─ Staff
   ├─ Brand & Marketing Manager
   │  ├─ Brand Lead ─ Creative Lead; Art Director; Marcomm Supervisor; Public Relation Supervisor
   │  ├─ Marketing Lead ─ KOL Specialist Supervisor; Performance Marketing Supervisor; CRM Supervisor
   │  └─ Commercial Lead ─ Business Development; Revenue Strategy; Merchandiser
   └─ Retail Operational ─ Store Supervisor (Jabar 1/2, Jabodetabek 1/2, Jateng, Jatim, Makassar) ─ Store Leader ─ Shopkeeper; Admin Store & Control
```

### Temuan untuk aturan "Supervisor ke atas" (Izin Leader) dan approval

- **"Lead" tidak seragam levelnya.** Brand Lead, Marketing Lead, dan Commercial Lead berada **di atas** Supervisor (setara manager); Creative Lead setara Supervisor. Sebaliknya Store Leader dan Sample Maker Lead berada **di bawah** Supervisor. Skrip level 20-10-2026 mengecualikan semua "Lead", tetapi jabatan Brand/Marketing/Commercial/Creative Lead belum ada di master Posisi live, jadi belum ada yang terdampak (Q-APV-1).
- **Jabatan di org chart yang belum ada di master Posisi live**: Operations Manager, Brand & Marketing Manager, Brand/Marketing/Commercial/Creative Lead, Finance/Accounting/Tax Supervisor, Internal Audit Supervisor. Live memakai judul lain (mis. "Creative Marketing Manager", "Finance & Accounting Supervisor").
- **"Support" dan "Retail Operational"** di org chart adalah node kelompok, bukan jabatan. Atasan langsung Store Supervisor perlu ditegaskan (Retail Operation Manager atau GM).

## Data live yang menghambat approval atasan langsung (20-10-2026)

EPIC-063 tidak mengubah data live. HR memperbaikinya lewat kartu "karyawan aktif tanpa atasan langsung" di halaman Karyawan (laporan baca saja) atas persetujuan user.

| Masalah | Jumlah | Dampak |
|---|---|---|
| Karyawan aktif tanpa `reporting_to` | 55 dari 374 (23 `staff`, 15 `Staff`, 12 tanpa jabatan, 4 Store Manager, 1 CEO) | Pengajuan mereka hanya bisa diputus HR |
| `reporting_to` menunjuk diri sendiri | 1 (Retail Operation Manager) | Kode menolak self-approval dan tidak mengirim WA ke dirinya sendiri (EPIC-063); data tetap perlu diperbaiki user |
| Store Manager tanpa atasan | 4 (termasuk akun uji "QA Dimas Dua Entitas") | Tidak ada atasan untuk time-off/lembur |
