# EPIC-066: Input Lembur Setelah Selesai — Istirahat, Dokumentasi Wajib, Koreksi oleh Atasan Langsung

status: backlog
environment: dev
retries: 0

## Goal
Mengubah alur lembur karyawan dari **pengajuan sebelum lembur** (MOM AP-02, EPIC-063) menjadi **input lemburan setelah lembur selesai**, sesuai kebiasaan HR Paddy (catatan HR 22-10-2026, pembanding: Talenta):

1. Karyawan menginput lembur setelah jam selesai lembur lewat (contoh: lembur 21 Okt 17.00–19.00 baru bisa diinput mulai 21 Okt 19.00, atau besok paginya), selama periode absensi tanggal itu belum di-closing.
2. Input berisi tanggal, jam mulai–selesai, dan **istirahat dalam menit** (seperti Talenta); jam lembur dibayar = durasi − istirahat, minimal 1 jam, maksimal 12 jam.
3. **Dokumentasi wajib**: 1–3 file PDF/JPG/PNG/WebP, masing-masing ≤ 5 MB. Keterangan opsional.
4. **Atasan langsung** menyetujui, menolak, atau mengoreksi jam/istirahat sebelum menyetujui. Hanya atasan langsung yang mengoreksi lembur karyawan yang punya atasan; HR (HRD) hanya cadangan untuk karyawan tanpa atasan langsung.
5. Tarif tetap: hari kerja biasa 1,5 × Gaji Pokok / 173 × jam dibayar; hari libur nasional / istirahat mingguan 2 × Gaji Pokok / 173 × jam dibayar; minimal 1 jam (`aturan-payroll.md`, sudah berjalan sejak EPIC-059). Kalkulator payroll tidak berubah.

## Keputusan user (22-10-2026)
- Batas input: sejak jam selesai lembur sampai periode absensi tanggal itu di-closing (sama dengan pengajuan koreksi absen EPIC-065).
- Persetujuan: atasan langsung setujui / tolak / ubah jam lalu setujui; HR cadangan untuk karyawan tanpa atasan.
- Lampiran: 1–3 file PDF/gambar, ≤ 5 MB per file; keterangan opsional.
- Aturan AP-02 "lembur wajib diajukan sebelum dilaksanakan" **diganti** oleh epic ini (perubahan atas MOM 19-10-2026 atas permintaan HR).

## Tasks
Lihat `docs/superpowers/plans/2026-10-22-input-lembur-setelah-selesai.md` (8 task).

## Acceptance Criteria
- Input lembur sendiri sebelum jam selesai lewat → 400 `Lembur baru bisa diinput setelah selesai (<tanggal> <jam selesai> WIB)`; tanggal di periode yang sudah di-closing → 409 pesan periode yang sudah ada.
- Istirahat 0–720 menit; jam dibayar = durasi − istirahat; < 1 jam → 400 `Durasi lembur minimal 1 jam setelah dikurangi istirahat`; durasi > 12 jam → 400.
- Tanpa lampiran → 400 `Lampirkan minimal 1 dokumentasi lembur`; > 3 → 400; lampiran harus hasil unggah karyawan itu sendiri.
- Lampiran bisa dibuka oleh karyawan pemilik, atasan langsungnya, dan HR; orang lain 403.
- Atasan langsung bisa mengubah jam mulai/selesai/istirahat saat menyetujui; perubahan diaudit (before/after) dan jam dihitung ulang. HR hanya bisa mengoreksi bila karyawan tidak punya atasan langsung yang valid.
- Penugasan lembur oleh HR (`source = company`) tidak berubah.
- Payroll: jam lembur yang dibayar = jam tersimpan (sudah dikurangi istirahat), tetap dibatasi lembur absensi bila ada; tarif tidak berubah.

## Automation Log
- 2026-10-22: Epic dibuat dari catatan HR; keputusan user di atas; rencana ditulis.
