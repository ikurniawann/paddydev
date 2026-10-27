-- =============================================================================
-- EPIC-068 tindak lanjut: sisa bawaan dihitung ulang otomatis.
--
-- 1) hris.leave_carry_over_reviews: antrean tinjauan HR untuk baris yang sisa
--    bawaannya turun di bawah yang sudah terpakai (kelebihan = terpakai - bawaan
--    baru). Keberadaan tabel ini juga MEMBUKA KUNCI pengaturan sisa bawaan
--    (loadLeaveSettings.CarryOverReady): selama tabel belum ada, pengaturan tidak
--    bisa dinyalakan dan job 1 Januari tidak membuat bawaan.
-- 2) hris.leave_balances.carry_over_source: 'auto' = dikelola rumus
--    (dihitung ulang); 'manual' = diisi HR lewat PUT/import (tidak ditimpa).
--    Baris yang sudah punya bawaan atau tanggal kedaluwarsa dijadikan 'manual'
--    saat kolom dibuat (nilai itu tidak boleh tertimpa); sisanya 'auto'.
-- 3) CHECK leave_balances_carry_over_valid kehilangan bagian
--    carry_over_used <= carry_over_days: hitung ulang boleh menurunkan bawaan di
--    bawah yang terpakai (disimpan apa adanya, tinjauan HR dibuka). CHECK lain
--    tetap (tidak negatif, kelipatan 0,5, sisa jatah >= 0).
--
-- Pengaturan carry_over_enabled TIDAK diubah (tetap mati sampai HR menyalakan).
-- Aditif dan idempoten.
-- =============================================================================

-- ── 1) Tabel tinjauan ────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS hris.leave_carry_over_reviews (
  id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  employee_id      uuid NOT NULL REFERENCES hris.employees(id) ON DELETE CASCADE,
  year             integer NOT NULL,
  carry_over_days  numeric(5,2) NOT NULL,
  carry_over_used  numeric(5,2) NOT NULL,
  excess           numeric(5,2) NOT NULL CHECK (excess > 0),
  cause            jsonb,
  opened_at        timestamptz NOT NULL DEFAULT now(),
  resolved_at      timestamptz,
  resolved_by      uuid,
  resolved_by_name text,
  resolution       text,
  CONSTRAINT leave_carry_over_reviews_resolved_valid CHECK (
    (resolved_at IS NULL AND resolution IS NULL)
    OR (resolved_at IS NOT NULL AND resolution IS NOT NULL))
);

-- Satu tinjauan terbuka per karyawan dan tahun.
CREATE UNIQUE INDEX IF NOT EXISTS leave_carry_over_reviews_open_uniq
  ON hris.leave_carry_over_reviews (employee_id, year) WHERE resolved_at IS NULL;

CREATE INDEX IF NOT EXISTS leave_carry_over_reviews_year_idx
  ON hris.leave_carry_over_reviews (year, opened_at);

COMMENT ON TABLE hris.leave_carry_over_reviews IS
  'EPIC-068 tindak lanjut: baris saldo yang sisa bawaannya turun di bawah yang terpakai butuh tinjauan HR. Ada tabel ini = pengaturan sisa bawaan terbuka kuncinya.';
COMMENT ON COLUMN hris.leave_carry_over_reviews.excess IS 'carry_over_used - carry_over_days saat dibuka/diperbarui (> 0).';
COMMENT ON COLUMN hris.leave_carry_over_reviews.resolution IS
  '''otomatis'' = kelebihan hilang sendiri (hitung ulang/koreksi); selain itu catatan HR saat menandai selesai.';

-- ── 2) Sumber bawaan + backfill (sekali, saat kolom dibuat) ──────────────────
DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM information_schema.columns
                 WHERE table_schema = 'hris' AND table_name = 'leave_balances' AND column_name = 'carry_over_source') THEN
    ALTER TABLE hris.leave_balances
      ADD COLUMN carry_over_source text NOT NULL DEFAULT 'auto'
      CONSTRAINT leave_balances_carry_over_source_valid CHECK (carry_over_source IN ('auto', 'manual'));
    UPDATE hris.leave_balances SET carry_over_source = 'manual'
    WHERE carry_over_days > 0 OR carry_over_expires_on IS NOT NULL;
  END IF;
END
$$;

COMMENT ON COLUMN hris.leave_balances.carry_over_source IS
  '''auto'' = bawaan dihitung ulang oleh rumus; ''manual'' = diisi HR (PUT/import) dan tidak ditimpa.';

-- ── 3) CHECK bawaan tanpa batas atas oleh terpakai ───────────────────────────
ALTER TABLE hris.leave_balances DROP CONSTRAINT IF EXISTS leave_balances_carry_over_valid;
ALTER TABLE hris.leave_balances
  ADD CONSTRAINT leave_balances_carry_over_valid CHECK (carry_over_days >= 0 AND carry_over_used >= 0);
