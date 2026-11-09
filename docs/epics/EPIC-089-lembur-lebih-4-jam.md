# EPIC-089: Lembur Lebih dari 4 Jam dengan Alasan, Dokumentasi Opsional (`MODULE-HRIS`)

status: review
environment: dev
retries: 0

## Goal
Permintaan user 05-11-2026 merevisi EPIC-085 R3:

> "Maksimal jam kerja lembur 4 jam/hari, tapi ketika karyawan ajukan lembur lebih dari 4 jam maka wajib isi keterangan kenapa bisa lebih dari 4 jam, lalu opsional sertakan upload dokumentasi. Yang jadi maksimal adalah total akumulasi lembur periode berjalan: 18 jam/minggu atau 72 jam."

### Jawaban user 05-11
- **Dokumentasi** (lampiran) jadi **opsional untuk semua** lembur.
- **Keterangan > 4 jam** memakai kolom **Keterangan/Alasan** yang sudah ada; tidak ada kolom baru.
- **Penugasan HR** (`source='company'`) bebas: aturan > 4 jam tidak berlaku.
- **Batas harian dihapus**, termasuk durasi kotor 12 jam. Batas keras hanya 18 jam/minggu (Senin–Minggu) dan 72 jam/periode.

## Kondisi kode per 05-11 (branch EPIC-088 `ec574a95`, = main + #76)
- `CreateOvertime` (`hris/overtime.go`):
  - pengajuan sendiri: lampiran wajib 1–3 (`checkOvertimeAttachments`), Keterangan opsional;
  - penugasan HR: alasan minimal 5 karakter;
  - durasi kotor > 12 jam → 400 "Durasi lembur maksimal 12 jam". Cek yang sama ada di approve-koreksi (`overtime.go:385`) dan koreksi HR (`overtime_correction.go:93`).
- `checkOvertimeCaps` (`overtime_tnc.go:49`) → `domain.OvertimeCapError(hours, day, week, period, …)` memblokir lebih dari 4 jam per tanggal, 18 jam per minggu, dan 72 jam per periode. Dipanggil di create, approve-koreksi, dan koreksi HR.
- ESS Lembur (`ess/components/lembur-page.tsx`):
  - kolom "Keterangan (opsional)";
  - toast "Lampirkan minimal 1 dokumentasi lembur";
  - catatan "Lembur diinput setelah selesai, dengan dokumentasi";
  - `OVERTIME_RULES_HINT` (`lib/hris/overtime-rules.ts`) "… maks. 4 jam/hari, 18 jam/minggu, 72 jam/periode".
- Payroll membayar jam yang disetujui tanpa batas harian (tidak berubah).

## Ruling
- **R1 Batas.** `OvertimeCapError` tidak lagi memeriksa batas harian. Pesan mingguan dan periode tetap sama persis. Cek durasi kotor 12 jam dihapus di ketiga tempat; jam mulai = jam selesai tetap ditolak.
- **R2 Lebih dari 4 jam.** Pada pengajuan **milik sendiri**, bila jam dibayar (setelah istirahat) **> 4**, Keterangan wajib minimal 10 karakter setelah trim. Bila tidak: 400 "Lembur lebih dari 4 jam: isi Keterangan alasannya (minimal 10 karakter)".
  - 4,00 jam pas tidak wajib.
  - Penugasan HR, koreksi atasan, dan koreksi HR tidak dicek: mereka penyetuju.
- **R3 Dokumentasi opsional.** Pengajuan sendiri boleh tanpa lampiran. Bila ada, aturan lama tetap berlaku: maksimal 3, tidak boleh sama, milik sendiri, dan berkasnya ada.
- **R4 ESS Lembur.**
  - Toast lampiran wajib dihapus. Label lampiran menyebut "(opsional)".
  - Bila jam dibayar > 4: label menjadi "Keterangan (wajib — lebih dari 4 jam)" dengan petunjuk "Jelaskan kenapa lembur lebih dari 4 jam", dan klien menolak dengan pesan R2 sebelum kirim.
  - Catatan bawah: "Lembur diinput setelah selesai; dokumentasi opsional. Lembur yang disetujui atasan dibayar sesuai jam setelah dikurangi istirahat."
  - `OVERTIME_RULES_HINT`: "Lembur hari kerja dimulai setelah jam kerja selesai · lebih dari 4 jam/hari wajib keterangan · maks. 18 jam/minggu, 72 jam/periode".
- **R5 Acuan.**
  - `.claude/skills/paddy-hris-acuan/SKILL.md` baris Lembur ditambah: "> 4 jam/hari wajib keterangan; maks. 18 jam/minggu & 72 jam/periode; dokumentasi opsional (EPIC-089)".
  - EPIC-085 mendapat catatan "R3 direvisi EPIC-089".
- **R6 Lintas hari (review akhir 05-11).** Tanpa batas 12 jam, satu lembur bisa melewati tengah malam sampai jam kerja besok dan terbayar ganda. Lembur pada tanggal D ditolak bila menabrak:
  - lembur aktif D−1 yang lintas hari: "Lembur bertabrakan dengan lembur {tgl} ({mulai}–{selesai})";
  - lembur aktif D+1: pesan yang sama;
  - jam masuk shift pertama D+1: "Lembur melewati jam masuk kerja {tgl} ({HH:MM})".

  Berlaku di pengajuan sendiri, penugasan HR, koreksi atasan, dan koreksi HR (bila jam/tanggal berubah). Libur nasional di D+1 tidak dikecualikan; jadwal yang menentukan.
- **Di luar cakupan**: fallback route Next lembur (tidak terjangkau selama Go aktif); label "> 4 jam" khusus di antrean atasan (Keterangan sudah tampil).

## Tasks
Rencana: `docs/superpowers/plans/2026-11-05-lembur-lebih-4-jam.md`.

## Acceptance Criteria
- [x] Karyawan bisa mengajukan lembur > 4 jam bila Keterangan diisi (≥ 10 karakter); tanpa Keterangan ditolak dengan pesan R2.
- [x] Lembur bisa diajukan tanpa dokumentasi.
- [x] Batas 18 jam/minggu dan 72 jam/periode tetap memblokir; tidak ada lagi batas 4 jam/hari atau 12 jam kotor.
- [x] Gates hijau; QA staging (HP).

## Automation Log
- 2026-11-05 — Pemetaan + 4 jawaban user. Epic + rencana ditulis. Worktree `.worktrees/epic089`, branch `feat/epic089-lembur-4-jam`, ditumpuk di atas `feat/epic088-verifikasi-lampiran` (#76) karena `overtime.go` sudah berubah di R11.
- 2026-11-05 — Implementasi native, 3 task (`2473aa99` domain, `e2ab0920` service, `e72d3be4` frontend + acuan).
  - Gates: `go test -p 1 ./internal/modules/hris/... ./internal/app/` ok, vet + gofmt bersih, vitest 4518 lulus, tsc + eslint bersih.
- 2026-11-05 — Review akhir (reviewer terpisah): 0 Critical, 1 Important, 4 Minor.
  - **Important**: tanpa batas 12 jam, lembur lintas hari bisa menabrak jam kerja besok atau lembur lain (bayar ganda). Diperbaiki dengan R6 (`2972f42e`); tes `TestOvertimeOverlapError` + `TestOvertimeCrossDayOverlap` RED→GREEN.
  - Ditunda: `TrimSpace` vs `JSTrim` (U+FEFF), import tes satu baris, pesan R2 sebagai var, komentar contoh "wajib, 1–3" di `private-attachments-field.tsx`.
- 2026-11-05 — **Staging** `local-20261105-141328` (gabungan lokal EPIC-087 + EPIC-089, yang memuat EPIC-088; merge `775ac78d` tidak di-push).
  - Backup `staging-db-before-epic089-20261105-141327.dump`. Verify 10774 objek, 0 berbeda.
  - QA lulus 8/8 (karyawan di HP, atasan di desktop):
    - label dokumentasi opsional;
    - 2 jam tanpa dokumentasi terkirim;
    - 6 jam tanpa Keterangan ditolak, dengan Keterangan terkirim (tersimpan tanpa lampiran);
    - akumulasi minggu 15 + 5 jam ditolak dengan pesan 18 jam/minggu;
    - atasan melihat Keterangan di Persetujuan Tim;
    - 0 error API dan console.
  - Data demo dan berkas dihapus.

