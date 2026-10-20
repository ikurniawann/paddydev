import { afterEach, describe, expect, it, vi } from "vitest";
import { ClockGeofenceError, submitClock } from "./api";
import type { EssClockPayload } from "./types";

const payload: EssClockPayload = { action: "clock-in", photo: "data:image/jpeg;base64,AAA" };

function mockFetch(status: number, body: unknown) {
  const fn = vi.fn().mockResolvedValue(
    new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } })
  );
  vi.stubGlobal("fetch", fn);
  return fn;
}

afterEach(() => vi.unstubAllGlobals());

describe("submitClock", () => {
  it("melempar ClockGeofenceError pada 422 geofence_note_required", async () => {
    mockFetch(422, {
      success: false,
      error: "Anda di luar area PVJ (±334 m, batas 100 m).",
      details: { code: "geofence_note_required", distance_m: 334, radius_m: 100, branch_name: "PVJ" },
    });
    const err = await submitClock(payload).catch((e: unknown) => e);
    expect(err).toBeInstanceOf(ClockGeofenceError);
    expect((err as ClockGeofenceError).info).toEqual({
      message: "Anda di luar area PVJ (±334 m, batas 100 m).",
      distanceM: 334,
      radiusM: 100,
      branchName: "PVJ",
    });
  });

  it("422 lain tetap Error biasa dengan pesan server", async () => {
    mockFetch(422, { success: false, error: "Validation failed", details: [] });
    const err = await submitClock(payload).catch((e: unknown) => e);
    expect(err).not.toBeInstanceOf(ClockGeofenceError);
    expect((err as Error).message).toBe("Validation failed");
  });

  it("mengembalikan respons sukses", async () => {
    const fetchFn = mockFetch(200, { message: "ok", data: { is_late: false } });
    await expect(submitClock({ ...payload, notes: "Ambil stok" })).resolves.toEqual({
      message: "ok",
      data: { is_late: false },
    });
    expect(JSON.parse(fetchFn.mock.calls[0][1].body)).toEqual({ ...payload, notes: "Ambil stok" });
  });
});
