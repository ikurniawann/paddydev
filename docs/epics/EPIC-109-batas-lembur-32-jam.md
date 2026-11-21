# EPIC-109: Batas Lembur 32 Jam per Periode (`MODULE-HRIS`)

status: ready-for-qa
environment: dev
retries: 0

## Goal
Permintaan user 13-11-2026 merevisi EPIC-085 R3 / EPIC-089 R1:

> "sebelum nya ada perubahan terkait maksimal jam lembur periode berjalan yaitu request perubahan: menjadi 32 jam perbulan ya mas"

Batas akumulasi lembur per periode absensi turun dari 72 jam menjadi **32 jam**. Batas 18 jam/minggu tetap berlaku. Perubahan ini masuk sebelum rilis production Fase 1.

## Tasks
Rencana: `docs/superpowers/plans/2026-11-13-batas-lembur-32-jam.md`.
1. Backend: `OvertimeMaxPeriodHours = 32`, ditambah test unit dan integrasi.
2. UI: `OVERTIME_RULES_HINT` "… maks. 18 jam/minggu, 32 jam/periode".
3. ~~Approve tanpa koreksi dicek dari total approved~~ — tidak dikerjakan (K-1 = A).
4. Gerbang, PR ke `development`, deploy staging, QA HR.
5. Dokumen acuan dan rencana rilis: gladi ulang untuk head baru.

## Keputusan
- "Per bulan" berarti per **periode absensi**. Cutoff live 21, jadi periodenya 22 s.d. 21. Yang dihitung adalah pending + approved, sama seperti EPIC-085.
- **K-1 (diputuskan user 13-11: A):** bagaimana pengajuan pending yang sudah ada dan membuat total di atas 32.
  - Rekomendasi A: tetap bisa di-approve. Atasan/HR masih bisa menolak atau mengoreksi.
  - Alternatif B: approve dicek dari total approved.
  - Data 13-11: di prod, 1 karyawan 37,75 jam pending; di staging, 1 karyawan 36,25 jam.

## Acceptance Criteria
- [ ] Pengajuan baru yang membuat total periode > 32 jam ditolak dengan pesan "Lembur periode {label} sudah {x} jam; maksimal 32 jam per periode (sisa {y} jam)". Tepat 32 jam diterima.
- [ ] Batas 18 jam/minggu tetap, dan pesannya tetap didahulukan.
- [ ] Koreksi yang mengurangi jam tetap boleh walau total di atas 32. Koreksi yang menambah jam ditolak.
- [ ] Hint halaman Lembur menampilkan "32 jam/periode".
- [ ] Gerbang hijau, review independen, dan QA staging oleh HR.

## Automation Log
- 13-11-2026: rencana ditulis. Data live dicek read-only (prod cutoff 21; 1 karyawan > 32 jam di periode 22 Okt – 21 Nov). Menunggu persetujuan rencana dan K-1.
- 13-11-2026: user memilih K-1 = A ("A: tetap bisa approve"): pengajuan pending lama tetap bisa di-approve; batas 32 untuk pengajuan baru dan koreksi yang menambah jam.
- 13-11-2026: diimplementasikan (eksekusi native) di erp-paddy branch `feat/lembur-batas-32-jam`, PR wit-id-indonesia/erp-paddy#7 (fffc4d5 backend, 69baa11 UI). Test baru RED→GREEN. Gerbang lokal: gofmt 0, vet ok, `go test ./...` 141 ok, vitest 6.699 passed, tsc 0, eslint 0. Review independen (opus): Critical 0, Important 0.
  - Ruling: test approve K-1 memakai baris `source='employee'`, karena penugasan `company` hanya bisa dikonfirmasi karyawannya sendiri (403).
  - Minor ditunda: koreksi HR pindah tanggal tetap dicek batas walau jam turun (perilaku lama; perhatikan saat QA HR); test memakai cutoff 0; urutan pesan minggu→periode tidak dipin test gabungan; fallback Next tidak menegakkan 18/32.
  - Berikutnya: merge PR #7 (izin user) → deploy staging otomatis → QA HR → gladi ulang head final → Fase 1.
- 13-11-2026 ±18.15: user meminta unit test diulang lalu merge. Unit test lokal di 69baa11: domain lembur 9 test, integrasi lembur 23 test, vitest 37, semua lulus; CI PR #7 hijau. Staging di-backup (`/mnt/hdd1/backup/paddydev/staging-before-epic109-20261113-181720.dump`, `pg_restore -l` 4.412 entri). Squash-merge ke `development` sebagai **38d9683** (izin user "commit dan push lalu merged ke development"). Deploy Staging 43417954923 sukses: image `local-20261113-183310`, health 200, hint "32 jam/periode" ada di build. Menunggu QA HR di staging.
- 13-11-2026 ±19.40: gladi rilis ulang untuk 38d9683 lulus (lihat rencana rilis 0.8). Di salinan prod, periode berjalan menurut sistem adalah 21 Okt – 21 Nov, sehingga total karyawan tadi 41,25 jam (bukan 37,75). Pengajuan barunya ditolak dengan pesan 32 jam; pending lama tetap bisa di-approve (K-1 = A).
