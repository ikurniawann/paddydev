import { afterEach, describe, expect, it, vi } from "vitest";
import { fetchHrApproveAccess } from "./api";

afterEach(() => vi.unstubAllGlobals());

describe("fetchHrApproveAccess", () => {
  it("membaca objek datar dari endpoint Go (tanpa amplop data)", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ attendance: true, leaves: false, overtime: true }), { status: 200 })
    );
    vi.stubGlobal("fetch", fetchMock);
    const got = await fetchHrApproveAccess();
    expect(got).toEqual({ attendance: true, leaves: false, overtime: true });
    for (const k of ["attendance", "leaves", "overtime"] as const) expect(typeof got[k]).toBe("boolean");
    expect(fetchMock.mock.calls[0][0]).toBe("/api/hris/me/approve-access");
  });

  it("menolak (error) saat server membalas 403 agar UI gagal tutup", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response(JSON.stringify({ error: "x" }), { status: 403 })));
    await expect(fetchHrApproveAccess()).rejects.toBeTruthy();
  });
});
