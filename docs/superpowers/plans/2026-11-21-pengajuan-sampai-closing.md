# Pengajuan Karyawan Dibuka sampai Closing Periode (EPIC-125) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Karyawan bisa mengajukan koreksi absen, cuti/izin, dan lembur untuk tanggal di periode yang sudah lewat tanggal akhirnya, selama periode itu belum di-closing.

**Architecture:** Kunci cut-off EPIC-084 R3 hanya ada di `ensureSubmissionOpen` (repo `erp-paddy`, `apps/backend/internal/modules/hris/attendance_periods.go`), yang dipanggil pembuatan pengajuan absensi, cuti/izin, dan lembur milik sendiri. Fungsi itu dibuat sama dengan `ensureAttendanceOpen`: yang menolak hanya periode yang sudah closed (manual, atau otomatis). Aturan lain (H-n cuti, sakit hari H, lembur H+1, jalur HR) tidak berubah.

**Tech Stack:** Go (`apps/backend`), tes integrasi DB paket `hris`. Frontend tidak berubah: form ESS tidak punya cek cut-off sendiri, dan kartu "Absen terlewat" sudah memakai jendela sampai closing (`openAttendanceWindow`).

**Spec:** [EPIC-125](../../epics/EPIC-125-pengajuan-sampai-closing.md) (dibuat di Task 2). Permintaan user 21-11-2026: bug "ada yang ga bisa koreksi absen padahal settingnya udh bener". Contoh: koreksi tanggal 06-11-2026 diajukan 21-11-2026 ditolak "Periode 21 Okt – 20 Nov 2026 sudah lewat cut off; pengajuan untuk tanggal ini ditutup", padahal periode November (21 Okt–20 Nov) masih **Terbuka** dengan closing otomatis 22 Nov 2026. Keputusan user: **"Open until closing"**. Ini mengganti jawaban K-20 (03-11-2026) dan EPIC-084 R3.

## Global Constraints

- Pengajuan milik sendiri untuk tanggal di periode yang belum closed diterima, walaupun hari ini > `end_date` periode.
- Periode closed (manual, atau otomatis sejak hari sesudah tanggal closing) tetap menolak dengan pesan `closedPeriodError` yang ada: "Periode absensi {label} sudah di-closing. Absensi di periode ini tidak bisa diubah; minta super admin/HR manager membuka ulang periode bila perlu koreksi."
- Pesan "sudah lewat cut off; pengajuan untuk tanggal ini ditutup" tidak dipakai lagi.
- Aturan EPIC-084 lain tetap: Cuti Tahunan H-3, Cuti Ibadah H-30, Sakit hari H, lembur milik sendiri paling lambat H+1, HR untuk karyawan lain bebas H-n.
- Tidak ada migrasi dan tidak ada perubahan setting.

## Review Focus

- Pengajuan dikirim di hari closing lalu periode closed otomatis sebelum atasan/HR memutus: approve pengajuan absensi tetap 409 "sudah di-closing" dan pengajuan tetap pending (perilaku EPIC-065). Approve cuti/lembur untuk periode closed belum dijaga (sudah begitu sebelum perubahan ini); catat di epic sebagai follow-up, jangan diubah di sini.
- Batas closing: hari closing itu sendiri masih terbuka; hari sesudahnya tertutup (`periodContaining` menutup otomatis saat hari ini > `closing_date`).
- Lembur tanggal 20 diajukan tanggal 22: tetap ditolak oleh aturan H+1, bukan oleh periode.
- Cuti yang mulai di periode lama (belum closed) dan berakhir di periode berjalan: diterima.
- HR yang mengajukan untuk karyawan lain: tidak berubah.

---

### Task 1: Kunci pengajuan mengikuti closing periode

**Files:**
- Modify: `apps/backend/internal/modules/hris/attendance_periods.go:273-293` (`ensureSubmissionOpen`)
- Modify: `apps/backend/internal/modules/hris/leave_tnc_integration_test.go` (`expectCutoff`, kasus (c) dan (n) di `TestLeaveTnCSubmission`, `TestLeaveTnCCutoff20`)
- Modify: komentar `// EPIC-084 R3` di `attendance_requests.go:90`, `leaves.go:310`, `overtime.go:174` menjadi `// EPIC-125: terbuka sampai closing`

**Interfaces:**
- Produces: `func (s *Service) ensureSubmissionOpen(ctx context.Context, date string) error` dengan tanda tangan yang sama; perilakunya = `ensureAttendanceOpen`.

- [ ] **Step 1: Tulis tes yang gagal**

Tes baru `TestSubmissionOpenUntilClosing` di `leave_tnc_integration_test.go`, dengan setting seperti production:
- `UPDATE hris.attendance_settings SET cutoff_day = 20, closing_offset_days = 2` (periode 21 Okt–20 Nov, closing 22 Nov).
- `h.setNow("2026-11-21 08:00")`, karyawan `self` (staff "pos", berakun):
  - `POST /api/hris/attendance-requests` `attReqBody("2026-11-06", "07:50", "17:16", "Tidak clockin karena HP error")` → 200.
  - `POST /api/hris/leaves` `tncLeave("bereavement", "2026-11-18", "2026-11-18")` → 200.
  - `POST /api/hris/overtime` `h.otSelfBody(&self, "2026-11-20", "17:00", "19:00")` → 200.
- `h.setNow("2026-11-22 21:00")` (hari closing): pengajuan absensi tanggal `2026-11-09` → 200; lembur tanggal `2026-11-20` → 400 "Lembur diajukan paling lambat H+1 (21-11-2026)".
- `h.setNow("2026-11-23 08:00")` (sudah closed otomatis): pengajuan absensi tanggal `2026-11-10` → 409 berisi "sudah di-closing"; cuti `bereavement` `2026-11-19` → 409 berisi "sudah di-closing".

Ganti helper `expectCutoff` menjadi `expectClosed(t, c)` yang memeriksa status 409 dan teks "sudah di-closing".

- [ ] **Step 2: Jalankan dan pastikan gagal**

Run: `cd apps/backend && TEST_DATABASE_URL=<DB tes> $HOME/.local/go/bin/go test ./internal/modules/hris/ -run TestSubmissionOpenUntilClosing -count=1`
Expected: FAIL. Pengajuan tanggal 21-11 mendapat 409 "sudah lewat cut off".

- [ ] **Step 3: Ubah `ensureSubmissionOpen`**

Hapus cabang `today > p.End`. Isi fungsi menjadi pemanggilan `ensureAttendanceOpen`. Komentar fungsi menyebut EPIC-125: pengajuan milik sendiri terbuka sampai periode closed (mengganti EPIC-084 R3).

- [ ] **Step 4: Sesuaikan tes lama**

- `TestLeaveTnCSubmission` kasus (c) dan (n): hapus asersi cut-off dari tes ini, karena hasilnya bergantung pada setting default harness. Cakupannya pindah ke tes Step 1.
- `TestLeaveTnCCutoff20` (setting 20/3, sekarang 22-11): cuti `2026-11-18`–`2026-11-23` sekarang → 200. Ganti nama menjadi `TestLeaveAcrossPeriodsBeforeClosing` dan perbarui komentarnya.

Run: `cd apps/backend && TEST_DATABASE_URL=<DB tes> $HOME/.local/go/bin/go test ./internal/modules/hris/ -count=1`
Expected: lulus, kecuali kegagalan yang sudah ada di baseline `development` (catat namanya sebelum mulai).

- [ ] **Step 5: Commit**

```bash
git add apps/backend/internal/modules/hris
git commit -m "fix(hris): pengajuan karyawan terbuka sampai closing periode, bukan sampai cut off (EPIC-125)"
```

### Task 2: Dokumen acuan dan epic

**Files (repo paddydev, branch `docs/rencana-rilis-prod-erp-paddy`):**
- Create: `docs/epics/EPIC-125-pengajuan-sampai-closing.md` (format EPIC-124), baris registry di `docs/epics/README.md` (`MODULE-HRIS`, status `review`)
- Modify: `docs/epics/EPIC-084-ketentuan-cuti-izin.md` R3: tambahkan "Diganti EPIC-125 (21-11-2026): terbuka sampai closing periode"
- Modify: `docs/hris/acuan-mom-20261019/pertanyaan-rekomendasi.md` baris K-20: tambahkan keputusan baru user 21-11-2026 ("Open until closing")
- Modify: `docs/hris/acuan-mom-20261019/status-requirement.md` baris ES-13: aturan baru + "EPIC-125, menunggu review & rilis"

- [ ] **Step 1:** Ubah keempat dokumen sesuai daftar di atas. Epic memuat:
  - contoh kasus 06-11 / 21-11;
  - follow-up dari Review Focus baris pertama (approve cuti/lembur setelah closing);
  - langkah rilis: PR ke `development`, staging, gladi, rilis production atas izin user. Tanpa migrasi.
- [ ] **Step 2: Commit**

```bash
git add docs/epics docs/hris/acuan-mom-20261019
git commit -m "docs: EPIC-125 pengajuan karyawan terbuka sampai closing periode"
```

## Self-review

- **Cakupan:**
  - koreksi absen, cuti, dan lembur sesudah tanggal akhir periode → Task 1, tes Step 1 (ketiga jalur);
  - periode closed tetap menolak → Task 1, tes Step 1 (23-11);
  - aturan lain tetap → lembur H+1 di Step 1 dan tes EPIC-084 lain tidak diubah;
  - dokumen → Task 2.
- **Review Focus:**
  - batas hari closing → Step 1 (22-11 vs 23-11);
  - H+1 → Step 1;
  - cuti lintas periode → Step 4 (`TestLeaveAcrossPeriodsBeforeClosing`);
  - HR → tes HR yang ada tidak diubah;
  - approve setelah closing → dicatat sebagai follow-up di epic.
- **Proporsi:** satu fungsi berubah dan tiga tes lama disesuaikan; plan tidak memuat kode selain nama tes dan nilai.
