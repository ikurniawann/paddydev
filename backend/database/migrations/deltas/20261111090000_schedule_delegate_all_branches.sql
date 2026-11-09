-- EPIC-093 R1/R2: pengatur jadwal toko yang dicentang HR "Boleh atur jadwal
-- semua toko" mengatur jadwal (grid, pola, unggah) di semua toko aktif.
-- Penempatan dan checkout otomatis tetap hanya toko wilayahnya
-- (hris.schedule_delegate_branches). Default mati: pengatur lama tidak berubah.
ALTER TABLE hris.schedule_delegates
  ADD COLUMN IF NOT EXISTS schedule_all_branches boolean NOT NULL DEFAULT false;
