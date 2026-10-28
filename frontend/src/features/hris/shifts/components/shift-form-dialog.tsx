"use client";

import { useState } from "react";
import { Loader2 } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter } from "@/components/ui/dialog";
import { EMPTY_SHIFT_FORM, shiftCodeError, shiftFormFrom, shiftPayload } from "@/lib/hris/shifts-view";
import { useSaveShift } from "../mutations";
import { isProtectedShiftCode, shiftSummary, suggestShiftName } from "../shift-list";
import type { ShiftRow } from "../types";

interface ShiftFormDialogProps {
  open: boolean;
  shift: ShiftRow | null;
  onOpenChange: (open: boolean) => void;
}

export function ShiftFormDialog({ open, shift, onOpenChange }: ShiftFormDialogProps) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-sm">
        <DialogHeader>
          <DialogTitle>{shift ? `Edit ${shift.name}` : "Tambah Shift"}</DialogTitle>
        </DialogHeader>
        {/* Konten dialog dipasang ulang tiap dibuka, jadi state form selalu segar. */}
        <ShiftFormBody key={shift?.id ?? "new"} shift={shift} onClose={() => onOpenChange(false)} />
      </DialogContent>
    </Dialog>
  );
}

function ShiftFormBody({ shift, onClose }: { shift: ShiftRow | null; onClose: () => void }) {
  const [form, setForm] = useState(() => (shift ? shiftFormFrom(shift) : EMPTY_SHIFT_FORM));
  const save = useSaveShift();
  const codeError = shiftCodeError(form.code);
  const suggestedName = suggestShiftName(form.code, form.start_time, form.end_time);
  const summary = shiftSummary(form.start_time, form.end_time, form.late_tolerance_minutes, form.is_overnight);
  const used = shift?.employees_using ?? 0;

  function handleSave() {
    if (codeError) {
      toast.error(codeError);
      return;
    }
    // Nama kosong → pakai saran dari kode + jam.
    const name = form.name.trim() || suggestedName;
    if (!name) {
      toast.error("Nama shift wajib diisi");
      return;
    }
    save.mutate(
      { payload: shiftPayload({ ...form, name }), id: shift?.id },
      {
        onSuccess: () => {
          toast.success(shift ? "Shift diperbarui" : "Shift dibuat");
          onClose();
        },
        onError: (error) => toast.error(error.message || "Gagal menyimpan"),
      }
    );
  }

  return (
    <>
      <div className="space-y-3">
        <div>
          <label htmlFor="shift-code" className="mb-1 block text-xs font-medium text-gray-600">
            Kode <span className="text-danger">*</span>
          </label>
          <Input
            id="shift-code"
            placeholder="cth. SPG1"
            value={form.code}
            maxLength={12}
            disabled={!!shift && isProtectedShiftCode(shift.code)}
            onChange={(e) => setForm((f) => ({ ...f, code: e.target.value.toUpperCase() }))}
          />
          <p className={`mt-1 text-xs ${form.code && codeError ? "text-danger" : "text-gray-500"}`}>
            {form.code && codeError ? codeError : "Dipakai di file upload Jadwal Kerja"}
          </p>
        </div>
        <div>
          <label htmlFor="shift-name" className="mb-1 block text-xs font-medium text-gray-600">
            Nama shift
          </label>
          <Input
            id="shift-name"
            placeholder={suggestedName || "cth. Shift Pagi"}
            value={form.name}
            onChange={(e) => setForm((f) => ({ ...f, name: e.target.value }))}
          />
          {!form.name.trim() && suggestedName && (
            <p className="mt-1 text-xs text-gray-500">Dikosongkan = dipakai “{suggestedName}”</p>
          )}
        </div>
        <div className="grid grid-cols-2 gap-3">
          <div>
            <label htmlFor="shift-start" className="mb-1 block text-xs font-medium text-gray-600">Jam mulai</label>
            <Input
              id="shift-start"
              type="time"
              value={form.start_time}
              onChange={(e) => setForm((f) => ({ ...f, start_time: e.target.value }))}
            />
          </div>
          <div>
            <label htmlFor="shift-end" className="mb-1 block text-xs font-medium text-gray-600">Jam selesai</label>
            <Input
              id="shift-end"
              type="time"
              value={form.end_time}
              onChange={(e) => setForm((f) => ({ ...f, end_time: e.target.value }))}
            />
          </div>
        </div>
        <div className="grid grid-cols-2 gap-3">
          <div>
            <label htmlFor="shift-break" className="mb-1 block text-xs font-medium text-gray-600">Istirahat (menit)</label>
            <Input
              id="shift-break"
              type="number"
              value={form.break_minutes}
              onChange={(e) => setForm((f) => ({ ...f, break_minutes: e.target.value }))}
            />
          </div>
          <div>
            <label htmlFor="shift-tolerance" className="mb-1 block text-xs font-medium text-gray-600">
              Toleransi terlambat (menit)
            </label>
            <Input
              id="shift-tolerance"
              type="number"
              value={form.late_tolerance_minutes}
              onChange={(e) => setForm((f) => ({ ...f, late_tolerance_minutes: e.target.value }))}
            />
          </div>
        </div>
        <label className="flex items-center gap-2 text-sm text-gray-700">
          <input
            type="checkbox"
            checked={form.is_overnight}
            onChange={(e) => setForm((f) => ({ ...f, is_overnight: e.target.checked }))}
          />
          Shift malam (jam selesai jatuh keesokan hari)
        </label>
        {summary && <p className="rounded-lg bg-gray-50 px-3 py-2 text-sm text-gray-700">{summary}</p>}
        {shift && used > 0 && (
          <p className="rounded-lg bg-warning-soft px-3 py-2 text-sm text-warning">
            Dipakai {used} karyawan. Perubahan jam langsung berlaku di jadwal mereka.
          </p>
        )}
      </div>
      <DialogFooter>
        <Button variant="outline" onClick={onClose}>
          Batal
        </Button>
        <Button onClick={handleSave} disabled={save.isPending || !!codeError}>
          {save.isPending ? <Loader2 className="h-4 w-4 animate-spin" /> : "Simpan"}
        </Button>
      </DialogFooter>
    </>
  );
}
