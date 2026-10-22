"use client";

import { useState } from "react";
import { toast } from "sonner";
import { Loader2, MapPin } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { LocationPickerField } from "@/components/maps/location-picker-field";
import { isShortMapsLink, parseLocationText, resolveRadiusM } from "@/lib/maps/coords";
import {
  GEOFENCE_DEFAULT_RADIUS_M,
  toBranchLocationPayload,
  validateBranchLocation,
  type BranchLocationForm,
} from "@/lib/hris/geofence";
import { MIN_WORK_HOURS_ERROR, formatHours, toMinWorkHoursPayload, validateMinWorkHours } from "@/lib/hris/short-hours";
import { searchPlaces } from "../api";
import { useSaveBranchLocation, useSaveBranchMinWorkHours } from "../mutations";
import type { StoreBranch } from "../types";

const EMPTY: BranchLocationForm = { lat: "", lng: "", radius: "" };
/** Hanya angka dengan titik/koma desimal; bentuk lain (mis. "1e1", "abc") ditolak, bukan dikosongkan. */
const MIN_HOURS_FORMAT = /^\d+([.,]\d+)?$/;
const hasCoords = (b: StoreBranch) => b.lat !== null && b.lng !== null;
const toForm = (b: StoreBranch): BranchLocationForm => ({
  lat: b.lat === null ? "" : String(Number(b.lat)),
  lng: b.lng === null ? "" : String(Number(b.lng)),
  radius: b.geofence_radius_m == null ? "" : String(b.geofence_radius_m),
});

/**
 * HRIS → Penempatan Karyawan → Lokasi Toko (EPIC-062, R9): HR mengisi titik
 * toko lewat peta (OpenLayers + OSM), cari nama tempat/alamat, lokasi
 * perangkat, atau tempel koordinat/link Google Maps, plus override radius
 * absensi. Toko tanpa koordinat membuat absen "Belum dicek".
 */
export function BranchLocationDialog({
  open,
  onOpenChange,
  branches,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  branches: StoreBranch[];
}) {
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [form, setForm] = useState<BranchLocationForm>(EMPTY);
  const [paste, setPaste] = useState("");
  const [minHours, setMinHours] = useState("");
  const save = useSaveBranchLocation();
  const saveHours = useSaveBranchMinWorkHours();
  const missing = branches.filter((b) => !hasCoords(b)).length;
  const selected = branches.find((b) => b.id === selectedId) ?? null;
  const setField = (key: keyof BranchLocationForm) => (e: React.ChangeEvent<HTMLInputElement>) =>
    setForm((f) => ({ ...f, [key]: e.target.value }));

  function choose(b: StoreBranch) {
    setSelectedId(b.id);
    setForm(toForm(b));
    setPaste("");
    setMinHours(b.min_work_hours == null ? "" : String(Number(b.min_work_hours)));
  }

  function applyPaste(text: string) {
    setPaste(text);
    const point = parseLocationText(text);
    if (point) setForm((f) => ({ ...f, lat: String(point.lat), lng: String(point.lng) }));
  }

  const pasteHint =
    paste.trim() === "" || parseLocationText(paste)
      ? ""
      : isShortMapsLink(paste)
        ? "Link pendek tidak memuat koordinat. Buka link itu, lalu salin URL lengkap dari address bar, atau salin angka koordinatnya."
        : "Tidak terbaca sebagai koordinat atau link Google Maps yang memuat koordinat.";

  function submit() {
    if (!selected) return;
    const error = validateBranchLocation(form);
    if (error) {
      toast.error(error);
      return;
    }
    save.mutate(
      { id: selected.id, payload: toBranchLocationPayload(form) },
      {
        onSuccess: () => toast.success(`Lokasi ${selected.name} disimpan`),
        onError: (e) => toast.error(e.message || "Gagal menyimpan lokasi"),
      }
    );
  }

  function submitHours() {
    if (!selected) return;
    const input = minHours.trim();
    const error = input !== "" && !MIN_HOURS_FORMAT.test(input) ? MIN_WORK_HOURS_ERROR : validateMinWorkHours(input);
    if (error) {
      toast.error(error);
      return;
    }
    saveHours.mutate(
      { id: selected.id, payload: toMinWorkHoursPayload(input) },
      {
        onSuccess: () => toast.success(`Jam kerja minimum ${selected.name} disimpan`),
        onError: (e) => toast.error(e.message || "Gagal menyimpan jam kerja minimum"),
      }
    );
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[calc(100dvh-2rem)] overflow-y-auto sm:max-w-4xl">
        <DialogHeader>
          <DialogTitle>Lokasi Toko</DialogTitle>
          <DialogDescription>
            {missing} dari {branches.length} toko belum punya koordinat
          </DialogDescription>
        </DialogHeader>
        <div className="grid gap-4 md:grid-cols-[14rem_1fr]">
          <ul className="max-h-[420px] space-y-1 overflow-y-auto pr-1">
            {branches.map((b) => (
              <li key={b.id}>
                <button
                  type="button"
                  onClick={() => choose(b)}
                  className={`flex w-full items-center justify-between rounded-md px-2 py-1.5 text-left text-sm ${
                    b.id === selectedId ? "bg-blue-50 text-blue-700" : "hover:bg-gray-50"
                  }`}
                >
                  <span className="min-w-0 truncate">
                    {b.name}
                    {b.min_work_hours != null && (
                      <span className="ml-1 text-[10px] text-indigo-600">min {formatHours(b.min_work_hours)} jam</span>
                    )}
                  </span>
                  {hasCoords(b) ? (
                    <MapPin className="ml-2 h-3.5 w-3.5 shrink-0 text-green-600" />
                  ) : (
                    <span className="ml-2 shrink-0 text-xs text-amber-600">kosong</span>
                  )}
                </button>
              </li>
            ))}
          </ul>
          {selected ? (
            <div className="space-y-3">
              {/* Peta tidak dibungkus <label>: klik peta akan diteruskan ke input pertama. */}
              <LocationPickerField
                lat={form.lat}
                lng={form.lng}
                radiusM={resolveRadiusM(Number(form.radius))}
                onPick={(p) => setForm((f) => ({ ...f, lat: String(p.lat), lng: String(p.lng) }))}
                onSearch={searchPlaces}
              />
              <div className="grid gap-3 sm:grid-cols-3">
                <label className="text-xs font-medium text-gray-600">
                  Latitude
                  <Input type="number" step="any" value={form.lat} onChange={setField("lat")} />
                </label>
                <label className="text-xs font-medium text-gray-600">
                  Longitude
                  <Input type="number" step="any" value={form.lng} onChange={setField("lng")} />
                </label>
                <label className="text-xs font-medium text-gray-600">
                  Radius (meter)
                  <Input
                    type="number"
                    min={25}
                    max={1000}
                    step={1}
                    placeholder={String(GEOFENCE_DEFAULT_RADIUS_M)}
                    value={form.radius}
                    onChange={setField("radius")}
                  />
                </label>
              </div>
              <label className="block text-xs font-medium text-gray-600">
                Tempel dari Google Maps
                <Input
                  value={paste}
                  placeholder="-6.888900, 107.596000 atau https://www.google.com/maps/…"
                  onChange={(e) => applyPaste(e.target.value)}
                />
              </label>
              {pasteHint && <p className="text-xs text-amber-700">{pasteHint}</p>}
              <p className="text-xs text-gray-500">
                Radius kosong = {GEOFENCE_DEFAULT_RADIUS_M} m (25–1000). Koordinat yang sama dipakai peta lokasi di
                situs publik.
              </p>
              <div className="rounded-lg border border-gray-100 p-3">
                <label className="block text-xs font-medium text-gray-600">
                  Jam kerja minimum (jam)
                  <Input
                    type="text"
                    inputMode="decimal"
                    placeholder="Kosong = tanpa aturan"
                    value={minHours}
                    onChange={(e) => setMinHours(e.target.value)}
                  />
                </label>
                <p className="mt-1 text-xs text-gray-500">
                  Absen dengan durasi masuk–pulang di bawah angka ini diberi label &quot;Kurang jam / pulang lebih
                  cepat&quot;. Isi 9 untuk kantor pusat (HQ); kosongkan untuk toko. 1–24 jam, kelipatan 0,25. Disimpan
                  terpisah dari lokasi.
                </p>
                <Button size="sm" variant="outline" className="mt-2" disabled={saveHours.isPending} onClick={submitHours}>
                  {saveHours.isPending && <Loader2 className="mr-1.5 h-4 w-4 animate-spin" />}
                  Simpan jam minimum
                </Button>
              </div>
            </div>
          ) : (
            <p className="self-center text-sm text-gray-500">Pilih toko di kiri untuk mengisi lokasinya.</p>
          )}
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            Tutup
          </Button>
          <Button disabled={!selected || save.isPending} onClick={submit}>
            {save.isPending && <Loader2 className="mr-1.5 h-4 w-4 animate-spin" />}
            Simpan lokasi
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
