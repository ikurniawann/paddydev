# EPIC-123: Batas, Keterangan, dan Tampilan Lembur Memakai Jam Dibulatkan (`MODULE-HRIS`)

status: ready-for-qa
environment: dev
retries: 0

## Goal
Lanjutan EPIC-122 (sisa Plan 4 EPIC-PAY-01). Setelah payroll membayar jam lembur yang dibulatkan per pengajuan, aturan pengajuan dan tampilan di layar harus memakai angka yang sama. Karyawan juga harus melihat sisa kuota sebelum mengirim pengajuan.

Rencana: [2026-11-20-lembur-batas-dan-tampilan-dibulatkan.md](../superpowers/plans/2026-11-20-lembur-batas-dan-tampilan-dibulatkan.md).

## Keputusan user (20-11-2026)
| No | Pertanyaan | Keputusan |
| --- | --- | --- |
| Q-46 | Tarif lembur hari istirahat mingguan vs libur nasional | Sama, 2× |
| TC-01 | Lembur di bawah 60 menit | Tetap ditolak saat diajukan; tepat 60 menit sah |
| K-01, K-02, K-03 | Tarif bertingkat PP 35/2021 dan dasar upah | Tetap flat (1,5× / 2×) dari gaji pokok ÷ 173; risiko kepatuhan dicatat |
| Q-11 | Pembulatan jam | Per pengajuan, pada jam yang dibayar: sisa ≥ 31 menit naik, ≤ 30 menit turun (sudah di EPIC-122) |
| Q-30 | Pembulatan rupiah | Rp1 per komponen (tidak berubah) |
| Baru | Batas 32 jam per periode | Dihitung dari jam yang sudah dibulatkan |
| Baru | Keterangan wajib di atas 4 jam | Dihitung dari jam yang sudah dibulatkan |
| Baru | Cara rilis | Bersama EPIC-118 s.d. 120 dan 122, setelah gladi di salinan production |

Kutipan user untuk Q-11: "syarat utama minimal lembur 1 jam (di atas 60 menit) syarat kedua setelah yg utama terpenuhi rounding per pengajuan sama roundingnya >31 menit naik ke atas <=30 menit turun kebawah". Klarifikasi: 60 menit sah, sisa 31 menit naik, pembulatan pada jam yang dibayar.

Plan 1 EPIC-PAY-01 (parameter store) tidak diperlukan: tarif tetap flat dan aturan pembulatan sudah pasti.

## Temuan (development d6d90ec, 20-11)
- Form lembur ESS menampilkan "X jam dibayar" dari `paidOvertimeHours` tanpa pembulatan (1j20m tampil 1,33 jam, slip membayar 1 jam). Dialog setujui lembur dan halaman Persetujuan Tim memakai fungsi yang sama.
- Batas 18 jam/minggu dan 32 jam/periode menjumlah jam mentah (`OvertimeHoursUsed`, `sum(hours)`), dan keterangan wajib di atas 4 jam memakai jam mentah (`OvertimeReasonError`).
- Sisa kuota hanya muncul di pesan tolak. Belum ada endpoint kuota lembur.

## Tasks
Lihat rencana.

## Acceptance Criteria
- Form ESS: 1j20m tampil "1 jam dibayar"; 2j31m tampil "3 jam dibayar".
- Dua pengajuan 15j24m (dibulatkan 15 + 15) lalu pengajuan 2j24m (dibulatkan 2) diterima, total 32 jam. Dengan jam mentah (33,2 jam) pengajuan ketiga akan ditolak.
- Pengajuan 4j20m tanpa keterangan diterima; 4j31m tanpa keterangan ditolak.
- Form ESS menampilkan sisa kuota minggu dan periode sebelum pengajuan dikirim.
- Setelah rilis, run payroll November dihitung ulang dan slip memakai jam dibulatkan.

## Automation Log
- 20-11-2026: keputusan Q-46, TC-01, K-01–K-03, Q-11, batas 32 jam, keterangan 4 jam, dan cara rilis dikonfirmasi user. Rencana ditulis, menunggu review user sebelum dikerjakan.
- 20-11-2026: implementasi di erp-paddy branch `feat/epic123-lembur-dibulatkan` (4b383c6, 44e14cb, d3feaf9, 8791b2c, c311f25), PR https://github.com/wit-id-indonesia/erp-paddy/pull/35 ke `development`, belum merge. Ruling: batas mingguan 18 jam ikut jam dibulatkan. Tambahan dari review: daftar lembur ESS dan tabel Lembur HR ikut jam dibulatkan; koreksi baru dicek batas bila jam dibayar bertambah; baris lama < 1 jam dihitung 0 di batas/kuota.
- 20-11-2026: gate lokal (Actions tertahan billing): gofmt 0, go vet ok, `go test ./...` 143 paket ok (`TestGapDigestWarnsWithoutRecipients` gagal hanya saat full run paralel, lulus sendiri, tidak terkait lembur), vitest 7.078 lulus, tsc 0, eslint 0. Review akhir: 2 Important + 2 Minor diperbaiki, 1 Minor ditunda (invalidasi kuota pada koreksi HR atas baris sendiri).
- 20-11-2026 18.20 WIB: deploy staging dari branch PR, image `local-20261120-182014` (sebelumnya `local-20261120-170645`), tanpa migrasi, verify 0 selisih. Backup `/mnt/hdd1/backup/paddydev/staging-db-before-epic123-20261120-181958.dump`. Endpoint kuota 401 tanpa sesi, bundle memuat form kuota. QA UI di staging menunggu user (login staging dikunci untuk karyawan).
- 20-11-2026 18.37 WIB: QA user — baris "X jam dibayar (durasi …)" di form Input Lembur ESS dihapus atas permintaan user ("keterangan di tandain dihilangkan aja ga usah ada"); layar lain tetap. Commit 3bbc7e6 di PR #35, staging `local-20261120-183733` (backup `staging-db-before-epic123b-20261120-183725.dump`).
- 20-11-2026: atas perintah user ("merge PR #35"), PR #35 di-merge ke `development` (778d0bd). CI Actions tidak berjalan (billing: "recent account payments have failed"), gate lokal hijau. Tree `development` = 3bbc7e6 = staging `local-20261120-183733`. Production belum; rilis bersama EPIC-118 s.d. 122 setelah gladi, lalu hitung ulang run payroll November.
- 20-11-2026 18.50–19.35 WIB: gladi rilis gabungan EPIC-118 s.d. 123 (kandidat development 778d0bd = image staging `local-20261120-183733`) di salinan production 18.50 (`/mnt/hdd1/backup/paddydev/prod-copy-gladi-epic123-20261120-185017.dump`, mode 600), jaringan Docker `--internal` (egress tertutup). Baseline app production 3b780ba dibangun lokal (`paddydev-gladi-*:3b780ba`). Hasil: 4 migrasi 1,8 detik, verify 12.393 objek 0 selisih; health 200; cookie lama tetap login 3 role; crawl super admin/HR/karyawan 378 + publik 67 halaman, 0 regresi, 0 respons 5xx; rollback app lama di skema baru 0 regresi. Payroll November (PADDY-GROUP, 285 karyawan): 52 berubah karena lembur (10 naik, 42 turun), total lembur Rp14.264.492 → Rp13.970.033 (−Rp294.459), 73 hanya berubah alokasi toko (EPIC-119 head office ERP-BANDUNG, 48 slip HQ tidak lagi dibagi). Contoh Gantari Garini cocok (6 jam Rp103.006, 1 jam Rp22.890). Kuota: 0 karyawan di atas 32 jam (3 tepat 32). Skrip Bungsu: pratinjau 7 pola + 4 absensi, 3 terlambat dihapus; eksekusi, tolak jalan kedua, dan pembalik terbukti di salinan. Rekomendasi: go; HR menghitung ulang run November sesudah rilis; eksekusi Bungsu di production menunggu izin user. PR #34 (rilis EPIC-122 saja) akan tergantikan.
- 20-11-2026 19.13–19.17 WIB: **rilis production** (rilis ke-4, gabungan EPIC-118 s.d. 123) atas izin user ("Ya, rilis sekarang"), setelah gladi 18.50 lulus. Backup `~/backups/prod-db-before-rilis-epic123-20261120-1913.dump` + `~/paddydev-src.before-rilis-epic123-20261120-1913`. PR #36 (development → production) di-merge (bbf7dfc, tree = development 778d0bd). Source dari `git archive origin/production` di-rsync ke `paddydev-src` (147 file berubah, 2 file lama dihapus), lalu `bootstrap.sh` + `deploy-paddydev.sh`. Log `~/backups/deploy-prod-epic123-20261120-1913.log`: 4 migrasi, verify 12.393 objek 0 selisih, image `local-20261120-191457`, EXIT=0. Smoke: `/api/ready`, `/api/health`, `/login` 200; `/api/hris/overtime/quota` tanpa login 401; 0 error log; 1.203 sesi tetap; head office ERP-BANDUNG. Rollback: deploy ulang dari folder `.before-rilis-epic123-20261120-1913` (image lama `local-20261120-090127`), tanpa membatalkan migrasi. Sisa: HR menghitung ulang run November (draft); skrip Bungsu menunggu izin. PR #34 sudah ditutup 19.09 WIB dengan komentar (dicek ulang atas perintah user "Ya, tutup PR #34").
