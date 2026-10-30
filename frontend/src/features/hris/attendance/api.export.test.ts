import { afterEach, describe, expect, it, vi } from "vitest";
import { exportAttendanceCsv } from "./api";

function stubFetch() {
  const fn = vi.fn().mockImplementation(async () => new Response("x", { status: 200 }));
  vi.stubGlobal("fetch", fn);
  return fn;
}

afterEach(() => vi.unstubAllGlobals());

describe("exportAttendanceCsv (EPIC-075 R1)", () => {
  it("mengirim filter toko dan hanya terlambat", async () => {
    const fn = stubFetch();
    await exportAttendanceCsv({ start_date: "2026-10-01", end_date: "2026-10-31", branch_id: "b1", is_late: true }, "xlsx");
    const url = String(fn.mock.calls[0][0]);
    expect(url).toContain("branch_id=b1");
    expect(url).toContain("is_late=true");
    expect(url).toContain("format=xlsx");
  });

  it("tanpa filter toko (semua toko) tidak mengirim branch_id", async () => {
    const fn = stubFetch();
    await exportAttendanceCsv({ start_date: "2026-10-01", end_date: "2026-10-31", branch_id: "all" });
    await exportAttendanceCsv({ start_date: "2026-10-01", end_date: "2026-10-31" });
    for (const call of fn.mock.calls) {
      expect(String(call[0])).not.toContain("branch_id");
      expect(String(call[0])).not.toContain("is_late");
    }
  });
});
