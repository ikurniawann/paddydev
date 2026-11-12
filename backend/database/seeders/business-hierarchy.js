#!/usr/bin/env node
/**
 * Seeder: Business hierarchy kantor pusat (EPIC-106) — holding Paddy Group (PADDY) dengan perusahaan
 * CV Cipta Kreasi Kreatif (ERP), sejajar dengan Paddy (PADDY-RETAIL, toko, dibuat seeders/paddy-spa.js).
 * Perusahaan ERP dicari per kode di holding mana pun (dulu di holding PROLOGE) lalu dipindah ke Paddy
 * Group: tidak pernah membuat perusahaan ERP kedua. Holding lama PROLOGE yang kosong dinonaktifkan.
 *
 * Usage:
 *   node database/seeders/business-hierarchy.js
 *   npm run db:seed:business-hierarchy
 */

const fs = require("fs");
const path = require("path");
const { Client } = require("pg");
const { sslForUrl, assertLocalTarget } = require("../scripts/pg-utils");
const { COMPANY_CODE, BRANCH_CODE, BRANCH_NAME } = require("../scripts/items-business-scope");

// Struktur target EPIC-106; sengaja tidak memakai SEED_HOLDING_CODE supaya nama Paddy Group tidak tertimpa.
const GROUP = { code: "PADDY", name: "Paddy Group" };
const OLD_HOLDING_CODE = "PROLOGE";
const COMPANY_NAME = "CV Cipta Kreasi Kreatif";

const ROOT = path.join(__dirname, "..", "..");

function loadEnv() {
  const shellKeys = new Set(Object.keys(process.env));
  for (const name of [".env", ".env.local"]) {
    const file = path.join(ROOT, name);
    if (!fs.existsSync(file)) continue;
    for (const line of fs.readFileSync(file, "utf-8").split("\n")) {
      const t = line.trim();
      if (!t || t.startsWith("#")) continue;
      const i = t.indexOf("=");
      if (i <= 0) continue;
      const k = t.slice(0, i).trim();
      let v = t.slice(i + 1).trim();
      if ((v.startsWith('"') && v.endsWith('"')) || (v.startsWith("'") && v.endsWith("'"))) {
        v = v.slice(1, -1);
      }
      if (!shellKeys.has(k)) process.env[k] = v;
    }
  }
}

async function main() {
  loadEnv();
  const url = process.env.MIGRATE_DATABASE_URL || process.env.DATABASE_URL;
  if (!url) {
    console.error("ERROR: Set MIGRATE_DATABASE_URL / DATABASE_URL di .env / .env.local");
    process.exit(1);
  }
  try {
    assertLocalTarget(url, "MIGRATE_DATABASE_URL");
  } catch (err) {
    console.error(err.message);
    process.exit(1);
  }

  const client = new Client({ connectionString: url, ssl: sslForUrl(url) });
  await client.connect();

  try {
    await client.query("BEGIN");

    await client.query(
      `INSERT INTO configuration.holdings (name, code) VALUES ($1, $2) ON CONFLICT (code) DO NOTHING`,
      [GROUP.name, GROUP.code]
    );

    const { rows: holdingRows } = await client.query(
      `SELECT id, name FROM configuration.holdings WHERE code = $1 LIMIT 1`,
      [GROUP.code]
    );
    const holdingId = holdingRows[0].id;

    // ERP di holding mana pun (dulu PROLOGE) dipindah ke Paddy Group; baru dibuat bila belum ada sama sekali.
    const { rows: existing } = await client.query(
      `SELECT id FROM configuration.companies WHERE code = $2 ORDER BY (holding_id = $1) DESC, created_at LIMIT 1`,
      [holdingId, COMPANY_CODE]
    );
    if (existing[0]) {
      await client.query(
        `UPDATE configuration.companies SET holding_id = $2, name = $3, updated_at = NOW()
         WHERE id = $1 AND (holding_id, name) IS DISTINCT FROM ($2, $3)`,
        [existing[0].id, holdingId, COMPANY_NAME]
      );
    } else {
      await client.query(
        `INSERT INTO configuration.companies (holding_id, name, code) VALUES ($1, $2, $3)`,
        [holdingId, COMPANY_NAME, COMPANY_CODE]
      );
    }

    const { rows: companyRows } = await client.query(
      `SELECT id, name FROM configuration.companies WHERE holding_id = $1 AND code = $2 LIMIT 1`,
      [holdingId, COMPANY_CODE]
    );
    const companyId = companyRows[0].id;

    // Akun perusahaan ini ikut holding-nya; holding lama yang kosong dinonaktifkan (tidak dihapus).
    await client.query(
      `UPDATE configuration.users SET holding_id = $1, updated_at = NOW()
       WHERE company_id = $2 AND holding_id IS DISTINCT FROM $1`,
      [holdingId, companyId]
    );
    await client.query(
      `UPDATE configuration.users SET holding_id = $1, updated_at = NOW()
       WHERE company_id IS NULL AND holding_id = (SELECT id FROM configuration.holdings WHERE code = $2)`,
      [holdingId, OLD_HOLDING_CODE]
    );
    await client.query(
      `UPDATE configuration.holdings h SET is_active = false, updated_at = NOW()
       WHERE h.code = $1 AND h.is_active
         AND NOT EXISTS (SELECT 1 FROM configuration.companies c WHERE c.holding_id = h.id)`,
      [OLD_HOLDING_CODE]
    );

    const { rows: legacyBranches } = await client.query(
      `SELECT id, code, name
       FROM configuration.branches
       WHERE company_id = $1
         AND code IN ('MIZU-BRAGA', 'MIZU-DAGO')
       ORDER BY CASE code WHEN 'MIZU-BRAGA' THEN 0 WHEN 'MIZU-DAGO' THEN 1 ELSE 2 END
       LIMIT 1`,
      [companyId]
    );

    if (legacyBranches[0]) {
      await client.query(
        `UPDATE configuration.branches
         SET name = $1, code = $2, is_active = true, updated_at = NOW()
         WHERE id = $3`,
        [BRANCH_NAME, BRANCH_CODE, legacyBranches[0].id]
      );
      console.log(`  ↪ cabang ${legacyBranches[0].code} → ${BRANCH_CODE}`);
    } else {
      await client.query(
        `INSERT INTO configuration.branches (company_id, name, code)
         VALUES ($1, $2, $3)
         ON CONFLICT (company_id, code) DO UPDATE
           SET name = EXCLUDED.name, is_active = true, updated_at = NOW()`,
        [companyId, BRANCH_NAME, BRANCH_CODE]
      );
    }

    const { rows: branchRows } = await client.query(
      `SELECT id, name FROM configuration.branches WHERE company_id = $1 AND code = $2 LIMIT 1`,
      [companyId, BRANCH_CODE]
    );

    await client.query("COMMIT");
    console.log("Business hierarchy siap:");
    console.log(`  ${holdingRows[0].name} → ${companyRows[0].name} → ${branchRows[0].name}`);
  } catch (err) {
    await client.query("ROLLBACK").catch(() => {});
    console.error("Gagal:", err.message);
    process.exitCode = 1;
  } finally {
    await client.end();
  }
}

main().catch((err) => {
  console.error("Fatal:", err.message);
  process.exit(1);
});
