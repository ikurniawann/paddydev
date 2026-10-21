import { describe, expect, it } from "vitest";
import { formatStallLabel, stallDisplayName, stallLabel, stallsSpanBranches } from "./stall-label";

describe("stallLabel", () => {
  it("memakai label dari server, jatuh ke nama bila kosong atau tidak ada", () => {
    expect(stallLabel({ name: "Main Storage", label: "Main Storage — Alpha" })).toBe("Main Storage — Alpha");
    expect(stallLabel({ name: "Main Storage", label: "" })).toBe("Main Storage");
    expect(stallLabel({ name: "Main Storage", label: null })).toBe("Main Storage");
    expect(stallLabel({ name: "Main Storage" })).toBe("Main Storage");
  });
});

describe("formatStallLabel / stallsSpanBranches", () => {
  const a = { name: "Main Storage", branch_id: "a", branch_name: "Alpha" };
  const b = { name: "Main Storage", branch_id: "b", branch_name: "Beta" };
  it("menambah nama branch hanya bila diminta dan nama branch diketahui", () => {
    expect(formatStallLabel(a, false)).toBe("Main Storage");
    expect(formatStallLabel(a, true)).toBe("Main Storage — Alpha");
    expect(formatStallLabel({ ...a, branch_name: " " }, true)).toBe("Main Storage");
  });
  it("mendeteksi daftar lintas branch", () => {
    expect(stallsSpanBranches([])).toBe(false);
    expect(stallsSpanBranches([a, { ...a, name: "Stall 1" }])).toBe(false);
    expect(stallsSpanBranches([a, b])).toBe(true);
  });
});

describe("stallDisplayName", () => {
  const a = { id: "m-a", name: "Main Storage", branch_id: "a", branch_name: "Alpha" };
  const b = { id: "m-b", name: "Main Storage", branch_id: "b", branch_name: "Beta" };
  it("memuat branch bila daftar akses melintasi branch", () => {
    expect(stallDisplayName([a, b], { id: "m-b", name: "Main Storage" })).toBe("Main Storage — Beta");
  });
  it("nama polos untuk satu branch atau outlet di luar daftar", () => {
    expect(stallDisplayName([a], { id: "m-a", name: "Main Storage" })).toBe("Main Storage");
    expect(stallDisplayName([a, b], { id: "zzz", name: "Lain" })).toBe("Lain");
  });
});
