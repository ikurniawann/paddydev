import { describe, expect, it } from "vitest";
import { GEOFENCE_DEFAULT_RADIUS_M } from "@/lib/hris/geofence";
import { coordsFromInputs, isUsableCoord, parseLatLng, resolveRadiusM, roundCoord } from "./coords";

describe("coords", () => {
  it("membulatkan ke 6 desimal (numeric(9,6))", () => {
    expect(roundCoord(-6.88891234)).toBe(-6.888912);
    expect(roundCoord(107.5960004)).toBe(107.596);
  });
  it("menolak 0,0 dan koordinat di luar rentang", () => {
    expect(isUsableCoord(-6.9, 107.6)).toBe(true);
    expect(isUsableCoord(0, 0)).toBe(false);
    expect(isUsableCoord(-91, 107.6)).toBe(false);
    expect(isUsableCoord(-6.9, 181)).toBe(false);
    expect(isUsableCoord(Number.NaN, 107.6)).toBe(false);
  });
  it("membaca input form lat/lng", () => {
    expect(coordsFromInputs("-6.9", "107.6")).toEqual({ lat: -6.9, lng: 107.6 });
    expect(coordsFromInputs("", "107.6")).toBeNull();
    expect(coordsFromInputs("abc", "107.6")).toBeNull();
    expect(coordsFromInputs("0", "0")).toBeNull();
  });
  it("membaca salinan koordinat Google Maps", () => {
    expect(parseLatLng("-6.888900, 107.596000")).toEqual({ lat: -6.8889, lng: 107.596 });
    expect(parseLatLng(" -6.8889 107.596 ")).toEqual({ lat: -6.8889, lng: 107.596 });
    expect(parseLatLng("-6.2, 106.8")).toEqual({ lat: -6.2, lng: 106.8 });
    expect(parseLatLng("-6.88891234, 107.5960004")).toEqual({ lat: -6.888912, lng: 107.596 });
    expect(parseLatLng("-6.2,106.8")).toEqual({ lat: -6.2, lng: 106.8 });
    expect(parseLatLng("-6.2 ,106.8")).toEqual({ lat: -6.2, lng: 106.8 });
    expect(parseLatLng("Jl. Sukajadi 131")).toBeNull();
    expect(parseLatLng("0, 0")).toBeNull();
    expect(parseLatLng("-95, 107")).toBeNull();
  });
  it("tidak memecah desimal koma Indonesia menjadi lat/lng", () => {
    expect(parseLatLng("-6,9")).toBeNull();
    expect(parseLatLng("-6,9 107,6")).toBeNull();
    expect(parseLatLng("-6,5,107,6")).toBeNull();
    expect(parseLatLng("-6,9, 107,6")).toBeNull();
    expect(parseLatLng("-6.9,107")).toBeNull();
  });
  it("radius lingkaran jatuh ke bawaan bila kosong atau tidak valid", () => {
    expect(resolveRadiusM(250)).toBe(250);
    expect(resolveRadiusM(null)).toBe(GEOFENCE_DEFAULT_RADIUS_M);
    expect(resolveRadiusM(undefined)).toBe(GEOFENCE_DEFAULT_RADIUS_M);
    expect(resolveRadiusM(0)).toBe(GEOFENCE_DEFAULT_RADIUS_M);
    expect(resolveRadiusM(-5)).toBe(GEOFENCE_DEFAULT_RADIUS_M);
    expect(resolveRadiusM(Number.NaN)).toBe(GEOFENCE_DEFAULT_RADIUS_M);
  });
});
