-- =============================================================================
-- EPIC-069 Work Location toko dari Excel "20261001 - Work Location - New"
-- (keputusan user 28-10-2026):
--   1. Lengkapi profil 39 toko yang sudah ada + Paddy HQ Bandung + Warehouse
--      Paddy: nama "KODE - Nama Store", alamat, kota, kode pos, koordinat pin
--      Google Maps (koordinat hanya diisi bila ada; Paddy HQ Bandung memakai
--      koordinat yang sudah ada).
--   2. Tambah 3 toko baru, aktif: TWSP, TWMS, TWEM (perusahaan Paddy).
--   3. Pindahkan penempatan utama 22 staf toko ke toko di Excel: penempatan
--      lama diakhiri kemarin (WIB), penempatan utama baru mulai hari ini.
-- Toko lain yang tidak ada di Excel, Paddy Pudak/Salam, dan staf HQ/Warehouse/
-- mobile TIDAK disentuh. Koordinat terisi = geofence absen 100 m aktif.
-- Audit: branch.work_location_import, placement.work_location_moved.
--
-- Pratinjau (transaksi read-only + ROLLBACK):
--   psql "$U" -v ON_ERROR_STOP=1 -v preview=1 -v expected_stores=44 -v expected_moves=22 -f backend/database/sql/fixes/2026-10-28_work_location_toko.sql
-- Terapkan (setelah backup):
--   psql "$U" -v ON_ERROR_STOP=1 -v expected_stores=44 -v expected_moves=22 -f backend/database/sql/fixes/2026-10-28_work_location_toko.sql
-- Kembalikan: entri audit di atas (before) atau restore backup.
-- =============================================================================
\if :{?expected_stores}
\else
  \echo 'Wajib -v expected_stores=<jumlah baris toko> -v expected_moves=<jumlah pindah>'
  \quit
\endif
\if :{?expected_moves}
\else
  \echo 'Wajib -v expected_moves=<jumlah pindah>'
  \quit
\endif

BEGIN;
SET LOCAL search_path = configuration, hris, public;
SET LOCAL TIME ZONE 'Asia/Jakarta';

CREATE TEMP TABLE toko_data (code text PRIMARY KEY, name text, address text, city text, postcode text,
  lat numeric(9,6), lng numeric(9,6), is_new boolean NOT NULL) ON COMMIT DROP;
INSERT INTO toko_data VALUES
  ('MIZU-BANDUNG', NULL, 'Jl. BKR No.37A, Pasirluyu, Kec. Regol, Kota Bandung, Jawa Barat 40253', 'Bandung', '40253', NULL, NULL, false),
  ('ODOO-BXST', 'BXST - Mini Paddy Playstore Bobobox Sultan Tirtayasa', 'Jl. Sultan Tirtayasa No.11, Citarum, Kec. Bandung Wetan, Kota Bandung, Jawa Barat 40115', 'Bandung', '40115', -6.902374, 107.612979, false),
  ('ODOO-CBBM', 'CBBM - Mini Paddy Playstore CIMB Blok M', 'Mall Pasaraya Blok M, Jl. Iskandarsyah II No.2, RT.3/RW.1, Melawai, Kec. Kby. Baru, Kota Jakarta Selatan, Daerah Khusus Ibukota Jakarta 12160', 'Jakarta', '12160', -6.244367, 106.803472, false),
  ('ODOO-MBIP', 'MBIP - Mini Paddy Playstore BIP', 'Bandung Indah Plaza, Jl. Merdeka No.56, Citarum, Kec. Bandung Wetan, Kota Bandung, Jawa Barat 40115', 'Bandung', '40115', -6.90719, 107.611105, false),
  ('ODOO-MBRM', 'MBRM - Paddy Bags Studio Express RE Martadinata', '3 Second FS Riau Bandung Jl. R.E. Martadinata No.61, Citarum, Bandung Wetan, Bandung City, West Java 40114', 'Bandung', '40114', NULL, NULL, false),
  ('ODOO-MCCM', 'MCCM - Mini Paddy Playstore', 'Cibinong City Mall Jl. Tegar Beriman No.1, Pakansari, Kec. Cibinong, Kabupaten Bogor, Jawa Barat 16914', 'Bogor', '16914', -6.484246, 106.842032, false),
  ('ODOO-MDTS', 'MDTS - Mini Paddy Playstore 23 Semarang', '23 (Dua Tiga) Semarang Shopping Mall Kav. 1 Pearl of Java, Tambakharjo, Semarang, Jawa Tengah 50144', 'Semarang', '50144', -6.953711, 110.380588, false),
  ('ODOO-MLMS', 'MLMS - Mini Paddy Playstore', 'Lippo Plaza Mall Sidoarjo Jl. Jati Raya No.1, Jati, Kec. Sidoarjo, Kabupaten Sidoarjo, Jawa Timur 61226', 'Sidoarjo', '61226', -7.446089, 112.698397, false),
  ('ODOO-MPIK', 'MPIK - Mini Paddy Playstore By The Sea PIK', 'By The Sea Shopping District at Golf Island PIK Jl. Golf Island Timur, Kamal Muara, Daerah Khusus Ibukota Jakarta 14470', 'Jakarta', '14470', -6.093314, 106.750516, false),
  ('ODOO-MPSL', 'MPSL - Mini Paddy Playstore Paskal 23', 'Paskal 23 Jl. Pasir Kaliki No.25-27, Kb. Jeruk, Kec. Andir, Kota Bandung, Jawa Barat 40241', 'Bandung', '40241', -6.915101, 107.593769, false),
  ('ODOO-MPVJ', 'MPVJ - Mini Paddy Playstore PVJ', 'Paris Van Java Mall, Jl. Sukajadi Blok B No.50, Cipedes, Kec. Sukajadi, Kota Bandung, Jawa Barat 40162', 'Bandung', '40162', -6.889048, 107.595714, false),
  ('ODOO-MTSM', 'MTSM - Mini Paddy Playstore TSM', 'Trans Studio Mall Bandung, Jl. Gatot Subroto No.289, Cibangkong, Kec. Batununggal, Kota Bandung, Jawa Barat 40273', 'Bandung', '40273', -6.925542, 107.636668, false),
  ('ODOO-MUBM', 'MUBM - Mini Paddy Playstore MU+KU', 'MATAHARI/MU+KU Plaza Blok M', 'Jakarta', NULL, NULL, NULL, false),
  ('ODOO-PBSA', 'PBSA - Paddy Bags Studio Soka', 'Jl. Soka No.23A, Merdeka, Kec. Sumur Bandung, Kota Bandung, Jawa Barat 40113', 'Bandung', '40113', -6.915432, 107.628257, false),
  ('ODOO-PLAY', 'PLAY - Paddy Parfume Labs Bogor', 'Jl. Ahmad Yani No.40A, RT.02/RW.01, Tanah Sareal, Kec. Tanah Sereal, Kota Bogor, Jawa Barat 16161', 'Bogor', '16161', -6.577332, 106.799589, false),
  ('ODOO-PLBM', 'PLBM - Paddy Parfume Labs Blok M', 'Blok M Jl. Bulungan I No. 12, Kelurahan Kramat Pela, Kecamatan Kebayoran Baru, Jakarta Selatan', 'Jakarta', NULL, -6.244987, 106.797284, false),
  ('ODOO-PLKG', 'PLKG - Paddy Parfume Labs', 'Jl. Kaliurang KM 6, Kayen, Kel. Condong Catur, Kec. Depok, Kabupaten Sleman, Daerah Istimewa Yogyakarta 55281', 'Yogyakarta', '55281', -7.749683, 110.386475, false),
  ('ODOO-PLMR', 'PLMR - Paddy Parfume Labs Makassar', 'Jl. Boulevard Ruko Jasper II No. 245-246, Masale, Kec. Makassar, Kota Makassar, Sulawesi Selatan 90231', 'Makassar', '90231', -5.156483, 119.44654, false),
  ('ODOO-PLPK', 'PLPK - Paddy Parfume Labs', 'By The Sea Shopping District at Golf Island PIK Jl. Golf Island Timur, Kamal Muara, Daerah Khusus Ibukota Jakarta 14470', 'Jakarta', '14470', -6.093246, 106.749688, false),
  ('ODOO-PLRM', 'PLRM - Paddy Parfume Labs', 'Jl. L. L. R.E. Martadinata No.139 Cihapit, Kec. Bandung Wetan, Kota Bandung, Jawa Barat 40114', 'Bandung', '40114', -6.908684, 107.624746, false),
  ('ODOO-PPAY', 'PPAY - Paddy Playstore Dreamscape', 'Jl. Ahmad Yani No.40A, RT.02/RW.01, Tanah Sareal, Kec. Tanah Sereal, Kota Bogor, Jawa Barat 16161', 'Bogor', '16161', -6.577332, 106.799589, false),
  ('ODOO-PPBA', 'PPBA - Paddy Playstore Braga', 'Jl. Braga No.74, Braga, Kec. Sumur Bandung, Kota Bandung, Jawa Barat 40111', 'Bandung', '40111', -6.916786, 107.609348, false),
  ('ODOO-PPBI', 'PPBI - Paddy Playstore Rainforest', 'Jl. Boulevard Raya, RGG 6, RT.005/RW.017, Kel. Jaka Setia, Kec. Bekasi, Selatan, Kota Bekasi, Jawa Barat 17148', 'Bekasi', '17148', -6.273132, 106.973431, false),
  ('ODOO-PPCN', 'PPCN - Paddy Playstore Groovy Groceries', 'Jl. Kartini No.32, Sukapura, Kec. Kejaksan, Kota Cirebon, Jawa Barat 45123', 'Cirebon', '45123', -6.711266, 108.554346, false),
  ('ODOO-PPCY', 'PPCY - Paddy Playstore Splashy World', 'Kawasan Coyudan Jl. Dr. Rajiman No.147Jayengan, Kec. Serengan, Kota Surakarta, Jawa Tengah 57152', 'Surakarta', '57152', -7.574051, 110.821727, false),
  ('ODOO-PPGA', 'PPGA - Paddy Playstore Backyard', 'Jalan Gandapura No.16, Merdeka, Kecamatan Sumur Bandung, Kota Bandung, Jawa Barat 40113', 'Bandung', '40113', -6.915353, 107.626064, false),
  ('ODOO-PPGI', 'PPGI - Paddy Playstore Grand Indonesia', 'Grand Indonesia, Jl. MH Thamrin No.1, Menteng, Kec. Menteng, Kota Jakarta Pusat, Daerah Khusus Ibukota Jakarta 10310', 'Jakarta', '10310', -6.19605, 106.822107, false),
  ('ODOO-PPJA', 'PPJA - Paddy Playstore Starry Dream', 'Jl. Melawai I No 28A, Melawai, Kec. Kby. Baru, Kota Jakarta Selatan, Daerah Khusus Ibukota Jakarta 12160', 'Jakarta', '12160', -6.243878, 106.801666, false),
  ('ODOO-PPLN', 'PPLN - Paddy Playstore Lippo Mall Nusantara Tomorrow', 'Lippo Mall Nusantara, Kawasan Bisnis Granadha, Jl. Jend. Sudirman No.Kav. 50 LG Floor, Karet Semanggi, Kecamatan Setiabudi, Kota Jakarta Selatan, Daerah Khusus Ibukota Jakarta 12930', 'Jakarta', '12930', -6.220003, 106.814335, false),
  ('ODOO-PPMR', 'PPMR - Paddy Playstore Makassar', 'Jl. Boulevard Ruko Jasper II No. 245-246, Masale, Kec. Makassar, Kota Makassar, Sulawesi Selatan 90231', 'Makassar', '90231', -5.156503, 119.446559, false),
  ('ODOO-PPMT', 'PPMT - Paddy Playstore Orbit Rush', 'Jl. Ahmad Yani No.3 A, Magelang, Kec. Magelang Tengah, Kota Magelang, Jawa Tengah 56117', 'Magelang', '56117', -7.475407, 110.218427, false),
  ('ODOO-PPPA', 'PPPA - Paddy Playstore Lucky Clinic', 'Jl. Pemuda No.15, Pandansari, Kec. Semarang Tengah, Kota Semarang, Jawa Tengah 50139', 'Semarang', '50139', -6.971893, 110.421717, false),
  ('ODOO-PPSU', 'PPSU - Paddy Playstore Stomp & Roar', 'Jl. Semeru Kav A, Oro-oro Dowo, Kec. Klojen, Kota Malang, Jawa Timur 65119', 'Malang', '65119', -7.976521, 112.628832, false),
  ('ODOO-PPTN', 'PPTN - Paddy Playstore Cosmic Tennis', 'Jl. Tunjungan No.45A, Genteng, Kec. Genteng, Surabaya, Jawa Timur 60275', 'Surabaya', '60275', -7.259243, 112.738979, false),
  ('ODOO-PPVJ', 'PPVJ - Paddy Playstore Pool Pals', 'Paris Van Java Mall, Jl. Sukajadi Blok B No.50, Cipedes, Kec. Sukajadi, Kota Bandung, Jawa Barat 40162', 'Bandung', '40162', -6.889539, 107.595727, false),
  ('ODOO-PPYA', 'PPYA - Paddy Playstore Bake Lab', 'Jl. Affandi No.18, Karang Gayam, Caturtunggal, Kec. Depok, Kabupaten Sleman, Daerah Istimewa Yogyakarta 55281', 'Yogyakarta', '55281', -7.764042, 110.393468, false),
  ('ODOO-TWLC', 'TWLC - Paddy Playstore Tomorrow Lippo Icon Cibubur', 'Jl. Jambore No.1, RT.8/RW.7, Cibubur, Kec. Ciracas, Kota Jakarta Timur, Daerah Khusus Ibukota Jakarta 13720', 'Jakarta', '13720', -6.369365, 106.894177, false),
  ('ODOO-TWLK', 'TWLK - Paddy Playstore Tomorrow Lippo Mall Kemang', 'Jl. Pangeran Antasari No.36, RT.11/RW.5, Kel. Bangka, Kec. Mampang Prapatan, Jakarta Selatan, DKI Jakarta 12730', 'Jakarta', '12730', NULL, NULL, false),
  ('ODOO-TWPG', 'TWPG - Paddy Playstore Tomorrow Palembang Square', 'Jl. Angkatan 45, Lorok Pakjo, Kec. Ilir Bar. I, Kota Palembang, Sumatera Selatan 30137', 'Palembang', '30137', -2.976588, 104.742131, false),
  ('ODOO-TWPS', 'TWPS - Paddy Playstore Tomorrow Pakuwon Mall Surabaya', 'Pakuwon Mall Surabaya Jl. Pakuwon Trade Center No.2, Lontar, Kec. Wiyung, Surabaya, Jawa Timur 60227', 'Surabaya', '60227', -7.289311, 112.675655, false),
  ('ODOO-WAREHOUSE-PADDY', NULL, 'Komplek Angkasa Mekar, Jl. Angkasa Mekar No.8, RT.03/RW.22, Cangkuang Kulon, Kec. Dayeuhkolot, Kabupaten Bandung, Jawa Barat 40239', 'Kabupaten Bandung', '40239', -6.9645, 107.591939, false),
  ('TWEM', 'TWEM - Paddy Playstore Tomorrow Lombok Epicentrum Mall', 'Lombok Epicentrum Mall, Jl. Sriwijaya No.333, Punia, Kec. Mataram, Kota Mataram, Nusa Tenggara Barat 83127', 'Mataram', '83127', NULL, NULL, true),
  ('TWMS', 'TWMS - Paddy Playstore Tomorrow Mall Sidoarjo', 'Lippo Plaza Mall Sidoarjo Jl. Jati Raya No.1, Jati, Kec. Sidoarjo, Kabupaten Sidoarjo, Jawa Timur 61226', 'Sidoarjo', '61226', -7.446089, 112.698397, true),
  ('TWSP', 'TWSP - Paddy Playstore Tomorrow Sun Plaza', 'Sun Plaza Jl. KH. Zainul Arifin No.7, Madras Hulu, Kec. Medan Polonia, Kota Medan, Sumatera Utara 20152', 'Medan', '20152', NULL, NULL, true);

CREATE TEMP TABLE pindah (employee_id uuid PRIMARY KEY, nama text, dari text, ke text) ON COMMIT DROP;
INSERT INTO pindah VALUES
  ('3904085e-4570-5e16-9e0a-625c99249160'::uuid, 'Ricky Pratama Sidik', 'MIZU-BANDUNG', 'ODOO-PPGA'),
  ('8e674d2f-a32c-507d-8e3f-ad3eff331c1d'::uuid, 'Gritt Alvrin', 'MIZU-BANDUNG', 'ODOO-PPTN'),
  ('c6b3dd22-b6e0-549e-b863-541e0db41779'::uuid, 'Azka Hanifah Hamidah', 'MIZU-BANDUNG', 'ODOO-PLRM'),
  ('40c3d6d7-0c10-5c01-868a-179fa40437f2'::uuid, 'Syifa Listia', 'ODOO-MBIP', 'ODOO-PPBA'),
  ('cd28ed93-e174-5c57-98c6-a9adafff2cfd'::uuid, 'Ivan Sendina', 'ODOO-MCPZ', 'ODOO-PPGA'),
  ('85f84d10-a2d4-52a3-9335-a6ce66ed56c1'::uuid, 'Devina Hanida', 'ODOO-PPVJ', 'ODOO-PPGA'),
  ('05e33542-bc35-5c84-a184-d6b7e94ad72d'::uuid, 'Irvan Ramadhana Putra', 'MIZU-BANDUNG', 'ODOO-PPYA'),
  ('d9e2a38e-39a3-59ce-8ce3-514d1e61d0b4'::uuid, 'Diandra Pradnadya', 'ODOO-PPAY', 'ODOO-PLAY'),
  ('05972eb0-1273-5c47-8760-22f07b8af91d'::uuid, 'Siti Maudi', 'ODOO-PPAY', 'ODOO-PLAY'),
  ('afe6eaf0-2fd7-5bb0-8424-a06d11133b01'::uuid, 'Farhan Farras Prawira', 'MIZU-BANDUNG', 'ODOO-PPJA'),
  ('392b5233-167a-54ad-95a5-4de60cbbe449'::uuid, 'Riswandi', 'MIZU-BANDUNG', 'ODOO-PBSA'),
  ('eaeac6e5-0565-5449-8497-66593f6b65b3'::uuid, 'Muhammad Taufik Hidayat', 'ODOO-MPVJ', 'ODOO-PBSA'),
  ('b2aedd91-e094-57e4-b67f-87ca96a99962'::uuid, 'Riki Juliana', 'MIZU-BANDUNG', 'ODOO-PBSA'),
  ('8339f08d-09d1-58e4-8e46-a1635a91d687'::uuid, 'Gaza Fiqih Aulia', 'ODOO-EVENT-PVJ', 'ODOO-PBSA'),
  ('cced5b9f-894a-5e15-ab26-006def3b2097'::uuid, 'Tya Nur Azizah', 'ODOO-EVENT-PVJ', 'ODOO-PPVJ'),
  ('9081137b-fa9d-5acd-8f1f-92bc646e06fc'::uuid, 'Muhamad Daniel', 'ODOO-PPLN', 'ODOO-PLPK'),
  ('2ae7f611-9ddb-5f0d-a611-ae2a66280b0c'::uuid, 'Ryan Fazar', 'ODOO-PPLN', 'ODOO-PLBM'),
  ('83317571-d72f-53c3-ba79-662b258debfa'::uuid, 'Jusmiati', 'MIZU-BANDUNG', 'ODOO-PPMR'),
  ('aec6135c-c164-5e43-9ae3-d8189e73e6d8'::uuid, 'Annisa Rahma', 'ODOO-MBIP', 'ODOO-MTSM'),
  ('b60f708a-b06a-5745-a486-24254528c9d0'::uuid, 'Stevendino Marselino', 'ODOO-PPVJ', 'ODOO-PBSA'),
  ('3f76acc5-a7c4-5c5a-bb8e-4123daa6fe4b'::uuid, 'Adelia Puspita Sari', 'ODOO-PPMR', 'ODOO-PLMR'),
  ('c42a394f-adfd-5917-9be6-015e0ce1dbd1'::uuid, 'Lala Oliviana', 'ODOO-PPPA', 'ODOO-MDTS');

\if :{?preview}
  SET TRANSACTION READ ONLY;
\endif

\echo 'Toko (baru / kode tidak ditemukan / koordinat diisi):'
SELECT t.code, t.is_new AS baru, (b.id IS NOT NULL) AS ada, coalesce(t.name, b.name) AS nama,
       (t.lat IS NOT NULL) AS koordinat_baru, (b.lat IS NOT NULL) AS koordinat_lama
FROM toko_data t LEFT JOIN branches b ON b.code = t.code ORDER BY t.is_new DESC, t.code;

\echo 'Penempatan yang dipindah (penempatan aktif sekarang harus sama dengan kolom dari):'
SELECT p.nama, e.nip, p.dari, p.ke, string_agg(b.code, ',') AS aktif_sekarang, e.is_active
FROM pindah p JOIN employees e ON e.id = p.employee_id
LEFT JOIN employee_placements ep ON ep.employee_id = p.employee_id AND (ep.end_date IS NULL OR ep.end_date >= current_date)
LEFT JOIN branches b ON b.id = ep.branch_id
GROUP BY p.nama, e.nip, p.dari, p.ke, e.is_active ORDER BY p.ke, p.nama;

\echo 'Jadwal shift mendatang di toko lama (tidak diubah skrip; info untuk HR):'
SELECT p.nama, b.code AS toko_jadwal, count(*) AS hari, min(d.work_date) AS mulai, max(d.work_date) AS sampai
FROM pindah p JOIN employee_shift_days d ON d.employee_id = p.employee_id AND d.work_date >= current_date
JOIN branches b ON b.id = d.branch_id AND b.code = p.dari
GROUP BY p.nama, b.code ORDER BY p.nama;

SELECT set_config('paddy.expected_stores', :'expected_stores', true),
       set_config('paddy.expected_moves', :'expected_moves', true);
DO $$
DECLARE
  n int; diubah int;
  hari_ini date := current_date;
  paddy uuid;
  actor constant text := 'Ops (Claude Code atas persetujuan user)';
  alasan constant text := 'Work Location Excel 20261001 (EPIC-069, 28-10-2026)';
BEGIN
  SELECT count(*) INTO n FROM toko_data;
  IF n <> current_setting('paddy.expected_stores')::int THEN
    RAISE EXCEPTION 'Baris toko % (expected %)', n, current_setting('paddy.expected_stores');
  END IF;
  SELECT count(*) INTO n FROM pindah;
  IF n <> current_setting('paddy.expected_moves')::int THEN
    RAISE EXCEPTION 'Baris pindah % (expected %)', n, current_setting('paddy.expected_moves');
  END IF;
  SELECT count(*) INTO n FROM toko_data t WHERE NOT t.is_new AND NOT EXISTS (SELECT 1 FROM branches b WHERE b.code = t.code);
  IF n > 0 THEN RAISE EXCEPTION '% kode toko lama tidak ditemukan', n; END IF;
  SELECT count(*) INTO n FROM toko_data t WHERE t.is_new AND EXISTS (SELECT 1 FROM branches b WHERE b.code = t.code);
  IF n > 0 THEN RAISE EXCEPTION '% toko baru sudah ada — data berubah sejak pratinjau', n; END IF;
  SELECT b.company_id INTO paddy FROM branches b WHERE b.code = 'ODOO-TWLC';
  IF paddy IS NULL THEN RAISE EXCEPTION 'Perusahaan toko (dari ODOO-TWLC) tidak ditemukan'; END IF;

  -- Setiap karyawan: aktif, tepat satu penempatan aktif, di toko "dari"; toko "ke" ada.
  SELECT count(*) INTO n FROM pindah p
  WHERE NOT EXISTS (SELECT 1 FROM employees e WHERE e.id = p.employee_id AND e.is_active)
     OR (SELECT count(*) FROM employee_placements ep WHERE ep.employee_id = p.employee_id
           AND (ep.end_date IS NULL OR ep.end_date >= hari_ini)) <> 1
     OR NOT EXISTS (SELECT 1 FROM employee_placements ep JOIN branches b ON b.id = ep.branch_id
           WHERE ep.employee_id = p.employee_id AND (ep.end_date IS NULL OR ep.end_date >= hari_ini)
             AND b.code = p.dari AND ep.is_primary)
     OR NOT EXISTS (SELECT 1 FROM branches b WHERE b.code = p.ke);
  IF n > 0 THEN RAISE EXCEPTION '% karyawan tidak sesuai kondisi pratinjau — tidak ada perubahan', n; END IF;

  IF current_setting('transaction_read_only') = 'on' THEN
    RETURN; -- pratinjau
  END IF;

  -- 1. Toko baru (trigger membuat gudang bawaan + brand cermin).
  INSERT INTO branches (company_id, code, name, address, city, postcode, lat, lng, is_active, is_public)
  SELECT paddy, t.code, t.name, t.address, t.city, t.postcode, t.lat, t.lng, true, false
  FROM toko_data t WHERE t.is_new;
  GET DIAGNOSTICS diubah = ROW_COUNT;
  INSERT INTO audit.audit_log (actor_name, action, entity, entity_id, entity_label, before, after, reason)
  SELECT actor, 'branch.work_location_import', 'configuration.branches', b.id::text, b.name, NULL,
         jsonb_build_object('code', b.code, 'name', b.name, 'address', b.address, 'city', b.city,
                            'postcode', b.postcode, 'lat', b.lat, 'lng', b.lng, 'is_active', b.is_active),
         alasan || ' — toko baru'
  FROM toko_data t JOIN branches b ON b.code = t.code WHERE t.is_new;
  RAISE NOTICE 'Toko baru: %', diubah;

  -- 2. Lengkapi toko lama.
  INSERT INTO audit.audit_log (actor_name, action, entity, entity_id, entity_label, before, after, reason)
  SELECT actor, 'branch.work_location_import', 'configuration.branches', b.id::text, coalesce(t.name, b.name),
         jsonb_build_object('name', b.name, 'address', b.address, 'city', b.city, 'postcode', b.postcode, 'lat', b.lat, 'lng', b.lng),
         jsonb_build_object('name', coalesce(t.name, b.name), 'address', t.address, 'city', t.city, 'postcode', t.postcode,
                            'lat', coalesce(t.lat, b.lat), 'lng', coalesce(t.lng, b.lng)),
         alasan
  FROM toko_data t JOIN branches b ON b.code = t.code WHERE NOT t.is_new;
  UPDATE branches b SET name = coalesce(t.name, b.name), address = t.address, city = t.city, postcode = t.postcode,
         lat = coalesce(t.lat, b.lat), lng = coalesce(t.lng, b.lng), updated_at = now()
  FROM toko_data t WHERE b.code = t.code AND NOT t.is_new;
  GET DIAGNOSTICS diubah = ROW_COUNT;
  IF diubah <> (SELECT count(*) FROM toko_data WHERE NOT is_new) THEN
    RAISE EXCEPTION 'Toko lama diubah % — dibatalkan', diubah;
  END IF;
  RAISE NOTICE 'Toko dilengkapi: %', diubah;

  -- 3. Pindah penempatan utama.
  CREATE TEMP TABLE pindah_lama ON COMMIT DROP AS
  SELECT p.*, ep.id AS placement_id, ep.start_date, ep.branch_id AS dari_id, k.id AS ke_id
  FROM pindah p
  JOIN employee_placements ep ON ep.employee_id = p.employee_id AND (ep.end_date IS NULL OR ep.end_date >= hari_ini)
  JOIN branches k ON k.code = p.ke;

  INSERT INTO audit.audit_log (actor_name, action, entity, entity_id, entity_label, before, after, reason)
  SELECT actor, 'placement.work_location_moved', 'hris.employee_placements', l.placement_id::text, l.nama,
         jsonb_build_object('branch', l.dari, 'start_date', l.start_date),
         jsonb_build_object('branch', l.ke, 'start_date', hari_ini,
                            'old_end_date', CASE WHEN l.start_date < hari_ini THEN hari_ini - 1 END),
         alasan
  FROM pindah_lama l;

  -- Penempatan lama yang mulai sebelum hari ini diakhiri kemarin; yang mulai hari ini diganti tokonya.
  UPDATE employee_placements ep SET end_date = hari_ini - 1
  FROM pindah_lama l WHERE ep.id = l.placement_id AND l.start_date < hari_ini;
  INSERT INTO employee_placements (employee_id, branch_id, is_primary, start_date, notes)
  SELECT l.employee_id, l.ke_id, true, hari_ini, alasan FROM pindah_lama l WHERE l.start_date < hari_ini;
  UPDATE employee_placements ep SET branch_id = l.ke_id, notes = alasan
  FROM pindah_lama l WHERE ep.id = l.placement_id AND l.start_date >= hari_ini;

  SELECT count(*) INTO n FROM pindah p
  WHERE (SELECT count(*) FROM employee_placements ep JOIN branches b ON b.id = ep.branch_id
         WHERE ep.employee_id = p.employee_id AND (ep.end_date IS NULL OR ep.end_date >= hari_ini)
           AND ep.is_primary AND b.code = p.ke) = 1
    AND (SELECT count(*) FROM employee_placements ep
         WHERE ep.employee_id = p.employee_id AND (ep.end_date IS NULL OR ep.end_date >= hari_ini)) = 1;
  IF n <> (SELECT count(*) FROM pindah) THEN
    RAISE EXCEPTION 'Penempatan sesudah pindah sesuai % dari % — dibatalkan', n, (SELECT count(*) FROM pindah);
  END IF;
  RAISE NOTICE 'Penempatan dipindah: %', n;
END $$;

\echo 'Sesudah — toko aktif perusahaan Paddy tanpa koordinat:'
SELECT b.code, b.name FROM branches b JOIN companies c ON c.id = b.company_id
WHERE b.is_active AND b.lat IS NULL ORDER BY c.name, b.code;

\if :{?preview}
  ROLLBACK;
\else
  COMMIT;
\endif
