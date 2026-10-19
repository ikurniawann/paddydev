"use client";

import { useState } from "react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { applyFixedSchedule } from "@/features/users/api";
import { dmy } from "@/lib/hris/schedule-type";

interface Props {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  branchId: string;
  branchName: string;
  today: string;
  onDone: () => void;
}

/** Jadwal tetap Sen–Jum 09.00–18.00 untuk semua karyawan Non-Shift di cabang. */
export function FixedScheduleDialog({ open, onOpenChange, branchId, branchName, today, onDone }: Props) {
  const [from, setFrom] = useState(today);
  const [busy, setBusy] = useState(false);

  async function apply() {
    setBusy(true);
    try {
      const res = await applyFixedSchedule({ branch_id: branchId, effective_from: from });
      toast.success(res.message ?? "Jadwal tetap diterapkan");
      onDone();
      onOpenChange(false);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Gagal menerapkan jadwal tetap");
    } finally {
      setBusy(false);
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle>Terapkan jadwal tetap — {branchName}</DialogTitle>
        </DialogHeader>
        <div className="space-y-3 text-sm">
          <label className="block">
            Berlaku mulai
            <Input type="date" min={today} value={from} onChange={(e) => setFrom(e.target.value)} className="mt-1" />
          </label>
          <p className="rounded-lg bg-info-soft p-3 text-info">
            Semua karyawan Non-Shift di {branchName} mendapat jadwal Senin–Jumat 09.00–18.00, Sabtu–Minggu libur, mulai{" "}
            {dmy(from)}.
          </p>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            Batal
          </Button>
          <Button disabled={busy || from < today} onClick={apply}>
            Terapkan
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
