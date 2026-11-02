import { describe, expect, it } from "vitest";
import {
  defaultImportSelection,
  emptyHolidayForm,
  groupHolidaysByMonth,
  holidayDayLabel,
  holidayFormFrom,
  holidayPayload,
  toggleInSet,
  withHolidayType,
} from "./holidays-view";

describe("form hari libur", () => {
  it("form kosong mulai 1 Januari tahun terpilih, aktif, libur nasional", () => {
    expect(emptyHolidayForm(2027)).toEqual({
      holiday_date: "2027-01-01",
      name: "",
      type: "nasional",
      deducts_leave: false,
      is_day_off: false,
      status: "aktif",
      note: "",
    });
  });

  it("catatan null jadi string kosong saat edit", () => {
    const form = holidayFormFrom({
      holiday_date: "2026-08-17",
      name: "HUT RI",
      type: "nasional",
      deducts_leave: false,
      status: "aktif",
      note: null,
    });
    expect(form.note).toBe("");
  });

  it("cuti bersama otomatis memotong cuti, libur nasional tidak", () => {
    const base = emptyHolidayForm(2026);
    expect(withHolidayType(base, "cuti_bersama").deducts_leave).toBe(true);
    expect(withHolidayType({ ...base, deducts_leave: true }, "nasional").deducts_leave).toBe(false);
  });

  it("payload merapikan nama dan catatan kosong jadi null", () => {
    const payload = holidayPayload({ ...emptyHolidayForm(2026), name: "  Nyepi ", note: "   " });
    expect(payload.name).toBe("Nyepi");
    expect(payload.note).toBeNull();
  });
});

describe("groupHolidaysByMonth", () => {
  it("mengelompokkan per bulan dengan 12 slot", () => {
    const groups = groupHolidaysByMonth([
      { holiday_date: "2026-01-01" },
      { holiday_date: "2026-08-17" },
      { holiday_date: "2026-01-29" },
    ]);
    expect(groups).toHaveLength(12);
    expect(groups[0].map((r) => r.holiday_date)).toEqual(["2026-01-01", "2026-01-29"]);
    expect(groups[7]).toHaveLength(1);
    expect(groups[5]).toHaveLength(0);
  });
});

describe("holidayDayLabel", () => {
  it("tanggal dua digit dan nama hari singkat", () => {
    expect(holidayDayLabel("2026-08-17")).toEqual({ day: "17", weekday: "Sen" });
    expect(holidayDayLabel("2026-01-01").day).toBe("01");
  });
});

describe("pilihan impor", () => {
  it("hanya yang disarankan dan belum diimpor yang tercentang", () => {
    const selection = defaultImportSelection([
      { source_ref: "a", suggested: true, already_imported: false },
      { source_ref: "b", suggested: false, already_imported: false },
      { source_ref: "c", suggested: true, already_imported: true },
    ]);
    expect([...selection]).toEqual(["a"]);
  });

  it("toggleInSet menambah dan menghapus tanpa mengubah set asal", () => {
    const start = new Set(["a"]);
    expect([...toggleInSet(start, "b")]).toEqual(["a", "b"]);
    expect([...toggleInSet(start, "a")]).toEqual([]);
    expect([...start]).toEqual(["a"]);
  });
});
