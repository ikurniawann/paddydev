import { NextResponse } from "next/server";
import { ApiError, requireIamMenuPrefix } from "@/lib/api/auth";
import { apiHandler } from "@/lib/api/handler";
import { IAM } from "@/lib/iam/prefixes";
import { query } from "@/lib/db";
import { uploadFile, validateFile } from "@/lib/storage";
import { readCvFile } from "@/lib/recruitment/candidate-cv";
import { requireCandidate } from "@/lib/recruitment/candidates-repo";
import { HRIS_LEAF, auditHrisDelete, requireHrisDelete } from "@/lib/hris/delete-guard";

interface RouteParams {
  params: Promise<{ id: string }>;
}

const setCvUrl = (id: string, url: string | null) =>
  query("UPDATE recruitment.candidates SET cv_url = $1, updated_at = now() WHERE id = $2", [url, id]);

// POST /api/candidates/[id]/cv-upload
export const POST = apiHandler(async (request: Request, { params }: RouteParams) => {
  await requireIamMenuPrefix(IAM.hrisRecruitment);
  const { id } = await params;
  await requireCandidate(id);

  const file = await readCvFile(request);
  const validation = validateFile(file);
  if (!validation.valid) throw ApiError.badRequest(validation.error ?? "File tidak valid");

  const { url, error: uploadError } = await uploadFile("candidates", file, "cvs");
  if (uploadError) {
    console.error("[cv-upload] upload gagal:", uploadError);
    throw ApiError.server("Upload gagal");
  }

  await setCvUrl(id, url);
  return NextResponse.json({ data: { cv_url: url }, message: "CV berhasil diupload" });
}, "cv-upload");

// DELETE /api/candidates/[id]/cv-upload
export const DELETE = apiHandler(async (request: Request, { params }: RouteParams) => {
  const { user, reason } = await requireHrisDelete(request, HRIS_LEAF.kandidat);
  const { id } = await params;
  await requireCandidate(id);
  await setCvUrl(id, null);
  await auditHrisDelete(request, user, "recruitment.candidate_cv", id, reason);
  return NextResponse.json({ message: "CV berhasil dihapus" });
}, "cv-upload");
