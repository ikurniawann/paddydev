import { NextRequest, NextResponse } from "next/server";
import { requireIamMenuPrefix } from "@/lib/api/auth";
import { apiHandler } from "@/lib/api/handler";
import { IAM } from "@/lib/iam/prefixes";
import { employeeDocumentPatchSchema } from "@/lib/hris/employee-document-schema";
import { deleteEmployeeDocument, updateEmployeeDocument } from "@/lib/hris/employees-records";
import { readJson } from "@/lib/hris/workforce-route";
import { HRIS_LEAF, auditHrisDelete, requireHrisDelete } from "@/lib/hris/delete-guard";

/**
 * DELETE /api/hris/employees/documents/[doc_id] — hapus record dokumen.
 * PATCH  /api/hris/employees/documents/[doc_id] — ubah metadata.
 *
 * Keamanan (audit 2026-09-17): wajib sesi + menu kepegawaian; PATCH memakai
 * allowlist Zod sehingga kolom verifikasi (is_verified, verified_by),
 * employee_id, dan uploader tidak bisa dipaksa lewat body.
 */

interface RouteParams {
  params: Promise<{ doc_id: string }>;
}

export const DELETE = apiHandler(async (request: NextRequest, { params }: RouteParams) => {
  const { user, reason } = await requireHrisDelete(request, HRIS_LEAF.karyawan);
  const { doc_id } = await params;
  await deleteEmployeeDocument(doc_id);
  await auditHrisDelete(request, user, "hris.employee_documents", doc_id, reason);
  return NextResponse.json({ message: "Dokumen berhasil dihapus" });
}, "hris/employees/documents DELETE");

export const PATCH = apiHandler(async (request: NextRequest, { params }: RouteParams) => {
  await requireIamMenuPrefix(IAM.hrisKepegawaian);
  const { doc_id } = await params;
  const patch = await readJson(request, employeeDocumentPatchSchema, "Data tidak valid");
  const data = await updateEmployeeDocument(doc_id, patch);
  return NextResponse.json({ data, message: "Dokumen berhasil diupdate" });
}, "hris/employees/documents PATCH");
