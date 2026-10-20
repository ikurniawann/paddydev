import { NextRequest, NextResponse } from "next/server";
import { ApiError, requireIamMenuPrefix } from "@/lib/api/auth";
import { apiHandler } from "@/lib/api/handler";
import { IAM } from "@/lib/iam/prefixes";
import {
  contractActionSchema,
  deleteDraftContract,
  loadContract,
  runContractAction,
} from "@/lib/hris/contracts-lifecycle";
import { readJson, requireUuid } from "@/lib/hris/workforce-route";
import { HRIS_LEAF, auditHrisDelete, requireHrisDelete } from "@/lib/hris/delete-guard";

/**
 * PATCH  /api/hris/contracts/[id] — aksi siklus hidup kontrak (lihat
 *        lib/hris/contracts-lifecycle). Perpanjangan membalas 201.
 * DELETE /api/hris/contracts/[id] — hapus draft saja.
 */

interface RouteParams {
  params: Promise<{ id: string }>;
}

export const PATCH = apiHandler(async (req: NextRequest, { params }: RouteParams) => {
  const user = await requireIamMenuPrefix(IAM.hrisKepegawaian);
  const id = requireUuid((await params).id, "ID kontrak tidak valid");
  const contract = await loadContract(id);
  if (!contract) throw ApiError.notFound("Kontrak tidak ditemukan");

  const body = await readJson(req, contractActionSchema);
  const result = await runContractAction(contract, body, user);
  return NextResponse.json(result, { status: body.action === "renew" ? 201 : 200 });
}, "hris/contracts PATCH");

export const DELETE = apiHandler(async (request: NextRequest, { params }: RouteParams) => {
  const { user, reason } = await requireHrisDelete(request, HRIS_LEAF.kontrak);
  const id = requireUuid((await params).id, "ID kontrak tidak valid");
  await deleteDraftContract(id);
  await auditHrisDelete(request, user, "hris.employment_contracts", id, reason);
  return NextResponse.json({ message: "Draft kontrak dihapus" });
}, "hris/contracts DELETE");
