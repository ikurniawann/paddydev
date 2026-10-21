"use client";

import { useState } from "react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { todayWib } from "@/lib/dates";
import {
  EMPTY_ATTENDANCE_REQUEST_FORM,
  toAttendanceRequestPayload,
  validateAttendanceRequestForm,
  type AttendanceRequestForm,
} from "@/lib/hris/attendance-requests";
import { useCreateAttendanceRequest } from "../mutations";

/** Isian awal: dari kartu "Absen terlewat" (tanggal + petunjuk) atau tombol umum (hari ini). */
export interface AttendanceRequestPrefill {
  date: string;
  hint?: string;
}

const MAX_PHOTO_BYTES = 5 * 1024 * 1024;

/**
 * ESS → Absensi → Ajukan absen manual / koreksi (EPIC-065, MOM AB-04/AB-06):
 * tanggal (hari ini atau sebelumnya di periode terbuka), jam yang benar,
 * keterangan wajib, foto bukti opsional. HR yang memutus. Dirender hanya
 * saat terbuka, sehingga setiap pembukaan mulai dari isian awal.
 */
export function AttendanceRequestDialog({ prefill, onClose }: { prefill: AttendanceRequestPrefill; onClose: () => void }) {
  const [form, setForm] = useState<AttendanceRequestForm>({ ...EMPTY_ATTENDANCE_REQUEST_FORM, date: prefill.date });
  const [photo, setPhoto] = useState<{ name: string; dataUrl: string } | null>(null);
  const create = useCreateAttendanceRequest();
  const set =
    (key: keyof AttendanceRequestForm) => (e: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement>) =>
      setForm((f) => ({ ...f, [key]: e.target.value }));

  function pickPhoto(e: React.ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0];
    e.target.value = "";
    if (!file) return;
    if (!file.type.startsWith("image/")) {
      toast.error("Lampiran harus berupa gambar JPG/PNG/WebP");
      return;
    }
    if (file.size > MAX_PHOTO_BYTES) {
      toast.error("Ukuran lampiran maksimal 5 MB");
      return;
    }
    const reader = new FileReader();
    reader.onloadend = () => setPhoto({ name: file.name, dataUrl: reader.result as string });
    reader.onerror = () => toast.error("Gagal membaca foto bukti");
    reader.readAsDataURL(file);
  }

  function submit() {
    const error = validateAttendanceRequestForm(form, todayWib());
    if (error) {
      toast.error(error);
      return;
    }
    create.mutate(toAttendanceRequestPayload(form, photo?.dataUrl), {
      onSuccess: (json) => {
        toast.success(json.message || "Pengajuan absensi terkirim");
        onClose();
      },
      onError: (e) => toast.error(e.message || "Gagal mengirim pengajuan absensi"),
    });
  }

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="max-h-[90dvh] overflow-y-auto">
        <DialogHeader>
          <DialogTitle>Ajukan absen manual / koreksi</DialogTitle>
          <DialogDescription>
            Untuk absen yang terlewat atau jamnya salah (mis. internet mati). HR yang menyetujui; jam yang disetujui
            menggantikan catatan absen.
          </DialogDescription>
        </DialogHeader>
        <div className="min-w-0 space-y-3">
          {prefill.hint && <p className="rounded-md bg-amber-50 px-3 py-2 text-xs text-amber-800">{prefill.hint}</p>}
          <label className="block min-w-0 text-xs font-medium text-gray-600">
            Tanggal
            <Input type="date" max={todayWib()} value={form.date} onChange={set("date")} />
          </label>
          <div className="grid grid-cols-2 gap-3">
            <label className="min-w-0 text-xs font-medium text-gray-600">
              Jam masuk
              <Input type="time" value={form.clock_in} onChange={set("clock_in")} />
            </label>
            <label className="min-w-0 text-xs font-medium text-gray-600">
              Jam pulang
              <Input type="time" value={form.clock_out} onChange={set("clock_out")} />
            </label>
          </div>
          <p className="text-xs text-gray-500">
            Kosongkan jam yang sudah benar. Bila belum ada absen sama sekali, jam masuk wajib diisi.
          </p>
          <label className="block min-w-0 text-xs font-medium text-gray-600">
            Keterangan
            <Textarea
              rows={3}
              maxLength={1000}
              placeholder="Mis. internet toko mati saat jam masuk"
              value={form.reason}
              onChange={set("reason")}
            />
          </label>
          <label className="block min-w-0 text-xs font-medium text-gray-600">
            Foto bukti (opsional, maks 5 MB)
            <Input type="file" accept="image/jpeg,image/png,image/webp" onChange={pickPhoto} />
          </label>
          {photo && (
            <p className="flex min-w-0 items-center gap-2 text-xs text-gray-500">
              <span className="min-w-0 truncate">📎 {photo.name}</span>
              <button type="button" className="shrink-0 text-blue-600 underline" onClick={() => setPhoto(null)}>
                Hapus
              </button>
            </p>
          )}
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={onClose}>
            Batal
          </Button>
          <Button disabled={create.isPending} onClick={submit}>
            Kirim pengajuan
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
