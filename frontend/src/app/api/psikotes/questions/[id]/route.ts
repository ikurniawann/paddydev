import { NextResponse, type NextRequest } from "next/server";
import { requireIamMenuPrefix } from "@/lib/api/auth";
import { apiHandler } from "@/lib/api/handler";
import { IAM } from "@/lib/iam/prefixes";
import { deleteQuestion, updateQuestion } from "@/lib/recruitment/psikotes-admin";
import { assertUuid, enforceRateLimit } from "@/lib/recruitment/route-helpers";
import { HRIS_LEAF, auditHrisDelete, requireHrisDelete } from "@/lib/hris/delete-guard";

/**
 * PUT    /api/psikotes/questions/[id]: update soal (validasi per kind instrumen).
 * DELETE /api/psikotes/questions/[id]: hard delete. Untuk sekadar
 *        mengecualikan dari undian, pakai `is_active=false` via PUT.
 */

type Ctx = { params: Promise<{ id: string }> };

export const PUT = apiHandler(async (req: NextRequest, { params }: Ctx) => {
  const user = await requireIamMenuPrefix(IAM.hrisRecruitment);
  const { id } = await params;
  assertUuid(id, "ID soal tidak valid");
  enforceRateLimit(`psikotes_question_put_${user.id}`);
  const updated = await updateQuestion(id, req);
  return NextResponse.json({ data: updated, message: "Soal tersimpan" });
}, "psikotes-question");

export const DELETE = apiHandler(async (request: NextRequest, { params }: Ctx) => {
  const { user, reason } = await requireHrisDelete(request, HRIS_LEAF.psikotes);
  const { id } = await params;
  assertUuid(id, "ID soal tidak valid");
  enforceRateLimit(`psikotes_question_delete_${user.id}`);
  const deleted = await deleteQuestion(id);
  await auditHrisDelete(request, user, "recruitment.psikotes_questions", id, reason);
  return NextResponse.json({ data: deleted, message: "Soal dihapus" });
}, "psikotes-question");
