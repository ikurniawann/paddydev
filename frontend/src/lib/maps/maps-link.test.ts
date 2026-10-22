import { describe, expect, it } from "vitest";
import { isShortMapsLink, parseLocationText } from "./coords";

describe("parseLocationText", () => {
  it("tetap membaca koordinat polos", () => {
    expect(parseLocationText("-6.888900, 107.596000")).toEqual({ lat: -6.8889, lng: 107.596 });
  });
  it("mengutamakan pin tempat (!3d!4d) dari link Google Maps", () => {
    const url =
      "https://www.google.com/maps/place/Paddy+Dago/@-6.8850000,107.6100000,17z/data=!3m1!4b1!4m6!3m5!1s0x0:0x0!8m2!3d-6.8889123!4d107.6131456";
    expect(parseLocationText(url)).toEqual({ lat: -6.888912, lng: 107.613146 });
  });
  it("memakai pusat peta (@lat,lng) bila tidak ada pin", () => {
    expect(parseLocationText("https://www.google.com/maps/@-6.9147,107.6098,15z")).toEqual({ lat: -6.9147, lng: 107.6098 });
  });
  it("membaca parameter q, query, ll, dan /search/", () => {
    expect(parseLocationText("https://maps.google.com/?q=-6.9147,107.6098")).toEqual({ lat: -6.9147, lng: 107.6098 });
    expect(parseLocationText("https://www.google.com/maps/search/?api=1&query=-6.9147%2C107.6098")).toEqual({
      lat: -6.9147,
      lng: 107.6098,
    });
    expect(parseLocationText("https://maps.google.co.id/maps?ll=-6.9147,107.6098&z=16")).toEqual({ lat: -6.9147, lng: 107.6098 });
    expect(parseLocationText("https://www.google.com/maps/search/-6.9147,+107.6098?entry=tts")).toEqual({
      lat: -6.9147,
      lng: 107.6098,
    });
  });
  it("menolak link tanpa koordinat, teks biasa, dan 0,0", () => {
    expect(parseLocationText("https://www.google.com/maps/place/Paddy+Dago")).toBeNull();
    expect(parseLocationText("Jl. Sukajadi 131")).toBeNull();
    expect(parseLocationText("https://www.google.com/maps/@0,0,3z")).toBeNull();
    expect(parseLocationText("https://example.com/?q=-6.9147,107.6098")).toBeNull();
  });
  it("tidak melempar error pada % rusak (bisa terjadi saat link diketik)", () => {
    expect(parseLocationText("https://www.google.com/maps/search/50%")).toBeNull();
    expect(parseLocationText("https://www.google.com/maps/place/Toko%ZZ/@-6.9,107.6,17z")).toEqual({ lat: -6.9, lng: 107.6 });
  });
});

describe("isShortMapsLink", () => {
  it("mengenali link pendek bagikan Google Maps", () => {
    expect(isShortMapsLink("https://maps.app.goo.gl/AbCdEf123")).toBe(true);
    expect(isShortMapsLink(" https://goo.gl/maps/AbCdEf ")).toBe(true);
    expect(isShortMapsLink("https://www.google.com/maps/@-6.9,107.6,15z")).toBe(false);
    expect(isShortMapsLink("-6.9, 107.6")).toBe(false);
  });
});
