/**
 * Lokasi GPS untuk clock-in/out ESS. Fix pertama dari HP sering masih kasar
 * (jaringan sel/Wi-Fi sebelum GPS terkunci); bila akurasinya di atas batas
 * lokasi perkiraan, tunggu sebentar lewat watchPosition untuk fix yang lebih
 * akurat, lalu kirim fix terbaik. HP yang izin lokasinya "Perkiraan"
 * (Android) atau Lokasi Akurat mati (iPhone) tidak pernah membaik: setelah
 * batas waktu tetap dikirim dan server menilainya "Lokasi tidak akurat".
 */

import { isImpreciseAccuracy } from "./geofence";

export interface ClockLocation {
  latitude: number;
  longitude: number;
  accuracy: number;
}

/** Lama menunggu fix akurat setelah fix pertama kasar (ms). */
export const CLOCK_LOCATION_SETTLE_MS = 6000;

const toLocation = (pos: GeolocationPosition): ClockLocation => ({
  latitude: pos.coords.latitude,
  longitude: pos.coords.longitude,
  accuracy: pos.coords.accuracy,
});

export function acquireClockLocation(
  geo: Geolocation | undefined = typeof navigator === "undefined" ? undefined : navigator.geolocation,
  settleMs = CLOCK_LOCATION_SETTLE_MS
): Promise<ClockLocation | null> {
  return new Promise((resolve) => {
    if (!geo) return resolve(null);
    geo.getCurrentPosition(
      (pos) => {
        let best = toLocation(pos);
        if (!isImpreciseAccuracy(best.accuracy) || typeof geo.watchPosition !== "function") return resolve(best);
        let done = false;
        let watchId: number | null = null;
        const finish = () => {
          if (done) return;
          done = true;
          clearTimeout(timer);
          if (watchId !== null) geo.clearWatch(watchId);
          resolve(best);
        };
        const timer = setTimeout(finish, settleMs);
        watchId = geo.watchPosition(
          (next) => {
            const loc = toLocation(next);
            if (loc.accuracy < best.accuracy) best = loc;
            if (!isImpreciseAccuracy(best.accuracy)) finish();
          },
          () => finish(),
          { enableHighAccuracy: true, maximumAge: 0 }
        );
        // watchPosition bisa memanggil callback secara sinkron dan finish sebelum id tersedia.
        if (done) geo.clearWatch(watchId);
      },
      () => resolve(null),
      { enableHighAccuracy: true, timeout: 10000, maximumAge: 0 }
    );
  });
}
