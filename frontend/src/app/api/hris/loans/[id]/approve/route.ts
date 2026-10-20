import { NextRequest, NextResponse } from "next/server";
import { requireIamAction } from "@/lib/api/auth";
import { apiHandler } from "@/lib/api/handler";
import { readJson } from "@/lib/hris/workforce-route";
import { IAM } from "@/lib/iam/prefixes";
import { createServerPgClient } from "@/lib/pg/create-client";
import { decideLoan, decideLoanSchema } from "@/lib/payroll/loan-requests";

/**
 * POST /api/hris/loans/[id]/approve { approved, rejection_reason? } — aksi
 * "approve" di menu Pinjaman, sama dengan backend Go.
 */
export const POST = apiHandler(
  async (request: NextRequest, { params }: { params: Promise<{ id: string }> }) => {
    const user = await requireIamAction(IAM.hrisLoans, "approve");
    const { id } = await params;
    const input = await readJson(request, decideLoanSchema);
    return NextResponse.json(await decideLoan(await createServerPgClient(), user.id, id, input));
  },
  "hris/loans/[id]/approve.POST"
);
