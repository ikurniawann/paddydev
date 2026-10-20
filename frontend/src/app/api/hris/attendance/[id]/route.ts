import { NextRequest, NextResponse } from "next/server";
import { ApiError } from "@/lib/api/auth";
import { apiHandler } from "@/lib/api/handler";
import {
  attendanceUpdateSchema,
  deleteAttendance,
  getAttendance,
  updateAttendance,
} from "@/lib/hris/attendance-repo";
import { readJson, requireWorkforceActor } from "@/lib/hris/workforce-route";
import { HRIS_LEAF, auditHrisDelete, requireHrisDelete } from "@/lib/hris/delete-guard";

/**
 * GET    /api/hris/attendance/:id — detail (HR, atau pemilik record)
 * PUT    /api/hris/attendance/:id — koreksi/validasi (HR)
 * DELETE /api/hris/attendance/:id — hapus (aksi delete di menu Absensi + alasan)
 */

interface RouteParams {
  params: Promise<{ id: string }>;
}

export const GET = apiHandler(async (_request: NextRequest, { params }: RouteParams) => {
  const actor = await requireWorkforceActor();
  const { id } = await params;
  return NextResponse.json({ data: await getAttendance(id, actor) });
}, "hris/attendance/[id] GET");

export const PUT = apiHandler(async (request: NextRequest, { params }: RouteParams) => {
  const actor = await requireWorkforceActor();
  if (!actor.isHr) {
    throw ApiError.forbidden("Forbidden: Only HRD or managers can update attendance");
  }
  const { id } = await params;
  const body = await readJson(request, attendanceUpdateSchema, "Validation failed");
  const data = await updateAttendance(id, body, actor);
  return NextResponse.json({ message: "Attendance updated successfully", data });
}, "hris/attendance/[id] PUT");

export const DELETE = apiHandler(async (request: NextRequest, { params }: RouteParams) => {
  const { user, reason } = await requireHrisDelete(request, HRIS_LEAF.absensi);
  const { id } = await params;
  await deleteAttendance(id);
  await auditHrisDelete(request, user, "hris.attendance", id, reason);
  return NextResponse.json({ message: "Attendance deleted successfully" });
}, "hris/attendance/[id] DELETE");
