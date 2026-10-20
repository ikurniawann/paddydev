import { NextRequest, NextResponse } from "next/server";
import { apiHandler } from "@/lib/api/handler";
import { deleteLeave, getLeave, leaveUpdateSchema, updateLeave } from "@/lib/hris/leaves-repo";
import { readJson, requireWorkforceActor } from "@/lib/hris/workforce-route";
import { HRIS_LEAF, auditHrisDelete, requireHrisDelete } from "@/lib/hris/delete-guard";

type Ctx = { params: Promise<{ id: string }> };

/** GET /api/hris/leaves/:id: HR, pemilik, atau atasan langsung. */
export const GET = apiHandler(async (_request: NextRequest, { params }: Ctx) => {
  const actor = await requireWorkforceActor();
  const { id } = await params;
  return NextResponse.json({ data: await getLeave(actor, id) });
}, "hris/leaves/[id].GET");

/** PUT /api/hris/leaves/:id: batalkan (pemilik/HR) atau edit alasan/lampiran (HR). */
export const PUT = apiHandler(async (request: NextRequest, { params }: Ctx) => {
  const actor = await requireWorkforceActor();
  const { id } = await params;
  const input = await readJson(request, leaveUpdateSchema);
  return NextResponse.json(await updateLeave(actor, id, input));
}, "hris/leaves/[id].PUT");

/** DELETE /api/hris/leaves/:id: aksi delete di menu Cuti & Izin + alasan. */
export const DELETE = apiHandler(async (request: NextRequest, { params }: Ctx) => {
  const { user, reason } = await requireHrisDelete(request, HRIS_LEAF.cuti);
  const { id } = await params;
  await deleteLeave(id);
  await auditHrisDelete(request, user, "hris.leaves", id, reason);
  return NextResponse.json({ message: "Leave request deleted successfully" });
}, "hris/leaves/[id].DELETE");
