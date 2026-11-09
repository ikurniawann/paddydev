"use client";

import { useState } from "react";
import { LocateFixed, MapPin } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Textarea } from "@/components/ui/textarea";
import { GEOFENCE_NOTE_MIN, type GeofenceNoteInfo } from "@/lib/hris/geofence";

/**
 * Popup ESS saat server meminta keterangan absen (MOM AB-03):
 * - di luar area toko: absen tetap boleh, tercatat "Di luar area";
 * - lokasi HP tidak akurat (lokasi perkiraan ±2000 m): panduan menyalakan
 *   Lokasi Akurat lalu Coba lagi, atau tetap absen dengan keterangan dan
 *   tercatat "Lokasi tidak akurat".
 * Radius yang ditampilkan berasal dari server (radius_m), bukan angka tetap.
 */
export function GeofenceNoteDialog({
  info,
  pending,
  onSubmit,
  onCancel,
  onRetry,
}: {
  info: GeofenceNoteInfo | null;
  pending: boolean;
  onSubmit: (note: string) => void;
  onCancel: () => void;
  /** Ambil lokasi ulang dan kirim ulang tanpa keterangan (hanya untuk lokasi tidak akurat). */
  onRetry?: () => void;
}) {
  const [note, setNote] = useState("");
  const trimmed = note.trim();
  const place = info?.branchName ?? "toko";
  const imprecise = info?.reason === "imprecise";
  const close = () => {
    setNote("");
    onCancel();
  };
  return (
    <Dialog open={info !== null} onOpenChange={(open) => !open && close()}>
      <DialogContent className="max-h-[90dvh] overflow-y-auto">
        <DialogHeader>
          {imprecise ? (
            <>
              <DialogTitle className="flex items-center gap-2">
                <LocateFixed className="h-5 w-5 text-amber-600" /> Lokasi HP tidak akurat
              </DialogTitle>
              <DialogDescription>
                HP Anda mengirim lokasi perkiraan
                {info?.accuracyM != null ? ` (±${info.accuracyM} m)` : ""}, jadi jarak ke {place} tidak bisa
                dipastikan. Nyalakan <b>Lokasi Akurat</b>, lalu tekan Coba lagi.
              </DialogDescription>
            </>
          ) : (
            <>
              <DialogTitle className="flex items-center gap-2">
                <MapPin className="h-5 w-5 text-red-600" /> Anda di luar area toko
              </DialogTitle>
              <DialogDescription>
                {info?.distanceM != null ? `Posisi Anda ±${info.distanceM} m dari ${place}` : `Posisi Anda di luar area ${place}`}
                {info ? ` (batas ${info.radiusM} m)` : ""}. Absen tetap tercatat dengan status <b>Di luar area</b> dan
                ditinjau HRD.
              </DialogDescription>
            </>
          )}
        </DialogHeader>
        {imprecise && <PreciseLocationGuide />}
        <label htmlFor="geofence-note" className="text-sm font-medium text-gray-700">
          Keterangan
        </label>
        <Textarea
          id="geofence-note"
          value={note}
          onChange={(e) => setNote(e.target.value)}
          placeholder={imprecise ? "Mis. sudah di lantai 1 HQ, GPS HP lemah" : "Mis. mengambil stok di gudang, antar barang ke pelanggan"}
          rows={3}
        />
        <p className="text-xs text-gray-500">
          Minimal {GEOFENCE_NOTE_MIN} karakter.
          {imprecise ? " Bila tetap absen, tercatat Lokasi tidak akurat dan ditinjau HRD." : ""}
        </p>
        <DialogFooter className="gap-2">
          <Button variant="outline" onClick={close}>
            Batal
          </Button>
          {imprecise && onRetry && (
            <Button
              variant="outline"
              disabled={pending}
              onClick={() => {
                setNote("");
                onRetry();
              }}
            >
              Coba lagi
            </Button>
          )}
          <Button
            disabled={pending || [...trimmed].length < GEOFENCE_NOTE_MIN}
            onClick={() => {
              onSubmit(trimmed);
              setNote("");
            }}
          >
            Tetap absen
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

/** Langkah menyalakan lokasi akurat untuk browser di Android dan iPhone. */
function PreciseLocationGuide() {
  return (
    <div className="space-y-3 rounded-md bg-amber-50 p-3 text-sm text-gray-700">
      <div>
        <p className="font-semibold">Android</p>
        <ol className="ml-4 list-decimal space-y-0.5">
          <li>Buka Setelan → Lokasi, pastikan Lokasi aktif dan Akurasi Lokasi Google menyala.</li>
          <li>
            Setelan → Aplikasi → Chrome (atau browser yang dipakai) → Izin → Lokasi: pilih Izinkan hanya saat aplikasi
            digunakan dan nyalakan <b>Gunakan lokasi akurat</b>.
          </li>
        </ol>
      </div>
      <div>
        <p className="font-semibold">iPhone</p>
        <ol className="ml-4 list-decimal space-y-0.5">
          <li>Buka Pengaturan → Privasi &amp; Keamanan → Layanan Lokasi, pastikan aktif.</li>
          <li>
            Pilih Situs Web Safari (atau Chrome bila memakai Chrome): pilih Saat Menggunakan App dan nyalakan{" "}
            <b>Lokasi Akurat</b>.
          </li>
        </ol>
      </div>
    </div>
  );
}
