import { z } from "zod";
import { MENU_TYPES } from "@/lib/iam/types";

const optionalText = z.string().nullable().optional();

export const menuPayloadSchema = z.object({
  parentId: z.string().nullable().optional(),
  code: z.string().trim().min(1).max(100),
  menuName: z.string().trim().min(1).max(200),
  description: optionalText,
  routePath: optionalText,
  module: optionalText,
  menuType: z.enum(MENU_TYPES).optional(),
  icon: optionalText,
  orderNumber: z.number().int().optional(),
  isVisible: z.boolean().optional(),
  isActive: z.boolean().optional(),
  openInNewTab: z.boolean().optional(),
  permissionContext: z.object({ actions: z.array(z.string()) }).optional(),
});

export const menuUpdateSchema = menuPayloadSchema.partial();

export const roleCreateSchema = z.object({
  code: z.string().trim().min(1, "Code and name are required"),
  name: z.string().trim().min(1, "Code and name are required"),
  description: z.string().nullable().optional(),
  isActive: z.boolean().optional(),
});

export const roleUpdateSchema = z.object({
  code: z.string().optional(),
  name: z.string().optional(),
  description: z.string().nullable().optional(),
  isActive: z.boolean().optional(),
});

/**
 * uuid dalam bentuk kanonik (huruf kecil, bertanda hubung); menerima bentuk bertanda hubung atau 32 hex,
 * boleh dalam kurung kurawal, huruf besar/kecil. null bila bukan uuid. Sama dengan Go access.canonicalUUID:
 * guard hak hapus HRIS membandingkan id menu, jadi semua id menu disimpan dan dicek dalam satu bentuk.
 */
export function canonicalUuid(value: string): string | null {
  let s = value;
  if (s.length >= 2 && s.startsWith("{") && s.endsWith("}")) s = s.slice(1, -1);
  if (s.length === 36) {
    if (s[8] !== "-" || s[13] !== "-" || s[18] !== "-" || s[23] !== "-") return null;
    s = s.slice(0, 8) + s.slice(9, 13) + s.slice(14, 18) + s.slice(19, 23) + s.slice(24);
  }
  if (!/^[0-9a-fA-F]{32}$/.test(s)) return null;
  s = s.toLowerCase();
  return `${s.slice(0, 8)}-${s.slice(8, 12)}-${s.slice(12, 16)}-${s.slice(16, 20)}-${s.slice(20)}`;
}

/** menuId baris izin: kosong (baris dilewati) atau uuid, dikanonikkan. */
const permissionMenuId = z
  .string()
  .optional()
  .transform((value, ctx) => {
    if (!value) return value;
    const id = canonicalUuid(value);
    if (id) return id;
    ctx.addIssue({ code: "custom", message: "Invalid UUID" });
    return z.NEVER;
  });

export const rolePermissionsSchema = z.object({
  permissions: z
    .array(
      z.object({
        menuId: permissionMenuId,
        isGranted: z.boolean().optional(),
        grantedActions: z.array(z.string()).optional(),
      })
    )
    .default([]),
});

/** Baris izin yang dicentang saja; tanpa daftar aksi berarti hanya "read". */
export function normalizeRolePermissions(input: z.infer<typeof rolePermissionsSchema>["permissions"]) {
  return input
    .filter((item): item is typeof item & { menuId: string } => Boolean(item.menuId) && item.isGranted !== false)
    .map((item) => ({ menuId: item.menuId, grantedActions: item.grantedActions ?? ["read"] }));
}
