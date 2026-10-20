import { NextResponse, type NextRequest } from "next/server";
import { ApiError, requireIamMenuPrefix } from "@/lib/api/auth";
import { apiHandler } from "@/lib/api/handler";
import { IAM } from "@/lib/iam/prefixes";
import { query } from "@/lib/db";
import { deletePrivateFolder } from "@/lib/storage-private";
import { candidateUpdateSchema, isUuid } from "@/lib/recruitment/candidate-query";
import { getCandidate, parseBody, requireCandidate, updateCandidate } from "@/lib/recruitment/candidates-repo";
import { HRIS_LEAF, auditHrisDelete, requireHrisDelete } from "@/lib/hris/delete-guard";

interface RouteParams {
  params: Promise<{ id: string }>;
}

// GET /api/candidates/[id]
export const GET = apiHandler(async (_request: NextRequest, { params }: RouteParams) => {
  await requireIamMenuPrefix(IAM.hrisRecruitment);
  const { id } = await params;
  const data = isUuid(id) ? await getCandidate(id) : null;
  if (!data) throw ApiError.notFound("Kandidat tidak ditemukan");
  return NextResponse.json({ data });
}, "api/candidates/[id]");

// PUT /api/candidates/[id]: ubah profil kandidat (field yang dikirim saja)
export const PUT = apiHandler(async (request: NextRequest, { params }: RouteParams) => {
  await requireIamMenuPrefix(IAM.hrisRecruitment);
  const { id } = await params;
  await requireCandidate(id);
  const patch = await parseBody(request, candidateUpdateSchema);
  const data = await updateCandidate(id, patch);
  return NextResponse.json({ data });
}, "api/candidates/[id]");

// DELETE /api/candidates/[id]
// Destruktif & ireversibel (termasuk purge bukti psikotes di storage), jadi
// penghapus dicatat di log server karena candidate_activities ikut ter-CASCADE.
export const DELETE = apiHandler(async (request: NextRequest, { params }: RouteParams) => {
  const { user, reason } = await requireHrisDelete(request, HRIS_LEAF.kandidat);
  const { id } = await params;
  await requireCandidate(id);

  // retensi psikotes: baris DB terhapus via CASCADE, tapi file di
  // storage/private (gambar tes + snapshot proctoring) harus dibersihkan manual
  const sessions = await query<{ id: string }>(
    "SELECT id FROM recruitment.psikotes_sessions WHERE candidate_id = $1",
    [id]
  );
  await query("DELETE FROM recruitment.candidates WHERE id = $1", [id]);

  if (sessions.length > 0) {
    console.info(
      `[candidates] DELETE ${id} oleh ${user.full_name} (${user.id}): purge ${sessions.length} folder bukti psikotes`
    );
  }
  for (const session of sessions) {
    await deletePrivateFolder(`psikotes/${session.id}`);
  }
  await auditHrisDelete(request, user, "recruitment.candidates", id, reason);
  return NextResponse.json({ message: "Kandidat dihapus" });
}, "api/candidates/[id]");
