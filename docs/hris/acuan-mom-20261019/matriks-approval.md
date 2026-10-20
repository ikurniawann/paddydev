# Matriks Approval & Struktur Organisasi Paddy

**Sumber acuan:** MOM 19-10-2026 §4.2 dan file `Org_Chart_Paddy_2026_-_Draft_5.xlsx` (SmartArt, 122 node, dikirim 20-10-2026).

## Matriks approval (MOM §4.2)

| Pengajuan | Approver | Ketentuan | Kode sekarang |
|---|---|---|---|
| Time-off (cuti/izin) | Atasan langsung | SPV mengajukan ke atasannya masing-masing | Satu tahap: HR **atau** `reporting_to`; atasan tidak punya antrean di ESS |
| Overtime | Atasan langsung | Wajib diajukan sebelum lembur; jenis Weekday / Weekend | Satu tahap: HR atau `reporting_to`; tanggal lampau diterima; jenis hanya regular/rest_day oleh HR |
| Koreksi Overtime | AdminHRIS | — | Hanya lewat koreksi jam lembur di absensi |
| Kasbon | Manager Departemen **dan** HR (akses Payroll) | Maks. 50% THP; tenor 1 dan 2 | Satu tahap: pemegang aksi `approve` menu Pinjaman |
| Absen Manual / Koreksi Absen | HR | Termasuk request absen terlewat (internet mati) dengan keterangan karyawan | HR mengedit langsung; belum ada pengajuan dari karyawan |
| Approval CEO dan GM | CEO, GM | Jenis pengajuan belum jelas (K-7) | Belum ada |

**Sumber "atasan langsung"** di sistem adalah `hris.employees.reporting_to`. Kolom ini bisa diisi lewat import Excel (kolom "NIP Atasan" / `manager_nip`), form karyawan, atau promosi rekrutmen. Org chart di `/dashboard/hris/org-chart` dibangun dari kolom ini, tetapi hanya memuat 200 karyawan pertama.

**"Manager Departemen"**: tabel `hris.departments` tidak punya kolom kepala departemen. Approval kasbon dua tahap butuh sumber ini (Q-APV-2).

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

| Masalah | Jumlah | Dampak |
|---|---|---|
| Karyawan aktif tanpa `reporting_to` | 55 dari 374 (23 `staff`, 15 `Staff`, 12 tanpa jabatan, 4 Store Manager, 1 CEO) | Pengajuan mereka hanya bisa diputus HR |
| `reporting_to` menunjuk diri sendiri | 1 (Retail Operation Manager) | Berisiko menyetujui pengajuannya sendiri (perlu dicek apakah DecideLeave/overtime menolak self-approval) |
| Store Manager tanpa atasan | 4 (termasuk akun uji "QA Dimas Dua Entitas") | Tidak ada atasan untuk time-off/lembur |
