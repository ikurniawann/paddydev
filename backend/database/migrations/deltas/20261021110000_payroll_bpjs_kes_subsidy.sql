-- Subsidi BPJS Kesehatan global (EPIC-059, tabel payslip Paddy): Rp100.000
-- untuk masa kerja > 1 tahun pada tanggal cutoff. 0 = nominal per struktur gaji.
ALTER TABLE hris.payroll_settings
  ADD COLUMN IF NOT EXISTS bpjs_kes_subsidy_amount numeric NOT NULL DEFAULT 0;
