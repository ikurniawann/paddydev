"use client";

import { useEffect, useState } from "react";
import { toast } from "sonner";
import { Clock, Loader2, MapPin, Store } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { CameraCapture } from "@/components/hris/CameraCapture";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { formatTime } from "@/lib/format";
import { GeofenceBadge } from "@/features/hris/attendance/components/geofence-badge";
import { GEOFENCE_DEFAULT_RADIUS_M, geofenceLabel, type GeofenceNoteInfo } from "@/lib/hris/geofence";
import { preferredClockStore } from "@/lib/hris/placements";
import { useClockPlacements } from "@/features/hris/placements/queries";
import { useTodayAttendance } from "../queries";
import { ClockGeofenceError } from "../api";
import { useSubmitClock } from "../mutations";
import type { EssClockLocation, EssClockPayload } from "../types";
import { GeofenceNoteDialog } from "./geofence-note-dialog";

/**
 * Panel clock-in/out ESS: selfie kamera WAJIB + lokasi GPS.
 * Alur: klik tombol → kamera (capture+konfirmasi) → ambil GPS → POST.
 * Toko clock-in: otomatis bila karyawan hanya punya satu penempatan aktif;
 * lebih dari satu → wajib pilih (bawaan: toko terdekat dari GPS bila koordinat
 * toko diisi, selain itu penempatan utama). Toko menentukan pembagian biaya
 * gaji per toko.
 * Di luar area: server menjawab 422, popup meminta keterangan, payload
 * (termasuk selfie) dikirim ulang.
 * State absensi hari ini dipulihkan dari server (tahan refresh). Setelah
 * berhasil, mutasi menyegarkan beranda dan kalender absensi.
 */

type ClockAction = EssClockPayload["action"];

function getLocation(): Promise<EssClockLocation | null> {
  return new Promise((resolve) => {
    if (!navigator.geolocation) return resolve(null);
    navigator.geolocation.getCurrentPosition(
      (pos) =>
        resolve({
          latitude: pos.coords.latitude,
          longitude: pos.coords.longitude,
          accuracy: pos.coords.accuracy,
        }),
      () => resolve(null),
      { enableHighAccuracy: true, timeout: 10000, maximumAge: 0 }
    );
  });
}

export function EssClockPanel() {
  const todayQuery = useTodayAttendance();
  const today = todayQuery.data ?? null;
  const clock = useSubmitClock();
  const [locating, setLocating] = useState(false);
  const [cameraFor, setCameraFor] = useState<ClockAction | null>(null);
  const [noteFor, setNoteFor] = useState<{ payload: EssClockPayload; info: GeofenceNoteInfo } | null>(null);
  const submitting = locating || clock.isPending;
  const clockedIn = Boolean(today?.clock_in);
  const storesQuery = useClockPlacements(undefined, !clockedIn);
  const storeData = storesQuery.data;
  const stores = storeData?.placements ?? [];
  const [pickedStore, setPickedStore] = useState<string | null>(null);
  const [nearestStore, setNearestStore] = useState<string | null>(null);

  // Lebih dari satu toko dan koordinat toko tersedia: pilih toko terdekat dari GPS.
  useEffect(() => {
    const options = storeData?.placements ?? [];
    if (clockedIn || options.length < 2 || !options.some((s) => s.lat !== null && s.lng !== null)) return;
    let cancelled = false;
    getLocation().then((loc) => {
      if (!cancelled && loc) {
        setNearestStore(preferredClockStore(options, storeData?.default_branch_id ?? null, loc));
      }
    });
    return () => {
      cancelled = true;
    };
  }, [clockedIn, storeData]);

  const storeId =
    pickedStore ?? nearestStore ?? preferredClockStore(stores, storesQuery.data?.default_branch_id ?? null, null);
  const needsStore = stores.length > 1 && !storeId;

  async function submit(action: ClockAction, photo: string) {
    setCameraFor(null);
    setLocating(true);
    const location = await getLocation();
    setLocating(false);

    const payload: EssClockPayload = { action, photo };
    if (action === "clock-in") {
      if (location) payload.clock_in_location = location;
      if (storeId) payload.branch_id = storeId;
    } else {
      payload.attendance_id = today?.id;
      if (location) payload.clock_out_location = location;
    }

    send(payload);
  }

  function send(payload: EssClockPayload) {
    clock.mutate(payload, {
      onSuccess: (json) => {
        const data = json.data;
        const late = data?.is_late ? ` — terlambat ${data.late_minutes} menit` : "";
        const geo =
          payload.action === "clock-in"
            ? geofenceLabel(data?.clock_in_geofence, data?.clock_in_distance_m)
            : geofenceLabel(data?.clock_out_geofence, data?.clock_out_distance_m);
        toast.success(payload.action === "clock-in" ? "✅ Clock-in berhasil" : "✅ Clock-out berhasil", {
          description: `${formatTime(new Date())}${late}${geo ? ` · 📍 ${geo}` : ""}`,
        });
      },
      onError: (error) => {
        if (error instanceof ClockGeofenceError) {
          setNoteFor({ payload, info: error.info });
          return;
        }
        toast.error("Gagal", { description: error.message || "Coba lagi" });
      },
    });
  }

  const clockedOut = Boolean(today?.clock_out);

  return (
    <div className="space-y-3">
      {!clockedIn && stores.length === 1 && (
        <p className="flex items-center gap-1.5 text-sm text-gray-600">
          <Store className="h-4 w-4 text-gray-400" /> Toko: <b>{stores[0].branch_name}</b>
        </p>
      )}
      {!clockedIn && stores.length > 1 && (
        <div className="max-w-sm">
          <label className="mb-1 flex items-center gap-1.5 text-xs font-medium text-gray-600">
            <Store className="h-3.5 w-3.5" /> Toko tempat Anda bekerja hari ini <span className="text-red-500">*</span>
          </label>
          <Select value={storeId ?? ""} onValueChange={setPickedStore}>
            <SelectTrigger className="h-11">
              <SelectValue placeholder="Pilih toko" />
            </SelectTrigger>
            <SelectContent>
              {stores.map((s) => (
                <SelectItem key={s.branch_id} value={s.branch_id}>
                  {s.branch_name}
                  {s.is_primary ? " (utama)" : ""}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
      )}
      <div className="flex flex-wrap items-center gap-3">
        <Button
          className="h-12 bg-green-600 px-6 hover:bg-green-700"
          disabled={todayQuery.isLoading || submitting || clockedIn || needsStore || storesQuery.isLoading}
          onClick={() => setCameraFor("clock-in")}
        >
          {submitting && cameraFor !== "clock-out" ? (
            <Loader2 className="mr-2 h-4 w-4 animate-spin" />
          ) : (
            <Clock className="mr-2 h-4 w-4" />
          )}
          Clock In
        </Button>
        <Button
          variant="outline"
          className="h-12 px-6"
          disabled={todayQuery.isLoading || submitting || !clockedIn || clockedOut}
          onClick={() => setCameraFor("clock-out")}
        >
          <Clock className="mr-2 h-4 w-4" /> Clock Out
        </Button>
        <p className="flex items-center gap-1 text-xs text-gray-500">
          <MapPin className="h-3.5 w-3.5" /> Foto selfie wajib; aktifkan GPS agar lokasi bisa dicek (radius toko {GEOFENCE_DEFAULT_RADIUS_M} m)
        </p>
      </div>

      {today && clockedIn && (
        <div className="flex flex-wrap items-center gap-2 text-sm text-gray-600">
          <span>
            Masuk <b>{formatTime(today.clock_in)}</b>
            {today.branch ? <> di <b>{today.branch.name}</b></> : null}
            {clockedOut ? (
              <>
                {" "}· Pulang <b>{formatTime(today.clock_out)}</b>
              </>
            ) : null}
          </span>
          {today.is_late ? (
            <Badge className="bg-red-100 text-red-700 hover:bg-red-100">
              Terlambat {today.late_minutes} mnt
            </Badge>
          ) : (
            <Badge className="bg-green-100 text-green-700 hover:bg-green-100">Tepat waktu</Badge>
          )}
          <GeofenceBadge status={today.clock_in_geofence} distanceM={today.clock_in_distance_m} prefix="Masuk" />
          <GeofenceBadge status={today.clock_out_geofence} distanceM={today.clock_out_distance_m} prefix="Pulang" />
          {clockedOut && (
            <Badge variant="outline" className="text-green-700">
              Absensi hari ini selesai
            </Badge>
          )}
        </div>
      )}

      <CameraCapture
        open={cameraFor !== null}
        title={cameraFor === "clock-out" ? "Selfie Clock-Out" : "Selfie Clock-In"}
        onConfirm={(photo) => cameraFor && submit(cameraFor, photo)}
        onCancel={() => setCameraFor(null)}
      />
      <GeofenceNoteDialog
        info={noteFor?.info ?? null}
        pending={clock.isPending}
        onCancel={() => setNoteFor(null)}
        onSubmit={(note) => {
          const pending = noteFor;
          setNoteFor(null);
          if (pending) send({ ...pending.payload, notes: note });
        }}
      />
    </div>
  );
}
