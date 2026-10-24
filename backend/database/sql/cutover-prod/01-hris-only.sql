-- Cutover production (EPIC: cutover paddyadorable, 23-10-2026): kosongkan data
-- non-HRIS yang tidak dibuat migrasi. Satu transaksi; gagal = tidak ada perubahan.
-- Wajib superuser (session_replication_role). Lihat README.md di folder ini.
\set ON_ERROR_STOP on
BEGIN;
-- Sama dengan sesi aplikasi (backend/internal/platform/database.SearchPath): trigger
-- memakai nama tabel tanpa schema dan role DB production tidak menyetel search_path.
SET LOCAL search_path = public, iam, configuration, hris, performance, recruitment, item, purchasing, inventory, manufacturing, pos, crm, accounting, auth;
SET LOCAL timezone = 'Asia/Jakarta';

CREATE TEMP TABLE cutover_empty (tbl text PRIMARY KEY) ON COMMIT DROP;
INSERT INTO cutover_empty (tbl) VALUES
  ('crm.wa_messages'),
  ('dataroom.nodes'),
  ('inventory.finished_goods_inventory'),
  ('inventory.finished_goods_movements'),
  ('inventory.inventory'),
  ('inventory.inventory_movements'),
  ('inventory.stock_batches'),
  ('inventory.stock_batch_movements'),
  ('item.product_categories'),
  ('item.products'),
  ('item.raw_material_categories'),
  ('item.raw_materials'),
  ('item.raw_material_unit_conversions'),
  ('item.units'),
  ('manufacturing.bom_items'),
  ('manufacturing.production_batches'),
  ('manufacturing.production_order_materials'),
  ('manufacturing.production_orders'),
  ('manufacturing.raw_material_bom_items'),
  ('platform.outbox_deliveries'),
  ('platform.outbox_events'),
  ('platform.rate_limits'),
  ('pos.pos_customers'),
  ('pos.pos_inventory_settings'),
  ('pos.pos_product_images'),
  ('pos.pos_products'),
  ('pos.pos_product_skus'),
  ('pos.pos_wallet_sweep_runs'),
  ('public.ai_assistant_logs'),
  ('public.ai_assistant_messages'),
  ('public.ai_assistant_sessions'),
  ('purchasing.supplier_price_lists'),
  ('purchasing.suppliers'),
  ('recruitment.candidate_activities'),
  ('recruitment.candidate_ai_analysis'),
  ('recruitment.candidate_notes'),
  ('recruitment.candidate_offers'),
  ('recruitment.candidate_psikotes_summary'),
  ('recruitment.candidates'),
  ('recruitment.candidate_screenings'),
  ('recruitment.interview_ai_proctor_events'),
  ('recruitment.interview_ai_sessions'),
  ('recruitment.interview_ai_turns'),
  ('recruitment.interviews'),
  ('recruitment.job_openings'),
  ('recruitment.live_monitor_frames'),
  ('recruitment.live_signals'),
  ('recruitment.psikotes_proctor_events'),
  ('recruitment.psikotes_questions'),
  ('recruitment.psikotes_sessions'),
  ('recruitment.psikotes_session_tests'),
  ('shop.product_channels'),
  ('shop.shipping_settings'),
  ('spa.commission_rules'),
  ('spa.outlets'),
  ('spa.public_booking_funnel_events'),
  ('spa.treatments'),
  ('spa.treatment_variants'),
  ('spa.variant_outlet_prices'),
  ('spa.variant_pos_products');

-- Semua tabel yang dikosongkan harus ada (nama salah = batal).
DO $$
DECLARE missing text;
BEGIN
  SELECT string_agg(tbl, ', ') INTO missing FROM cutover_empty WHERE to_regclass(tbl) IS NULL;
  IF missing IS NOT NULL THEN RAISE EXCEPTION 'tabel tidak ditemukan: %', missing; END IF;
END $$;

-- Snapshot jumlah baris semua tabel lain (yang dipertahankan).
CREATE TEMP TABLE cutover_kept ON COMMIT DROP AS
SELECT t.table_schema || '.' || t.table_name AS tbl,
       (xpath('/row/c/text()', query_to_xml(format('select count(*) as c from %I.%I', t.table_schema, t.table_name), false, true, '')))[1]::text::bigint AS n
FROM information_schema.tables t
WHERE t.table_type = 'BASE TABLE'
  AND t.table_schema NOT IN ('pg_catalog', 'information_schema')
  AND t.table_schema NOT LIKE 'pg_temp%'
  AND t.table_schema || '.' || t.table_name NOT IN (SELECT tbl FROM cutover_empty);

-- Hapus tanpa trigger/cascade FK supaya tidak ada efek ke tabel yang dipertahankan.
SET LOCAL session_replication_role = replica;
DO $$
DECLARE r record;
BEGIN
  FOR r IN SELECT tbl FROM cutover_empty ORDER BY tbl LOOP
    EXECUTE format('DELETE FROM %s', r.tbl);
  END LOOP;
END $$;
SET LOCAL session_replication_role = origin;

-- Verifikasi 1: tabel yang dipertahankan tidak berubah.
DO $$
DECLARE changed text;
BEGIN
  SELECT string_agg(k.tbl || ' ' || k.n || '→' || now_n, ', ') INTO changed
  FROM (SELECT tbl, n, (xpath('/row/c/text()', query_to_xml(format('select count(*) as c from %s', tbl), false, true, '')))[1]::text::bigint AS now_n
        FROM cutover_kept) k
  WHERE k.n <> k.now_n;
  IF changed IS NOT NULL THEN RAISE EXCEPTION 'tabel dipertahankan berubah: %', changed; END IF;
END $$;

-- Verifikasi 2: tidak ada baris di tabel lain yang masih menunjuk tabel yang dikosongkan.
DO $$
DECLARE r record; n bigint; orphans text := '';
BEGIN
  FOR r IN
    SELECT cn.nspname || '.' || c.relname AS src, con.conname,
           (SELECT string_agg(format('%I', a.attname), ',') FROM unnest(con.conkey) k JOIN pg_attribute a ON a.attrelid = con.conrelid AND a.attnum = k) AS cols
    FROM pg_constraint con
    JOIN pg_class c ON c.oid = con.conrelid JOIN pg_namespace cn ON cn.oid = c.relnamespace
    JOIN pg_class f ON f.oid = con.confrelid JOIN pg_namespace fn ON fn.oid = f.relnamespace
    WHERE con.contype = 'f'
      AND fn.nspname || '.' || f.relname IN (SELECT tbl FROM cutover_empty)
      AND cn.nspname || '.' || c.relname NOT IN (SELECT tbl FROM cutover_empty)
  LOOP
    EXECUTE format('SELECT count(*) FROM %s WHERE (%s) IS NOT NULL', r.src, replace(r.cols, ',', ') IS NOT NULL AND (')) INTO n;
    IF n > 0 THEN orphans := orphans || r.src || '(' || r.conname || ')=' || n || ' '; END IF;
  END LOOP;
  IF orphans <> '' THEN RAISE EXCEPTION 'baris yatim: %', orphans; END IF;
END $$;

DO $$
BEGIN
  RAISE NOTICE 'cutover 01 OK: % tabel dikosongkan, % tabel dipertahankan tidak berubah',
    (SELECT count(*) FROM cutover_empty), (SELECT count(*) FROM cutover_kept);
END $$;
COMMIT;
