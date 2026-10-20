import { NextRequest, NextResponse } from "next/server";
import { ApiError, requireIamMenuPrefix } from "@/lib/api/auth";
import { apiHandler } from "@/lib/api/handler";
import { IAM } from "@/lib/iam/prefixes";
import { softDeleteHoliday, updateHoliday } from "@/lib/hris/holidays-db";
import { holidayBodySchema, validateHolidayPatch } from "@/lib/hris/holidays-input";
import { readJson, requireUuid } from "@/lib/hris/workforce-route";
import { HRIS_LEAF, auditHrisDelete, requireHrisDelete } from "@/lib/hris/delete-guard";

/**
 * PATCH  /api/hris/holidays/[id] — ubah satu hari libur (termasuk menyetujui
 *        baris `draft` hasil impor menjadi `aktif`).
 * DELETE /api/hris/holidays/[id] — soft delete.
 */

interface RouteParams {
  params: Promise<{ id: string }>;
}

export const PATCH = apiHandler(async (req: NextRequest, { params }: RouteParams) => {
  const user = await requireIamMenuPrefix(IAM.hrisKepegawaian);
  const id = requireUuid((await params).id, "ID libur tidak valid");
  const body = await readJson(req, holidayBodySchema);
  const invalid = validateHolidayPatch(body);
  if (invalid) throw ApiError.badRequest(invalid);

  const updated = await updateHoliday(id, body, user.id);
  if (!updated) throw ApiError.notFound("Hari libur tidak ditemukan");
  return NextResponse.json({ message: `Libur "${updated.name}" diperbarui` });
}, "hris/holidays PATCH");

export const DELETE = apiHandler(async (request: NextRequest, { params }: RouteParams) => {
  const { user, reason } = await requireHrisDelete(request, HRIS_LEAF.hariLibur);
  const id = requireUuid((await params).id, "ID libur tidak valid");
  const deleted = await softDeleteHoliday(id, user.id);
  if (!deleted) throw ApiError.notFound("Hari libur tidak ditemukan");
  await auditHrisDelete(request, user, "hris.public_holidays", id, reason);
  return NextResponse.json({ message: `Libur "${deleted.name}" dihapus` });
}, "hris/holidays DELETE");
