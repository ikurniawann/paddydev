import { describe, expect, it } from "vitest";
import { scheduleTypeConfirmText, scheduleTypeLabel } from "./schedule-type";

describe("schedule type", () => {
  it("label", () => {
    expect(scheduleTypeLabel("non_shift")).toBe("Non-Shift");
    expect(scheduleTypeLabel(undefined)).toBe("—");
  });
  it("teks konfirmasi ke Non-Shift menyebut jadwal yang dihapus", () => {
    expect(scheduleTypeConfirmText("non_shift", "2026-10-21", 3)).toBe(
      "3 jadwal per tanggal mulai 21/10/2026 akan dihapus dan diganti jadwal tetap Senin–Jumat 09.00–18.00."
    );
    expect(scheduleTypeConfirmText("non_shift", "2026-10-21", 0)).toBe(
      "Jadwal tetap Senin–Jumat 09.00–18.00 berlaku mulai 21/10/2026."
    );
  });
  it("teks konfirmasi ke Shift mengingatkan penugasan", () => {
    expect(scheduleTypeConfirmText("shift", "2026-10-21", 0)).toBe(
      "Jadwal tetap berhenti mulai 21/10/2026. Karyawan perlu ditugaskan di Jadwal Kerja."
    );
  });
});
