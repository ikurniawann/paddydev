"use client";

import { useState } from "react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { SCHEDULE_TYPE_OPTIONS, scheduleTypeConfirmText, type ScheduleType } from "@/lib/hris/schedule-type";
import { bulkScheduleType, type ScheduleTypeResult } from "../api";
import { retryWithReason } from "@/lib/hris/delete-reason";

interface Props {
  open: boolean;
  ids: string[];
  today: string;
  onOpenChange: (open: boolean) => void;
  onDone: () => void;
}

/** Ubah massal tipe jadwal: dampak dicek dulu (dry run), baru disimpan. */
export function BulkScheduleTypeDialog({ open, ids, today, onOpenChange, onDone }: Props) {
  const [target, setTarget] = useState<ScheduleType | null>(null);
  const [from, setFrom] = useState(today);
  const [impact, setImpact] = useState<ScheduleTypeResult | null>(null);
  const [busy, setBusy] = useState(false);

  const body = (dryRun: boolean) => ({ employee_ids: ids, schedule_type: target as ScheduleType, effective_from: from, dry_run: dryRun });

  async function check() {
    if (!target) return;
    setBusy(true);
    try {
      setImpact((await bulkScheduleType(body(true))).data);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Gagal memeriksa dampak");
    } finally {
      setBusy(false);
    }
  }

  async function save() {
    if (!target || !impact) return;
    setBusy(true);
    try {
      const res = await retryWithReason("Hapus jadwal per tanggal", (reason) =>
        bulkScheduleType({ ...body(false), ...(reason ? { reason } : {}) })
      );
      res.data.warnings.forEach((w) => toast.warning(w));
      toast.success(res.message ?? "Tipe jadwal disimpan");
      onDone();
      onOpenChange(false);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Gagal menyimpan tipe jadwal");
    } finally {
      setBusy(false);
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle>Set tipe jadwal — {ids.length} karyawan</DialogTitle>
        </DialogHeader>
        <div className="space-y-4 text-sm">
          <div role="radiogroup" aria-label="Tipe jadwal" className="flex gap-2">
            {SCHEDULE_TYPE_OPTIONS.map((o) => (
              <label key={o.value} className="flex flex-1 cursor-pointer items-center gap-2 rounded-lg border p-3">
                <input
                  type="radio"
                  name="bulk_schedule_type"
                  aria-label={o.label}
                  checked={target === o.value}
                  onChange={() => {
                    setTarget(o.value);
                    setImpact(null);
                  }}
                />
                {o.label}
              </label>
            ))}
          </div>
          <label className="block">
            Berlaku mulai
            <Input
              type="date"
              min={today}
              value={from}
              onChange={(e) => {
                setFrom(e.target.value);
                setImpact(null);
              }}
              className="mt-1"
            />
          </label>
          {impact && target ? (
            <div className="space-y-1 rounded-lg bg-warning-soft p-3 text-warning">
              <p>
                {impact.changed} karyawan berubah tipe ({ids.length - impact.changed} sudah {target === "shift" ? "Shift" : "Non-Shift"}).
              </p>
              {impact.changed > 0 ? <p>{scheduleTypeConfirmText(target, from, impact.cleared_days)}</p> : null}
            </div>
          ) : null}
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            Batal
          </Button>
          {impact ? (
            <Button disabled={busy || impact.changed === 0} onClick={save}>
              Simpan
            </Button>
          ) : (
            <Button disabled={busy || !target || from < today} onClick={check}>
              Periksa dampak
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
