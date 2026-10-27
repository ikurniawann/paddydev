# EPIC-068 I3: Koreksi Saldo Tahun Lalu sampai Kedaluwarsa Sisa Bawaan

**Keputusan user (27-10-2026):** HR boleh mengoreksi saldo cuti tahun lalu setelah 1 Januari sampai tanggal kedaluwarsa sisa bawaan (diatur admin HRIS, default 31 Maret).

**Goal:** PUT `/api/hris/leave-balances/{id}` dan import Excel menerima tahun N-1 selama hari ini (WIB) ≤ tanggal kedaluwarsa setting di tahun N. Perubahan saldo N-1 menghitung ulang sisa bawaan N lewat jalur yang sudah ada (`carryAfterHRInput` → `recomputeCarryOver(N)`): baris `auto` saja, review HR bila bawaan baru < terpakai.

**Kenapa batasnya kedaluwarsa:** setelah tanggal itu sisa bawaan N tidak bisa dipakai lagi, sehingga koreksi N-1 tidak berdampak ke karyawan, hanya mengubah catatan.

## Tugas

1. **Backend: aturan tahun (`leave_balances.go`).**
   - Tipe `balanceYears {Now, Today, PrevUntil}` + `allows(year)` + pesan tolak; dibaca dari `loadLeaveSettings` (bulan/hari kedaluwarsa) di dalam transaksi.
   - `validateLeaveBalance` memakai `balanceYears` alih-alih `nowYear`.
   - Pesan saat jendela tertutup tidak berubah: "Tahun saldo hanya boleh N (tahun berjalan) atau N+1 (tahun depan)". Saat terbuka: "Tahun saldo hanya boleh N-1 (tahun lalu, sampai DD-MM-YYYY), N (tahun berjalan), atau N+1 (tahun depan)".
   - Uji integrasi (jam 2027-02-10 dan 2027-04-01 WIB): PUT 2026 diterima di Februari; sisa bawaan 2027 (`auto`, setting menyala) ikut dihitung ulang dengan audit; ditolak 1 April; kedaluwarsa setting 30 April memperpanjang jendela; 2025 tetap ditolak.
2. **Backend: import (`leave_balance_import.go`).** `leaveCheck` dan konteks upload memakai `balanceYears`; teks Info template diperbarui. Uji: pratinjau tahun lalu di dalam jendela tanpa error tahun, di luar jendela tetap error.
3. **Frontend: halaman Saldo Cuti.**
   - Helper `previousYearEditableUntil(settings, thisYear)` di `balance-math.ts`.
   - `canEdit` = punya `update` dan (tahun ≥ tahun berjalan, atau tahun lalu dan hari ini ≤ batas).
   - Uji vitest: helper dan tombol Ubah untuk tahun lalu di dalam/di luar jendela.
4. **Docs:** Automation Log EPIC-068 (keputusan I3 + bukti), acuan bila menyebut aturan tahun.

## Di luar cakupan

- Temuan minor pratinjau menghitung bawaan yang sudah kedaluwarsa (terpisah).
- Tahun N-2 dan sebelumnya tetap tertutup.
