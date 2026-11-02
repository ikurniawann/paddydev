-- =============================================================================
-- Aturan cuti TnC 03-11-2026 (EPIC-084).
-- 1) Label notifikasi in-app: 5 jenis baru; paternity menjadi "Istri
--    Melahirkan" dan bereavement menjadi "Berkabung" (R1).
-- 2) hris.default_annual_leave (R7): mulai saldo 2027, di tahun genap 1 tahun
--    jatahnya = bulan sesudah bulan genap 1 tahun dikurangi cuti bersama
--    (aktif, libur, memotong cuti) di bulan-bulan itu, minimal 0; genap di
--    Januari = 12. Tahun <= 2026 tetap rumus EPIC-068, jadi baris saldo yang
--    ada tidak berubah. STABLE karena membaca hris.public_holidays.
--    Kembar dengan domain.DefaultAnnualLeave (Go).
-- =============================================================================

CREATE OR REPLACE FUNCTION public.fn_notify_leave_changes()
 RETURNS trigger
 LANGUAGE plpgsql
 SECURITY DEFINER
AS $function$
DECLARE
  v_employee_name text;
  v_employee_user_id uuid;
  v_leave_label text;
BEGIN
  SELECT full_name, user_id INTO v_employee_name, v_employee_user_id
  FROM employees WHERE id = COALESCE(NEW.employee_id, OLD.employee_id);

  v_leave_label := CASE COALESCE(NEW.leave_type, OLD.leave_type)::text
    WHEN 'annual'      THEN 'Tahunan'
    WHEN 'sick'        THEN 'Sakit'
    WHEN 'maternity'   THEN 'Melahirkan'
    WHEN 'paternity'   THEN 'Istri Melahirkan'
    WHEN 'personal'    THEN 'Izin Pribadi'
    WHEN 'leader'      THEN 'Izin Leader'
    WHEN 'pilgrimage'  THEN 'Ibadah Haji/Umrah'
    WHEN 'marriage'    THEN 'Menikah'
    WHEN 'bereavement' THEN 'Berkabung'
    WHEN 'birthday'    THEN 'Ulang Tahun'
    WHEN 'miscarriage' THEN 'Keguguran'
    WHEN 'wife_miscarriage' THEN 'Istri Keguguran'
    WHEN 'child_marriage'   THEN 'Menikahkan Anak'
    WHEN 'child_ceremony'   THEN 'Khitanan/Baptis Anak'
    WHEN 'unpaid'      THEN 'Tidak Dibayar'
    WHEN 'emergency'   THEN 'Darurat'
    WHEN 'menstrual'   THEN 'Haid'
    ELSE 'Lainnya'
  END;

  IF TG_OP = 'INSERT' THEN
    PERFORM notify_hrd(
      '📋 Pengajuan Cuti Baru',
      v_employee_name || ' mengajukan cuti ' || v_leave_label ||
        ' (' || NEW.total_days || ' hari) — ' ||
        to_char(NEW.start_date::date, 'DD Mon YYYY') || ' s/d ' ||
        to_char(NEW.end_date::date, 'DD Mon YYYY'),
      'approval',
      '/dashboard/hris/leaves'
    );

  ELSIF TG_OP = 'UPDATE' AND OLD.status IS DISTINCT FROM NEW.status THEN

    IF NEW.status = 'approved' THEN
      PERFORM notify_user(
        v_employee_user_id,
        '✅ Cuti Disetujui',
        'Pengajuan cuti ' || v_leave_label || ' kamu (' || NEW.total_days || ' hari) telah disetujui.',
        'status_change',
        '/dashboard/hris/leaves'
      );
      PERFORM notify_hrd(
        '✅ Cuti Disetujui: ' || v_employee_name,
        'Cuti ' || v_leave_label || ' (' || NEW.total_days || ' hari) telah disetujui.',
        'status_change',
        '/dashboard/hris/leaves'
      );

    ELSIF NEW.status = 'rejected' THEN
      PERFORM notify_user(
        v_employee_user_id,
        '❌ Cuti Ditolak',
        'Pengajuan cuti ' || v_leave_label || ' kamu (' || NEW.total_days || ' hari) ditolak.' ||
          CASE WHEN NEW.rejection_reason IS NOT NULL
               THEN ' Alasan: ' || NEW.rejection_reason
               ELSE '' END,
        'alert',
        '/dashboard/hris/leaves'
      );

    ELSIF NEW.status = 'cancelled' THEN
      PERFORM notify_hrd(
        '🚫 Cuti Dibatalkan',
        v_employee_name || ' membatalkan pengajuan cuti ' || v_leave_label || '.',
        'status_change',
        '/dashboard/hris/leaves'
      );
    END IF;

  ELSIF TG_OP = 'DELETE' THEN
    PERFORM notify_hrd(
      '🗑️ Pengajuan Cuti Dihapus',
      'Pengajuan cuti ' || v_leave_label || ' dari ' || v_employee_name || ' telah dihapus.',
      'alert',
      '/dashboard/hris/leaves'
    );
  END IF;

  RETURN COALESCE(NEW, OLD);
END;
$function$;


-- ── 2) hris.default_annual_leave ─────────────────────────────────────────────
CREATE OR REPLACE FUNCTION hris.default_annual_leave(p_join date, p_year int)
RETURNS numeric
LANGUAGE sql STABLE
AS $$
  WITH a AS (SELECT (p_join + interval '12 months')::date AS d)
  SELECT CASE
    WHEN p_join IS NULL THEN 12::numeric
    WHEN a.d > make_date(p_year, 12, 31) THEN 0::numeric
    WHEN a.d <= make_date(p_year, 1, 1) THEN 12::numeric
    WHEN p_year >= 2027 AND extract(month FROM a.d) = 1 THEN 12::numeric
    WHEN p_year >= 2027 THEN GREATEST(0, 12 - extract(month FROM a.d)::int - (
      SELECT count(DISTINCT h.holiday_date)::int FROM hris.public_holidays h
      WHERE h.type = 'cuti_bersama' AND h.status = 'aktif' AND h.deleted_at IS NULL
        AND h.is_day_off AND h.deducts_leave
        AND h.holiday_date > (date_trunc('month', a.d) + interval '1 month - 1 day')::date
        AND h.holiday_date <= make_date(p_year, 12, 31)
    ))::numeric
    ELSE (
      (24 * (make_date(p_year, 12, 31) - a.d + 1))
        / (make_date(p_year + 1, 1, 1) - make_date(p_year, 1, 1))
    )::numeric / 2
  END
  FROM a
$$;

COMMENT ON FUNCTION hris.default_annual_leave(date, int) IS
  'EPIC-068 R1 + EPIC-084 R7: jatah cuti tahunan default tahun Y. Kembar dengan domain.DefaultAnnualLeave (Go).';
