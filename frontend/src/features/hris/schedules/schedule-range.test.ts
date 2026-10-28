import { expect, it } from "vitest";
import { addDays, dayDiff, dm, mondayOf, rangeError, uploadRange, weekdayIndex } from "./schedule-range";

it("rentang 7–45 hari", () => {
  expect(rangeError("2026-11-02", "2026-11-07")).toBe("Rentang tanggal 7–45 hari");
  expect(rangeError("2026-11-02", "2026-11-08")).toBeNull();
  expect(rangeError("2026-11-02", "2026-12-16")).toBeNull();
  expect(rangeError("2026-11-02", "2026-12-17")).toBe("Rentang tanggal 7–45 hari");
});

it("addDays, dayDiff, weekdayIndex, mondayOf, dm", () => {
  expect(addDays("2026-11-02", 7)).toBe("2026-11-09");
  expect(addDays("2026-11-02", -1)).toBe("2026-11-01");
  expect(dayDiff("2026-11-02", "2026-11-29")).toBe(27);
  expect(weekdayIndex("2026-11-02")).toBe(0); // Senin
  expect(weekdayIndex("2026-11-08")).toBe(6); // Minggu
  expect(mondayOf("2026-10-29")).toBe("2026-10-26"); // Kamis → Senin
  expect(mondayOf("2026-11-01")).toBe("2026-10-26"); // Minggu → Senin sebelumnya
  expect(mondayOf("2026-11-02")).toBe("2026-11-02");
  expect(dm("2026-11-04")).toBe("04/11");
});

it("uploadRange: unggah/template mulai hari ini, minimal 7 hari", () => {
  // Minggu tampil mulai Senin lalu → mulai hari ini, tetap sampai akhir tampilan.
  expect(uploadRange("2026-11-02", "2026-11-15", "2026-11-04")).toEqual({ from: "2026-11-04", to: "2026-11-15" });
  // Sisa tampilan < 7 hari → diperpanjang jadi 7 hari.
  expect(uploadRange("2026-11-02", "2026-11-08", "2026-11-06")).toEqual({ from: "2026-11-06", to: "2026-11-12" });
  // Melihat minggu lampau → mulai hari ini.
  expect(uploadRange("2026-10-19", "2026-10-25", "2026-11-04")).toEqual({ from: "2026-11-04", to: "2026-11-10" });
  // Minggu depan → apa adanya.
  expect(uploadRange("2026-11-09", "2026-12-06", "2026-11-04")).toEqual({ from: "2026-11-09", to: "2026-12-06" });
  expect(rangeError("2026-11-04", "2026-12-06")).toBeNull();
});
