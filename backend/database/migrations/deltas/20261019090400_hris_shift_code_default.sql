-- =============================================================================
-- Kode shift otomatis (S1, S2, …) untuk insert yang tidak mengisi code
-- (seeder, modul lain, jalur lama). Form & API Shift Kerja tetap mewajibkan
-- kode; default ini hanya menjaga NOT NULL agar jalur lama tidak gagal.
-- =============================================================================
CREATE SEQUENCE IF NOT EXISTS hris.shifts_code_seq;
ALTER TABLE hris.shifts ALTER COLUMN code SET DEFAULT ('S' || nextval('hris.shifts_code_seq'));
