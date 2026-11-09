-- EPIC-091 (ralat user 05-11-2026, "route tetap bisa 2 atasan"): cuti/izin,
-- lembur, dan perjalanan dinas boleh diputus SPV terdekat
-- (hris.request_approver) ATAU Manager di atasnya; cukup salah satu.
-- request_manager = atasan terdekat berlevel Manager ke atas (aktif, berakun);
-- sama dengan payroll.LoanApproverSQL untuk kasbon/pinjaman.
CREATE OR REPLACE FUNCTION hris.request_manager(p_employee uuid)
RETURNS uuid
LANGUAGE sql
STABLE
AS $$
  WITH RECURSIVE chain AS (
    SELECT e.reporting_to AS id, 1 AS depth, ARRAY[e.id] AS seen
    FROM hris.employees e WHERE e.id = p_employee
    UNION ALL
    SELECT e.reporting_to, c.depth + 1, c.seen || e.id
    FROM chain c JOIN hris.employees e ON e.id = c.id
    WHERE c.depth < 10 AND e.reporting_to IS NOT NULL AND NOT e.reporting_to = ANY(c.seen || e.id)
  )
  SELECT m.id FROM chain c
  JOIN hris.employees m ON m.id = c.id AND m.is_active
  JOIN hris.positions p ON p.id = m.job_title_id
  WHERE c.id IS NOT NULL AND c.id <> p_employee AND m.user_id IS NOT NULL
    AND lower(trim(p.level)) IN ('manager', 'senior manager', 'general manager', 'director', 'c-level')
  ORDER BY c.depth LIMIT 1
$$;

-- is_request_approver: p_actor boleh memutus pengajuan p_employee (SPV
-- terdekat atau Manager di atasnya).
CREATE OR REPLACE FUNCTION hris.is_request_approver(p_employee uuid, p_actor uuid)
RETURNS boolean
LANGUAGE sql
STABLE
AS $$
  SELECT p_actor IS NOT NULL AND p_actor <> p_employee
     AND (hris.request_approver(p_employee) = p_actor OR hris.request_manager(p_employee) = p_actor)
$$;

COMMENT ON FUNCTION hris.request_manager(uuid) IS
  'EPIC-091: atasan terdekat berlevel Manager ke atas (aktif, berakun); boleh memutus cuti/lembur/dinas selain SPV';
