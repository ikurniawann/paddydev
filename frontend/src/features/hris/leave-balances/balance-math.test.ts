import { describe, expect, it } from "vitest";
import { formatDays, isHalfStep, parseDays, previewBalance, previousYearEditableUntil } from "./balance-math";

describe("parseDays", () => {
  it("menerima titik dan koma desimal, spasi, dan negatif", () => {
    expect(parseDays("9,5")).toBe(9.5);
    expect(parseDays(" 9.5 ")).toBe(9.5);
    expect(parseDays("-1,5")).toBe(-1.5);
    expect(parseDays("0")).toBe(0);
  });
  it("kosong atau bukan angka = null", () => {
    expect(parseDays("")).toBeNull();
    expect(parseDays("   ")).toBeNull();
    expect(parseDays("abc")).toBeNull();
    expect(parseDays("1,2,3")).toBeNull();
  });
});

describe("isHalfStep", () => {
  it("kelipatan 0,5 saja", () => {
    for (const n of [0, 0.5, 1, 12, -1.5, 9.5]) expect(isHalfStep(n)).toBe(true);
    for (const n of [0.25, 9.3, -0.1, 1.75]) expect(isHalfStep(n)).toBe(false);
  });
});

describe("formatDays", () => {
  it("memakai koma desimal, tanpa nol di belakang", () => {
    expect(formatDays(9.5)).toBe("9,5");
    expect(formatDays(12)).toBe("12");
    expect(formatDays(-1.5)).toBe("-1,5");
  });
});

describe("previewBalance (R4)", () => {
  const base = { total: 12, adjustment: 0, used: 3, carryDays: 2, carryUsed: 0.5, expiresOn: "2027-03-31", today: "2026-10-26" };

  it("sisa jatah = jatah + penyesuaian - terpakai; bawaan berlaku sampai tanggal kedaluwarsa", () => {
    expect(previewBalance(base)).toEqual({ remaining: 9, carryRemaining: 1.5, carryActive: true, available: 10.5 });
  });
  it("penyesuaian negatif mengurangi sisa jatah", () => {
    expect(previewBalance({ ...base, adjustment: -2 }).remaining).toBe(7);
  });
  it("bawaan kedaluwarsa tidak dihitung; hari kedaluwarsa itu sendiri masih berlaku", () => {
    expect(previewBalance({ ...base, expiresOn: "2026-10-25" })).toMatchObject({ carryActive: false, available: 9 });
    expect(previewBalance({ ...base, expiresOn: "2026-10-26" })).toMatchObject({ carryActive: true, available: 10.5 });
  });
  it("bawaan di bawah yang terpakai (dihitung ulang) dihitung 0, tidak negatif", () => {
    expect(previewBalance({ ...base, carryDays: 1, carryUsed: 3 })).toEqual({
      remaining: 9,
      carryRemaining: 0,
      carryActive: true,
      available: 9,
    });
  });
  it("tanpa tanggal kedaluwarsa bawaan tidak berlaku", () => {
    expect(previewBalance({ ...base, expiresOn: "" })).toMatchObject({ carryActive: false, available: 9 });
    expect(previewBalance({ ...base, expiresOn: null })).toMatchObject({ carryActive: false, available: 9 });
  });
});

describe("previousYearEditableUntil", () => {
  it("tanggal kedaluwarsa sisa bawaan di tahun berjalan, dua digit", () => {
    expect(previousYearEditableUntil({ carry_over_expiry_month: 3, carry_over_expiry_day: 31 }, 2027)).toBe("2027-03-31");
    expect(previousYearEditableUntil({ carry_over_expiry_month: 6, carry_over_expiry_day: 5 }, 2028)).toBe("2028-06-05");
  });
});
