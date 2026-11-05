-- EPIC-092: approval berjenjang SPV → Manager untuk cuti/izin dan perjalanan
-- dinas; HR boleh mewakili tahap yang sedang berjalan. Tahap SPV yang sudah
-- lewat dicatat di spv_approved_*; tahap Manager memakai kolom keputusan lama.
ALTER TABLE hris.leaves
  ADD COLUMN IF NOT EXISTS spv_approved_by uuid NULL REFERENCES hris.employees(id) ON DELETE SET NULL,
  ADD COLUMN IF NOT EXISTS spv_approved_name text NULL,
  ADD COLUMN IF NOT EXISTS spv_approved_at timestamptz NULL;

ALTER TABLE hris.business_trips
  ADD COLUMN IF NOT EXISTS spv_approved_by uuid NULL REFERENCES hris.employees(id) ON DELETE SET NULL,
  ADD COLUMN IF NOT EXISTS spv_approved_name text NULL,
  ADD COLUMN IF NOT EXISTS spv_approved_at timestamptz NULL;

-- Kasbon: nama yang menyetujui tahap SPV/Manager (bisa "HR (mewakili …)").
ALTER TABLE hris.loans
  ADD COLUMN IF NOT EXISTS supervisor_approved_name text NULL,
  ADD COLUMN IF NOT EXISTS manager_approved_name text NULL;

-- stage_approver: penyetuju tahap berjalan (SPV dulu bila ada Manager lain di
-- atasnya, lalu Manager); NULL = tanpa atasan (HR saja).
CREATE OR REPLACE FUNCTION hris.stage_approver(p_employee uuid, p_spv_done boolean)
RETURNS uuid
LANGUAGE sql
STABLE
AS $$
  SELECT CASE
           WHEN s IS NULL THEN m
           WHEN m IS NULL OR m = s THEN s
           WHEN p_spv_done THEN m
           ELSE s
         END
  FROM (SELECT hris.request_approver(p_employee) AS s, hris.request_manager(p_employee) AS m) x
$$;

COMMENT ON FUNCTION hris.stage_approver(uuid, boolean) IS
  'EPIC-092: penyetuju tahap berjalan pengajuan berjenjang (SPV lalu Manager); NULL = HR saja';
COMMENT ON COLUMN hris.leaves.spv_approved_at IS
  'EPIC-092: tahap SPV selesai (oleh SPV atau HR mewakili); tahap berikutnya Manager';
