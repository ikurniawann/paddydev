-- EPIC-091 (ralat user 05-11-2026): penyetuju semua pengajuan (cuti/izin,
-- lembur, perjalanan dinas, kasbon/pinjaman) = atasan terdekat di rantai
-- reporting_to yang berjabatan minimal Supervisor, aktif, dan punya akun.
-- Atasan dengan level jabatan di bawah Supervisor (mis. Staff / Store
-- Leader) dilewati; level kosong dianggap memenuhi (tidak terbukti di bawah
-- SPV). Tidak ada → NULL (HR memutus sendiri). Siklus dan atasan = dirinya
-- dihentikan; maksimal 10 tingkat.
CREATE OR REPLACE FUNCTION hris.request_approver(p_employee uuid)
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
  LEFT JOIN hris.positions p ON p.id = m.job_title_id
  WHERE c.id IS NOT NULL AND c.id <> p_employee AND m.user_id IS NOT NULL
    AND (nullif(lower(trim(p.level)), '') IS NULL
         OR lower(trim(p.level)) IN ('supervisor', 'assistant manager', 'manager', 'senior manager',
                                     'general manager', 'director', 'c-level'))
  ORDER BY c.depth LIMIT 1
$$;

COMMENT ON FUNCTION hris.request_approver(uuid) IS
  'EPIC-091: penyetuju pengajuan = atasan terdekat minimal Supervisor (aktif, berakun) di rantai reporting_to; NULL = HR';

-- Kasbon/pinjaman tiga tahap: SPV (bila berbeda dari Manager) → Manager → HR.
ALTER TABLE hris.loans
  ADD COLUMN IF NOT EXISTS supervisor_approver_id uuid NULL REFERENCES hris.employees(id) ON DELETE SET NULL,
  ADD COLUMN IF NOT EXISTS supervisor_approved_by uuid NULL REFERENCES hris.employees(id) ON DELETE SET NULL,
  ADD COLUMN IF NOT EXISTS supervisor_approved_at timestamptz NULL;

COMMENT ON COLUMN hris.loans.supervisor_approver_id IS
  'EPIC-091: tahap SPV sebelum Manager (manager_approver_id); NULL = tanpa tahap SPV (penyetuju terdekat sudah Manager, atau tidak ada)';
