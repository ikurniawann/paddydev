# EPIC-123: Batas, Keterangan, dan Tampilan Lembur Memakai Jam Dibulatkan (`MODULE-HRIS`)

status: backlog
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
