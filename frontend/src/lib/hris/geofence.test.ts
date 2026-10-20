import { describe, expect, it } from "vitest";
import {
  GEOFENCE_DEFAULT_RADIUS_M,
  GEOFENCE_META,
  geofenceLabel,
  geofenceNoteRequired,
  isGeofenceStatus,
} from "./geofence";

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

describe("geofenceNoteRequired", () => {
  const body = {
    success: false,
    error: "Anda di luar area PVJ (±334 m, batas 100 m). Isi keterangan minimal 5 karakter untuk tetap absen.",
    details: { code: "geofence_note_required", distance_m: 334, radius_m: 100, branch_name: "PVJ" },
  };
  it("membaca respons 422 di luar area", () => {
    expect(geofenceNoteRequired(422, body)).toEqual({
      message: body.error,
      distanceM: 334,
      radiusM: 100,
      branchName: "PVJ",
    });
  });
  it("memakai radius_m dari server, bukan angka tetap", () => {
    const custom = { ...body, details: { ...body.details, radius_m: 250 } };
    expect(geofenceNoteRequired(422, custom)?.radiusM).toBe(250);
  });
  it("jatuh ke radius bawaan bila server tidak mengirim radius", () => {
    const noRadius = { ...body, details: { code: "geofence_note_required" } };
    expect(geofenceNoteRequired(422, noRadius)).toMatchObject({
      distanceM: null,
      radiusM: GEOFENCE_DEFAULT_RADIUS_M,
      branchName: null,
    });
  });
  it("mengabaikan error lain", () => {
    expect(geofenceNoteRequired(400, body)).toBeNull();
    expect(geofenceNoteRequired(422, { error: "Validation failed", details: [] })).toBeNull();
    expect(geofenceNoteRequired(422, null)).toBeNull();
  });
});
