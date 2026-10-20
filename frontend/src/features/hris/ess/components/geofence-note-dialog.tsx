"use client";

import { useState } from "react";
import { MapPin } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Textarea } from "@/components/ui/textarea";
import { GEOFENCE_NOTE_MIN, type GeofenceNoteInfo } from "@/lib/hris/geofence";

/**
 * Popup ESS saat server menilai absen di luar area toko (MOM AB-03): absen
 * tetap boleh, tercatat "Di luar area", dan wajib keterangan untuk HRD.
 * Radius yang ditampilkan berasal dari server (radius_m), bukan angka tetap.
 */
export function GeofenceNoteDialog({
  info,
  pending,
  onSubmit,
  onCancel,
}: {
  info: GeofenceNoteInfo | null;
  pending: boolean;
  onSubmit: (note: string) => void;
  onCancel: () => void;
}) {
  const [note, setNote] = useState("");
  const trimmed = note.trim();
  const place = info?.branchName ?? "toko";
  return (
    <Dialog
      open={info !== null}
      onOpenChange={(open) => {
        if (!open) {
          setNote("");
          onCancel();
        }
      }}
    >
      <DialogContent>
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <MapPin className="h-5 w-5 text-red-600" /> Anda di luar area toko
          </DialogTitle>
          <DialogDescription>
            {info?.distanceM != null ? `Posisi Anda ±${info.distanceM} m dari ${place}` : `Posisi Anda di luar area ${place}`}
            {info ? ` (batas ${info.radiusM} m)` : ""}. Absen tetap tercatat dengan status <b>Di luar area</b> dan
            ditinjau HRD.
          </DialogDescription>
        </DialogHeader>
        <label htmlFor="geofence-note" className="text-sm font-medium text-gray-700">
          Keterangan
        </label>
        <Textarea
          id="geofence-note"
          value={note}
          onChange={(e) => setNote(e.target.value)}
          placeholder="Mis. mengambil stok di gudang, antar barang ke pelanggan"
          rows={3}
        />
        <p className="text-xs text-gray-500">Minimal {GEOFENCE_NOTE_MIN} karakter.</p>
        <DialogFooter>
          <Button
            variant="outline"
            onClick={() => {
              setNote("");
              onCancel();
            }}
          >
            Batal
          </Button>
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
