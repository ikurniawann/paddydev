-- EPIC-101: bagan organisasi berversi (Draft 6 dari klien). Tabel baru saja;
-- karyawan, jabatan (hris.positions), departemen, toko, dan approval tidak
-- disentuh. Node = slot jabatan di bagan; garis lapor antar slot lewat
-- parent_code di versi yang sama (FK komposit, BR-03).
CREATE TABLE IF NOT EXISTS hris.org_chart_versions (
  id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  label       text NOT NULL UNIQUE,
  source_file text,
  status      text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'approved', 'archived')),
  valid_from  date,
  imported_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS hris.org_chart_nodes (
  id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  version_id       uuid NOT NULL REFERENCES hris.org_chart_versions (id) ON DELETE CASCADE,
  code             text NOT NULL,
  parent_code      text,
  sort_order       int  NOT NULL,
  title            text NOT NULL,
  raw_title        text NOT NULL,
  is_position      boolean NOT NULL DEFAULT true,
  division_code    text NOT NULL,
  division_name    text NOT NULL,
  job_level        text CHECK (job_level ~ '^L[0-8]$'),
  level_confidence text,
  employment_type  text CHECK (employment_type IN ('PERMANENT_OR_CONTRACT', 'INTERN', 'FREELANCE', 'OUTSOURCE')),
  vendor           text,
  region           text,
  store_codes      text[] NOT NULL DEFAULT '{}',
  status           text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'planned', 'vacant', 'frozen')),
  needs_confirm    boolean NOT NULL DEFAULT false,
  flags            text[] NOT NULL DEFAULT '{}',
  UNIQUE (version_id, code),
  FOREIGN KEY (version_id, parent_code) REFERENCES hris.org_chart_nodes (version_id, code) DEFERRABLE INITIALLY DEFERRED
);
CREATE INDEX IF NOT EXISTS idx_org_chart_nodes_parent ON hris.org_chart_nodes (version_id, parent_code);

-- Peta judul jabatan HRIS → judul bagan (R3). alias = lower(btrim(judul HRIS)).
CREATE TABLE IF NOT EXISTS hris.org_title_aliases (
  alias        text PRIMARY KEY,
  title        text NOT NULL,
  region       text,
  needs_review boolean NOT NULL DEFAULT true,
  note         text
);
