import { describe, expect, it } from "vitest";
import { EMPTY_SHIFT_FORM, hhmm, shiftCodeError, shiftFormFrom, shiftPayload } from "./shifts-view";

describe("hhmm", () => {
  it("memotong detik dari kolom time", () => {
    expect(hhmm("08:00:00")).toBe("08:00");
    expect(hhmm("23:45")).toBe("23:45");
  });

  it("memakai fallback saat kosong", () => {
    expect(hhmm(null)).toBe("—");
    expect(hhmm("", "-")).toBe("-");
  });
});

describe("shiftFormFrom", () => {
  it("mengubah baris shift jadi nilai form", () => {
    expect(
      shiftFormFrom({
        code: "MALAM",
        name: "Shift Malam",
        start_time: "22:00:00",
        end_time: "06:00:00",
        break_minutes: 30,
        late_tolerance_minutes: 5,
        is_overnight: true,
      })
    ).toEqual({
      code: "MALAM",
      name: "Shift Malam",
      start_time: "22:00",
      end_time: "06:00",
      break_minutes: "30",
      late_tolerance_minutes: "5",
      is_overnight: true,
    });
  });
});

describe("shiftPayload", () => {
  it("merapikan nama dan mengubah menit jadi angka", () => {
    expect(shiftPayload({ ...EMPTY_SHIFT_FORM, name: "  Pagi ", code: "pagi8" })).toEqual({
      code: "PAGI8",
      name: "Pagi",
      start_time: "08:00",
      end_time: "16:00",
      break_minutes: 60,
      late_tolerance_minutes: 10,
      is_overnight: false,
    });
  });

  it("menit kosong atau tidak valid jadi 0", () => {
    const payload = shiftPayload({ ...EMPTY_SHIFT_FORM, break_minutes: "", late_tolerance_minutes: "abc" });
    expect(payload.break_minutes).toBe(0);
    expect(payload.late_tolerance_minutes).toBe(0);
  });
});

describe("kode shift", () => {
  it("dikirim huruf besar tanpa spasi", () => {
    expect(shiftPayload({ ...EMPTY_SHIFT_FORM, name: "SPG", code: " spg1 " }).code).toBe("SPG1");
  });
  it("menolak spasi dan karakter lain", () => {
    expect(shiftCodeError("SPG 1")).toBe("Kode 1–12 huruf/angka tanpa spasi");
    expect(shiftCodeError("")).toBe("Kode 1–12 huruf/angka tanpa spasi");
    expect(shiftCodeError("spg1")).toBeNull();
  });
});
