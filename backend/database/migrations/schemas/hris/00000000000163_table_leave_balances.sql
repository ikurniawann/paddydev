-- =============================================================================
-- AUTO-GENERATED dari PostgreSQL (introspeksi pg_catalog) — table: hris.leave_balances
-- JANGAN edit manual; regenerate via: npm run db:pull
-- Generated: 2026-06-24T07:29:27.947Z
-- Diselaraskan manual dengan keadaan setelah delta 20261028090000_leave_balance_allocation
-- (EPIC-068): kolom penyesuaian/sisa bawaan, annual_leave_remaining generated baru, CHECK baru.
-- Diselaraskan juga dengan delta 20261029090000_leave_carry_over_recalc (kolom carry_over_source,
-- CHECK carry_over_valid tanpa batas atas oleh terpakai).
-- Pull ulang berikutnya akan menimpa file ini dengan hasil introspeksi yang sama.
-- =============================================================================

-- Table: hris.leave_balances
CREATE TABLE "hris"."leave_balances" (
    "id" uuid DEFAULT uuid_generate_v4() NOT NULL,
    "employee_id" uuid NOT NULL,
    "year" integer NOT NULL,
    "annual_leave_total" numeric(5,2) DEFAULT 12 NOT NULL,
    "annual_leave_used" numeric(5,2) DEFAULT 0 NOT NULL,
    "sick_leave_used" numeric(5,2) DEFAULT 0 NOT NULL,
    "unpaid_leave_used" numeric(5,2) DEFAULT 0 NOT NULL,
    "maternity_leave_used" numeric(5,2) DEFAULT 0 NOT NULL,
    "paternity_leave_used" numeric(5,2) DEFAULT 0 NOT NULL,
    "created_at" timestamp with time zone DEFAULT now() NOT NULL,
    "updated_at" timestamp with time zone DEFAULT now() NOT NULL,
    "emergency_leave_used" integer DEFAULT 0 NOT NULL,
    "pilgrimage_leave_used" integer DEFAULT 0 NOT NULL,
    "menstrual_leave_used" integer DEFAULT 0 NOT NULL,
    "annual_leave_adjustment" numeric(5,2) DEFAULT 0 NOT NULL,
    "carry_over_days" numeric(5,2) DEFAULT 0 NOT NULL,
    "carry_over_used" numeric(5,2) DEFAULT 0 NOT NULL,
    "carry_over_expires_on" date,
    "carry_over_source" text DEFAULT 'auto' NOT NULL,
    "annual_leave_remaining" numeric(5,2) GENERATED ALWAYS AS (annual_leave_total + annual_leave_adjustment - annual_leave_used) STORED
);

ALTER TABLE ONLY "hris"."leave_balances"
    ADD CONSTRAINT "leave_balance_unique" UNIQUE (employee_id, year);

ALTER TABLE ONLY "hris"."leave_balances"
    ADD CONSTRAINT "leave_balances_pkey" PRIMARY KEY (id);

ALTER TABLE ONLY "hris"."leave_balances"
    ADD CONSTRAINT "annual_leave_non_negative" CHECK (annual_leave_remaining >= 0::numeric);

ALTER TABLE ONLY "hris"."leave_balances"
    ADD CONSTRAINT "leave_balance_year_valid" CHECK (year >= 2020);

ALTER TABLE ONLY "hris"."leave_balances"
    ADD CONSTRAINT "leave_balances_carry_over_valid" CHECK (carry_over_days >= 0::numeric AND carry_over_used >= 0::numeric);

ALTER TABLE ONLY "hris"."leave_balances"
    ADD CONSTRAINT "leave_balances_carry_over_source_valid" CHECK (carry_over_source = ANY (ARRAY['auto'::text, 'manual'::text]));

ALTER TABLE ONLY "hris"."leave_balances"
    ADD CONSTRAINT "leave_balances_half_steps" CHECK (mod(annual_leave_total * 2::numeric, 1::numeric) = 0::numeric AND mod(annual_leave_adjustment * 2::numeric, 1::numeric) = 0::numeric AND mod(carry_over_days * 2::numeric, 1::numeric) = 0::numeric);

CREATE INDEX idx_leave_balances_employee ON hris.leave_balances USING btree (employee_id);

CREATE INDEX idx_leave_balances_year ON hris.leave_balances USING btree (year);

COMMENT ON TABLE "hris"."leave_balances" IS 'Quota cuti tahunan per karyawan per tahun';

COMMENT ON COLUMN "hris"."leave_balances"."annual_leave_adjustment" IS 'Penyesuaian HR atas jatah tahun ini (boleh negatif, kelipatan 0,5).';

COMMENT ON COLUMN "hris"."leave_balances"."carry_over_days" IS 'Sisa bawaan dari tahun lalu (kelipatan 0,5). Berlaku hanya bila carry_over_expires_on terisi dan belum lewat (R4).';

COMMENT ON COLUMN "hris"."leave_balances"."carry_over_used" IS 'Bagian sisa bawaan yang sudah dipakai cuti tahunan disetujui.';

COMMENT ON COLUMN "hris"."leave_balances"."carry_over_expires_on" IS 'Batas akhir pemakaian sisa bawaan (inklusif); NULL = bawaan tidak berlaku.';

COMMENT ON COLUMN "hris"."leave_balances"."carry_over_source" IS '''auto'' = bawaan dihitung ulang oleh rumus; ''manual'' = diisi HR (PUT/import) dan tidak ditimpa.';
