import { NextRequest, NextResponse } from "next/server";
import { ApiError } from "@/lib/api/auth";
import { apiHandler } from "@/lib/api/handler";
import {
  createApiToken,
  isMissingTable,
  listApiTokens,
  requireHumanTokenAdmin,
  type ApiTokenInput,
} from "@/lib/admin/api-tokens";

/** EPIC-042: kelola Open API token (list + create), hanya sesi manusia. */

export const GET = apiHandler(async () => {
  await requireHumanTokenAdmin();
  try {
    return NextResponse.json({ success: true, data: await listApiTokens() });
  } catch (error) {
    if (isMissingTable(error)) return NextResponse.json({ success: true, data: [], migration_pending: true });
    throw error;
  }
}, "GET /api/admin/api-tokens");

export const POST = apiHandler(async (request: NextRequest) => {
  const admin = await requireHumanTokenAdmin();
  const body = ((await request.json()) ?? {}) as ApiTokenInput;
  const data = await createApiToken(admin, body).catch((error: unknown) => {
    if (isMissingTable(error)) {
      throw new ApiError(
        503,
        "Tabel api_tokens belum ada. Jalankan migrasi database dulu: pnpm db:migrate:apply"
      );
    }
    throw error;
  });
  // Nilai token hanya dikirim SEKALI di respons ini — DB cuma menyimpan hash.
  return NextResponse.json({ success: true, data }, { status: 201 });
}, "POST /api/admin/api-tokens");
