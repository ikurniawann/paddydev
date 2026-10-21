/**
 * Label outlet di switcher. Setiap branch punya "Main Storage" sendiri, jadi
 * nama saja tidak cukup begitu daftar melintasi beberapa branch. Server
 * (GET /api/auth/stall-options) mengisi `label`; UI cukup memakai `stallLabel`.
 * Modul ini murni (tanpa import) agar aman dipakai di client dan server.
 */

export interface StallLabelSource {
  name: string;
  /** "<nama> — <branch>" bila daftar melintasi branch, selain itu = nama. */
  label?: string | null;
}

/** Teks yang ditampilkan untuk sebuah outlet: label dari server, jatuh ke nama. */
export function stallLabel(stall: StallLabelSource): string {
  return stall.label?.trim() || stall.name;
}

export interface StallBranchSource {
  name: string;
  branch_id: string;
  branch_name: string;
}

/** true bila outlet pada daftar berasal dari lebih dari satu branch. */
export function stallsSpanBranches(stalls: ReadonlyArray<Pick<StallBranchSource, "branch_id">>): boolean {
  return stalls.some((stall) => stall.branch_id !== stalls[0].branch_id);
}

/** "<nama> — <branch>" bila withBranch dan nama branch diketahui, selain itu nama. */
export function formatStallLabel(stall: StallBranchSource, withBranch: boolean): string {
  const branch = stall.branch_name.trim();
  return withBranch && branch ? `${stall.name} — ${branch}` : stall.name;
}

/**
 * Nama tampilan outlet aktif di sidebar: label yang sama dengan baris
 * switcher-nya (nama + branch bila daftar akses melintasi branch). Outlet di
 * luar daftar jatuh ke nama polos.
 */
export function stallDisplayName(
  accessible: ReadonlyArray<StallBranchSource & { id: string }>,
  stall: { id: string; name: string }
): string {
  const match = accessible.find((row) => row.id === stall.id);
  return match ? formatStallLabel(match, stallsSpanBranches(accessible)) : stall.name;
}

const collator = new Intl.Collator(undefined, { numeric: true, sensitivity: "base" });

/** Urutan cabang: nama branch (natural, tak peduli huruf besar), lalu id branch. */
export function compareStallBranch(
  a: Pick<StallBranchSource, "branch_id" | "branch_name">,
  b: Pick<StallBranchSource, "branch_id" | "branch_name">
): number {
  return (
    collator.compare(a.branch_name, b.branch_name) ||
    (a.branch_id < b.branch_id ? -1 : a.branch_id > b.branch_id ? 1 : 0)
  );
}

/**
 * Untuk daftar outlet dari endpoint mana pun: isi `branch_name` dari peta
 * id→nama branch, urutkan per branch (Main Storage / default dulu, selebihnya
 * urutan asal), dan beri `label` dengan aturan yang sama dengan
 * /api/auth/stall-options. Field lama dipertahankan.
 */
export function withBranchLabels<
  T extends { name: string; branch_id: string; is_default?: boolean | null },
>(
  rows: ReadonlyArray<T>,
  branchNames: ReadonlyMap<string, string>
): Array<T & { branch_name: string; label: string }> {
  const named = rows.map((row) => ({
    ...row,
    branch_name: branchNames.get(row.branch_id) ?? "",
  }));
  named.sort(
    (a, b) =>
      compareStallBranch(a, b) || Number(b.is_default === true) - Number(a.is_default === true)
  );
  const withBranch = stallsSpanBranches(named);
  return named.map((row) => ({ ...row, label: formatStallLabel(row, withBranch) }));
}
