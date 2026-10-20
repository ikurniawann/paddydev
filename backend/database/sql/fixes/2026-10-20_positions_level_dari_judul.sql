-- =============================================================================
-- Isi hris.positions.level dari judul jabatan (EPIC-058: Izin Leader hanya
-- untuk Supervisor ke atas; keputusan user 20-10-2026). Import Odoo meninggalkan
-- semua jabatan berlevel Staff.
--   CEO / Chief Executive Officer  -> C-Level
--   General Manager                -> General Manager
--   Assistant Manager              -> Assistant Manager
--   ... Manager                    -> Manager
--   ... Supervisor / SPV           -> Supervisor
-- Tetap (tidak disentuh): Coordinator/Koordinator, Lead, Art Director.
-- Hanya menaikkan jabatan yang levelnya kini Staff/Senior Staff/kosong, atau
-- merapikan huruf level yang sudah sama (mis. "manager" -> "Manager"); level
-- lain tidak pernah diturunkan.
--
-- Pratinjau (tanpa perubahan: transaksi read-only + ROLLBACK):
--   psql "$U" -v ON_ERROR_STOP=1 -v preview=1 -f backend/database/sql/fixes/2026-10-20_positions_level_dari_judul.sql
-- Terapkan (setelah backup):
--   psql "$U" -v ON_ERROR_STOP=1 -f backend/database/sql/fixes/2026-10-20_positions_level_dari_judul.sql
-- =============================================================================
BEGIN;
CREATE TEMP VIEW level_usulan AS
WITH aturan AS (
  SELECT p.id, p.title, p.level AS level_lama,
    CASE
      WHEN p.title ~* '(coordinator|koordinator|\mlead\M|art director)' THEN NULL
      WHEN p.title ~* '(\mceo\M|chief executive officer)' THEN 'C-Level'
      WHEN p.title ~* 'general manager' THEN 'General Manager'
      WHEN p.title ~* 'assistant manager' THEN 'Assistant Manager'
      WHEN p.title ~* '\mmanager\M' THEN 'Manager'
      WHEN p.title ~* '(supervisor|\mspv\M)' THEN 'Supervisor'
    END AS target,
    p.title ~* '(coordinator|koordinator|\mlead\M|art director)' AS dikecualikan,
    (SELECT count(*) FROM hris.employees e WHERE e.job_title_id = p.id AND e.is_active) AS karyawan_aktif
  FROM hris.positions p)
SELECT a.*,
  CASE WHEN a.target IS NOT NULL
        AND (lower(trim(coalesce(a.level_lama, ''))) IN ('', 'staff', 'senior staff')
             OR lower(trim(a.level_lama)) = lower(a.target))
        AND a.level_lama IS DISTINCT FROM a.target
       THEN a.target END AS level_baru
FROM aturan a;

-- Pratinjau: transaksi dikunci read-only setelah view sementara dibuat, jadi
-- tidak ada penulisan apa pun (dan diakhiri ROLLBACK).
\if :{?preview}
  SET TRANSACTION READ ONLY;
\endif

\echo 'Akan diubah:'
SELECT title, level_lama, level_baru, karyawan_aktif FROM level_usulan
WHERE level_baru IS NOT NULL ORDER BY level_baru, title;

\echo 'Cocok judul tetapi level sudah lebih tinggi/berbeda (tidak diubah):'
SELECT title, level_lama, target, karyawan_aktif FROM level_usulan
WHERE target IS NOT NULL AND level_baru IS NULL AND level_lama IS DISTINCT FROM target ORDER BY title;

\echo 'Dikecualikan (tetap):'
SELECT title, level_lama, karyawan_aktif FROM level_usulan WHERE dikecualikan ORDER BY title;

\if :{?preview}
\else
  UPDATE hris.positions p SET level = u.level_baru
  FROM level_usulan u WHERE u.id = p.id AND u.level_baru IS NOT NULL;
\endif

-- Pratinjau: proyeksi sesudah; terapkan: level_baru sudah NULL, jadi level tersimpan.
\echo 'Karyawan aktif per level jabatan (sesudah):'
SELECT coalesce(nullif(trim(coalesce(u.level_baru, u.level_lama)), ''), '(tanpa jabatan)') AS level,
  count(*) AS karyawan_aktif
FROM hris.employees e LEFT JOIN level_usulan u ON u.id = e.job_title_id
WHERE e.is_active GROUP BY 1 ORDER BY 2 DESC;

\if :{?preview}
  ROLLBACK;
\else
  COMMIT;
\endif
