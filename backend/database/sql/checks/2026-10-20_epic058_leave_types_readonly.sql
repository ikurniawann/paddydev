-- EPIC-058 — cek read-only sebelum rilis revisi jenis cuti. Tidak menulis apa pun.
BEGIN READ ONLY;

-- 1. Riwayat jenis yang dihapus (per status).
SELECT leave_type::text, status::text, count(*) AS jumlah, COALESCE(sum(total_days), 0) AS hari
FROM hris.leaves
WHERE leave_type::text IN ('emergency', 'unpaid', 'menstrual')
GROUP BY 1, 2 ORDER BY 1, 2;

-- 2. Pengajuan pending jenis yang dihapus: tetap bisa diproses, tetapi HR perlu tahu.
SELECT l.id, e.nip, e.full_name, l.leave_type::text, l.start_date, l.end_date, l.reason
FROM hris.leaves l JOIN hris.employees e ON e.id = l.employee_id
WHERE l.status = 'pending' AND l.leave_type::text IN ('emergency', 'unpaid', 'menstrual')
ORDER BY l.start_date;

-- 3. Sebaran level jabatan karyawan aktif.
SELECT COALESCE(p.level, '(tanpa jabatan)') AS level, count(*) AS karyawan
FROM hris.employees e LEFT JOIN hris.positions p ON p.id = e.job_title_id
WHERE e.is_active GROUP BY 1 ORDER BY 2 DESC;

-- 4. Jabatan yang judulnya seperti SPV ke atas tetapi levelnya masih Staff/kosong.
SELECT p.id, p.title, p.level, count(e.id) FILTER (WHERE e.is_active) AS karyawan_aktif
FROM hris.positions p LEFT JOIN hris.employees e ON e.job_title_id = p.id
WHERE p.title ~* '(supervisor|spv|manager|head|chief|director|direktur|general manager|ceo)'
  AND lower(trim(COALESCE(p.level, ''))) NOT IN
      ('supervisor', 'assistant manager', 'manager', 'senior manager', 'general manager', 'director', 'c-level')
GROUP BY p.id ORDER BY p.title;

-- 5. Tanggal yang sudah tumpang tindih (pending/approved): akan menolak pengajuan baru di tanggal itu.
SELECT a.employee_id, a.id AS leave_a, b.id AS leave_b, a.start_date, a.end_date, b.start_date, b.end_date
FROM hris.leaves a JOIN hris.leaves b
  ON a.employee_id = b.employee_id AND a.id < b.id
 AND a.status IN ('pending', 'approved') AND b.status IN ('pending', 'approved')
 AND a.start_date <= b.end_date AND b.start_date <= a.end_date
LIMIT 50;

ROLLBACK;
