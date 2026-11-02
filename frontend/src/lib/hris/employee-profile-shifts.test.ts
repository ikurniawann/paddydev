import { describe, expect, it } from "vitest";
import {
  SHIFT_OFF,
  currentShiftPattern,
  currentShift2Pattern,
  shiftPatternPayload,
  summarizeShiftHistory,
} from "./employee-profile-shifts";

const row = (day: number, shift: string | null, from: string, to: string | null = null) => ({
  day_of_week: day,
  shift_id: shift,
  effective_from: from,
  effective_to: to,
  shift_name: shift ? `Shift ${shift}` : null,
});

describe("currentShiftPattern", () => {
  it("memilih baris efektif terbaru per hari; hari tanpa baris = libur", () => {
    const rows = [
      row(1, "A", "2026-01-01", "2026-05-31"),
      row(1, "B", "2026-06-01"),
      row(2, "A", "2026-11-01"),
    ];
    const pattern = currentShiftPattern(rows, "2026-10-04");
    expect(pattern[1]).toBe("B");
    expect(pattern[2]).toBe(SHIFT_OFF);
    expect(Object.keys(pattern)).toHaveLength(7);
  });
});

describe("shiftPatternPayload", () => {
  it("libur jadi null", () => {
    const payload = shiftPatternPayload({
      1: "A",
      2: SHIFT_OFF,
      3: SHIFT_OFF,
      4: SHIFT_OFF,
      5: SHIFT_OFF,
      6: SHIFT_OFF,
      7: SHIFT_OFF,
    });
    expect(payload[0]).toEqual({ day_of_week: 1, shift_id: "A" });
    expect(payload[1]).toEqual({ day_of_week: 2, shift_id: null });
  });
});

describe("summarizeShiftHistory", () => {
  it("mengelompokkan per tanggal mulai", () => {
    const rows = [
      row(1, "A", "2026-06-01"),
      row(2, null, "2026-06-01"),
      row(1, null, "2026-01-01", "2026-05-31"),
    ];
    expect(summarizeShiftHistory(rows)).toEqual([
      { from: "2026-06-01", to: null, summary: "Sen Shift A" },
      { from: "2026-01-01", to: "2026-05-31", summary: "libur semua" },
    ]);
  });
});

describe("shift kedua (EPIC-080)", () => {
  const two = (day: number, first: string, second: string | null, from: string) => ({
    ...row(day, first, from),
    shift2_id: second,
    shift2_name: second ? `Shift ${second}` : null,
  });

  it("currentShift2Pattern membaca shift kedua pola berjalan", () => {
    const pattern2 = currentShift2Pattern([two(1, "A", "B", "2026-06-01"), two(2, "A", null, "2026-06-01")], "2026-10-04");
    expect(pattern2[1]).toBe("B");
    expect(pattern2[2]).toBe(SHIFT_OFF);
    expect(pattern2[3]).toBe(SHIFT_OFF);
  });

  it("payload mengirim shift2_id hanya untuk hari yang punya shift kedua", () => {
    const off = { 2: SHIFT_OFF, 3: SHIFT_OFF, 4: SHIFT_OFF, 5: SHIFT_OFF, 6: SHIFT_OFF, 7: SHIFT_OFF };
    const payload = shiftPatternPayload({ 1: "A", ...off }, { ...off, 1: "B", 2: "C" });
    expect(payload[0]).toEqual({ day_of_week: 1, shift_id: "A", shift2_id: "B" });
    // hari libur tidak boleh membawa shift kedua
    expect(payload[1]).toEqual({ day_of_week: 2, shift_id: null });
    expect(payload[1]).not.toHaveProperty("shift2_id");
  });

  it("ringkasan riwayat menampilkan A + B", () => {
    expect(summarizeShiftHistory([two(1, "A", "B", "2026-06-01")])).toEqual([
      { from: "2026-06-01", to: null, summary: "Sen Shift A + Shift B" },
    ]);
  });
});
