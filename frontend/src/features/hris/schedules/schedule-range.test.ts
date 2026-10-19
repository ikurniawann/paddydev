import { expect, it } from "vitest";
import { defaultRange, rangeError } from "./schedule-range";

it("rentang 7–45 hari", () => {
  expect(rangeError("2026-11-02", "2026-11-07")).toBe("Rentang tanggal 7–45 hari");
  expect(rangeError("2026-11-02", "2026-11-08")).toBeNull();
  expect(rangeError("2026-11-02", "2026-12-16")).toBeNull();
  expect(rangeError("2026-11-02", "2026-12-17")).toBe("Rentang tanggal 7–45 hari");
  expect(defaultRange("2026-10-21")).toEqual({ from: "2026-10-21", to: "2026-11-03" });
});
