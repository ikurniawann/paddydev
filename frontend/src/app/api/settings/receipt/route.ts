import { NextRequest, NextResponse } from "next/server";
import { requireIamMenuPrefix } from "@/lib/api/auth";
import { apiHandler } from "@/lib/api/handler";
import { IAM } from "@/lib/iam/prefixes";
import { withBranchLabels } from "@/lib/auth/stall-label";
import { createPgClient } from "@/lib/pg/create-client";
import { loadPosReceiptSettingsRows, normalizeReceiptSettings } from "@/lib/pos/receipt-settings";
import { parseReceiptScopeInput, upsertReceiptScope } from "@/lib/settings/receipt-scope";

/**
 * GET /api/settings/receipt — semua baris konfigurasi struk aktif
 * (global + per-branch + per-warehouse), untuk UI Settings → Business. EPIC-040.
 */
export const GET = apiHandler(async () => {
  await requireIamMenuPrefix(IAM.settingsBusiness);
  const db = createPgClient();
  const rows = await loadPosReceiptSettingsRows(db);
  // Opsi scope untuk UI — di-query sendiri supaya halaman ini tidak
  // bergantung pada permission endpoint warehouse milik modul lain.
  const { data: stalls, error } = await db
    .from("warehouses")
    .select("id, name, branch_id, is_default")
    .order("name", { ascending: true });
  if (error) throw error;
  // Tiap branch punya "Main Storage" sendiri: beri nama branch + label.
  const { data: branches, error: branchError } = await db.from("branches").select("id, name");
  if (branchError) throw branchError;
  const branchNames = new Map(
    ((branches ?? []) as Array<{ id: string; name: string }>).map((branch) => [branch.id, branch.name])
  );
  const options = withBranchLabels(
    (stalls ?? []) as Array<{ id: string; name: string; branch_id: string; is_default: boolean | null }>,
    branchNames
  ).map(({ id, name, branch_id, branch_name, label }) => ({ id, name, branch_id, branch_name, label }));
  return NextResponse.json({
    success: true,
    data: rows.map((row) => normalizeReceiptSettings(row)),
    stalls: options,
  });
}, "GET /api/settings/receipt");

/**
 * PUT /api/settings/receipt — upsert satu scope.
 * Body: { warehouse_id?, branch_id?, header_lines, footer_lines, show_stall_name }.
 */
export const PUT = apiHandler(async (request: NextRequest) => {
  await requireIamMenuPrefix(IAM.settingsBusiness);
  const input = parseReceiptScopeInput(await request.json().catch(() => ({})));
  const db = createPgClient();
  await upsertReceiptScope(db, input);
  const rows = await loadPosReceiptSettingsRows(db);
  return NextResponse.json({ success: true, data: rows.map((row) => normalizeReceiptSettings(row)) });
}, "PUT /api/settings/receipt");
