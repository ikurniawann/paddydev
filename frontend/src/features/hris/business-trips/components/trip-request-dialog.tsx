"use client";

import { useState } from "react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Combobox } from "@/components/ui/combobox";
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { todayWib } from "@/lib/dates";
import { EMPTY_TRIP_FORM, tripDays, validateTripForm, type TripForm } from "@/lib/hris/business-trips";
import { useCreateBusinessTrip } from "../mutations";

/**
 * Form Perjalanan Dinas. Tanpa `employees`: pengajuan karyawan sendiri
 * (menunggu atasan). Dengan `employees` (HRD): pilih karyawan, langsung disetujui.
 */
export function TripRequestDialog({
  open,
  onOpenChange,
  employees,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  employees?: { value: string; label: string }[];
}) {
  const [form, setForm] = useState<TripForm>(EMPTY_TRIP_FORM);
  const [employeeId, setEmployeeId] = useState("");
  const create = useCreateBusinessTrip();
  const set = <K extends keyof TripForm>(k: K, v: TripForm[K]) => setForm((f) => ({ ...f, [k]: v }));
  const days = tripDays(form.start_date, form.end_date);

  function handleOpenChange(next: boolean) {
    if (!next) {
      setForm(EMPTY_TRIP_FORM);
      setEmployeeId("");
    }
    onOpenChange(next);
  }

  function submit() {
    if (employees && !employeeId) {
      toast.error("Pilih karyawan");
      return;
    }
    const error = validateTripForm(form, todayWib());
    if (error) {
      toast.error(error);
      return;
    }
    create.mutate(
      { ...form, destination: form.destination.trim(), purpose: form.purpose.trim(), ...(employees ? { employee_id: employeeId } : {}) },
      {
        onSuccess: (json) => {
          toast.success(json.message || "Perjalanan Dinas tersimpan");
          handleOpenChange(false);
        },
        onError: (e) => toast.error(e.message || "Gagal menyimpan Perjalanan Dinas"),
      }
    );
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{employees ? "Catat Perjalanan Dinas" : "Ajukan Perjalanan Dinas"}</DialogTitle>
        </DialogHeader>
        <div className="space-y-3">
          {employees && (
            <Combobox
              options={employees}
              value={employeeId}
              onChange={setEmployeeId}
              placeholder="Pilih karyawan"
              searchPlaceholder="Cari nama/NIP…"
              emptyMessage="Karyawan tidak ditemukan"
              className="!w-full"
            />
          )}
          <div className="grid grid-cols-2 gap-3">
            <label className="text-xs font-medium text-gray-600">
              Tanggal mulai
              <Input type="date" min={todayWib()} value={form.start_date} onChange={(e) => set("start_date", e.target.value)} />
            </label>
            <label className="text-xs font-medium text-gray-600">
              Tanggal selesai
              <Input type="date" min={form.start_date || todayWib()} value={form.end_date} onChange={(e) => set("end_date", e.target.value)} />
            </label>
          </div>
          {days > 0 && <p className="text-xs text-gray-500">{days} hari · absen di tanggal ini tidak dicek radius toko</p>}
          <label className="block text-xs font-medium text-gray-600">
            Tujuan
            <Input value={form.destination} maxLength={200} placeholder="Mis. Gudang Cikarang" onChange={(e) => set("destination", e.target.value)} />
          </label>
          <label className="block text-xs font-medium text-gray-600">
            Keperluan
            <Textarea value={form.purpose} maxLength={1000} rows={3} onChange={(e) => set("purpose", e.target.value)} />
          </label>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => handleOpenChange(false)}>
            Batal
          </Button>
          <Button disabled={create.isPending} onClick={submit}>
            {employees ? "Simpan" : "Kirim pengajuan"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
