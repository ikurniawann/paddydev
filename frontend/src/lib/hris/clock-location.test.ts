import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { acquireClockLocation } from "./clock-location";

type Ok = (p: { coords: { latitude: number; longitude: number; accuracy: number } }) => void;
const fix = (accuracy: number, latitude = -6.938) => ({ coords: { latitude, longitude: 107.618, accuracy } });

let watchOk: Ok | null;
let watchErr: ((e: { code: number }) => void) | null;
const clearWatch = vi.fn();
function geo(first: ReturnType<typeof fix> | null, withWatch = true) {
  watchOk = null;
  return {
    getCurrentPosition: vi.fn((ok: Ok, err: () => void) => (first ? ok(first) : err())),
    ...(withWatch
      ? {
          watchPosition: vi.fn((ok: Ok, err: (e: { code: number }) => void) => {
            watchOk = ok;
            watchErr = err;
            return 7;
          }),
          clearWatch,
        }
      : {}),
  } as unknown as Geolocation;
}

beforeEach(() => {
  vi.useFakeTimers();
  clearWatch.mockClear();
});
afterEach(() => vi.useRealTimers());

describe("acquireClockLocation", () => {
  it("fix akurat langsung dipakai tanpa menunggu", async () => {
    const g = geo(fix(12));
    await expect(acquireClockLocation(g)).resolves.toEqual({ latitude: -6.938, longitude: 107.618, accuracy: 12 });
    expect(g.watchPosition).not.toHaveBeenCalled();
  });

  it("fix perkiraan: menunggu fix akurat dari watchPosition", async () => {
    const g = geo(fix(2000));
    const p = acquireClockLocation(g);
    watchOk!(fix(900));
    watchOk!(fix(35, -6.9381));
    await expect(p).resolves.toEqual({ latitude: -6.9381, longitude: 107.618, accuracy: 35 });
    expect(clearWatch).toHaveBeenCalledWith(7);
  });

  it("tetap perkiraan sampai batas waktu: kirim fix terbaik", async () => {
    const g = geo(fix(2000));
    const p = acquireClockLocation(g, 6000);
    watchOk!(fix(1500, -6.95));
    await vi.advanceTimersByTimeAsync(6000);
    await expect(p).resolves.toEqual({ latitude: -6.95, longitude: 107.618, accuracy: 1500 });
    expect(clearWatch).toHaveBeenCalledWith(7);
  });

  it("gangguan sesaat (iOS POSITION_UNAVAILABLE) tidak menghentikan penantian; izin dicabut menghentikan", async () => {
    const g = geo(fix(2000));
    const p = acquireClockLocation(g, 6000);
    watchErr!({ code: 2 });
    watchOk!(fix(40, -6.9382));
    await expect(p).resolves.toMatchObject({ accuracy: 40, latitude: -6.9382 });

    const denied = acquireClockLocation(geo(fix(2000)), 6000);
    watchErr!({ code: 1 });
    await expect(denied).resolves.toMatchObject({ accuracy: 2000 });
  });

  it("tanpa watchPosition atau tanpa izin lokasi", async () => {
    await expect(acquireClockLocation(geo(fix(2000), false))).resolves.toMatchObject({ accuracy: 2000 });
    await expect(acquireClockLocation(geo(null))).resolves.toBeNull();
    await expect(acquireClockLocation(undefined)).resolves.toBeNull();
  });
});
