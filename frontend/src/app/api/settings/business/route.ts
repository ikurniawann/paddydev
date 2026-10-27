import { NextRequest, NextResponse } from "next/server";
import { z } from "zod";
import { requireIamMenuPrefix, validateBody } from "@/lib/api/auth";
import { apiHandler } from "@/lib/api/handler";
import { IAM } from "@/lib/iam/prefixes";
import { createBusinessEntity, fetchBusinessTree } from "@/lib/configuration/business-repository";
import { BUSINESS_ENTITY_TYPES } from "@/lib/settings/business-entity";
import { rethrowUserFacing } from "@/lib/settings/route-errors";

const createSchema = z.object({
  type: z.enum(BUSINESS_ENTITY_TYPES, { message: "Tipe entitas tidak valid" }),
  name: z.string().default(""),
  code: z.string().optional(),
  parentId: z.string().nullable().optional(),
  is_active: z.boolean().optional(),
});

// Manajemen User ikut membaca pohon ini untuk memilih scope akun di Buat Akun Login.
export const GET = apiHandler(async () => {
  await requireIamMenuPrefix([...IAM.settingsBusiness, ...IAM.settingsUsers]);
  return NextResponse.json({ data: await fetchBusinessTree() });
}, "GET /api/settings/business");

export const POST = apiHandler(async (request: NextRequest) => {
  await requireIamMenuPrefix(IAM.settingsBusiness);
  const { type, parentId, ...fields } = await validateBody(request, createSchema);
  const result = await createBusinessEntity(type, { ...fields, parentId: parentId ?? undefined }).catch(
    rethrowUserFacing(/wajib|valid/)
  );
  return NextResponse.json({ data: result, tree: await fetchBusinessTree() }, { status: 201 });
}, "POST /api/settings/business");
