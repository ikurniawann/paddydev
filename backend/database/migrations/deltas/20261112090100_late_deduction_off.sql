-- EPIC-094 R7: kebijakan Paddy tidak ada potongan keterlambatan absensi
-- (keputusan user 06-11-2026). Keterlambatan ditangani lewat tindakan
-- disipliner berjenjang (Surat Teguran, SP1, SP2, SP3) yang diterbitkan HR.
-- Potongan absensi tetap hanya Izin Sakit dan Izin Pribadi sesuai tabel
-- payslip 20-10-2026. HR masih bisa mengubahnya di Pengaturan Payroll.
UPDATE hris.payroll_settings
SET late_deduction_mode = 'off', updated_at = now()
WHERE late_deduction_mode <> 'off';
