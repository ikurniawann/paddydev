// node --test seeders/paddy-hris-admins.test.js (dari backend/database).
// Bagian DB hanya jalan bila TEST_DATABASE_URL diset, di dalam transaksi yang di-rollback.
const test = require("node:test");
const assert = require("node:assert/strict");
const { Client } = require("pg");
const { planAdminHrisGrants, ensureRole } = require("./paddy-hris-admins");

const g = (menu_id, granted_actions, is_active = true) => ({ menu_id, granted_actions, is_active });

test("admin_hris kosong (DB baru): salin sumber apa adanya dan cabut grant lain", () => {
  const source = [g("a", ["read"]), g("b", ["read", "create"])];
  const plan = planAdminHrisGrants([g("x", ["read"], false)], source);
  assert.equal(plan.revokeOthers, true);
  assert.deepEqual(plan.upserts.map((u) => [u.menu_id, u.granted_actions]), [["a", ["read"]], ["b", ["read", "create"]]]);
});

test("admin_hris sudah punya grant: tidak ada yang dicabut, hanya menambah yang belum ada", () => {
  const existing = [
    g("access", ["read", "update"]), // hris.access, tidak ada di HRD
    g("att", ["read", "delete"]), // delete dari Hak Akses HRIS
    g("ann", ["read", "update"]), // delete sengaja dimatikan AdminHRIS
    g("pay", ["read"], false), // Penggajian sengaja dimatikan AdminHRIS
  ];
  const source = [
    g("att", ["read"]),
    g("ann", ["read", "create", "update", "delete"]),
    g("pay", ["read"]),
    g("new", ["read", "export"]),
  ];
  const plan = planAdminHrisGrants(existing, source);
  assert.equal(plan.revokeOthers, false);
  assert.deepEqual(plan.upserts.map((u) => [u.menu_id, u.granted_actions]), [
    ["ann", ["read", "update", "create"]],
    ["new", ["read", "export"]],
  ]);
});

const url = process.env.TEST_DATABASE_URL;
test("ensureRole menjaga grant admin_hris yang ada (DB test, di-rollback)", { skip: !url || /:55432\b/.test(url) ? "TEST_DATABASE_URL" : false }, async () => {
  const c = new Client({ connectionString: url });
  await c.connect();
  const grant = async (menu) => (await c.query(`SELECT p.granted_actions, p.is_active FROM iam.role_menu_permissions p
      JOIN iam.roles r ON r.id = p.role_id JOIN iam.menus m ON m.id = p.menu_id WHERE r.code = 'admin_hris' AND m.code = $1`, [menu])).rows[0];
  try {
    await c.query("BEGIN");
    const has = await c.query(`SELECT 1 FROM iam.role_menu_permissions p JOIN iam.roles r ON r.id = p.role_id
        WHERE r.code = 'admin_hris' AND p.is_active LIMIT 1`);
    assert.ok(has.rowCount, "admin_hris di DB test belum punya grant");
    // Penggajian dimatikan, Hari Libur belum pernah ada: yang kedua ditambah, yang pertama tetap mati.
    await c.query(`UPDATE iam.role_menu_permissions p SET is_active = false FROM iam.roles r, iam.menus m
        WHERE r.id = p.role_id AND m.id = p.menu_id AND r.code = 'admin_hris' AND m.code = 'hris.compensation.payroll'`);
    await c.query(`DELETE FROM iam.role_menu_permissions p USING iam.roles r, iam.menus m
        WHERE r.id = p.role_id AND m.id = p.menu_id AND r.code = 'admin_hris' AND m.code = 'hris.kepegawaian.holidays'`);
    await c.query(`UPDATE iam.role_menu_permissions p SET granted_actions = '["read","delete"]'::jsonb, is_active = true
        FROM iam.roles r, iam.menus m WHERE r.id = p.role_id AND m.id = p.menu_id AND r.code = 'admin_hris'
          AND m.code = 'hris.workforce.attendance'`);
    const access = await grant("hris.access");
    const res = await ensureRole(c);
    assert.equal(res.revoked, 0);
    assert.deepEqual(await grant("hris.access"), access);
    // EPIC-067: "approve" dari grant HRD ikut ditambahkan; "delete" yang ada tetap.
    assert.deepEqual((await grant("hris.workforce.attendance")).granted_actions, ["read", "delete", "approve"]);
    assert.equal((await grant("hris.compensation.payroll")).is_active, false);
    const holidays = await grant("hris.kepegawaian.holidays");
    assert.ok(holidays && holidays.is_active, "Hari Libur dari HRD tidak ditambahkan");
  } finally {
    await c.query("ROLLBACK").catch(() => {});
    await c.end();
  }
});
