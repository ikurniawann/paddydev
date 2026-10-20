"use client";

import dynamic from "next/dynamic";
import { coordsFromInputs, type LatLngPoint } from "@/lib/maps/coords";

const DEFAULT_HEIGHT = 280;

// OpenLayers butuh window: muat hanya di browser, di bundle terpisah.
const LocationPickerMap = dynamic(() => import("./location-picker-map").then((m) => m.LocationPickerMap), {
  ssr: false,
  loading: () => <div className="h-full animate-pulse rounded-lg bg-gray-100" />,
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
  height = DEFAULT_HEIGHT,
  note,
}: {
  lat: string;
  lng: string;
  radiusM?: number | null;
  onPick: (point: LatLngPoint) => void;
  height?: number;
  /** Kalimat tambahan di baris bantuan yang sama. */
  note?: string;
}) {
  return (
    <div className="space-y-1">
      {/* Pembungkus bertinggi tetap: kerangka pemuatan mengisinya sebelum peta siap. */}
      <div style={{ height }}>
        <LocationPickerMap value={coordsFromInputs(lat, lng)} radiusM={radiusM} onPick={onPick} height={height} />
      </div>
      <p className="text-xs text-gray-500">
        Klik peta untuk menaruh titik toko. Latitude/longitude tetap bisa diketik atau ditempel dari Google Maps.
        {note ? ` ${note}` : ""}
      </p>
    </div>
  );
}
