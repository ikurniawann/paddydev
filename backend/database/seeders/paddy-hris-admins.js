#!/usr/bin/env node
/**
 * Seeder akun Admin HRIS Paddy — akun HR yang HANYA membuka modul HRIS + Area Karyawan (+ Beranda).
 *
 * 1. Role IAM `admin_hris` ("Admin HRIS"): menu HRIS & Beranda disalin dari role HRD, menu Area
 *    Karyawan disalin dari role Karyawan (employee). TANPA menu Settings atau modul lain; grant lain
 *    milik role ini dinonaktifkan setiap seeder dijalankan.
 * 2. Per akun: auth.users (password bcrypt), configuration.users dengan peran aplikasi `hrd` (logika HR
 *    di backend — approve cuti, closing absensi, slip semua karyawan — memakai peran ini) di lokasi HQ:
 *    cabang MIZU-BANDUNG (entitas Paddy Kantor Pusat) + gudang WH-01; iam.user_roles HANYA
 *    admin_hris, sehingga menu yang terbuka hanya menu role tersebut (lihat platform/auth/grants.go).
 * 3. Data karyawan (hris.employees, NIP EMP-<tahun>-NNNNN) terhubung ke akun + penempatan utama di HQ,
 *    tanpa struktur gaji, supaya Area Karyawan menampilkan profil sendiri. Karyawan dengan email sama
 *    dipakai ulang.
 *
 * 4. Role turunan per akun (EPIC-057): hris_manager (seluruh HRIS), hris_non_payroll (tanpa Penggajian),
 *    hris_non_rekrutmen_kpi (tanpa Rekrutmen & Kinerja) dibuat migrasi 20261019100000 dan disinkronkan
 *    ulang dari admin_hris (iam.sync_hris_staff_roles) setiap seeder jalan. Akun yang sudah memegang
 *    role HR tetap pada role itu, kecuali role ditulis di HRIS_ADMINS (email|Nama|role).
 *
 * Usage (email & password tidak disimpan di repo):
 *   HRIS_ADMINS="email1|Nama 1;email2|Nama 2|hris_non_payroll" HRIS_ADMIN_PASSWORD='...' pnpm db:seed:paddy-hris-admins
 *   tambahkan --allow-remote untuk target non-lokal
 *   akun yang sudah ada dengan role lain TIDAK ditimpa kecuali --force
 *
 * Catatan keamanan: peran aplikasi `hrd` + role IAM admin_hris. Jangan simpan ulang akun ini dari
 * Settings → Manajemen User dengan role berbeda — perubahan role mengganti role IAM ke role aplikasi.
 */

const bcrypt = require("bcryptjs");
const { Client } = require("pg");
const { sslForUrl } = require("../scripts/pg-utils");
const { assertLocalOrAllowed } = require("./lib/seed-password");
const D = require("./lib/paddy-spa-data");

const ROLE = { code: "admin_hris", name: "Admin HRIS", description: "Admin HRIS: modul HRIS + Area Karyawan saja" };
const HQ_BRANCH = "MIZU-BANDUNG";
const HQ_WAREHOUSE = "WH-01";
const MARK = "Seeder Admin HRIS";
const HR_ROLE_CODES = ["admin_hris", "hris_manager", "hris_non_payroll", "hris_non_rekrutmen_kpi"];

function parseAdmins(raw) {
  return (raw || "").split(";").map((s) => s.trim()).filter(Boolean).map((pair) => {
    const [email, name, role] = pair.split("|").map((x) => (x || "").trim());
    if (!/^[^@\s]+@[^@\s]+$/.test(email) || !name) throw new Error(`HRIS_ADMINS tidak valid: "${pair}" (format email|Nama[|role])`);
    if (role && !HR_ROLE_CODES.includes(role)) {
      throw new Error(`HRIS_ADMINS tidak valid: role "${role}" (pilih: ${HR_ROLE_CODES.join(", ")})`);
    }
    return { email: email.toLowerCase(), name, role: role || undefined };
  });
}

async function one(c, sql, params, what) {
  const row = (await c.query(sql, params)).rows[0];
  if (!row) throw new Error(`${what} tidak ditemukan`);
  return row;
}

/** Role admin_hris dengan grant: subtree HRIS + Beranda dari HRD, subtree Area Karyawan dari employee. */
async function ensureRole(c) {
  const roleId = (await c.query(`INSERT INTO iam.roles (code, name, description, is_system, is_active)
     VALUES ($1, $2, $3, false, true)
     ON CONFLICT (code) DO UPDATE SET name = EXCLUDED.name, description = EXCLUDED.description, is_active = true,
       deleted_at = NULL, updated_at = NOW()
     RETURNING id::text`, [ROLE.code, ROLE.name, ROLE.description])).rows[0].id;
  const grants = (await c.query(`
    WITH RECURSIVE tree AS (
      SELECT id, code AS root FROM iam.menus WHERE parent_id IS NULL AND code IN ('hris', 'dashboard', 'ess')
      UNION ALL
      SELECT m.id, t.root FROM iam.menus m JOIN tree t ON m.parent_id = t.id)
    SELECT t.id::text AS menu_id, rmp.granted_actions
      FROM tree t
      JOIN iam.roles src ON src.code = CASE WHEN t.root = 'ess' THEN 'employee' ELSE 'hrd' END
      JOIN iam.role_menu_permissions rmp ON rmp.role_id = src.id AND rmp.menu_id = t.id AND rmp.is_active`)).rows;
  if (!grants.length) throw new Error("Grant sumber (role hrd/employee) kosong — jalankan db:seed:iam dulu");
  // Direktori karyawan HRIS: HRD hanya "read" (HRD menulis lewat Manajemen User); Admin HRIS menulis
  // data karyawan langsung di HRIS → Karyawan (akun login tetap khusus Manajemen User).
  const karyawan = (await c.query(`SELECT id::text FROM iam.menus WHERE code = 'hris.kepegawaian.users'`)).rows[0];
  if (karyawan) {
    const g = grants.find((x) => x.menu_id === karyawan.id);
    if (g) g.granted_actions = ["read", "create", "update"];
    else grants.push({ menu_id: karyawan.id, granted_actions: ["read", "create", "update"] });
  }
  for (const g of grants) {
    await c.query(`INSERT INTO iam.role_menu_permissions (role_id, menu_id, granted_actions, is_active)
       VALUES ($1, $2, $3::jsonb, true)
       ON CONFLICT (role_id, menu_id) DO UPDATE SET granted_actions = EXCLUDED.granted_actions, is_active = true,
         updated_at = NOW()`, [roleId, g.menu_id, JSON.stringify(g.granted_actions)]);
  }
  const revoked = await c.query(`UPDATE iam.role_menu_permissions SET is_active = false, updated_at = NOW()
     WHERE role_id = $1 AND is_active AND NOT (menu_id = ANY($2::uuid[]))`, [roleId, grants.map((g) => g.menu_id)]);
  return { roleId, menus: grants.length, revoked: revoked.rowCount };
}

async function nextNip(c, year) {
  const r = await c.query(`SELECT coalesce(max(substring(nip from '^EMP-${year}-(\\d{5})$')::int), 0) + 1 AS n
     FROM hris.employees WHERE nip ~ '^EMP-${year}-\\d{5}$'`);
  return `EMP-${year}-${String(r.rows[0].n).padStart(5, "0")}`;
}

async function seedAccount(c, a, ref, passwordHash, today, force) {
  const existing = (await c.query(`SELECT u.id::text,
       (SELECT r.code FROM iam.user_roles ur JOIN iam.roles r ON r.id = ur.role_id
         WHERE ur.user_id = u.id AND r.code = ANY($2::text[]) ORDER BY ur.is_primary DESC LIMIT 1) AS hr_role
     FROM auth.users u WHERE lower(u.email) = $1`, [a.email, HR_ROLE_CODES])).rows[0];
  if (existing && !existing.hr_role && !force) {
    throw new Error(`${a.email} sudah punya akun dengan role lain — tidak ditimpa. Pakai --force bila memang disengaja.`);
  }
  const userId = (await c.query(
    `INSERT INTO auth.users (email, password_hash, email_verified_at, raw_user_meta_data, raw_app_meta_data)
     VALUES ($1, $2, NOW(), $3::jsonb, '{"role":"hrd"}'::jsonb)
     ON CONFLICT (email) DO UPDATE SET password_hash = EXCLUDED.password_hash, email_verified_at = NOW(),
       raw_user_meta_data = EXCLUDED.raw_user_meta_data, raw_app_meta_data = EXCLUDED.raw_app_meta_data, updated_at = NOW()
     RETURNING id::text`, [a.email, passwordHash, JSON.stringify({ full_name: a.name, role: "hrd" })])).rows[0].id;
  await c.query(`UPDATE auth.users SET banned_until = NULL WHERE id = $1`, [userId]);
  await c.query(
    `INSERT INTO configuration.users (id, full_name, role, email, status, business_scope, holding_id, company_id, branch_id,
       default_warehouse_id, can_switch_stall)
     VALUES ($1, $2, 'hrd', $3, 'active', 'branch', $4, $5, $6, $7, false)
     ON CONFLICT (id) DO UPDATE SET full_name = EXCLUDED.full_name, role = 'hrd', email = EXCLUDED.email, status = 'active',
       business_scope = 'branch', holding_id = EXCLUDED.holding_id, company_id = EXCLUDED.company_id,
       branch_id = EXCLUDED.branch_id, default_warehouse_id = EXCLUDED.default_warehouse_id, updated_at = NOW()`,
    [userId, a.name, a.email, ref.holdingId, ref.companyId, ref.branchId, ref.warehouseId]);
  await c.query(`INSERT INTO configuration.user_warehouses (user_id, warehouse_id, is_active) VALUES ($1, $2, true)
     ON CONFLICT (user_id, warehouse_id) DO UPDATE SET is_active = true`, [userId, ref.warehouseId]);
  // Tanpa izin approval modul lain (approval PO, diskon, dll.).
  await c.query(`UPDATE configuration.user_approval_permissions SET is_active = false, updated_at = NOW()
     WHERE user_id = $1 AND is_active`, [userId]);
  // Hanya satu role HR: role IAM lain dicabut agar menu tidak melebar. Role yang sudah dipegang
  // (mis. hris_non_payroll) dipertahankan kecuali HRIS_ADMINS menyebut role lain.
  const roleCode = a.role || existing?.hr_role || ROLE.code;
  const roleRow = (await c.query(`SELECT id::text FROM iam.roles WHERE code = $1 AND deleted_at IS NULL`, [roleCode])).rows[0];
  if (!roleRow) throw new Error(`Role IAM ${roleCode} belum ada — jalankan migrasi dulu`);
  await c.query(`DELETE FROM iam.user_roles WHERE user_id = $1 AND role_id <> $2`, [userId, roleRow.id]);
  await c.query(`INSERT INTO iam.user_roles (user_id, role_id, is_primary) VALUES ($1, $2, true)
     ON CONFLICT (user_id, role_id) DO UPDATE SET is_primary = true`, [userId, roleRow.id]);

  let emp = (await c.query(`SELECT id::text, nip, user_id::text FROM hris.employees WHERE lower(email) = $1
     ORDER BY created_at LIMIT 1`, [a.email])).rows[0];
  if (emp && emp.user_id && emp.user_id !== userId) {
    throw new Error(`Data karyawan ${emp.nip} (${a.email}) sudah terhubung ke akun lain — tidak dihubungkan ulang.`);
  }
  if (!emp) {
    const nip = await nextNip(c, today.slice(0, 4));
    emp = (await c.query(`INSERT INTO hris.employees (full_name, nip, email, phone, join_date, is_active, is_access_app, user_id, city,
         province, notes)
       VALUES ($1, $2, $3, '', $4, true, true, $5, 'Bandung', 'Jawa Barat', $6) RETURNING id::text, nip`,
      [a.name, nip, a.email, today, userId, `${MARK} — akun HR kantor pusat`])).rows[0];
  } else {
    await c.query(`UPDATE hris.employees SET user_id = $2, is_access_app = true, is_active = true, updated_at = NOW() WHERE id = $1`,
      [emp.id, userId]);
  }
  const placed = await c.query(`SELECT 1 FROM hris.employee_placements WHERE employee_id = $1 AND branch_id = $2
     AND (end_date IS NULL OR end_date >= $3::date)`, [emp.id, ref.branchId, today]);
  if (!placed.rowCount) {
    await c.query(`INSERT INTO hris.employee_placements (employee_id, branch_id, is_primary, start_date, notes)
       VALUES ($1, $2, NOT EXISTS (SELECT 1 FROM hris.employee_placements WHERE employee_id = $1 AND is_primary
         AND (end_date IS NULL OR end_date >= $3::date)), $3, $4)`, [emp.id, ref.branchId, today, `HR kantor pusat · ${MARK}`]);
  }
  return { userId, nip: emp.nip, roleCode };
}

async function main() {
  D.loadEnv();
  const admins = parseAdmins(process.env.HRIS_ADMINS);
  const password = process.env.HRIS_ADMIN_PASSWORD;
  if (!admins.length || !password) {
    console.error('ERROR: set HRIS_ADMINS="email|Nama;..." dan HRIS_ADMIN_PASSWORD');
    process.exit(1);
  }
  const url = process.env.MIGRATE_DATABASE_URL || process.env.DATABASE_URL;
  if (!url) {
    console.error("ERROR: Set MIGRATE_DATABASE_URL / DATABASE_URL");
    process.exit(1);
  }
  assertLocalOrAllowed(url);
  const c = new Client({ connectionString: url, ssl: sslForUrl(url) });
  await c.connect();
  try {
    const passwordHash = await bcrypt.hash(password, 10);
    const today = new Date(Date.now() + 7 * 3600 * 1000).toISOString().slice(0, 10); // WIB
    await c.query("BEGIN");
    const branch = await one(c, `SELECT b.id::text, b.company_id::text, co.holding_id::text, co.name AS company, b.name
       FROM configuration.branches b JOIN configuration.companies co ON co.id = b.company_id WHERE b.code = $1`,
      [HQ_BRANCH], `Cabang ${HQ_BRANCH}`);
    const wh = await one(c, `SELECT id::text, name FROM configuration.warehouses WHERE code = $1 AND branch_id = $2`,
      [HQ_WAREHOUSE, branch.id], `Gudang ${HQ_WAREHOUSE} di ${HQ_BRANCH}`);
    const role = await ensureRole(c);
    // Role turunan per akun mengikuti grant admin_hris terbaru (EPIC-057).
    if ((await c.query(`SELECT to_regprocedure('iam.sync_hris_staff_roles()') IS NOT NULL AS ok`)).rows[0].ok) {
      await c.query(`SELECT iam.sync_hris_staff_roles()`);
    }
    const ref = { roleId: role.roleId, branchId: branch.id, companyId: branch.company_id, holdingId: branch.holding_id, warehouseId: wh.id };
    const out = [];
    const force = process.argv.includes("--force");
    for (const a of admins) out.push({ ...a, ...(await seedAccount(c, a, ref, passwordHash, today, force)) });
    await c.query("COMMIT");
    console.log(`${MARK} selesai.`);
    console.log(`  role ${ROLE.code}: ${role.menus} menu (HRIS + Beranda dari HRD, Area Karyawan dari Karyawan)${role.revoked ? `, ${role.revoked} grant lain dicabut` : ""}`);
    console.log(`  lokasi: ${branch.name} (${branch.company}) · gudang ${wh.name}`);
    for (const a of out) console.log(`  ${a.email.padEnd(36)} ${a.name.padEnd(26)} karyawan ${a.nip} · role ${a.roleCode}`);
    console.log("  password: dari HRIS_ADMIN_PASSWORD (tidak dicetak)");
  } catch (err) {
    await c.query("ROLLBACK").catch(() => {});
    throw err;
  } finally {
    await c.end();
  }
}

if (require.main === module) {
  main().catch((err) => {
    console.error(err.message || err);
    process.exit(1);
  });
}

module.exports = { parseAdmins };
