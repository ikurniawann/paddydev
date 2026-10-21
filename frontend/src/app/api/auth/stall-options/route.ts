import { NextResponse } from "next/server";
import { ApiError, requireApiUser } from "@/lib/api/auth";
import { getApiUserScope } from "@/lib/api/scope";
import { findStallOption, getStallAccess, sortStallsByBranch } from "@/lib/auth/stall-access";
import type { StallOption } from "@/lib/auth/stall-access";
import { formatStallLabel, stallsSpanBranches } from "@/lib/auth/stall-label";
import { resolveActiveStallFromCookies } from "@/lib/auth/active-stall";
import { sortWarehouses } from "@/lib/configuration/sort-warehouses";

/** Bentuk outlet di respons: field lama + branch dan label tampilan. */
function stallView(stall: StallOption, withBranch: boolean) {
  return {
    id: stall.id,
    name: stall.name,
    code: stall.code,
    branch_id: stall.branch_id,
    branch_name: stall.branch_name,
    label: formatStallLabel(stall, withBranch),
  };
}

/**
 * Daftar stall yang boleh dipilih user di switcher / kasir gate.
 * all_access=false → "Semua Stall" tidak tersedia; pilihan terbatas penempatan.
 *
 * Semua user login boleh membaca stall AKTIF-nya (dipakai struk POS untuk
 * mencetak asal stall); daftar pilihan hanya dikirim bila user memang boleh
 * pindah (can_switch) — POST /api/auth/active-stall tetap penjaga izinnya.
 *
 * Tiap branch punya "Main Storage" sendiri, jadi `label` memuat nama branch
 * ("<nama> — <branch>") bila daftar melintasi beberapa branch; selain itu = nama.
 * Urutan daftar: nama branch, lalu Main Storage, lalu urutan stall biasa.
 */
export async function GET() {
  try {
    const user = await requireApiUser();
    const scope = await getApiUserScope();
    const access = await getStallAccess(user.id, user.role, scope?.branchId ?? null);
    const canSwitch =
      user.role === "super_admin" ||
      user.role === "admin" ||
      access.allAccess ||
      access.stalls.length > 1;
    const stalls = sortStallsByBranch(access.stalls);
    const listSpans = stallsSpanBranches(stalls);

    // Stall aktif ikut dikirim supaya tombol switcher di kasir bisa
    // menandai posisi sekarang tanpa endpoint tambahan. Cookie kosong =
    // mengikuti penempatan (stall pertama), "all" = Semua Stall.
    const resolved = await resolveActiveStallFromCookies();
    // Stall dari cookie bisa di luar daftar (akses penuh lintas branch):
    // ambil branch-nya, dan hitung label bersama daftar.
    const resolvedOption =
      resolved.mode === "stall" ? await findStallOption(resolved.stall.id) : null;
    // Fallback cookie unset: user akses penuh = "Semua Stall" (selaras
    // resolvePosSellScope); user biasa mengikuti penempatan pertamanya
    // (Main Storage dulu, bukan urutan branch).
    const placementDefault = sortWarehouses(access.stalls)[0];
    const active = resolvedOption
      ? stallView(resolvedOption, stallsSpanBranches([resolvedOption, ...stalls]))
      : resolved.mode === "all" || access.allAccess
        ? null
        : placementDefault
          ? stallView(placementDefault, listSpans)
          : null;

    return NextResponse.json({
      success: true,
      data: {
        all_access: access.allAccess,
        can_switch: canSwitch,
        active,
        stalls: canSwitch ? stalls.map((stall) => stallView(stall, listSpans)) : [],
      },
    });
  } catch (error) {
    if (error instanceof ApiError) return error.toResponse();
    const message = error instanceof Error ? error.message : "Failed to load outlet options";
    console.error("[api/auth/outlet-options] failed:", error);
    return NextResponse.json({ error: message }, { status: 500 });
  }
}
