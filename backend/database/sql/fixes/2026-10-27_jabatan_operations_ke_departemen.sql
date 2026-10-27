-- =============================================================================
-- Pindahkan jabatan berdepartemen "Operations" ke departemen yang benar
-- (keputusan user 27-10-2026). "Operations" bukan departemen: itu nilai
-- bawaan saat jabatan disimpan tanpa departemen (Master Data → Jabatan,
-- lowongan Rekrutmen, import Odoo). Form karyawan hanya menampilkan jabatan
-- yang teks departemennya sama dengan nama departemen terpilih, sehingga
-- jabatan "Operations" tidak pernah bisa dipilih.
--
-- Aturan: departemen tujuan = departemen semua karyawan pemegang jabatan itu
-- (aktif maupun tidak). Jabatan yang pemegangnya tersebar di lebih dari satu
-- departemen membatalkan skrip. Jabatan tanpa pemegang TIDAK disentuh; daftarnya
-- dicetak untuk diputuskan HR.
-- Setiap perubahan dicatat di audit.audit_log (action position.department_fixed).
--
-- Pratinjau (transaksi read-only + ROLLBACK):
--   psql "$U" -v ON_ERROR_STOP=1 -v preview=1 -v expected=73 -f backend/database/sql/fixes/2026-10-27_jabatan_operations_ke_departemen.sql
-- Terapkan (setelah backup; expected = jumlah "Akan dipindah" dari pratinjau):
--   psql "$U" -v ON_ERROR_STOP=1 -v expected=73 -f backend/database/sql/fixes/2026-10-27_jabatan_operations_ke_departemen.sql
-- Kembalikan: lihat entri audit position.department_fixed (before.department).
-- =============================================================================
\if :{?expected}
\else
  \echo 'Wajib -v expected=<jumlah jabatan yang akan dipindah>'
  \quit
\endif

BEGIN;
SET LOCAL search_path = hris, public;

CREATE TEMP TABLE jabatan_peta ON COMMIT DROP AS
SELECT p.id, p.title,
       min(d.name) AS departemen_baru,
       count(DISTINCT d.id) AS jumlah_departemen,
       count(DISTINCT e.id) AS pemegang,
       count(DISTINCT e.id) FILTER (WHERE e.is_active) AS pemegang_aktif
FROM hris.positions p
JOIN hris.employees e ON e.job_title_id = p.id
JOIN hris.departments d ON d.id = e.department_id
WHERE p.department = 'Operations'
GROUP BY p.id, p.title;

\if :{?preview}
  SET TRANSACTION READ ONLY;
\endif

\echo 'Akan dipindah (judul, departemen baru, pemegang, pemegang aktif):'
SELECT title, departemen_baru, pemegang, pemegang_aktif FROM jabatan_peta
WHERE jumlah_departemen = 1 ORDER BY departemen_baru, title;

\echo 'Pemegang di lebih dari satu departemen (membatalkan skrip):'
SELECT m.title, string_agg(DISTINCT d.name, ', ') AS departemen
FROM jabatan_peta m
JOIN hris.employees e ON e.job_title_id = m.id
JOIN hris.departments d ON d.id = e.department_id
WHERE m.jumlah_departemen > 1 GROUP BY m.title ORDER BY m.title;

\echo 'Tidak disentuh — jabatan "Operations" tanpa pemegang (perlu keputusan HR):'
SELECT p.title, p.level, p.is_active FROM hris.positions p
WHERE p.department = 'Operations'
  AND NOT EXISTS (SELECT 1 FROM jabatan_peta m WHERE m.id = p.id)
ORDER BY p.title;

SELECT set_config('paddy.expected', :'expected', true);
DO $$
DECLARE
  multi int;
  siap int;
  diubah int;
BEGIN
  SELECT count(*) INTO multi FROM jabatan_peta WHERE jumlah_departemen > 1;
  IF multi > 0 THEN
    RAISE EXCEPTION '% jabatan dipegang karyawan dari lebih dari satu departemen — tidak ada perubahan', multi;
  END IF;
  SELECT count(*) INTO siap FROM jabatan_peta;
  IF siap <> current_setting('paddy.expected')::int THEN
    RAISE EXCEPTION 'Jabatan yang akan dipindah % (expected %) — data berubah sejak pratinjau, tidak ada perubahan',
      siap, current_setting('paddy.expected');
  END IF;
  IF current_setting('transaction_read_only') = 'on' THEN
    RETURN; -- pratinjau
  END IF;

  INSERT INTO audit.audit_log (actor_name, action, entity, entity_id, entity_label, before, after, reason)
  SELECT 'Ops (Claude Code atas persetujuan user)', 'position.department_fixed', 'hris.positions', m.id::text, m.title,
         jsonb_build_object('department', 'Operations'), jsonb_build_object('department', m.departemen_baru),
         'Departemen jabatan diturunkan dari pemegangnya (27-10-2026)'
  FROM jabatan_peta m;

  UPDATE hris.positions p SET department = m.departemen_baru
  FROM jabatan_peta m WHERE p.id = m.id AND p.department = 'Operations';
  GET DIAGNOSTICS diubah = ROW_COUNT;
  IF diubah <> siap THEN
    RAISE EXCEPTION 'Diubah % dari % — dibatalkan', diubah, siap;
  END IF;
  RAISE NOTICE 'Jabatan dipindah: %', diubah;
END $$;

\echo 'Sesudah — jumlah jabatan per teks departemen yang tidak cocok dengan departemen mana pun:'
SELECT coalesce(p.department, '(kosong)') AS departemen, count(*) FROM hris.positions p
WHERE NOT EXISTS (SELECT 1 FROM hris.departments d WHERE d.name = p.department)
GROUP BY 1 ORDER BY 1;

\if :{?preview}
  ROLLBACK;
\else
  COMMIT;
\endif
