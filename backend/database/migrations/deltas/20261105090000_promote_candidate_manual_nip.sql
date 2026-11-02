-- =============================================================================
-- EPIC-081: NIP karyawan diisi manual, wajib. Promosi kandidat tidak lagi
-- membuat nomor EMP-<tahun>-<urut>: HR mengetik NIP di dialog promote.
--   public.promote_candidate_to_employee(..., p_nip text) adalah versi baru
--   dengan parameter NIP (dipangkas, wajib, maksimal 50 karakter). Bentrok NIP
--   tetap lewat unique constraint karyawan (aplikasi memeriksanya dulu supaya
--   pesannya menyebut pemilik NIP).
--   Versi lama (5 argumen) DIBIARKAN supaya rollback aplikasi tetap bisa
--   memanggilnya; aplikasi tidak lagi memakainya. Boleh dihapus pada
--   pembersihan berikutnya.
-- Aman dijalankan ulang dan di DB baru.
-- =============================================================================

CREATE OR REPLACE FUNCTION public.promote_candidate_to_employee(
  p_candidate_id uuid,
  p_join_date date,
  p_employment_status character varying,
  p_department_id uuid,
  p_reporting_to uuid,
  p_nip text
) RETURNS uuid
LANGUAGE plpgsql
AS $$
DECLARE
  v_candidate recruitment.candidates%ROWTYPE;
  v_nip text := btrim(COALESCE(p_nip, ''));
  v_employee_id uuid;
BEGIN
  IF v_nip = '' THEN
    RAISE EXCEPTION 'NIP wajib diisi' USING ERRCODE = 'check_violation';
  END IF;
  IF char_length(v_nip) > 50 THEN
    RAISE EXCEPTION 'NIP maksimal 50 karakter' USING ERRCODE = 'check_violation';
  END IF;

  SELECT * INTO v_candidate FROM recruitment.candidates WHERE id = p_candidate_id FOR UPDATE;
  IF NOT FOUND THEN
    RAISE EXCEPTION 'Kandidat tidak ditemukan' USING ERRCODE = 'no_data_found';
  END IF;
  IF v_candidate.promoted_to_employee_id IS NOT NULL THEN
    RAISE EXCEPTION 'Kandidat sudah dipromosikan menjadi employee' USING ERRCODE = 'unique_violation';
  END IF;
  IF v_candidate.status NOT IN ('hired', 'talent_pool') THEN
    RAISE EXCEPTION 'Status kandidat harus hired atau talent_pool' USING ERRCODE = 'check_violation';
  END IF;

  INSERT INTO hris.employees (
    nip, full_name, email, phone, join_date, employment_status,
    department_id, job_title_id, reporting_to, is_active
  ) VALUES (
    v_nip,
    v_candidate.full_name, v_candidate.email, COALESCE(v_candidate.phone, ''),
    p_join_date, p_employment_status,
    p_department_id, v_candidate.position_id, p_reporting_to, true
  ) RETURNING id INTO v_employee_id;

  UPDATE recruitment.candidates
     SET promoted_to_employee_id = v_employee_id, updated_at = now()
   WHERE id = p_candidate_id;

  RETURN v_employee_id;
END;
$$;
