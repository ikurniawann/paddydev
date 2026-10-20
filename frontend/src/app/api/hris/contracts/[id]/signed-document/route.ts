import { NextRequest, NextResponse } from "next/server";
import { ApiError, requireIamMenuPrefix } from "@/lib/api/auth";
import { apiHandler } from "@/lib/api/handler";
import { IAM } from "@/lib/iam/prefixes";
import { readPrivateFile } from "@/lib/storage-private";
import {
  loadSignedDocumentContract,
  MAX_SIGNED_DOCUMENT_BYTES,
  removeSignedDocument,
  saveSignedDocument,
} from "@/lib/hris/contracts-repo";
import { requireUuid } from "@/lib/hris/workforce-route";
import { HRIS_LEAF, auditHrisDelete, requireHrisDelete } from "@/lib/hris/delete-guard";

/**
 * Dokumen kontrak bertanda tangan (scan PDF/JPG/PNG/WebP, maks 10 MB),
 * disimpan di storage private, path di kolom signed_document_url.
 *   POST   upload (re-upload menimpa file lama)
 *   GET    sajikan file (inline, ber-auth)
 *   DELETE hapus dokumen + kosongkan kolom
 */

interface RouteParams {
  params: Promise<{ id: string }>;
}

async function guardAndLoad(params: RouteParams["params"]) {
  await requireIamMenuPrefix(IAM.hrisKepegawaian);
  const id = requireUuid((await params).id, "ID kontrak tidak valid");
  return loadSignedDocumentContract(id);
}

export const POST = apiHandler(async (req: NextRequest, { params }: RouteParams) => {
  const contract = await guardAndLoad(params);
  if (!contract) throw ApiError.notFound("Kontrak tidak ditemukan");

  const formData = await req.formData().catch(() => null);
  if (!formData) throw ApiError.badRequest("Form data tidak valid");
  const file = formData.get("file");
  if (!(file instanceof File)) throw ApiError.badRequest("File tidak ditemukan");
  if (file.size > MAX_SIGNED_DOCUMENT_BYTES) {
    throw ApiError.badRequest("Ukuran dokumen maksimal 10 MB");
  }

  await saveSignedDocument(contract, Buffer.from(await file.arrayBuffer()));
  return NextResponse.json({
    message: `Dokumen bertanda tangan kontrak ${contract.contract_number} tersimpan`,
  });
}, "hris/contracts/signed-document POST");

export const GET = apiHandler(async (_req: NextRequest, { params }: RouteParams) => {
  const contract = await guardAndLoad(params);
  if (!contract?.signed_document_url) {
    throw ApiError.notFound("Kontrak ini belum punya dokumen bertanda tangan");
  }

  const { data, mime } = await readPrivateFile(contract.signed_document_url);
  if (!data) throw ApiError.notFound("File tidak ditemukan");
  return new NextResponse(new Uint8Array(data), {
    status: 200,
    headers: {
      "Content-Type": mime ?? "application/octet-stream",
      "Content-Disposition": `inline; filename="ttd-${contract.contract_number.replace(/[^a-zA-Z0-9.-]/g, "_")}"`,
      "Cache-Control": "private, max-age=3600",
    },
  });
}, "hris/contracts/signed-document GET");

export const DELETE = apiHandler(async (request: NextRequest, { params }: RouteParams) => {
  const { user, reason } = await requireHrisDelete(request, HRIS_LEAF.kontrak);
  const contract = await guardAndLoad(params);
  if (!contract) throw ApiError.notFound("Kontrak tidak ditemukan");
  await removeSignedDocument(contract);
  await auditHrisDelete(request, user, "hris.contract_signed_document", (await params).id, reason);
  return NextResponse.json({ message: "Dokumen bertanda tangan dihapus" });
}, "hris/contracts/signed-document DELETE");
