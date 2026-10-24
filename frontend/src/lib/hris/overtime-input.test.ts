import { describe, expect, it } from "vitest";
import { overtimeEndLabel, overtimeNotFinished, paidOvertimeHours } from "./overtime-rules";

const wib = (s: string) => new Date(`${s}:00+07:00`);

describe("input lembur setelah selesai (EPIC-066)", () => {
  it("baru boleh setelah jam selesai, termasuk lewat tengah malam", () => {
    expect(overtimeNotFinished("2026-10-21", "17:00", "19:00", wib("2026-10-21T18:59"))).toBe(true);
    expect(overtimeNotFinished("2026-10-21", "17:00", "19:00", wib("2026-10-21T19:00"))).toBe(false);
    expect(overtimeNotFinished("2026-10-21", "22:00", "02:00", wib("2026-10-22T01:00"))).toBe(true);
    expect(overtimeNotFinished("2026-10-21", "22:00", "02:00", wib("2026-10-22T02:00"))).toBe(false);
    expect(overtimeEndLabel("2026-10-21", "22:00", "02:00")).toBe("22-10-2026 02:00");
  });
  it("jam dibayar = durasi − istirahat, tidak negatif", () => {
    expect(paidOvertimeHours("17:00", "20:00", 30)).toBe(2.5);
    expect(paidOvertimeHours("22:00", "02:00", 60)).toBe(3);
    expect(paidOvertimeHours("17:00", "18:00", 90)).toBe(0);
  });
});
