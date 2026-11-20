# EPIC-122: Pembulatan Jam Lembur per Pengajuan (`MODULE-PAYROLL`)

status: review
environment: dev
retries: 0

## Goal
Permintaan user 20-11-2026:

> perhitungan lembur di payroll belum sesuai dengan rumus ini gaji pokok/173*1,5*jumlah jam lembur (weekday) gaji pokok/173*2*jumlah jam lembur (weekend)

> jadi rounding per pengajuan itu mengacu ke rules perhitungan lembur tadi ( roundingnya >31 menit naik ke atas <=30 menit turun kebawah)

Rencana: [2026-11-20-pembulatan-jam-lembur.md](../superpowers/plans/2026-11-20-pembulatan-jam-lembur.md).

## Temuan (production, read-only 20-11)
- Slip November Gantari Garini (gaji pokok 1.980.000): Lembur Hari Kerja 5,75 jam = Rp98.714, Lembur Libur/Istirahat 1,33 jam = Rp30.444. Rumus tarif sudah sesuai (1.980.000 / 173 × 1,5 × 5,75 = 98.714).
- Selisihnya ada di jam: payroll menjumlahkan jam pecahan (2j55m + 2j50m = 5,75; 1j20m = 1,33). Aturan user: tiap pengajuan dibulatkan ke jam penuh, sisa > 30 menit naik, ≤ 30 menit turun.
- Dengan aturan itu: 18-11 2j55m → 3, 19-11 2j50m → 3, 08-11 1j20m → 1. Lembur hari kerja 6 jam = Rp103.006, libur 1 jam = Rp22.890.

## Keputusan desain
- Pembulatan per pengajuan, setelah jam dibatasi absensi aktual dan setelah cek minimum 1 jam (EPIC-059). Tarif tetap dari setting (1,5 / 2 / 173).
- 30 menit tepat dibulatkan turun.
- Fallback Next ikut dibulatkan dengan aturan yang sama.

## Tasks
Lihat rencana.

## Acceptance Criteria
- Slip Gantari Garini November setelah dihitung ulang: Lembur Hari Kerja 6 jam Rp103.006, Lembur Libur/Istirahat 1 jam Rp22.890.
- Pengajuan 2j30m dibayar 2 jam; 2j31m dibayar 3 jam.

## Automation Log
- 20-11-2026: temuan production dicek read-only; aturan pembulatan dikonfirmasi user.
- 20-11-2026: implementasi branch erp-paddy `fix/epic122-pembulatan-lembur` (3d434f9 Go, e4097a8 fallback Next). Tes: Go payroll (integrasi) ok, Go domain ok, vitest payroll 205 lulus, tsc 0, eslint 0.
- 20-11-2026: atas izin user ("Ya, merge & staging") PR #33 di-merge ke development (d6d90ec); staging berjalan di image `local-20261120-170645` dari d6d90ec, `/api/ready` 200. User mengonfirmasi pembulatan memakai menit asli (1j20m tercatat 1,33 → 1 jam, bukan 2).
- 20-11-2026 17.45 WIB: persiapan rilis atas izin user ("Ya, siapkan rilis"): branch `release/epic122-prod` = production 3b780ba + cherry-pick a99e6b3, 3bf88d6; PR #34 ke production (belum di-merge). Tes: vet 0, Go payroll + hris ok, tsc 0, vitest 6928. Gladi di salinan backup prod 14.00 (`gladi122`): tidak ada migrasi baru, verify 0 selisih. Run November dihitung di dua salinan lokal (kode lama vs baru, data per 14.00): 53 dari 285 karyawan berubah (10 naik, 40 turun), total lembur Rp12.084.271 → Rp11.825.121 (−Rp259.150); penurunan terbesar dari pengajuan 1j30m yang dibulatkan turun.
- 20-11-2026: atas keputusan user ("Cancel, use combined"), rilis EPIC-122 saja jam 22.00 dibatalkan dan PR #34 ditutup. EPIC-122 ikut rilis gabungan EPIC-118 s.d. 123 (gladi 18.50–19.35 WIB lulus, lihat EPIC-123).
