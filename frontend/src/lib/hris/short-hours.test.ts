import { describe, expect, it } from "vitest";
import {
  MIN_WORK_HOURS_ERROR,
  SHORT_HOURS_LABEL,
  formatHours,
  shortHoursDetail,
  toMinWorkHoursPayload,
  validateMinWorkHours,
} from "./short-hours";

describe("kurang jam (AB-05)", () => {
  it("memformat jam numeric pg", () => {
    expect(formatHours("9.00")).toBe("9");
    expect(formatHours("8.50")).toBe("8,5");
    expect(formatHours(9)).toBe("9");
    expect(formatHours(null)).toBe("");
    expect(formatHours(undefined)).toBe("");
  });

  it("keterangan label", () => {
    expect(SHORT_HOURS_LABEL).toBe("Kurang jam / pulang lebih cepat");
    expect(shortHoursDetail("9.00")).toBe("Hadir kurang dari 9 jam (jam minimum toko)");
    expect(shortHoursDetail(null)).toBe(SHORT_HOURS_LABEL);
  });

  it.each([
    ["", ""],
    ["9", ""],
    ["8,5", ""],
    ["8.25", ""],
    ["0.5", MIN_WORK_HOURS_ERROR],
    ["25", MIN_WORK_HOURS_ERROR],
    ["9.1", MIN_WORK_HOURS_ERROR],
    ["abc", MIN_WORK_HOURS_ERROR],
  ])("validasi %s → %s", (input, msg) => {
    expect(validateMinWorkHours(input)).toBe(msg);
  });

  it("payload: kosong = tanpa aturan", () => {
    expect(toMinWorkHoursPayload(" ")).toEqual({ min_work_hours: null });
    expect(toMinWorkHoursPayload("8,5")).toEqual({ min_work_hours: 8.5 });
  });
});
