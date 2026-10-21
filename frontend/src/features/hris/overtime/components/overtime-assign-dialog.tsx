"use client";

import { useState } from "react";
import { ClockIcon } from "@heroicons/react/24/outline";
import { Loader2 } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Combobox } from "@/components/ui/combobox";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter } from "@/components/ui/dialog";
import { OVERTIME_DAY_TYPE_OPTIONS } from "@/lib/hris/overtime-rules";
import { EMPTY_OVERTIME_ASSIGN_FORM, validateOvertimeAssignForm } from "@/lib/hris/overtime-view";
import { useAssignOvertime } from "../mutations";
import { useEmployeeOptions } from "@/features/hris/shared/use-employee-options";

interface OvertimeAssignDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

export function OvertimeAssignDialog({ open, onOpenChange }: OvertimeAssignDialogProps) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle>Tugaskan Lembur (dari Perusahaan)</DialogTitle>
        </DialogHeader>
        <OvertimeAssignBody onClose={() => onOpenChange(false)} />
      </DialogContent>
    </Dialog>
  );
}

function OvertimeAssignBody({ onClose }: { onClose: () => void }) {
  const [form, setForm] = useState(EMPTY_OVERTIME_ASSIGN_FORM);
  const { data: employeeOptions = [] } = useEmployeeOptions();
  const assign = useAssignOvertime();

  function handleAssign() {
    const invalid = validateOvertimeAssignForm(form);
    if (invalid) {
      toast.error(invalid);
      return;
    }
    assign.mutate(form, {
      onSuccess: (res) => {
        toast.success(res.message || "Penugasan lembur dibuat");
        onClose();
      },
      onError: (error) => toast.error(error.message || "Gagal membuat penugasan"),
    });
  }

  return (
    <>
      <div className="space-y-3">
        <div>
          <label className="mb-1 block text-xs font-medium text-gray-600">Karyawan</label>
          <Combobox
            options={employeeOptions}
            value={form.employee_id}
            onChange={(value) => setForm((f) => ({ ...f, employee_id: value }))}
            placeholder="Pilih karyawan"
          />
        </div>
        <div>
          <label className="mb-1 block text-xs font-medium text-gray-600">Tanggal</label>
          <Input
            type="date"
            value={form.date}
            onChange={(e) => setForm((f) => ({ ...f, date: e.target.value }))}
          />
        </div>
        <div className="grid grid-cols-2 gap-3">
          <div>
            <label className="mb-1 block text-xs font-medium text-gray-600">Jam Mulai</label>
            <Input
              type="time"
              value={form.start_time}
              onChange={(e) => setForm((f) => ({ ...f, start_time: e.target.value }))}
            />
          </div>
          <div>
            <label className="mb-1 block text-xs font-medium text-gray-600">Jam Selesai</label>
            <Input
              type="time"
              value={form.end_time}
              onChange={(e) => setForm((f) => ({ ...f, end_time: e.target.value }))}
            />
          </div>
        </div>
        <div>
          <label className="mb-1 block text-xs font-medium text-gray-600">Jenis lembur</label>
          <select
            value={form.day_type ?? "regular"}
            onChange={(e) => setForm((f) => ({ ...f, day_type: e.target.value as typeof f.day_type }))}
            className="w-full rounded-md border border-gray-200 px-3 py-2 text-sm"
          >
            {OVERTIME_DAY_TYPE_OPTIONS.map((option) => (
              <option key={option.value} value={option.value}>
                {option.label}
              </option>
            ))}
          </select>
        </div>
        <div>
          <label className="mb-1 block text-xs font-medium text-gray-600">
            Pekerjaan yang dilembur (min. 5 karakter)
          </label>
          <Input
            placeholder="cth. stock opname akhir bulan"
            value={form.reason}
            onChange={(e) => setForm((f) => ({ ...f, reason: e.target.value }))}
          />
        </div>
        <p className="rounded-lg bg-amber-50 px-3 py-2 text-xs text-amber-700">
          <ClockIcon className="mr-1 inline h-3.5 w-3.5" />
          Penugasan menunggu konfirmasi karyawan di halaman ESS Lembur.
          Jam lembur dibayar sesuai realisasi absensi.
        </p>
      </div>
      <DialogFooter>
        <Button variant="outline" onClick={onClose}>
          Batal
        </Button>
        <Button onClick={handleAssign} disabled={assign.isPending}>
          {assign.isPending ? <Loader2 className="h-4 w-4 animate-spin" /> : "Buat Penugasan"}
        </Button>
      </DialogFooter>
    </>
  );
}
