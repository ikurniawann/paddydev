-- EPIC-083 / DM-01 — PRATINJAU baca-saja cleanup data per 20 Oktober 2026. Tidak menulis apa pun.
--
-- Dasar: MOM 29 Oktober 2026 (WIT × Paddy) §2 No. 6, DM-01 ("database cleanup per tanggal
-- 20 Oktober 2026 … didahului backup") dan K-22. Jawaban user 02-11-2026:
--   - cakupan: data bertanggal ≤ 20 Oktober 2026 (absensi, pengajuan absensi, cuti & izin,
--     lembur, perjalanan dinas, pinjaman/kasbon);
--   - hapus permanen, didahului backup penuh database dan file lampiran;
--   - pinjaman yang masih berjalan (approved, sisa > 0) tidak dihapus;
--   - saldo cuti (hris.leave_balances) tidak diubah.
-- Pasangannya: 2026-11-03_dm01_cleanup_core.sql (predikat harus identik; dijaga tes integrasi
-- backend/internal/modules/hris/dm01_cleanup_integration_test.go).
--
-- Pemakaian (tanpa -1; transaksi READ ONLY sudah ada di file ini):
--   psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -f backend/database/sql/fixes/2026-11-03_dm01_preview.sql
--
-- Keluaran: satu baris per hasil, kolom dipisah TAB, urut per bagian:
--   HAPUS        <tabel> <n>                       baris yang akan dihapus
--   TETAP        <tabel> <n>                       baris yang tetap ada (seluruh tabel)
--   STATUS       <tabel> hapus|tetap <status> <n>  rincian status
--   PENGECUALIAN <tabel> <alasan> <n>              bertanggal ≤ 20 Okt tetapi TIDAK dihapus
--   CHECKSUM     leave_balances <jumlah baris> <md5>  harus sama sebelum dan sesudah eksekusi
--   BERKAS       <kolom> referensi_hapus|dipakai_baris_tetap|di_luar_folder_baris|tidak_valid <n>
--   FILE_LAIN    <nilai>   nilai lampiran di luar folder milik barisnya (tidak dihapus otomatis;
--                          tinjau manual)
--   FILE         <path>    path relatif di bawah <storage>/private milik baris yang dihapus,
--                          sudah dikurangi path yang juga dirujuk baris yang tetap ada
--
-- FILE hanya untuk nilai berbentuk <folder kolom>/<employee_id baris>/<nama file>, sama dengan
-- tata letak upload (storage.savePrivate: <folder>/<pemilik>/<nama>):
--   attendance.clock_in/out_photo_url   attendance/<id>/…          (attendance_clock.go saveSelfie)
--   attendance_requests.attachment_path attendance-requests/<id>/… (attendance_requests.go)
--   leaves.attachment_url               leave-attachments/<id>/…   (http_files.go uploadLeaveAttachment)
--   overtime_requests.attachments       overtime-attachments/<id>/… (http_files.go, cek pemilik)
--   loans.attachments / collateral_…    loan-attachments/<id>/…    (payroll/loan_files.go, cek pemilik)
-- Nilai lain (folder lain, karyawan lain, URL, path absolut, '..') menjadi FILE_LAIN.
--
-- Batas tanggal (R1, inklusif): attendance.date, attendance_requests.date, overtime_requests.date;
-- leaves.end_date dan business_trips.end_date (yang melintasi 21 Okt tetap ada); loans.requested_date
-- (timestamptz, dibaca sebagai tanggal WIB). Keputusan user 06-11-2026 (R2'): pinjaman/kasbon
-- TIDAK dibersihkan; yang bertanggal ≤ 20 Okt muncul di PENGECUALIAN.
\set QUIET on
\set ON_ERROR_STOP on
\pset tuples_only on
\pset format unaligned
BEGIN READ ONLY;

WITH cutoff AS (SELECT DATE '2026-10-20' AS d),
tabel (urut, nama) AS (
  VALUES (1, 'attendance_requests'), (2, 'attendance'), (3, 'leaves'),
         (4, 'overtime_requests'), (5, 'business_trips'), (6, 'loans')
),
att AS (
  SELECT a.employee_id, a.status::text AS status, a.date <= c.d AS hapus, a.clock_in_photo_url, a.clock_out_photo_url
  FROM hris.attendance a, cutoff c
),
req AS (
  SELECT r.employee_id, r.status, r.date <= c.d AS hapus, r.attachment_path
  FROM hris.attendance_requests r, cutoff c
),
lv AS (
  SELECT l.employee_id, l.status::text AS status, l.end_date <= c.d AS hapus,
         CASE WHEN l.end_date > c.d AND l.start_date <= c.d THEN 'melintasi 21 Okt' END AS alasan,
         l.attachment_url
  FROM hris.leaves l, cutoff c
),
ot AS (
  SELECT o.employee_id, o.status, o.date <= c.d AS hapus, o.attachments
  FROM hris.overtime_requests o, cutoff c
),
bt AS (
  SELECT b.status, b.end_date <= c.d AS hapus,
         CASE WHEN b.end_date > c.d AND b.start_date <= c.d THEN 'melintasi 21 Okt' END AS alasan
  FROM hris.business_trips b, cutoff c
),
-- R2' (keputusan user 06-11-2026): pinjaman/kasbon tidak dibersihkan; yang bertanggal ≤ batas
-- tetap dihitung di PENGECUALIAN supaya terlihat di pratinjau.
ln AS (
  SELECT l.employee_id, l.status, false AS hapus,
         CASE WHEN (l.requested_date AT TIME ZONE 'Asia/Jakarta')::date <= c.d
              THEN 'pinjaman tidak dibersihkan (keputusan user 06-11)' END AS alasan,
         l.attachments, l.collateral_attachments
  FROM hris.loans l, cutoff c
),
baris AS (
  SELECT 'attendance' AS tabel, status, hapus, NULL::text AS alasan FROM att
  UNION ALL SELECT 'attendance_requests', status, hapus, NULL FROM req
  UNION ALL SELECT 'leaves', status, hapus, alasan FROM lv
  UNION ALL SELECT 'overtime_requests', status, hapus, NULL FROM ot
  UNION ALL SELECT 'business_trips', status, hapus, alasan FROM bt
  UNION ALL SELECT 'loans', status, hapus, alasan FROM ln
),
rujukan AS (
  SELECT 'attendance.clock_in_photo_url' AS kolom, 'attendance' AS folder, employee_id, clock_in_photo_url AS nilai, hapus FROM att
  UNION ALL SELECT 'attendance.clock_out_photo_url', 'attendance', employee_id, clock_out_photo_url, hapus FROM att
  UNION ALL SELECT 'attendance_requests.attachment_path', 'attendance-requests', employee_id, attachment_path, hapus FROM req
  UNION ALL SELECT 'leaves.attachment_url', 'leave-attachments', employee_id, attachment_url, hapus FROM lv
  UNION ALL SELECT 'overtime_requests.attachments', 'overtime-attachments', employee_id, p.nilai, hapus
            FROM ot, unnest(ot.attachments) AS p(nilai)
  UNION ALL SELECT 'loans.attachments', 'loan-attachments', employee_id, p.nilai, hapus
            FROM ln, unnest(ln.attachments) AS p(nilai)
  UNION ALL SELECT 'loans.collateral_attachments', 'loan-attachments', employee_id, p.nilai, hapus
            FROM ln, unnest(ln.collateral_attachments) AS p(nilai)
),
berisi AS (
  SELECT kolom, folder, employee_id, nilai, hapus FROM rujukan WHERE nilai IS NOT NULL AND btrim(nilai) <> ''
),
tetap AS (SELECT DISTINCT nilai FROM berisi WHERE NOT hapus),
hapus_berkas AS (
  SELECT b.kolom, b.nilai, t.nilai IS NOT NULL AS dipakai,
         CASE
           WHEN b.nilai ~ '[[:cntrl:]]' THEN 'tidak_valid'
           WHEN b.nilai ~ '^[a-z-]+/[0-9a-f-]+/[A-Za-z0-9._-]+$'
            AND split_part(b.nilai, '/', 1) = b.folder
            AND split_part(b.nilai, '/', 2) = b.employee_id::text
            AND split_part(b.nilai, '/', 3) NOT IN ('.', '..') THEN 'relatif'
           ELSE 'di_luar_folder_baris'
         END AS bentuk
  FROM berisi b LEFT JOIN tetap t ON t.nilai = b.nilai
  WHERE b.hapus
),
keluaran (bagian, kunci, baris) AS (
  SELECT 1, lpad(t.urut::text, 2, '0'), format(E'HAPUS\t%s\t%s', t.nama, count(b.tabel) FILTER (WHERE b.hapus))
  FROM tabel t LEFT JOIN baris b ON b.tabel = t.nama GROUP BY t.urut, t.nama
  UNION ALL
  SELECT 2, lpad(t.urut::text, 2, '0'), format(E'TETAP\t%s\t%s', t.nama, count(b.tabel) FILTER (WHERE NOT b.hapus))
  FROM tabel t LEFT JOIN baris b ON b.tabel = t.nama GROUP BY t.urut, t.nama
  UNION ALL
  SELECT 3, concat_ws('/', lpad(t.urut::text, 2, '0'), NOT b.hapus, COALESCE(b.status, '(kosong)')),
         format(E'STATUS\t%s\t%s\t%s\t%s', t.nama, CASE WHEN b.hapus THEN 'hapus' ELSE 'tetap' END,
                COALESCE(b.status, '(kosong)'), count(*))
  FROM tabel t JOIN baris b ON b.tabel = t.nama GROUP BY t.urut, t.nama, b.hapus, b.status
  UNION ALL
  SELECT 4, concat_ws('/', lpad(t.urut::text, 2, '0'), b.alasan), format(E'PENGECUALIAN\t%s\t%s\t%s', t.nama, b.alasan, count(*))
  FROM tabel t JOIN baris b ON b.tabel = t.nama WHERE NOT b.hapus AND b.alasan IS NOT NULL GROUP BY t.urut, t.nama, b.alasan
  UNION ALL
  SELECT 5, '', format(E'CHECKSUM\tleave_balances\t%s\t%s', count(*), COALESCE(md5(string_agg(lb::text, '' ORDER BY lb.id)), ''))
  FROM hris.leave_balances lb
  UNION ALL
  SELECT 6, kolom || '/1', format(E'BERKAS\t%s\treferensi_hapus\t%s', kolom, count(*) FILTER (WHERE NOT dipakai))
  FROM hapus_berkas GROUP BY kolom
  UNION ALL
  SELECT 6, kolom || '/2', format(E'BERKAS\t%s\tdipakai_baris_tetap\t%s', kolom, count(*) FILTER (WHERE dipakai))
  FROM hapus_berkas GROUP BY kolom
  UNION ALL
  SELECT 6, kolom || '/' || bentuk, format(E'BERKAS\t%s\t%s\t%s', kolom, bentuk, count(*))
  FROM hapus_berkas WHERE bentuk <> 'relatif' AND NOT dipakai GROUP BY kolom, bentuk
  UNION ALL
  SELECT DISTINCT 7, nilai, E'FILE_LAIN\t' || nilai FROM hapus_berkas WHERE bentuk = 'di_luar_folder_baris' AND NOT dipakai
  UNION ALL
  SELECT DISTINCT 8, nilai, E'FILE\t' || nilai FROM hapus_berkas WHERE bentuk = 'relatif' AND NOT dipakai
)
SELECT baris FROM keluaran ORDER BY bagian, kunci;

ROLLBACK;
