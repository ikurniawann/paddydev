import { getIamPool, iamDbQuery, iamDbQueryOne } from "./pg-client";
import type { IamRolePermissionRow, IamRoleRow } from "./role-mapper";

const ROLE_COLUMNS = `
  id, code, name, description, is_system, is_active, created_at, updated_at, version
`;

export async function listIamRolesFromDb(): Promise<IamRoleRow[]> {
  return iamDbQuery<IamRoleRow>(
    `SELECT ${ROLE_COLUMNS},
      (
        SELECT COUNT(*)::int
        FROM iam.role_menu_permissions rmp
        WHERE rmp.role_id = r.id AND rmp.is_active = true
      ) AS menu_permission_count
     FROM iam.roles r
     WHERE r.deleted_at IS NULL
     ORDER BY r.code ASC`
  );
}

export async function getIamRoleFromDb(id: string): Promise<IamRoleRow | null> {
  return iamDbQueryOne<IamRoleRow>(
    `SELECT ${ROLE_COLUMNS},
      (
        SELECT COUNT(*)::int
        FROM iam.role_menu_permissions rmp
        WHERE rmp.role_id = r.id AND rmp.is_active = true
      ) AS menu_permission_count
     FROM iam.roles r
     WHERE r.id = $1 AND r.deleted_at IS NULL`,
    [id]
  );
}

export async function listRolePermissionsMatrixFromDb(
  roleId: string
): Promise<IamRolePermissionRow[]> {
  return iamDbQuery<IamRolePermissionRow>(
    `SELECT
      m.id AS menu_id,
      m.code AS menu_code,
      m.menu_name,
      m.menu_type,
      m.parent_id,
      m.level,
      m.order_number,
      m.permission_context,
      rmp.granted_actions,
      rmp.id AS permission_id,
      rmp.is_active AS permission_is_active
     FROM iam.menus m
     LEFT JOIN iam.role_menu_permissions rmp
       ON rmp.menu_id = m.id AND rmp.role_id = $1
     WHERE m.deleted_at IS NULL
     ORDER BY m.level ASC, m.order_number ASC, m.menu_name ASC`,
    [roleId]
  );
}

export async function createIamRoleInDb(
  payload: Record<string, unknown>
): Promise<string> {
  const row = await iamDbQueryOne<{ id: string }>(
    `INSERT INTO iam.roles (code, name, description, is_system, is_active)
     VALUES ($1, $2, $3, false, $4)
     RETURNING id`,
    [
      payload.code,
      payload.name,
      payload.description ?? null,
      payload.isActive ?? true,
    ]
  );

  if (!row) throw new Error("Failed to create role");
  return row.id;
}

export async function updateIamRoleInDb(
  id: string,
  payload: Record<string, unknown>
): Promise<boolean> {
  const row = await iamDbQueryOne<{ id: string }>(
    `UPDATE iam.roles SET
      code = COALESCE($2, code),
      name = COALESCE($3, name),
      description = CASE WHEN $4::boolean THEN $5 ELSE description END,
      is_active = COALESCE($6, is_active),
      updated_at = now(),
      version = version + 1
     WHERE id = $1 AND deleted_at IS NULL
     RETURNING id`,
    [
      id,
      payload.code ?? null,
      payload.name ?? null,
      payload.description !== undefined,
      payload.description ?? null,
      payload.isActive ?? null,
    ]
  );

  return Boolean(row);
}

export async function deleteIamRoleInDb(id: string): Promise<boolean> {
  const row = await iamDbQueryOne<{ id: string; is_system: boolean }>(
    `SELECT id, is_system FROM iam.roles WHERE id = $1 AND deleted_at IS NULL`,
    [id]
  );

  if (!row) return false;
  if (row.is_system) {
    throw new Error("System roles cannot be deleted");
  }

  const deleted = await iamDbQueryOne<{ id: string }>(
    `UPDATE iam.roles SET
      deleted_at = now(),
      is_active = false,
      updated_at = now(),
      version = version + 1
     WHERE id = $1 AND deleted_at IS NULL
     RETURNING id`,
    [id]
  );

  return Boolean(deleted);
}

/** Query di dalam transaksi replaceIamRolePermissionsInDb. */
export type RoleTxQuery = (sql: string, params: unknown[]) => Promise<{ rows: unknown[] }>;

export async function replaceIamRolePermissionsInDb(
  roleId: string,
  permissions: Array<{ menuId: string; grantedActions: string[] }>,
  updatedBy: string,
  options: {
    /** admin_hris: jalankan sync role HR turunan di transaksi yang sama (EPIC-061, R10). */
    syncHrisStaffRoles?: boolean;
    /** Cek di transaksi setelah baris role dikunci, terhadap kode dan grant role saat ini (Go replacePermissions). */
    guard?: (query: RoleTxQuery, code: string) => Promise<void>;
  } = {}
): Promise<void> {
  const pool = getIamPool();
  const client = await pool.connect();

  try {
    await client.query("BEGIN");

    const locked = await client.query<{ code: string }>(`SELECT code FROM iam.roles WHERE id = $1 FOR UPDATE`, [roleId]);
    const code = locked.rows[0]?.code;
    if (code === undefined) throw new Error("Role not found");
    if (options.guard) await options.guard((sql, params) => client.query(sql, params), code);

    await client.query(
      `UPDATE iam.role_menu_permissions
       SET is_active = false, updated_at = now(), updated_by = $2
       WHERE role_id = $1`,
      [roleId, updatedBy]
    );

    for (const permission of permissions) {
      const actions =
        permission.grantedActions.length > 0 ? permission.grantedActions : ["read"];

      await client.query(
        `INSERT INTO iam.role_menu_permissions (
          role_id, menu_id, granted_actions, is_active, created_by, updated_by, updated_at
        ) VALUES ($1, $2, $3::jsonb, true, $4, $4, now())
        ON CONFLICT (role_id, menu_id) DO UPDATE SET
          granted_actions = EXCLUDED.granted_actions,
          is_active = true,
          updated_by = EXCLUDED.updated_by,
          updated_at = now()`,
        [roleId, permission.menuId, JSON.stringify(actions), updatedBy]
      );
    }

    if (options.syncHrisStaffRoles) {
      await client.query("SELECT iam.sync_hris_staff_roles()");
    }

    await client.query("COMMIT");
  } catch (error) {
    await client.query("ROLLBACK");
    throw error;
  } finally {
    client.release();
  }
}
