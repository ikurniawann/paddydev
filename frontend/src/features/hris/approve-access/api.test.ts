import { afterEach, describe, expect, it, vi } from "vitest";
import { fetchHrApproveAccess } from "./api";

afterEach(() => vi.unstubAllGlobals());

describe("fetchHrApproveAccess", () => {
  it("membuka amplop {data} dari endpoint Go", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ data: { attendance: true, leaves: false, overtime: true } }), { status: 200 })
    );
    vi.stubGlobal("fetch", fetchMock);
    await expect(fetchHrApproveAccess()).resolves.toEqual({ attendance: true, leaves: false, overtime: true });
    expect(fetchMock.mock.calls[0][0]).toBe("/api/hris/me/approve-access");
  });

  it("menolak (error) saat server membalas 403 agar UI gagal tutup", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response(JSON.stringify({ error: "x" }), { status: 403 })));
    await expect(fetchHrApproveAccess()).rejects.toBeTruthy();
  });
});
