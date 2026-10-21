/** Sort warehouses: default (Main Storage) first, then Stall 1, 2, … numerically. */
export function compareWarehouses(
  a: { is_default: boolean; code?: string | null; name?: string | null },
  b: { is_default: boolean; code?: string | null; name?: string | null }
): number {
  if (a.is_default !== b.is_default) {
    return a.is_default ? -1 : 1;
  }

  const aCode = String(a.code || "");
  const bCode = String(b.code || "");
  const aName = String(a.name || "");
  const bName = String(b.name || "");

  const stallNumber = (code: string) => {
    const match = code.trim().toUpperCase().match(/^STALL-0*(\d+)$/);
    return match ? Number.parseInt(match[1], 10) : null;
  };

  const aStall = stallNumber(aCode);
  const bStall = stallNumber(bCode);

  if (aStall !== null && bStall !== null) {
    return aStall - bStall;
  }
  if (aStall !== null) return 1;
  if (bStall !== null) return -1;

  // MAIN vs MAIN is a tie (0 both ways) so the comparator stays consistent;
  // lists spanning branches hold one MAIN per branch.
  const aMain = aCode.toUpperCase() === "MAIN";
  const bMain = bCode.toUpperCase() === "MAIN";
  if (aMain !== bMain) return aMain ? -1 : 1;

  return aCode.localeCompare(bCode, undefined, { numeric: true, sensitivity: "base" })
    || aName.localeCompare(bName, undefined, { numeric: true, sensitivity: "base" });
}

export function sortWarehouses<T extends { is_default: boolean; code: string; name: string }>(
  warehouses: T[]
): T[] {
  return [...warehouses].sort(compareWarehouses);
}
