"use client";

import { useEffect, useState } from "react";
import { PlusIcon, PencilIcon, TrashIcon, MoonIcon } from "@heroicons/react/24/outline";
import { Loader2 } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { hhmm } from "@/lib/hris/shifts-view";
import { useDeleteShift } from "../mutations";
import { useShifts } from "../queries";
import type { ShiftRow } from "../types";
import { ShiftFormDialog } from "./shift-form-dialog";

/**
 * HRIS → Kepegawaian → Shift Kerja: master shift (jam kerja + toleransi
 * terlambat) yang menjadi acuan jadwal per karyawan dan perhitungan
 * keterlambatan absensi.
 */
export function ShiftsPage() {
  const { data: shifts = [], isLoading, isError } = useShifts();
  const remove = useDeleteShift();
  const [dialogOpen, setDialogOpen] = useState(false);
  const [editing, setEditing] = useState<ShiftRow | null>(null);

  useEffect(() => {
    if (isError) toast.error("Gagal memuat shift");
  }, [isError]);

  function openDialog(shift: ShiftRow | null) {
    setEditing(shift);
    setDialogOpen(true);
  }

  function handleDelete(shift: ShiftRow) {
    if (!confirm(`Hapus shift "${shift.name}"?`)) return;
    remove.mutate(shift.id, {
      onSuccess: () => toast.success("Shift dihapus / dinonaktifkan"),
      onError: (error) => toast.error(error.message || "Gagal menghapus"),
    });
  }

  return (
    <div className="space-y-5">
      <div className="flex items-center justify-between border-b border-gray-200/70 pb-4">
        <div>
          <h1 className="text-2xl font-bold text-gray-900">Shift Kerja</h1>
          <p className="text-sm text-gray-500">
            Master jam kerja — dipakai jadwal per karyawan & perhitungan keterlambatan absen
          </p>
        </div>
        <Button className="gap-2" onClick={() => openDialog(null)}>
          <PlusIcon className="h-4 w-4" /> Tambah Shift
        </Button>
      </div>

      {isLoading ? (
        <div className="flex justify-center py-14">
          <Loader2 className="h-6 w-6 animate-spin text-gray-400" />
        </div>
      ) : (
        <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
          {shifts.map((shift) => (
            <div
              key={shift.id}
              className={`rounded-xl border bg-white p-4 shadow-sm ${
                shift.is_active ? "border-gray-200/70" : "border-dashed border-gray-300 opacity-60"
              }`}
            >
              <div className="flex items-start justify-between">
                <div>
                  <p className="flex items-center gap-1.5 font-semibold text-gray-900">
                    <span className="rounded-md bg-gray-100 px-1.5 py-0.5 font-mono text-xs text-gray-700">
                      {shift.code}
                    </span>
                    {shift.name}
                    {shift.is_default && (
                      <span className="rounded-full bg-info-soft px-2 py-0.5 text-xs text-info">Bawaan</span>
                    )}
                    {shift.is_overnight && <MoonIcon className="h-4 w-4 text-indigo-500" />}
                    {!shift.is_active && (
                      <span className="rounded-full bg-gray-100 px-2 py-0.5 text-xs">Nonaktif</span>
                    )}
                  </p>
                  <p className="mt-1 text-2xl font-bold text-gray-900">
                    {hhmm(shift.start_time)}–{hhmm(shift.end_time)}
                  </p>
                  <p className="mt-1 text-xs text-gray-500">
                    Istirahat {shift.break_minutes} mnt · toleransi terlambat{" "}
                    {shift.late_tolerance_minutes} mnt
                    {shift.is_overnight ? " · lewat tengah malam" : ""}
                  </p>
                </div>
                <div className="flex gap-1">
                  <Button size="sm" variant="ghost" onClick={() => openDialog(shift)}>
                    <PencilIcon className="h-4 w-4" />
                  </Button>
                  {/* DEF/NS dipakai jadwal bawaan & jadwal tetap Non-Shift: tidak bisa dihapus */}
                  {shift.code !== "DEF" && shift.code !== "NS" && (
                    <Button
                      size="sm"
                      variant="ghost"
                      className="text-red-600"
                      onClick={() => handleDelete(shift)}
                    >
                      <TrashIcon className="h-4 w-4" />
                    </Button>
                  )}
                </div>
              </div>
            </div>
          ))}
          {shifts.length === 0 && (
            <p className="col-span-full py-10 text-center text-sm text-gray-400">
              Belum ada shift — tambahkan dulu untuk mengatur jadwal karyawan.
            </p>
          )}
        </div>
      )}

      <ShiftFormDialog open={dialogOpen} shift={editing} onOpenChange={setDialogOpen} />
    </div>
  );
}
