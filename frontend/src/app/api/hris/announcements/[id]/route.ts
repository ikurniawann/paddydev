import { NextRequest, NextResponse } from "next/server";
import { requireIamMenuPrefix } from "@/lib/api/auth";
import { apiHandler } from "@/lib/api/handler";
import { IAM } from "@/lib/iam/prefixes";
import { canManageAnnouncements } from "@/lib/hris/announcements";
import {
  announcementUpsertSchema,
  deleteAnnouncement,
  getAnnouncement,
  updateAnnouncement,
} from "@/lib/hris/announcements-repo";
import { readJson, requireUuid, requireWorkforceActor } from "@/lib/hris/workforce-route";
import { HRIS_LEAF, auditHrisDelete, requireHrisDelete } from "@/lib/hris/delete-guard";

/**
 * GET    /api/hris/announcements/[id] — pengelola (semua) / karyawan (bila tayang & menyasar)
 * PATCH  /api/hris/announcements/[id] — update (pengelola)
 * DELETE /api/hris/announcements/[id] — hapus (pengelola)
 */

interface RouteParams {
  params: Promise<{ id: string }>;
}

export const GET = apiHandler(async (_req: NextRequest, { params }: RouteParams) => {
  const actor = await requireWorkforceActor();
  const id = requireUuid((await params).id);
  const data = await getAnnouncement(id, {
    canManage: canManageAnnouncements(actor.role),
    employeeId: actor.employeeId,
  });
  return NextResponse.json({ data });
}, "hris/announcements/[id] GET");

export const PATCH = apiHandler(async (req: NextRequest, { params }: RouteParams) => {
  await requireIamMenuPrefix(IAM.hrisKepegawaian);
  const id = requireUuid((await params).id);
  const body = await readJson(req, announcementUpsertSchema, "Validasi gagal");
  const data = await updateAnnouncement(id, body);
  return NextResponse.json({ data, message: "Pengumuman diperbarui" });
}, "hris/announcements/[id] PATCH");

export const DELETE = apiHandler(async (request: NextRequest, { params }: RouteParams) => {
  const { user, reason } = await requireHrisDelete(request, HRIS_LEAF.pengumuman);
  const id = requireUuid((await params).id);
  await deleteAnnouncement(id);
  await auditHrisDelete(request, user, "hris.announcements", id, reason);
  return NextResponse.json({ message: "Pengumuman dihapus" });
}, "hris/announcements/[id] DELETE");
