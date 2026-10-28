import { describe, expect, it } from "vitest";
import { shiftGroupName, shiftSummary, suggestShiftName } from "./shift-list";

describe("shiftGroupName", () => {
  it.each([
    ["PAGI8", "PAGI"],
    ["CS2", "CS"],
    ["SPG13", "SPG"],
    ["DEF", "Bawaan"],
    ["NS", "Bawaan"],
    ["OFF", "OFF"],
    ["7A", "Lainnya"],
  ])("%s → %s", (code, group) => expect(shiftGroupName(code)).toBe(group));
});

describe("shiftSummary", () => {
  it("melewati tengah malam untuk batas terlambat", () => {
    expect(shiftSummary("23:55", "07:00", 10, true)).toBe(
      "Masuk 23:55 · terlambat setelah 00:05 · pulang 07:00 (keesokan hari)"
    );
  });
  it("kosong bila jam belum lengkap", () => {
    expect(shiftSummary("", "16:00", 10, false)).toBeNull();
  });
});

describe("suggestShiftName", () => {
  it("memakai tanda hubung seperti data yang sudah ada", () => {
    expect(suggestShiftName("pagi8", "07:00", "15:00")).toBe("PAGI8 07:00-15:00");
  });
  it("kosong bila kode belum diisi", () => {
    expect(suggestShiftName("  ", "08:00", "16:00")).toBe("");
  });
});
