import { describe, expect, it } from "vitest";
import { compareWarehouses, sortWarehouses } from "./sort-warehouses";

describe("sortWarehouses", () => {
  it("does not throw when code or name is missing", () => {
    const rows = [
      { is_default: false, code: null as unknown as string, name: null as unknown as string },
      { is_default: true, code: "WH-01", name: "Operasional" },
      { is_default: false, code: "STALL-02", name: "Yakitori" },
    ];
    expect(() => sortWarehouses(rows)).not.toThrow();
    expect(sortWarehouses(rows)[0].code).toBe("WH-01");
  });

  it("MAIN vs MAIN is a consistent tie (0 both ways), so the sort is stable", () => {
    const a = { is_default: true, code: "MAIN", name: "Main Storage", id: "a" };
    const b = { is_default: true, code: "MAIN", name: "Main Storage", id: "b" };
    expect(compareWarehouses(a, b)).toBe(0);
    expect(compareWarehouses(b, a)).toBe(0);
    expect(sortWarehouses([a, b]).map((w) => w.id)).toEqual(["a", "b"]);
    expect(sortWarehouses([b, a]).map((w) => w.id)).toEqual(["b", "a"]);
    // A MAIN-coded non-default row still sorts before regular stalls.
    const kiosk = { is_default: false, code: "KIOSK-1", name: "Kiosk", id: "k" };
    const mainNonDefault = { is_default: false, code: "MAIN", name: "Main", id: "m" };
    expect(sortWarehouses([kiosk, mainNonDefault]).map((w) => w.id)).toEqual(["m", "k"]);
  });
});
