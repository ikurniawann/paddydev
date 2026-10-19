-- Spec §2.2: karyawan aktif dengan penempatan utama berjalan di
-- "Paddy HQ Bandung" dan "Warehouse Paddy" menjadi Non-Shift.
-- Jalankan bagian 1 dulu, tunjukkan daftarnya ke user, baru bagian 2.

-- 1. Daftar (baca saja)
SELECT b.name AS cabang, e.nip, e.full_name, e.schedule_type
FROM hris.employees e
JOIN hris.employee_placements p ON p.employee_id = e.id AND p.is_primary
  AND p.start_date <= current_date AND (p.end_date IS NULL OR p.end_date >= current_date)
JOIN configuration.branches b ON b.id = p.branch_id
WHERE e.is_active AND b.name IN ('Paddy HQ Bandung', 'Warehouse Paddy')
ORDER BY b.name, e.full_name;

-- 2. Ubah tipe (hanya kolom schedule_type; jadwal tetap diterapkan dari
--    halaman Jadwal Kerja → "Terapkan jadwal tetap" per cabang)
BEGIN;
UPDATE hris.employees e SET schedule_type = 'non_shift', updated_at = now()
FROM hris.employee_placements p, configuration.branches b
WHERE p.employee_id = e.id AND p.is_primary
  AND p.start_date <= current_date AND (p.end_date IS NULL OR p.end_date >= current_date)
  AND b.id = p.branch_id AND e.is_active AND b.name IN ('Paddy HQ Bandung', 'Warehouse Paddy')
  AND e.schedule_type <> 'non_shift';
-- Periksa jumlah baris (live 19-10: 66 + 71 = 137) sebelum COMMIT.
COMMIT;
