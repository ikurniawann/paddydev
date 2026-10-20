import { describe, expect, it } from "vitest";
import { GEOFENCE_META, geofenceLabel, isGeofenceStatus } from "./geofence";

describe("geofenceLabel", () => {
  it("menulis status dan jarak", () => {
    expect(geofenceLabel("valid", 56)).toBe("Dalam area · 56 m");
    expect(geofenceLabel("invalid", 334)).toBe("Di luar area · 334 m");
    expect(geofenceLabel("unchecked", null)).toBe("Belum dicek");
    expect(geofenceLabel("dinas", 334)).toBe("Perjalanan Dinas");
  });
  it("kosong untuk absen tanpa penilaian (sebelum EPIC-062 / dicatat HRD)", () => {
    expect(geofenceLabel(null, null)).toBeNull();
    expect(geofenceLabel("lainnya", 10)).toBeNull();
  });
  it("mengenali status yang valid saja", () => {
    expect(isGeofenceStatus("dinas")).toBe(true);
    expect(isGeofenceStatus("remote")).toBe(false);
    expect(GEOFENCE_META.invalid.label).toBe("Di luar area");
  });
  it("menolak kunci prototipe (Object.hasOwn, bukan in)", () => {
    expect(isGeofenceStatus("toString")).toBe(false);
    expect(isGeofenceStatus("constructor")).toBe(false);
    expect(geofenceLabel("__proto__", 1)).toBeNull();
  });
});
