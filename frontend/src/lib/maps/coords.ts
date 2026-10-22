/** Radius bawaan (MOM AB-01); dipakai hanya bila server tidak mengirim `radius_m`. */
export const GEOFENCE_DEFAULT_RADIUS_M = 100;

/** Koordinat titik peta (derajat desimal WGS84). */
export interface LatLngPoint {
  lat: number;
  lng: number;
}

/** 6 desimal (±11 cm), presisi kolom configuration.branches.lat/lng numeric(9,6). */
export const roundCoord = (v: number): number => Math.round(v * 1e6) / 1e6;

/** Koordinat yang masuk akal: dalam rentang dan bukan 0,0 (GPS gagal). */
export function isUsableCoord(lat: number, lng: number): boolean {
  if (!Number.isFinite(lat) || !Number.isFinite(lng)) return false;
  if (Math.abs(lat) > 90 || Math.abs(lng) > 180) return false;
  return lat !== 0 || lng !== 0;
}

/** Titik dari dua input form; null bila kosong atau tidak valid. */
export function coordsFromInputs(lat: string, lng: string): LatLngPoint | null {
  if (lat.trim() === "" || lng.trim() === "") return null;
  const point = { lat: Number(lat), lng: Number(lng) };
  return isUsableCoord(point.lat, point.lng) ? point : null;
}

/**
 * "-6.888900, 107.596000" (salin dari Google Maps) atau dipisah spasi; null bila bukan koordinat.
 * Koma tanpa spasi hanya dipisah bila kedua angka berdesimal titik: "-6,9" (desimal koma
 * Indonesia) bukan pasangan lat/lng.
 */
export function parseLatLng(text: string): LatLngPoint | null {
  const m = text.trim().match(/^(-?\d+(?:\.\d+)?)(\s*,\s*|\s+)(-?\d+(?:\.\d+)?)$/);
  if (!m) return null;
  const [, a, sep, b] = m;
  if (!/\s/.test(sep) && !(a.includes(".") && b.includes("."))) return null;
  const point = { lat: roundCoord(Number(a)), lng: roundCoord(Number(b)) };
  return isUsableCoord(point.lat, point.lng) ? point : null;
}

/** Radius lingkaran area absensi: nilai toko bila valid, selain itu radius bawaan (NULL = bawaan). */
export function resolveRadiusM(radiusM: number | null | undefined): number {
  return typeof radiusM === "number" && Number.isFinite(radiusM) && radiusM > 0 ? radiusM : GEOFENCE_DEFAULT_RADIUS_M;
}

const GOOGLE_HOST = /(^|\.)google\.[a-z.]+$/;
const PAIR = /^\s*(-?\d+(?:\.\d+)?)\s*,\s*\+?\s*(-?\d+(?:\.\d+)?)\s*$/;

/** decodeURIComponent yang tidak melempar: % rusak (link sedang diketik) dibiarkan apa adanya. */
function safeDecode(text: string): string {
  try {
    return decodeURIComponent(text);
  } catch {
    return text;
  }
}

function usablePoint(a: string, b: string): LatLngPoint | null {
  const point = { lat: roundCoord(Number(a)), lng: roundCoord(Number(b)) };
  return isUsableCoord(point.lat, point.lng) ? point : null;
}

/**
 * Titik dari link Google Maps lengkap: pin tempat (`!3d…!4d…`) lebih dulu,
 * lalu parameter `q`/`query`/`ll`/`center`/`destination`, path `/search/` atau
 * `/place/` berisi koordinat, terakhir pusat peta (`@lat,lng`). null bila
 * bukan link Google Maps atau tidak memuat koordinat.
 */
function parseGoogleMapsLink(text: string): LatLngPoint | null {
  let url: URL;
  try {
    url = new URL(text.trim());
  } catch {
    return null;
  }
  if (!GOOGLE_HOST.test(url.hostname)) return null;
  const href = safeDecode(url.href);
  const pin = href.match(/!3d(-?\d+(?:\.\d+)?)!4d(-?\d+(?:\.\d+)?)/);
  if (pin) return usablePoint(pin[1], pin[2]);
  for (const key of ["q", "query", "ll", "center", "destination"]) {
    const m = (url.searchParams.get(key) ?? "").match(PAIR);
    if (m) return usablePoint(m[1], m[2]);
  }
  const path = safeDecode(url.pathname).match(/\/(?:search|place)\/(-?\d+(?:\.\d+)?),\s*\+?(-?\d+(?:\.\d+)?)/);
  if (path) return usablePoint(path[1], path[2]);
  const center = href.match(/@(-?\d+(?:\.\d+)?),(-?\d+(?:\.\d+)?)/);
  return center ? usablePoint(center[1], center[2]) : null;
}

/** Isian tempel Lokasi Toko: koordinat polos atau link Google Maps lengkap. */
export function parseLocationText(text: string): LatLngPoint | null {
  return parseLatLng(text) ?? parseGoogleMapsLink(text);
}

/** Link pendek bagikan Google Maps (maps.app.goo.gl, goo.gl/maps): tidak memuat koordinat. */
export function isShortMapsLink(text: string): boolean {
  return /^https?:\/\/(maps\.app\.goo\.gl|goo\.gl\/maps)\//i.test(text.trim());
}

/** Hasil cari nama tempat/alamat (GET /api/hris/places). */
export interface PlaceResult {
  name: string;
  lat: number;
  lng: number;
}
