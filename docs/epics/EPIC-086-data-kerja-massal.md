# EPIC-086: Pengisian Massal Data Kerja Karyawan (Jadwal, Shift, Penempatan) via Template Excel (`MODULE-HRIS`)

status: review
environment: dev
retries: 0

## Goal
Permintaan user 03-11-2026. Banyak karyawan di production belum lengkap datanya: jadwal kerja, tipe jadwal, shift, atau penempatan. Mengisinya satu per satu di UI terlalu lama.

Alurnya:
1. HR/admin mengisi **satu file Excel** untuk semua cabang.
2. Tim mengunggahnya ke HRIS lewat **skrip ops sekali jalan**:
   - pratinjau dulu;
   - backup sebelum menulis;
   - satu transaksi dengan audit log.
3. Tidak ada fitur UI baru. Tidak ada akun atau sesi uji di production.

## Audit (staging = salinan prod; karyawan terbaru dibuat 27-10-2026)
| Tipe | Aktif | Tanpa penempatan utama | Tanpa pola mingguan | Tanpa jadwal per tanggal ke depan | Tanpa jadwal sama sekali | Tanpa dept | Tanpa jabatan |
|---|---|---|---|---|---|---|---|
| shift | 207 | 0 | 207 | 206 | **206** | 10 | 5 |
| non_shift | 125 | 1 | 0 | 125 | 0 | 0 | 2 |

- `schedule_type` selalu terisi.
- Non-Shift sudah punya pola tetap per departemen (EPIC-078).
- Celah terbesar: **staf Shift tanpa jadwal apa pun**.
- Angka production diambil ulang lewat mode `audit` (baca saja) di VPS.

## Fitur yang sudah ada (tidak dibangun ulang)
- Unggah Excel **Jadwal Kerja per tanggal** (Spec 2026-10-19, `domain/schedule_upload.go`, `POST /api/hris/schedules/import`):
  - per cabang, rentang 7–45 hari;
  - HR atau pengatur jadwal toko (EPIC-071).
- **Penempatan massal** (EPIC-070, `POST /api/hris/employee-placements/bulk`): per cabang, lewat UI.
- **Pola mingguan** per karyawan (`SaveShiftPattern`).
- Tipe jadwal massal (`changeScheduleTypeTx`):
  - ke Non-Shift memasang pola tetap departemen (`applyFixedSchedule`) dan menghapus roster;
  - ke Shift menghentikan pola mingguan.
- Impor **Data Karyawan** (Cabang Utama, Tipe Jadwal, Departemen, Jabatan).

**Celah:** belum ada satu file lintas cabang yang sekaligus memuat penempatan, tipe jadwal, pola mingguan dan tanggal berlaku, yang bisa diterapkan oleh tim ops di production tanpa sesi login.

## Ruling (dari permintaan user)
- **R1 Alat**: perintah Go `backend/cmd/hris-data-kerja` dengan mode:
  - `audit` — hitung celah, baca saja;
  - `template` — xlsx dari master data live;
  - `preview` — validasi file;
  - `apply` — terapkan.

  Logikanya di paket `hris` dan memakai ulang `changeScheduleTypeTx`, `SaveShiftPattern`, penempatan, `periodContaining`, dan `writeEmployeeAudit`.
- **Cara jalan**: binary statis di-`docker cp` ke container `paddydev-api`, lalu `docker exec`. DB URL dipakai dari env container, jadi rahasia tidak keluar dari server. Runner: `tools/ops/data-kerja.sh` (lokal staging dan `ssh paddy-prod`).
- **R2 Template** (sheet "Data"):
  - **Terkunci**: NIP, Nama, Departemen saat ini, Jabatan saat ini, Penempatan saat ini, Tipe saat ini, Yang Perlu Dilengkapi.
  - **Diisi HR**:
    - Penempatan Utama (**kode toko/cabang**);
    - Tipe Jadwal (`Shift`/`Non-Shift`);
    - Senin…Minggu (kode shift, `LIBUR`, atau `KODE1+KODE2`);
    - Berlaku Mulai (YYYY-MM-DD);
    - Departemen dan Jabatan baru (opsional);
    - Catatan.
  - Dropdown dari master live: kode toko aktif, kode shift aktif + `LIBUR`, tipe jadwal, departemen, jabatan.
  - Sheet "Petunjuk" dan sheet referensi (Kode Toko, Kode Shift, Departemen, Jabatan).
  - Isi: karyawan aktif yang belum lengkap. Baris kosong tambahan boleh untuk NIP lain.
  - Template yang dikirim 03-11 dari staging diganti versi ini.
- **R3 Validasi** (pratinjau, tanpa menulis). Sel kosong = tidak berubah. Kesalahan memblokir baris:
  - NIP tidak dikenal, nonaktif, atau ganda;
  - kode toko, shift, departemen, atau jabatan tidak dikenal atau nonaktif;
  - Tipe Jadwal bukan Shift/Non-Shift;
  - **Non-Shift dengan kode shift**: Non-Shift mengikuti pola tetap departemen, jadi Senin–Minggu harus kosong;
  - pola Shift tidak lengkap 7 hari;
  - dua shift tumpang tindih (`domain.TwoShiftIssue`);
  - Berlaku Mulai kosong padahal ada perubahan penempatan, tipe, atau pola; tidak valid; **lampau** (< hari ini WIB); atau periodenya **sudah closing**.
- **R3 Peringatan** (boleh lanjut):
  - pola Shift tanpa LIBUR, atau masuk lebih dari 6 hari;
  - karyawan sudah punya roster per tanggal setelah Berlaku Mulai (roster tetap menang);
  - perubahan tipe akan menghapus roster atau pola.
- **R4 Terapkan**: mode `apply` menolak bila argumen `-sha` ≠ sha256 file yang sudah dipratinjau atau masih ada baris error. Satu transaksi, urutan per karyawan:
  1. departemen/jabatan;
  2. penempatan utama mulai Berlaku Mulai: penempatan utama lama ditutup H-1 (atau penempatan non-utama di toko yang sama dijadikan utama);
  3. tipe jadwal (`changeScheduleTypeTx`);
  4. pola (`SaveShiftPattern`).

  Setiap perubahan diaudit dengan actor_name "Sistem (unggah massal data kerja)", ditambah satu baris ringkasan `hris.bulk_work_data` berisi sha file dan jumlah perubahan. Runner melakukan backup `pg_dump` sebelum `apply`.
- **R5 Alur production**:
  1. gladi di staging dengan file contoh;
  2. `template` dari prod → kirim ke user;
  3. tunggu file terisi;
  4. `preview` di prod;
  5. izin user dan HR;
  6. backup → `apply` → verifikasi `audit` ulang.

## Petunjuk ops
```bash
ENV=staging tools/ops/data-kerja.sh audit
ENV=prod tools/ops/data-kerja.sh template          # → ~/Desktop/paddydev-backups/data-kerja/prod-<stamp>/Template-*.xlsx
ENV=prod tools/ops/data-kerja.sh preview isi.xlsx   # laporan + sha256
DATA_KERJA_APPROVED=yes ENV=prod tools/ops/data-kerja.sh apply isi.xlsx <sha256>   # pg_dump dulu, satu transaksi
```

- Hasil (laporan, template, salinan input) disimpan di `~/Desktop/paddydev-backups/data-kerja/<env>-<stamp>/`.
- Binary dan file input dihapus dari server dan container setelah selesai.

## Tasks
Rencana: `docs/superpowers/plans/2026-11-03-data-kerja-massal.md`.

## Acceptance Criteria
- [x] `audit`, `template`, `preview`, dan `apply` berjalan di staging lewat runner.
- [x] Template dari master live (kode toko, shift + LIBUR) berisi karyawan yang belum lengkap.
- [x] Pratinjau melaporkan baris valid, error (dengan alasan), dan peringatan sesuai R3.
- [x] `apply` atomik: satu baris error → tidak ada yang tertulis; audit lengkap; `audit` ulang menunjukkan celah tertutup.
- [ ] Production hanya dengan izin user dan HR, setelah backup.

## Automation Log
- 2026-11-03 — Audit staging + pemetaan fitur; epic + rencana ditulis. Worktree `.worktrees/epic086`, branch `feat/epic086-data-kerja-massal`, ditumpuk di atas EPIC-085 (#70) supaya staging tetap utuh.
- 2026-11-03 — Implementasi native, 4 task + perbaikan review akhir; HEAD `784131d2`.
  - Gates:
    - `go vet` + `gofmt` bersih;
    - `go test -p 1 ./...` hijau;
    - tes runner PASS.
  - Review akhir: 0 Critical, 2 Important. Keduanya diperbaiki:
    - NIP bisa diketik di baris kosong;
    - ubah ke Shift tanpa pola = error; ubah ke Non-Shift yang menghapus roster = peringatan + catatan apply.
  - Minor yang ikut diperbaiki:
    - peringatan roster memakai tanggal roster terakhir;
    - kode toko/departemen ganda di master ditolak;
    - runner: validasi sha, `trap` cleanup, `umask 077`, `GOARCH=amd64`, path dump dikutip;
    - flag `-today` dihapus.
  - Minor ditunda:
    - audit penempatan tanpa "before" (pg_dump menjadi catatan);
    - perubahan departemen memicu jadwal tetap mulai hari ini tanpa peringatan pratinjau;
    - jabatan ganda tetap tampil di dropdown.
- 2026-11-03 — **Gladi staging** (runner, binary di container `paddydev-api`):
  - audit: shift 207/206 tanpa jadwal, non_shift 125/3 belum lengkap;
  - template dibuat;
  - file contoh berisi 1 error: pratinjau melaporkan `Kode toko ZZZ`, apply ditolak dan tidak menulis apa pun;
  - file benar:
    - pratinjau 4 valid;
    - apply (backup `db.dump`): 3 pola + 1 penempatan + 5 audit;
    - audit ulang 206 → 203.
  - Data contoh dihapus kembali dari staging; audit kembali 206. Baris audit append-only tetap ada.
  - Template staging 03-11 yang dikirim lebih awal sudah tidak dipakai. Template production dibuat dengan `ENV=prod … template` setelah izin.

