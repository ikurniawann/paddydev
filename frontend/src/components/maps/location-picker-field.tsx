"use client";

import dynamic from "next/dynamic";
import { coordsFromInputs, resolveRadiusM, type LatLngPoint } from "@/lib/maps/coords";

// OpenLayers butuh window: muat hanya di browser, di bundle terpisah.
const LocationPickerMap = dynamic(() => import("./location-picker-map").then((m) => m.LocationPickerMap), {
  ssr: false,
  loading: () => <div className="h-[280px] animate-pulse rounded-lg bg-gray-100" />,
});

/**
 * Peta pemilih titik untuk form yang menyimpan lat/lng sebagai teks: klik peta
 * mengisi input; mengetik di input memindahkan pin. Jangan dibungkus <label>:
 * klik di peta akan diteruskan ke kontrol pertama di dalam label.
 */
export function LocationPickerField({
  lat,
  lng,
  radiusM,
  onPick,
  height,
}: {
  lat: string;
  lng: string;
  radiusM?: number | null;
  onPick: (point: LatLngPoint) => void;
  height?: number;
}) {
  return (
    <div className="space-y-1">
      <LocationPickerMap value={coordsFromInputs(lat, lng)} radiusM={resolveRadiusM(radiusM)} onPick={onPick} height={height} />
      <p className="text-xs text-gray-500">
        Klik peta untuk menaruh titik toko. Latitude/longitude tetap bisa diketik atau ditempel dari Google Maps.
      </p>
    </div>
  );
}
