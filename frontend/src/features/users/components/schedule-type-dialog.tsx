"use client";

import { useEffect, useState } from "react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { SCHEDULE_TYPE_OPTIONS, scheduleTypeConfirmText, type ScheduleType } from "@/lib/hris/schedule-type";
import { fetchScheduleType, saveScheduleType } from "../api";
import { retryWithReason } from "@/lib/hris/delete-reason";

interface Props {
  open: boolean;
  employeeId: string;
  employeeName: string;
  today: string; // YYYY-MM-DD (WIB)
  onOpenChange: (open: boolean) => void;
  onDone: () => void;
}

/** Ubah tipe jadwal satu karyawan; selalu menampilkan dampaknya dulu. */
export function ScheduleTypeDialog({ open, employeeId, employeeName, today, onOpenChange, onDone }: Props) {
  const [current, setCurrent] = useState<ScheduleType | null>(null);
  const [futureDays, setFutureDays] = useState(0);
  const [target, setTarget] = useState<ScheduleType | null>(null);
  const [from, setFrom] = useState(today);
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    if (!open) return;
    let alive = true;
    fetchScheduleType(employeeId).then((res) => {
      if (!alive) return;
      setCurrent(res.data.schedule_type);
      setTarget(res.data.schedule_type);
      setFutureDays(res.data.future_days);
    });
    return () => {
      alive = false;
    };
  }, [open, employeeId]);

  const changed = target !== null && current !== null && target !== current;

  async function save() {
    if (!target || !changed) return;
    setSaving(true);
    try {
      const res = await retryWithReason("Hapus jadwal per tanggal", (reason) =>
        saveScheduleType(employeeId, { schedule_type: target, effective_from: from, ...(reason ? { reason } : {}) })
      );
      res.data.warnings.forEach((w) => toast.warning(w));
      toast.success(res.message ?? "Tipe jadwal disimpan");
      onDone();
      onOpenChange(false);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Gagal menyimpan tipe jadwal");
    } finally {
      setSaving(false);
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle>Tipe jadwal — {employeeName}</DialogTitle>
        </DialogHeader>
        <div className="space-y-4">
          <div role="radiogroup" aria-label="Tipe jadwal" className="flex gap-2">
            {SCHEDULE_TYPE_OPTIONS.map((o) => (
              <label key={o.value} className="flex flex-1 cursor-pointer items-center gap-2 rounded-lg border p-3 text-sm">
                <input type="radio" name="schedule_type" aria-label={o.label} checked={target === o.value} onChange={() => setTarget(o.value)} />
                {o.label}
              </label>
            ))}
          </div>
          <label className="block text-sm">
            Berlaku mulai
            <Input type="date" min={today} value={from} onChange={(e) => setFrom(e.target.value)} className="mt-1" />
          </label>
          {changed ? (
            <p className="rounded-lg bg-warning-soft p-3 text-sm text-warning">{scheduleTypeConfirmText(target, from, target === "non_shift" ? futureDays : 0)}</p>
          ) : null}
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>Batal</Button>
          <Button disabled={!changed || saving || from < today} onClick={save}>Simpan</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
