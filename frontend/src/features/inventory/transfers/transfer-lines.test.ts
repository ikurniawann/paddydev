import { describe, expect, it } from "vitest";
import {
  mainStorageFor,
  partitionWarehouses,
  stallsInSameBranch,
  toComboboxOptions,
  transferInputError,
  transferProgress,
  type TransferLine,
} from "./transfer-lines";

const line = (
  kode: string,
  available: number,
  input: string,
): TransferLine => ({
  key: kode,
  raw_material_id: kode,
  material_kode: kode,
  material_nama: kode,
  satuan: null,
  qty_available: available,
  qty_transfer_input: input,
});

describe("transferInputError", () => {
  it("wajib asal & tujuan berbeda", () => {
    expect(transferInputError([], "", "b")).toBe("Silakan pilih outlet asal");
    expect(transferInputError([], "a", "")).toBe("Silakan pilih outlet tujuan");
    expect(transferInputError([], "a", "a")).toBe(
      "Outlet asal dan outlet tujuan harus berbeda",
    );
  });

  it("minimal satu qty > 0 dan tidak melebihi stok", () => {
    expect(transferInputError([line("BB-1", 5, "0")], "a", "b")).toBe(
      "Isi qty transfer lebih dari nol untuk minimal satu bahan baku",
    );
    expect(
      transferInputError(
        [line("BB-1", 5, "2"), line("BB-2", 1.5, "3")],
        "a",
        "b",
      ),
    ).toBe("BB-2: qty melebihi stok tersedia (1,5)");
    expect(
      transferInputError(
        [line("BB-1", 5, "2"), line("BB-2", 1, "-1")],
        "a",
        "b",
      ),
    ).toBe(
      "Qty transfer harus berupa angka valid yang lebih besar atau sama dengan nol",
    );
    expect(transferInputError([line("BB-1", 5, "5")], "a", "b")).toBeNull();
  });
});

describe("transferProgress", () => {
  it("hitung terisi dan yang akan ditransfer", () => {
    expect(
      transferProgress([
        line("a", 5, "0"),
        line("b", 5, "1"),
        line("c", 5, ""),
      ]),
    ).toEqual({
      filled: 2,
      toTransfer: 1,
      total: 3,
    });
  });
});

describe("toComboboxOptions", () => {
  it("memakai label (nama + branch) bila ada, selain itu nama", () => {
    const base = { code: "MAIN", is_default: true };
    expect(
      toComboboxOptions([
        { id: "a", name: "Main Storage", label: "Main Storage — Alpha", ...base },
        { id: "b", name: "Stall 1", ...base },
      ]),
    ).toEqual([
      { value: "a", label: "Main Storage — Alpha", description: "MAIN" },
      { value: "b", label: "Stall 1", description: "MAIN" },
    ]);
  });
});

describe("cross-branch transfers", () => {
  const wh = (id: string, code: string, branch: string, isDefault = false) => ({
    id,
    name: isDefault ? "Main Storage" : code,
    code,
    is_default: isDefault,
    branch_id: branch,
    label: `${isDefault ? "Main Storage" : code} — ${branch.toUpperCase()}`,
  });
  const mainA = wh("m-a", "MAIN", "a", true);
  const mainB = wh("m-b", "MAIN", "b", true);
  const stallA = wh("s-a", "STALL-01", "a");
  const stallB = wh("s-b", "STALL-01", "b");
  const crossBranch = [mainA, stallA, mainB, stallB];

  it("locks the Main Storage of the chosen stall's branch", () => {
    expect(mainStorageFor(crossBranch, "b")?.id).toBe("m-b");
    expect(mainStorageFor(crossBranch, "a")?.id).toBe("m-a");
    expect(mainStorageFor([mainB, mainA, stallA, stallB], "b")?.id).toBe("m-b");
  });

  it("locks nothing before a stall is chosen when the list mixes branches", () => {
    expect(mainStorageFor(crossBranch, null)).toBeNull();
    expect(mainStorageFor(crossBranch, "zzz")).toBeNull();
  });

  it("single branch behaves as before: the Main Storage is locked from the start", () => {
    const single = [stallA, mainA];
    expect(mainStorageFor(single, null)?.id).toBe("m-a");
    expect(mainStorageFor(single, "a")?.id).toBe("m-a");
    expect(partitionWarehouses(single).main?.id).toBe("m-a");
    expect(partitionWarehouses(single).stalls.map((w) => w.id)).toEqual(["s-a"]);
    // Rows without branch_id (older payloads) count as one branch.
    const legacy = [{ id: "x", name: "Main Storage", code: "MAIN", is_default: true }];
    expect(mainStorageFor(legacy, null)?.id).toBe("x");
  });

  it("stall to stall only offers stalls of the source's branch", () => {
    expect(stallsInSameBranch([stallA, stallB], "s-a", crossBranch).map((w) => w.id)).toEqual(["s-a"]);
    expect(stallsInSameBranch([stallA, stallB], "", crossBranch).map((w) => w.id)).toEqual(["s-a", "s-b"]);
    expect(stallsInSameBranch([stallA], "s-a", [mainA, stallA]).map((w) => w.id)).toEqual(["s-a"]);
  });
});
