-- Saklar global PPh 21, BPJS Ketenagakerjaan, dan BPJS Kesehatan (EPIC-082).
-- Nonaktif = komponen tidak dihitung: tidak ada potongan karyawan dan tidak ada
-- iuran perusahaan. Kepesertaan per struktur gaji dan tarif tetap tersimpan.
ALTER TABLE hris.payroll_settings
  ADD COLUMN IF NOT EXISTS pph21_enabled boolean NOT NULL DEFAULT true,
  ADD COLUMN IF NOT EXISTS bpjs_tk_enabled boolean NOT NULL DEFAULT true,
  ADD COLUMN IF NOT EXISTS bpjs_kes_enabled boolean NOT NULL DEFAULT true;

COMMENT ON COLUMN hris.payroll_settings.pph21_enabled IS
  'Saklar PPh 21. false = PPh 21 tidak dihitung (termasuk Pasal 17 tahunan Desember) dan metode tercatat nonaktif.';
COMMENT ON COLUMN hris.payroll_settings.bpjs_tk_enabled IS
  'Saklar BPJS Ketenagakerjaan. false = JHT/JP karyawan dan JHT/JP/JKK/JKM perusahaan tidak dihitung.';
COMMENT ON COLUMN hris.payroll_settings.bpjs_kes_enabled IS
  'Saklar BPJS Kesehatan. false = potongan karyawan dan iuran perusahaan tidak dihitung. Subsidi BPJS Kesehatan (tunjangan) tidak terpengaruh.';

-- Keputusan user 02-11-2026: PPh 21 dan BPJS tidak dipakai dulu. Baris setting
-- yang sudah ada langsung dimatikan; HR bisa menyalakannya di Pengaturan Payroll.
UPDATE hris.payroll_settings
   SET pph21_enabled = false, bpjs_tk_enabled = false, bpjs_kes_enabled = false;
