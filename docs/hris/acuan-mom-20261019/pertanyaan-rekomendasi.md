# Pertanyaan & Rekomendasi sebelum Eksekusi

Setiap pertanyaan diberi rekomendasi agar bisa dijawab cepat ("ikuti rekomendasi" / "ubah: …"). Poin K-n merujuk MOM §6; Q-… adalah pertanyaan baru dari analisis kode dan data live 20-10-2026.

## Keputusan user 20-10-2026

> "Ikuti semua rekomendasi di pertanyaan-rekomendasi.md (K-12, K-10, Q-PY-1..4, K-6/Q-KP-1..2, Q-APV-1..3, K-13, K-3, K-14, K-9, Q-AB-1). K-7 tetap menunggu Paddy."

Semua rekomendasi pada tabel di bawah **berlaku sebagai keputusan**, kecuali **K-7** (approval CEO & GM), yang masih menunggu Paddy. Perubahan setting atau data live (Q-PY-4 cutoff 20, K-14 koordinat, perbaikan `reporting_to`) tetap dijalankan hanya setelah user menyetujui langkah eksekusinya.

## Pertanyaan & rekomendasi

| No | Pertanyaan | Rekomendasi (= keputusan, kecuali K-7) | Memblokir |
|---|---|---|---|
| K-12 | Pembagi harian untuk prorate, "1 hari THP", dan potongan tunjangan: hari kerja aktual per periode, angka tetap (23/24), atau hari kalender? Acuan payslip menulis "Total Hari Kerja Terjadwal" (gaji/tunjangan) dan "Total Hari Periode Berjalan" (Izin Pribadi) | Pakai **hari kerja terjadwal** karyawan dalam periode 21 → 20 untuk semuanya, termasuk Izin Pribadi, agar satu angka dipakai di seluruh slip | ES-06, PY-01, PY-03, PY-04 |
| K-10 | Jumlah hari kerja per periode: 23 (5 hari) dan 24 (6 hari)? | Jangan ditanam sebagai konstanta. Hitung dari jadwal karyawan; skema 5/6 hari hanya penentu hari libur mingguan | PY-03 |
| Q-PY-1 | "1 hari THP" = 4 komponen saja (MOM), tanpa lembur, insentif, dan bonus? | Ya, 4 komponen, sesuai MOM §2.6 dan Daftar Istilah. Berlaku juga untuk potongan telat tier ber-basis THP | ES-06, PY-01 |
| Q-PY-2 | Tunjangan Jabatan dibayar per "Hari Kerja Aktif yang Dibayar" (termasuk cuti dibayar), sedangkan transport/makan per "Hari Kerja Efektif" (hadir saja)? | Ya, sesuai tabel payslip; kode perlu membedakan kedua hitungan hari | PY-01 |
| Q-PY-3 | Potongan Izin Sakit dengan tunjangan mode `per_day`: hari sakit sudah tidak dibayar lewat tunjangan harian. Apakah PIS tetap dipotong lagi? | Tidak dipotong dua kali. Bila tunjangan `per_day`, hari sakit cukup tidak dihitung; PIS hanya berlaku untuk tunjangan `flat` | ES-06 |
| K-6 / Q-KP-2 | Kasbon: plafon 50% dari THP 4 komponen bulan berjalan; tenor hanya 1 atau 2; Probation, Freelance, Intern diblokir? | Ya ketiganya. Default 2 cicilan 50/50. Pengajuan lama tenor 3 (live: 1) tetap berjalan sampai lunas | KP-02 |
| Q-KP-1 | Pinjaman Darurat dihapus: 1 pengajuan darurat **pending** di live ditolak otomatis, dikonversi ke kasbon, atau diputus HR dulu? | HR memutus manual dulu; setelah itu jenis `emergency` disembunyikan dan ditolak untuk pengajuan baru (riwayat tetap terbaca) | KP-01 |
| Q-APV-2 | Siapa "Manager Departemen" untuk approval kasbon tahap 1? | Atasan berlevel Manager ke atas pertama di rantai `reporting_to` pemohon. Tidak perlu kolom kepala departemen baru | AP-05 |
| Q-APV-3 | Approval satu tahap "HR **atau** atasan" sekarang tetap dipertahankan untuk time-off & lembur, atau HR hanya boleh memutus bila atasan tidak ada? | Atasan langsung sebagai approver utama; HR sebagai cadangan (karyawan tanpa atasan, atasan cuti) dan tetap bisa override, dengan audit log | AP-01, AP-02 |
| Q-APV-1 | Izin Leader untuk jabatan "Lead": Brand/Marketing/Commercial Lead (di atas Supervisor di org chart) dan Creative Lead berhak? Store Leader & Sample Maker Lead (di bawah Supervisor) tidak berhak? | Ya. Saat jabatan Lead dibuat di master Posisi, isi levelnya: Brand/Marketing/Commercial Lead = Manager, Creative Lead = Supervisor, Store Leader/Sample Maker Lead = Staff | ES-02 |
| K-7 | Approval CEO dan GM: untuk pengajuan apa? | Usulan: time-off & lembur untuk level Manager (atasannya GM), dan kasbon di atas nominal tertentu. Tunggu jawaban Paddy | AP-06 |
| K-13 | Lembur Weekday vs Weekend: tarif berbeda? | Weekday 1,5×, Weekend/libur 2× (sudah sesuai setting live); ESS cukup memilih jenis, tarif tetap dari setting | AP-03 |
| K-3 | Sakit ≥ 2 hari wajib surat? | Ya: pengajuan `sick` dengan total hari ≥ 2 ditolak tanpa lampiran | ES-05 |
| K-14 | Koordinat tiap cabang untuk geofence 100 m? | Live: **0 dari 52 cabang punya koordinat**. Paddy mengirim daftar koordinat cabang aktif; HR mengisi lewat profil cabang. Selama koordinat kosong, status absen "Belum dicek", bukan Invalid | AB-01, AB-03 |
| K-9 | Alert absen terlewat (internet mati): alert ke siapa, kapan? | Gabungkan dengan pengajuan absen manual (AB-04): karyawan mengajukan dengan keterangan, HR menerima notifikasi dan menyetujui | AB-04, AB-06 |
| Q-AB-1 | Penanda branch HQ untuk aturan "kurang dari 9 jam"? | Tambah flag `is_hq` di cabang; label "Kurang jam / pulang lebih cepat" disimpan di baris absensi. Notifikasi lama (< 7 jam, semua cabang) dihapus atau disesuaikan | AB-05 |
| Q-PY-4 | Cut off 20 berlaku mulai periode kapan? Live masih `cutoff_day = 0` (bulan kalender) dan `payroll_day = 25` | Berlaku mulai periode 21 Okt – 20 Nov 2026. HR mengubah setting setelah periode Oktober ditutup. Tanggal bayar dikonfirmasi | PY-02 |

## Rekomendasi urutan eksekusi

1. **Rilis yang sudah siap:** merge PR #3 (intern), lalu deploy EPIC-058 (ES-01, ES-02, ES-04, ES-07, ES-08).
2. **Data & setting (tanpa kode):** bereskan `reporting_to` (55 kosong, 1 menunjuk diri sendiri), koordinat cabang (K-14), `cutoff_day = 20` (Q-PY-4).
3. **Paket Payroll:** basis "1 hari THP" 4 komponen, Cuti Melahirkan memotong transport+makan, minimum lembur 1 jam, subsidi BPJS Kes Rp100.000 / > 1 tahun, prorate karyawan keluar (setelah K-12).
4. **Paket Kasbon:** hapus Pinjaman Darurat, plafon 50% THP, tenor 1/2, blokir status, approval 2 tahap (setelah K-6, Q-KP-1, Q-APV-2).
5. **Paket Approval atasan:** antrean "Persetujuan Tim" di ESS (cuti, lembur), lembur wajib sebelum dilaksanakan, jenis Weekday/Weekend, koreksi lembur AdminHRIS + audit.
6. **Paket Absensi:** pengajuan absen manual/koreksi karyawan → approval HR (AB-04/AB-06), Perjalanan Dinas, geofence + popup out-of-area (setelah koordinat ada), flag HQ < 9 jam.
7. **Paket Cuti lanjutan:** konfigurasi kuota per tahun/kejadian, masa kerja 12 bulan, batas Sakit 3/bulan, lampiran wajib Sakit ≥ 2 hari.
8. **Audit (LA-01):** tambah event untuk export, keputusan & koreksi lembur, edit karyawan, cuti, payroll. Lebih baik dikerjakan bersama tiap paket di atas.
9. **Payroll mingguan daily worker (PY-05):** paket tersendiri, karena butuh periode dan run baru.
