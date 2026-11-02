-- EPIC-079 R4: hris.leaves.total_days dihitung aplikasi (staf shift dari
--   roster, Non-Shift dari Senin-Jumat dikurangi hari libur), tetapi trigger
--   leaves_calculate_total_days menimpanya pada setiap INSERT/UPDATE dengan
--   hitungan Senin-Jumat polos. Akibatnya hari yang disimpan dan dipotong dari
--   saldo berbeda dari pratinjau dan dari respons pengajuan.
--
--   Trigger kini hanya menghitung bila nilainya tidak diberikan:
--   - INSERT tanpa total_days (NULL): hitung Senin-Jumat seperti semula.
--   - UPDATE yang mengubah tanggal tanpa mengubah total_days: hitung ulang.
--   - selain itu nilai yang diberikan dipertahankan (approve, batal, dsb.
--     tidak lagi menimpa nilai yang sudah benar).
CREATE OR REPLACE FUNCTION public.calculate_leave_days()
RETURNS trigger
LANGUAGE plpgsql
AS $function$
BEGIN
  IF NEW.start_date IS NULL OR NEW.end_date IS NULL THEN
    RETURN NEW;
  END IF;
  IF TG_OP = 'INSERT' THEN
    IF NEW.total_days IS NOT NULL THEN
      RETURN NEW;
    END IF;
  ELSIF NEW.total_days IS DISTINCT FROM OLD.total_days
     OR (NEW.start_date = OLD.start_date AND NEW.end_date = OLD.end_date) THEN
    RETURN NEW;
  END IF;

  -- Hari kerja Senin-Jumat (tanpa libur), minimal 1 hari.
  NEW.total_days := (
    SELECT COUNT(*)::NUMERIC
    FROM GENERATE_SERIES(NEW.start_date, NEW.end_date, '1 day'::INTERVAL) AS d(date)
    WHERE EXTRACT(DOW FROM d.date) NOT IN (0, 6)
  );
  IF NEW.total_days < 1 THEN
    NEW.total_days := 1;
  END IF;
  RETURN NEW;
END;
$function$;
