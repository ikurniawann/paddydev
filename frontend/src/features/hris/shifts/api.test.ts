import { afterEach, describe, expect, it, vi } from "vitest";
import { fetchShifts, saveShift } from "./api";

afterEach(() => vi.unstubAllGlobals());

function stubFetch() {
  const fetchMock = vi.fn().mockResolvedValue(
    new Response(JSON.stringify({ data: [{ id: "s1", employees_using: 3 }] }), {
      status: 200,
      headers: { "Content-Type": "application/json" },
    })
  );
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}

describe("shifts api", () => {
  it("meminta daftar shift dengan jumlah pemakai", async () => {
    const fetchMock = stubFetch();
    const rows = await fetchShifts();
    expect(fetchMock.mock.calls[0][0]).toBe("/api/hris/shifts?with_usage=true");
    expect(rows[0].employees_using).toBe(3);
  });

  it("nonaktifkan memakai PATCH is_active saja", async () => {
    const fetchMock = stubFetch();
    await saveShift({ is_active: false }, "s1");
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toBe("/api/hris/shifts/s1");
    expect(init.method).toBe("PATCH");
    expect(JSON.parse(init.body)).toEqual({ is_active: false });
  });
});
