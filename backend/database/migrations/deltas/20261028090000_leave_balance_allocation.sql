-- =============================================================================
-- EPIC-068: alokasi saldo cuti oleh HR.
--
-- 1) hris.default_annual_leave(date, int): jatah cuti tahunan default (R1),
--    kembar dengan domain.DefaultAnnualLeave di Go. Cuti baru berhak setelah
--    12 bulan masa kerja: A = join + 12 bulan; A setelah 31-12-Y -> 0; A pada
--    atau sebelum 01-01-Y -> 12; selain itu prorata hari A..31-12-Y dari hari
--    dalam tahun Y, dibulatkan ke bawah ke kelipatan 0,5; join NULL -> 12.
-- 2) hris.leave_balances: kolom penyesuaian dan sisa bawaan (R3).
--    annual_leave_remaining diganti menjadi total + penyesuaian - terpakai
--    (generated); CHECK kelipatan 0,5 dan sisa >= 0. Tidak ada view/fungsi
--    yang bergantung pada kolom itu (pg_depend hanya constraint-nya).
-- 3) hris.leaves.carry_over_days_used: porsi sisa bawaan yang dipakai satu cuti
--    tahunan (R5). Cuti lama = 0, jadi pembatalannya kembali ke jatah.
-- 4) hris.leave_settings (satu baris) dan hris.leave_carry_over_runs (klaim job
--    1 Januari, satu baris per tahun).
-- 5) Menu daun hris.workforce.leave-balances "Saldo Cuti" + grant + masker (R7).
-- 6) Trigger karyawan baru memakai R1 untuk tahun bergabung dan tahun berjalan;
--    karyawan aktif tanpa baris tahun berjalan dibuatkan (R14). Baris yang sudah
--    ada TIDAK dihitung ulang (R2).
--
-- Aditif dan idempoten. Cek akses sebelum/sesudah:
-- database/sql/checks/2026-10-28_epic068_leave_balance_access.sql.
-- =============================================================================

-- ── 1) Jatah default ─────────────────────────────────────────────────────────
-- Aritmetika bilangan bulat agar pembulatan ke bawah tepat: floor(24 x hari /
-- hari-dalam-tahun) / 2. "+ interval '12 months'" menjepit 29-02 ke 28-02,
-- sama dengan Go.
CREATE OR REPLACE FUNCTION hris.default_annual_leave(p_join date, p_year int)
RETURNS numeric
LANGUAGE sql IMMUTABLE
AS $$
  SELECT CASE
    WHEN p_join IS NULL THEN 12::numeric
    WHEN (p_join + interval '12 months')::date > make_date(p_year, 12, 31) THEN 0::numeric
    WHEN (p_join + interval '12 months')::date <= make_date(p_year, 1, 1) THEN 12::numeric
    ELSE (
      (24 * (make_date(p_year, 12, 31) - (p_join + interval '12 months')::date + 1))
        / (make_date(p_year + 1, 1, 1) - make_date(p_year, 1, 1))
    )::numeric / 2
  END
$$;

COMMENT ON FUNCTION hris.default_annual_leave(date, int) IS
  'EPIC-068 R1: jatah cuti tahunan default tahun Y. Kembar dengan domain.DefaultAnnualLeave (Go).';

-- ── 2) hris.leave_balances ───────────────────────────────────────────────────
-- Data lama harus sudah kelipatan 0,5 sebelum CHECK dipasang; migrasi tidak
-- mengubah nilai baris yang ada, jadi berhenti dengan pesan yang jelas.
DO $$
DECLARE bad int;
BEGIN
  SELECT count(*) INTO bad FROM hris.leave_balances WHERE mod(annual_leave_total * 2, 1) <> 0;
  IF bad > 0 THEN
    RAISE EXCEPTION 'EPIC-068: % baris hris.leave_balances punya annual_leave_total bukan kelipatan 0,5; rapikan dulu', bad;
  END IF;
END
$$;

ALTER TABLE hris.leave_balances
  ADD COLUMN IF NOT EXISTS annual_leave_adjustment numeric(5,2) NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS carry_over_days         numeric(5,2) NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS carry_over_used         numeric(5,2) NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS carry_over_expires_on   date;

DO $$
BEGIN
  -- Ganti ekspresi kolom generated hanya bila belum memuat penyesuaian.
  IF NOT EXISTS (
    SELECT 1 FROM pg_attrdef d
    JOIN pg_attribute a ON a.attrelid = d.adrelid AND a.attnum = d.adnum
    WHERE d.adrelid = 'hris.leave_balances'::regclass AND a.attname = 'annual_leave_remaining'
      AND pg_get_expr(d.adbin, d.adrelid) LIKE '%annual_leave_adjustment%') THEN
    ALTER TABLE hris.leave_balances DROP CONSTRAINT IF EXISTS annual_leave_non_negative;
    ALTER TABLE hris.leave_balances DROP COLUMN annual_leave_remaining;
    ALTER TABLE hris.leave_balances
      ADD COLUMN annual_leave_remaining numeric(5,2)
      GENERATED ALWAYS AS (annual_leave_total + annual_leave_adjustment - annual_leave_used) STORED;
  END IF;

  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid = 'hris.leave_balances'::regclass AND conname = 'annual_leave_non_negative') THEN
    ALTER TABLE hris.leave_balances
      ADD CONSTRAINT annual_leave_non_negative CHECK (annual_leave_remaining >= 0::numeric);
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid = 'hris.leave_balances'::regclass AND conname = 'leave_balances_half_steps') THEN
    ALTER TABLE hris.leave_balances
      ADD CONSTRAINT leave_balances_half_steps CHECK (
        mod(annual_leave_total * 2, 1) = 0
        AND mod(annual_leave_adjustment * 2, 1) = 0
        AND mod(carry_over_days * 2, 1) = 0);
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid = 'hris.leave_balances'::regclass AND conname = 'leave_balances_carry_over_valid') THEN
    ALTER TABLE hris.leave_balances
      ADD CONSTRAINT leave_balances_carry_over_valid CHECK (
        carry_over_days >= 0 AND carry_over_used >= 0 AND carry_over_used <= carry_over_days);
  END IF;
END
$$;

COMMENT ON COLUMN hris.leave_balances.annual_leave_adjustment IS
  'Penyesuaian HR atas jatah tahun ini (boleh negatif, kelipatan 0,5).';
COMMENT ON COLUMN hris.leave_balances.carry_over_days IS
  'Sisa bawaan dari tahun lalu (kelipatan 0,5). Berlaku hanya bila carry_over_expires_on terisi dan belum lewat (R4).';
COMMENT ON COLUMN hris.leave_balances.carry_over_used IS 'Bagian sisa bawaan yang sudah dipakai cuti tahunan disetujui.';
COMMENT ON COLUMN hris.leave_balances.carry_over_expires_on IS 'Batas akhir pemakaian sisa bawaan (inklusif); NULL = bawaan tidak berlaku.';

-- ── 3) hris.leaves ───────────────────────────────────────────────────────────
ALTER TABLE hris.leaves
  ADD COLUMN IF NOT EXISTS carry_over_days_used numeric(5,2) NOT NULL DEFAULT 0;

DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid = 'hris.leaves'::regclass AND conname = 'leaves_carry_over_days_used_valid') THEN
    ALTER TABLE hris.leaves
      ADD CONSTRAINT leaves_carry_over_days_used_valid CHECK (carry_over_days_used >= 0);
  END IF;
END
$$;

COMMENT ON COLUMN hris.leaves.carry_over_days_used IS
  'Porsi total_days yang diambil dari sisa bawaan saat cuti tahunan disetujui (R5); sisanya dari jatah. Cuti sebelum EPIC-068 = 0.';

-- ── 4) Pengaturan dan klaim job ──────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS hris.leave_settings (
  id                        boolean PRIMARY KEY DEFAULT true CHECK (id),
  carry_over_enabled        boolean NOT NULL DEFAULT false,
  carry_over_max_days       numeric(5,2),
  carry_over_expiry_month   smallint NOT NULL DEFAULT 3 CHECK (carry_over_expiry_month BETWEEN 1 AND 12),
  carry_over_expiry_day     smallint NOT NULL DEFAULT 31 CHECK (carry_over_expiry_day BETWEEN 1 AND 31),
  updated_at                timestamptz NOT NULL DEFAULT now(),
  updated_by                uuid,
  updated_by_name           text,
  CONSTRAINT leave_settings_max_valid CHECK (
    carry_over_max_days IS NULL OR (carry_over_max_days >= 0 AND mod(carry_over_max_days * 2, 1) = 0)),
  -- 29-02 ditolak: tanggal harus ada di setiap tahun.
  CONSTRAINT leave_settings_expiry_date_valid CHECK (
    carry_over_expiry_day <= CASE carry_over_expiry_month
      WHEN 2 THEN 28 WHEN 4 THEN 30 WHEN 6 THEN 30 WHEN 9 THEN 30 WHEN 11 THEN 30 ELSE 31 END)
);

INSERT INTO hris.leave_settings (id) VALUES (true) ON CONFLICT (id) DO NOTHING;

COMMENT ON TABLE hris.leave_settings IS
  'Pengaturan sisa cuti bawaan (satu baris): carry_over_max_days NULL = semua sisa; kedaluwarsa pada bulan/hari setting di tahun berjalan.';

CREATE TABLE IF NOT EXISTS hris.leave_carry_over_runs (
  year        integer PRIMARY KEY,
  started_at  timestamptz NOT NULL DEFAULT now(),
  finished_at timestamptz,
  employees   integer NOT NULL DEFAULT 0,
  carried     integer NOT NULL DEFAULT 0
);

COMMENT ON TABLE hris.leave_carry_over_runs IS
  'Klaim job sisa bawaan 1 Januari: satu baris per tahun mencegah jalan ganda (dua replika, restart).';

-- ── 5) Menu, grant, masker ───────────────────────────────────────────────────
INSERT INTO iam.menus (code, menu_name, route_path, icon, menu_type, order_number, permission_context)
VALUES
  ('hris.workforce.leave-balances', 'Saldo Cuti', '/dashboard/hris/leave-balances',
   'clipboard', 'sidebar', 25, '{"actions":["read","update","import","export"]}'::jsonb)
ON CONFLICT (code) DO UPDATE SET
  menu_name = EXCLUDED.menu_name, route_path = EXCLUDED.route_path, icon = EXCLUDED.icon,
  menu_type = EXCLUDED.menu_type, order_number = EXCLUDED.order_number,
  permission_context = EXCLUDED.permission_context,
  is_active = true, is_visible = true, deleted_at = NULL, updated_at = now();

UPDATE iam.menus child SET parent_id = parent.id, module = 'hris', level = 3, updated_at = now()
FROM iam.menus parent
WHERE child.code = 'hris.workforce.leave-balances' AND parent.code = 'hris.workforce';

-- super_admin / admin: semua aksi (pola iam-admin-permissions).
INSERT INTO iam.role_menu_permissions (role_id, menu_id, granted_actions)
SELECT r.id, m.id, '["read","create","update","delete","approve","export","import","execute"]'::jsonb
FROM iam.roles r CROSS JOIN iam.menus m
WHERE r.code IN ('super_admin', 'admin') AND m.code = 'hris.workforce.leave-balances'
ON CONFLICT (role_id, menu_id) DO UPDATE SET
  is_active = true, granted_actions = EXCLUDED.granted_actions, updated_at = now();

-- Masker: non_payroll dan non_rekrutmen_kpi hanya melihat ("read" tidak bisa
-- dimasker). Prefix terpanjang menang, jadi baris ini mengalahkan prefix 'hris'.
INSERT INTO iam.hris_staff_role_masks (role_code, menu_prefix, exclude_menu, excluded_actions)
SELECT v.role_code, v.menu_prefix, false, v.excluded_actions
FROM (VALUES
  ('hris_non_payroll', 'hris.workforce.leave-balances', '{update,import,export}'::text[]),
  ('hris_non_rekrutmen_kpi', 'hris.workforce.leave-balances', '{update,import,export}'::text[])
) AS v(role_code, menu_prefix, excluded_actions)
JOIN iam.roles r ON r.code = v.role_code
ON CONFLICT (role_code, menu_prefix) DO NOTHING;

-- admin_hris memegang semua aksi; hris_manager dan dua role lain diturunkan
-- oleh iam.sync_hris_staff_roles() memakai masker di atas. Sync menolak sumber
-- kosong, jadi di database baru/CI (admin_hris belum punya grant dari seeder)
-- hanya grant daun ini yang dibuat dan sync dilewati.
DO $$
DECLARE seeded boolean;
BEGIN
  SELECT EXISTS (
    SELECT 1 FROM iam.role_menu_permissions rmp
    JOIN iam.roles r ON r.id = rmp.role_id AND r.code = 'admin_hris'
    JOIN iam.menus m ON m.id = rmp.menu_id AND m.deleted_at IS NULL
    WHERE rmp.is_active AND m.code <> 'hris.workforce.leave-balances') INTO seeded;

  INSERT INTO iam.role_menu_permissions (role_id, menu_id, granted_actions)
  SELECT r.id, m.id, '["read","update","import","export"]'::jsonb
  FROM iam.roles r CROSS JOIN iam.menus m
  WHERE r.code = 'admin_hris' AND m.code = 'hris.workforce.leave-balances'
  ON CONFLICT (role_id, menu_id) DO UPDATE SET
    is_active = true, granted_actions = EXCLUDED.granted_actions, updated_at = now();

  IF seeded THEN
    PERFORM iam.sync_hris_staff_roles();
  END IF;
END
$$;

-- ── 6) Trigger karyawan baru dan backfill ────────────────────────────────────
-- Baris tahun bergabung DAN tahun berjalan (WIB), jatah dari R1. leave_balances
-- mensyaratkan year >= 2020, jadi tahun bergabung lebih lama dilewati agar
-- insert karyawan tidak gagal.
CREATE OR REPLACE FUNCTION public.initialize_employee_leave_balance()
 RETURNS trigger
 LANGUAGE plpgsql
AS $function$
DECLARE
  join_year INTEGER;
  cur_year INTEGER := EXTRACT(YEAR FROM (now() AT TIME ZONE 'Asia/Jakarta'))::int;
BEGIN
  join_year := EXTRACT(YEAR FROM NEW.join_date)::int;
  IF join_year IS NOT NULL AND join_year >= 2020 THEN
    INSERT INTO hris.leave_balances (employee_id, year, annual_leave_total)
    VALUES (NEW.id, join_year, hris.default_annual_leave(NEW.join_date, join_year))
    ON CONFLICT (employee_id, year) DO NOTHING;
  END IF;
  IF cur_year IS DISTINCT FROM join_year THEN
    INSERT INTO hris.leave_balances (employee_id, year, annual_leave_total)
    VALUES (NEW.id, cur_year, hris.default_annual_leave(NEW.join_date, cur_year))
    ON CONFLICT (employee_id, year) DO NOTHING;
  END IF;
  RETURN NEW;
END;
$function$;

-- R14: karyawan aktif tanpa baris tahun berjalan. Baris yang sudah ada tidak disentuh.
INSERT INTO hris.leave_balances (employee_id, year, annual_leave_total)
SELECT e.id, y.year, hris.default_annual_leave(e.join_date, y.year)
FROM hris.employees e
CROSS JOIN (SELECT EXTRACT(YEAR FROM (now() AT TIME ZONE 'Asia/Jakarta'))::int AS year) y
WHERE e.is_active
  AND NOT EXISTS (SELECT 1 FROM hris.leave_balances b WHERE b.employee_id = e.id AND b.year = y.year)
ON CONFLICT (employee_id, year) DO NOTHING;
