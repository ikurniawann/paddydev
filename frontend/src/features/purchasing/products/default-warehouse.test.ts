import { describe, expect, it } from "vitest";
import { defaultWarehouseId } from "./default-warehouse";

const wh = (id: string, branch: string, isDefault = false) => ({
  id,
  name: isDefault ? "Main Storage" : "Stall",
  code: isDefault ? "MAIN" : "STALL-01",
  is_default: isDefault,
  branch_id: branch,
});

describe("defaultWarehouseId (new product)", () => {
  it("single branch keeps the default: Main Storage, else the first row", () => {
    expect(defaultWarehouseId([wh("s", "a"), wh("m", "a", true)])).toBe("m");
    expect(defaultWarehouseId([wh("s1", "a"), wh("s2", "a")])).toBe("s1");
    expect(defaultWarehouseId([])).toBe("");
  });

  it("a list spanning branches has no default: the user must choose", () => {
    expect(defaultWarehouseId([wh("m-a", "a", true), wh("m-b", "b", true)])).toBe("");
  });

  it("rows without branch_id count as one branch", () => {
    expect(defaultWarehouseId([{ id: "m", name: "Main", code: "MAIN", is_default: true }])).toBe("m");
  });
});
