import { NextRequest, NextResponse } from "next/server";
import { apiHandler } from "@/lib/api/handler";
import { ApiError } from "@/lib/api/auth";
import { getLeaveBalance, leaveBalanceQuerySchema } from "@/lib/hris/leave-balances-repo";
import { requireWorkforceActor } from "@/lib/hris/workforce-route";
import { parseInput, searchParamsOf } from "@/lib/payroll/request-input";

type Ctx = { params: Promise<{ employee_id: string }> };

const yearOf = (request: NextRequest) =>
  parseInput(leaveBalanceQuerySchema, searchParamsOf(request), "Tahun tidak valid").year ??
  new Date().getFullYear();

// EPIC-068: GET dan PUT saldo cuti dilayani backend Go (dua rute ada di
// go-routes.generated.json, jadi proxy tidak pernah sampai ke sini). Handler
// ini hanya sisa fallback lama: GET tetap, PUT sengaja ditolak karena aturan
// lama (terpakai bisa diubah, default 12) bertentangan dengan R11.

/** GET /api/hris/leave-balances/:employee_id?year: pemilik, HRD, atau manajer. */
export const GET = apiHandler(async (request: NextRequest, { params }: Ctx) => {
  const actor = await requireWorkforceActor();
  const { employee_id } = await params;
  return NextResponse.json({ data: await getLeaveBalance(actor, employee_id, yearOf(request)) });
}, "hris/leave-balances.GET");

/** PUT: dialihkan ke backend Go. */
export const PUT = apiHandler(async (_request: NextRequest, _ctx: Ctx) => {
  await requireWorkforceActor(); // tanpa sesi tetap 401, seperti rute lain
  throw new ApiError(501, "Ditangani backend Go");
}, "hris/leave-balances.PUT");
