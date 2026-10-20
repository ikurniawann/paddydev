export interface HrisAccessRole {
  code: string;
  name: string;
}

export interface HrisAccessMenu {
  code: string;
  name: string;
  parent_code: string | null;
  level: number;
  access_toggle: boolean;
  delete_toggle: boolean;
}

export interface HrisAccessAccount {
  user_id: string;
  full_name: string;
  email: string | null;
  hris_role: string | null;
}

export interface HrisAccessView {
  roles: HrisAccessRole[];
  menus: HrisAccessMenu[];
  grants: Record<string, Record<string, string[]>>;
  accounts: HrisAccessAccount[];
}

export interface HrisAccessChange {
  menu_code: string;
  access?: boolean;
  delete?: boolean;
}

export interface PermissionAuditRow {
  id: string;
  source: "role_permissions" | "hris_access" | "hris_mask" | "user_role";
  actor_name: string | null;
  role_code: string | null;
  menu_code: string | null;
  actions_before: unknown;
  actions_after: unknown;
  target_name: string | null;
  roles_before: string[] | null;
  roles_after: string[] | null;
  created_at: string;
}
