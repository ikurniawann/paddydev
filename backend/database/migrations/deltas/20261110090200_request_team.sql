-- EPIC-091 (kinerja): tim penyetuju dihitung sekali dari atas ke bawah.
-- hris.request_team(atasan) = karyawan yang pengajuannya boleh diputus atasan
-- itu, sama persis dengan hris.is_request_approver(karyawan, atasan), tetapi
-- tanpa menjalankan dua rekursi per karyawan (≈0,9 dtk untuk Manager dengan
-- ~300 bawahan di staging). Karyawan e masuk tim A bila A adalah atasan
-- minimal SPV (aktif, berakun) terdekat e, atau Manager terdekat e: tidak ada
-- atasan memenuhi syarat yang sama di antara e dan A.
CREATE OR REPLACE FUNCTION hris.request_team(p_actor uuid)
RETURNS SETOF uuid
LANGUAGE sql
STABLE
AS $$
  WITH RECURSIVE flags AS (
    SELECT e.id,
           e.is_active AND e.user_id IS NOT NULL
             AND (nullif(lower(trim(p.level)), '') IS NULL
                  OR lower(trim(p.level)) IN ('supervisor', 'assistant manager', 'manager', 'senior manager',
                                              'general manager', 'director', 'c-level')) AS spv_ok,
           e.is_active AND e.user_id IS NOT NULL
             AND lower(trim(p.level)) IN ('manager', 'senior manager', 'general manager', 'director', 'c-level') AS mgr_ok
    FROM hris.employees e LEFT JOIN hris.positions p ON p.id = e.job_title_id
  ),
  actor AS (SELECT spv_ok, coalesce(mgr_ok, false) AS mgr_ok FROM flags WHERE id = p_actor),
  down AS (
    -- blocked_*: an eligible approver of that kind sits strictly between the
    -- actor and this employee, so the actor is not the nearest one.
    SELECT e.id, ARRAY[p_actor, e.id] AS seen, false AS blocked_spv, false AS blocked_mgr, 1 AS depth
    FROM hris.employees e WHERE e.reporting_to = p_actor AND e.id <> p_actor
    UNION ALL
    SELECT c.id, d.seen || c.id,
           d.blocked_spv OR coalesce(f.spv_ok, false),
           d.blocked_mgr OR coalesce(f.mgr_ok, false),
           d.depth + 1
    FROM down d
    JOIN flags f ON f.id = d.id
    JOIN hris.employees c ON c.reporting_to = d.id
    WHERE d.depth < 10 AND NOT c.id = ANY(d.seen)
  )
  SELECT DISTINCT d.id FROM down d, actor a
  WHERE (a.spv_ok AND NOT d.blocked_spv) OR (a.mgr_ok AND NOT d.blocked_mgr)
$$;

COMMENT ON FUNCTION hris.request_team(uuid) IS
  'EPIC-091: karyawan yang pengajuannya boleh diputus atasan ini (SPV terdekat atau Manager terdekat); setara is_request_approver';
COMMENT ON FUNCTION hris.is_request_approver(uuid, uuid) IS
  'EPIC-091: p_actor boleh memutus pengajuan p_employee (SPV terdekat atau Manager di atasnya)';
