import { describe, expect, it } from "vitest";
import {
  GEOFENCE_DEFAULT_RADIUS_M,
  GEOFENCE_META,
  geofenceHasNote,
  geofenceLabel,
  isImpreciseAccuracy,
  geofenceNoteRequired,
  isGeofenceStatus,
  toBranchLocationPayload,
  validateBranchLocation,
} from "./geofence";

describe("geofenceLabel", () => {
  it("menulis status dan jarak", () => {
    expect(geofenceLabel("valid", 56)).toBe("Dalam area · 56 m");
    expect(geofenceLabel("invalid", 334)).toBe("Di luar area · 334 m");
    expect(geofenceLabel("unchecked", null)).toBe("Belum dicek");
    expect(geofenceLabel("dinas", 334)).toBe("Perjalanan Dinas");
  });
  it("lokasi perkiraan tanpa jarak (jaraknya tidak bermakna)", () => {
    expect(geofenceLabel("imprecise", 278)).toBe("Lokasi tidak akurat");
    expect(isGeofenceStatus("imprecise")).toBe(true);
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
      reason: "outside",
      distanceM: 334,
      radiusM: 100,
      accuracyM: null,
      branchName: "PVJ",
    });
  });
  it("membaca respons 422 lokasi tidak akurat", () => {
    const imprecise = {
      ...body,
      details: { ...body.details, reason: "imprecise", accuracy_m: 2000, distance_m: 278 },
    };
    expect(geofenceNoteRequired(422, imprecise)).toMatchObject({ reason: "imprecise", accuracyM: 2000, distanceM: 278 });
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

describe("validateBranchLocation", () => {
  it("sama dengan aturan server", () => {
    expect(validateBranchLocation({ lat: "", lng: "", radius: "" })).toBe("");
    expect(validateBranchLocation({ lat: "-6.8889", lng: "107.596", radius: "250" })).toBe("");
    expect(validateBranchLocation({ lat: "-6.8889", lng: "", radius: "" })).toBe(
      "Latitude dan longitude wajib diisi berdua atau dikosongkan berdua"
    );
    expect(validateBranchLocation({ lat: "0", lng: "0", radius: "" })).toBe(
      "Koordinat tidak valid (latitude -90..90, longitude -180..180, bukan 0,0)"
    );
    expect(validateBranchLocation({ lat: "-6.9", lng: "107.6", radius: "10" })).toBe("Radius absensi 25–1000 meter");
    expect(validateBranchLocation({ lat: "-6.9", lng: "107.6", radius: "120.5" })).toBe("Radius absensi 25–1000 meter");
  });
  it("menerima batas radius 25 dan 1000, menolak 24 dan 1001", () => {
    const f = (radius: string) => validateBranchLocation({ lat: "", lng: "", radius });
    expect(f("25")).toBe("");
    expect(f("1000")).toBe("");
    expect(f("24")).toBe("Radius absensi 25–1000 meter");
    expect(f("1001")).toBe("Radius absensi 25–1000 meter");
    expect(f("abc")).toBe("Radius absensi 25–1000 meter");
  });
  it("membentuk payload API", () => {
    expect(toBranchLocationPayload({ lat: "-6.8889", lng: "107.596", radius: "" })).toEqual({
      lat: -6.8889,
      lng: 107.596,
      geofence_radius_m: null,
    });
    expect(toBranchLocationPayload({ lat: "", lng: "", radius: "250" })).toEqual({
      lat: null,
      lng: null,
      geofence_radius_m: 250,
    });
  });
});

describe("lokasi perkiraan", () => {
  it("akurasi di atas 500 m dianggap lokasi perkiraan, sama dengan server", () => {
    expect(isImpreciseAccuracy(2000)).toBe(true);
    expect(isImpreciseAccuracy(501)).toBe(true);
    expect(isImpreciseAccuracy(500)).toBe(false);
    expect(isImpreciseAccuracy(12)).toBe(false);
    expect(isImpreciseAccuracy(null)).toBe(false);
    expect(isImpreciseAccuracy(undefined)).toBe(false);
  });
  it("status yang membawa keterangan karyawan", () => {
    expect(geofenceHasNote("invalid")).toBe(true);
    expect(geofenceHasNote("imprecise")).toBe(true);
    expect(geofenceHasNote("valid")).toBe(false);
    expect(geofenceHasNote(null)).toBe(false);
  });
});
