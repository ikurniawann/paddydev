-- =============================================================================
-- Penanda ringkasan absen terlewat harian untuk HR (EPIC-067 R6).
-- Satu baris per tanggal D yang sudah diringkas: loop notifikasi menyisipkan
-- baris ini (ON CONFLICT DO NOTHING) di transaksi yang sama dengan
-- notifikasinya, sehingga satu tanggal hanya dikirim sekali walau API
-- berjalan di beberapa instance. Jumlah dan banyaknya penerima dicatat
-- setelah notifikasi disisipkan.
-- =============================================================================

CREATE TABLE IF NOT EXISTS hris.attendance_gap_digests (
  date         date PRIMARY KEY,
  missing      int,
  no_clock_out int,
  unscheduled  int,
  recipients   int,
  created_at   timestamptz NOT NULL DEFAULT now(),
  sent_at      timestamptz
);

COMMENT ON TABLE hris.attendance_gap_digests IS
  'EPIC-067: penanda ringkasan absen terlewat harian ke HR; satu baris per tanggal, ditulis loop notifikasi Go.';
