import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiError, apiPost } from "./api-client";

function mockFetch(status: number, body: unknown) {
  vi.stubGlobal(
    "fetch",
    vi.fn().mockResolvedValue(new Response(JSON.stringify(body), { status }))
  );
}

afterEach(() => vi.unstubAllGlobals());

describe("api-client error parsing", () => {
  it("membawa status, body, dan details pada ApiError", async () => {
    mockFetch(422, { error: "Gagal", details: { code: "x" } });
    const err = await apiPost("/x", {}).catch((e: unknown) => e);
    expect(err).toBeInstanceOf(ApiError);
    expect(err).toBeInstanceOf(Error);
    expect(err).toMatchObject({
      message: "Gagal",
      status: 422,
      details: { code: "x" },
      body: { error: "Gagal", details: { code: "x" } },
    });
  });

  it("tetap membaca error.message bersarang", async () => {
    mockFetch(400, { error: { message: "Bersarang" } });
    await expect(apiPost("/x", {})).rejects.toThrow("Bersarang");
  });

  it("pesan bawaan bila body tanpa error", async () => {
    mockFetch(500, {});
    await expect(apiPost("/x", {})).rejects.toThrow("Request failed (500)");
  });
});
