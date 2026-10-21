import { describe, expect, it } from "vitest";
import { isSelfManager, managerOptions, NO_MANAGER_REASON_LABELS, SELF_MANAGER_ERROR } from "./reporting-to";

const ID = "3f2b8c1e-4a5d-4e6f-8a9b-0c1d2e3f4a5b";

describe("isSelfManager (EPIC-063 R9)", () => {
  it("menolak atasan = diri sendiri dalam bentuk uuid apa pun", () => {
    expect(isSelfManager(ID, ID)).toBe(true);
    expect(isSelfManager(ID, ID.toUpperCase())).toBe(true);
    expect(isSelfManager(ID, ID.replaceAll("-", ""))).toBe(true);
    expect(isSelfManager(ID, `{${ID}}`)).toBe(true);
  });

  it("atasan lain, kosong, atau bukan uuid lolos", () => {
    expect(isSelfManager(ID, "9f2b8c1e-4a5d-4e6f-8a9b-0c1d2e3f4a5b")).toBe(false);
    expect(isSelfManager(ID, null)).toBe(false);
    expect(isSelfManager(ID, undefined)).toBe(false);
    expect(isSelfManager(ID, "")).toBe(false);
    expect(isSelfManager("emp-1", "emp-1")).toBe(false);
  });

  it("pesan sama dengan Go errSelfManager", () => {
    expect(SELF_MANAGER_ERROR).toBe("Atasan langsung tidak boleh karyawan itu sendiri");
  });
});

describe("managerOptions (EPIC-063 R9)", () => {
  const managers = [
    { id: ID, full_name: "Diri Sendiri", nip: "N1" },
    { id: "9f2b8c1e-4a5d-4e6f-8a9b-0c1d2e3f4a5b", full_name: "Atasan", nip: "N2" },
  ];

  it("karyawan yang diedit tidak muncul sebagai pilihan atasannya sendiri", () => {
    expect(managerOptions(managers, ID.toUpperCase()).map((o) => o.label)).toEqual(["Atasan"]);
  });

  it("karyawan baru melihat semua pilihan", () => {
    expect(managerOptions(managers, null)).toEqual([
      { value: ID, label: "Diri Sendiri", description: "N1" },
      { value: "9f2b8c1e-4a5d-4e6f-8a9b-0c1d2e3f4a5b", label: "Atasan", description: "N2" },
    ]);
  });
});

describe("NO_MANAGER_REASON_LABELS", () => {
  it("memberi label untuk setiap alasan", () => {
    expect(NO_MANAGER_REASON_LABELS).toEqual({
      empty: "Atasan belum diisi",
      self: "Atasan = diri sendiri",
      inactive: "Atasan nonaktif",
    });
  });
});
