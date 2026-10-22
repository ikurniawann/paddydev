"use client";

import { useState } from "react";
import dynamic from "next/dynamic";
import { Loader2, LocateFixed, Search } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { coordsFromInputs, roundCoord, type LatLngPoint, type PlaceResult } from "@/lib/maps/coords";

const DEFAULT_HEIGHT = 280;
/** Di atas angka ini (meter) titik GPS dianggap kurang tepat untuk titik toko. */
const ROUGH_ACCURACY_M = 50;
const MIN_QUERY = 3;

// OpenLayers butuh window: muat hanya di browser, di bundle terpisah.
const LocationPickerMap = dynamic(() => import("./location-picker-map").then((m) => m.LocationPickerMap), {
  ssr: false,
  loading: () => <div className="h-full animate-pulse rounded-lg bg-gray-100" />,
});

type Notice = { tone: "info" | "warn" | "error"; text: string } | null;

const NOTICE_TONE = { info: "text-gray-600", warn: "text-amber-700", error: "text-red-600" } as const;

function geolocationError(err: GeolocationPositionError): string {
  if (err.code === 1) return "Izin lokasi ditolak. Izinkan akses lokasi untuk situs ini di browser, lalu coba lagi.";
  if (err.code === 3) return "Lokasi tidak didapat (waktu habis). Coba lagi, sebaiknya dekat jendela atau di luar ruangan.";
  return "Lokasi perangkat tidak tersedia. Pastikan GPS/lokasi aktif, atau klik peta.";
}

/**
 * Peta pemilih titik untuk form yang menyimpan lat/lng sebagai teks: klik peta
 * mengisi input; mengetik di input memindahkan pin. "Pakai lokasi saya"
 * mengambil GPS perangkat (untuk HR/Store Leader yang sedang di toko);
 * onSearch menampilkan kotak cari nama tempat/alamat. Jangan dibungkus
 * <label>: klik di peta akan diteruskan ke kontrol pertama di dalam label.
 */
export function LocationPickerField({
  lat,
  lng,
  radiusM,
  onPick,
  onSearch,
  height = DEFAULT_HEIGHT,
  note,
}: {
  lat: string;
  lng: string;
  radiusM?: number | null;
  onPick: (point: LatLngPoint) => void;
  /** Cari nama tempat/alamat; tanpa ini kotak cari tidak tampil. */
  onSearch?: (query: string) => Promise<PlaceResult[]>;
  height?: number;
  /** Kalimat tambahan di baris bantuan yang sama. */
  note?: string;
}) {
  const [query, setQuery] = useState("");
  const [results, setResults] = useState<PlaceResult[]>([]);
  const [searching, setSearching] = useState(false);
  const [locating, setLocating] = useState(false);
  const [notice, setNotice] = useState<Notice>(null);

  function locate() {
    if (typeof navigator === "undefined" || !navigator.geolocation) {
      setNotice({ tone: "error", text: "Perangkat atau browser ini tidak mendukung lokasi. Klik peta untuk menaruh titik." });
      return;
    }
    setLocating(true);
    setNotice(null);
    navigator.geolocation.getCurrentPosition(
      (pos) => {
        setLocating(false);
        const accuracy = Math.round(pos.coords.accuracy);
        onPick({ lat: roundCoord(pos.coords.latitude), lng: roundCoord(pos.coords.longitude) });
        setNotice(
          accuracy > ROUGH_ACCURACY_M
            ? {
                tone: "warn",
                text: `Akurasi rendah (±${accuracy} m). Pastikan GPS aktif dan Anda berada di dalam toko, atau rapikan titik dengan klik peta.`,
              }
            : { tone: "info", text: `Lokasi Anda dipakai sebagai titik toko (akurasi ±${accuracy} m).` }
        );
      },
      (err) => {
        setLocating(false);
        setNotice({ tone: "error", text: geolocationError(err) });
      },
      { enableHighAccuracy: true, timeout: 15000, maximumAge: 0 }
    );
  }

  async function search() {
    if (!onSearch || searching) return;
    const q = query.trim();
    if (q.length < MIN_QUERY) {
      setNotice({ tone: "error", text: `Ketik minimal ${MIN_QUERY} huruf untuk mencari lokasi.` });
      return;
    }
    setSearching(true);
    setNotice(null);
    setResults([]);
    try {
      const found = await onSearch(q);
      setResults(found);
      if (found.length === 0) {
        setNotice({ tone: "warn", text: "Tidak ditemukan. Coba nama jalan, area, atau kota; atau klik peta." });
      }
    } catch (e) {
      setNotice({ tone: "error", text: e instanceof Error && e.message ? e.message : "Pencarian lokasi gagal." });
    } finally {
      setSearching(false);
    }
  }

  function choose(place: PlaceResult) {
    onPick({ lat: roundCoord(place.lat), lng: roundCoord(place.lng) });
    setResults([]);
    setNotice({ tone: "info", text: `Titik dipindah ke hasil pencarian. Periksa pin, lalu klik peta bila perlu dirapikan.` });
  }

  return (
    <div className="space-y-2">
      <div className="flex flex-wrap items-center gap-2">
        {onSearch && (
          <div className="flex min-w-0 flex-1 gap-2">
            <Input
              value={query}
              aria-label="Cari lokasi toko"
              placeholder="Cari nama tempat atau alamat, mis. Paddy Dago Bandung"
              onChange={(e) => setQuery(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "Enter") {
                  e.preventDefault();
                  void search();
                }
              }}
            />
            <Button type="button" variant="outline" disabled={searching} onClick={() => void search()}>
              {searching ? <Loader2 className="h-4 w-4 animate-spin" /> : <Search className="h-4 w-4" />}
              <span className="sr-only sm:not-sr-only sm:ml-1.5">Cari</span>
            </Button>
          </div>
        )}
        <Button type="button" variant="outline" disabled={locating} onClick={locate}>
          {locating ? <Loader2 className="mr-1.5 h-4 w-4 animate-spin" /> : <LocateFixed className="mr-1.5 h-4 w-4" />}
          Pakai lokasi saya
        </Button>
      </div>
      {results.length > 0 && (
        <ul className="max-h-40 overflow-y-auto rounded-md border border-gray-200 text-sm">
          {results.map((p) => (
            <li key={`${p.lat},${p.lng},${p.name}`}>
              <button
                type="button"
                className="w-full px-3 py-1.5 text-left hover:bg-blue-50"
                onClick={() => choose(p)}
              >
                {p.name}
              </button>
            </li>
          ))}
        </ul>
      )}
      <p role="status" aria-live="polite" className={`text-xs ${notice ? NOTICE_TONE[notice.tone] : "sr-only"}`}>
        {notice?.text ?? (results.length > 0 ? `${results.length} hasil ditemukan` : "")}
      </p>
      {/* Pembungkus bertinggi tetap: kerangka pemuatan mengisinya sebelum peta siap. */}
      <div style={{ height }}>
        <LocationPickerMap value={coordsFromInputs(lat, lng)} radiusM={radiusM} onPick={onPick} height={height} />
      </div>
      <p className="text-xs text-gray-500">
        Klik peta untuk menaruh titik toko. Latitude/longitude tetap bisa diketik, atau tempel koordinat/link Google Maps.
        {note ? ` ${note}` : ""}
      </p>
    </div>
  );
}
