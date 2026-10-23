"use client";

import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { OvertimeAttachmentButtons } from "@/components/hris/overtime-attachment-preview";
import { paidOvertimeHours, type OvertimeTimes } from "@/lib/hris/overtime-rules";

export interface OvertimeApproveRow {
  id: string;
  date: string;
  start_time: string;
  end_time: string;
  break_minutes: number;
  attachments: string[];
  employee_name: string;
}

export type OvertimeApproveTimes = OvertimeTimes;

const hhmm = (v: string) => v.slice(0, 5);

/** Atasan langsung menyetujui input lembur, boleh mengubah jam & istirahat dulu (EPIC-066). */
export function OvertimeApproveDialog({
  row,
  open,
  onOpenChange,
  onApprove,
}: {
  row: OvertimeApproveRow;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onApprove: (times: OvertimeApproveTimes) => void;
}) {
  const [start, setStart] = useState(hhmm(row.start_time));
  const [end, setEnd] = useState(hhmm(row.end_time));
  const [brk, setBrk] = useState(String(row.break_minutes ?? 0));
  const [error, setError] = useState("");
  const hours = paidOvertimeHours(start, end, Number(brk || 0));

  function approve() {
    const breakMinutes = Number(brk || 0);
    if (!Number.isInteger(breakMinutes) || breakMinutes < 0 || breakMinutes > 720) {
      setError("Istirahat 0–720 menit");
      return;
    }
    if (hours < 1) {
      setError("Durasi lembur minimal 1 jam");
      return;
    }
    setError("");
    onApprove({ start_time: start, end_time: end, break_minutes: breakMinutes });
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>Setujui lembur {row.employee_name}</DialogTitle>
        </DialogHeader>
        <div className="space-y-3 text-sm">
          <OvertimeAttachmentButtons paths={row.attachments} buttonClassName="text-sm text-blue-600 underline" />
          <div className="grid grid-cols-3 gap-2">
            <label className="text-xs font-medium text-gray-600">
              Jam Mulai
              <Input type="time" value={start} onChange={(e) => setStart(e.target.value)} />
            </label>
            <label className="text-xs font-medium text-gray-600">
              Jam Selesai
              <Input type="time" value={end} onChange={(e) => setEnd(e.target.value)} />
            </label>
            <label className="text-xs font-medium text-gray-600">
              Istirahat (menit)
              <Input type="number" min={0} max={720} step={5} value={brk} onChange={(e) => setBrk(e.target.value)} />
            </label>
          </div>
          <p className="text-xs text-gray-600">{hours.toLocaleString("id-ID")} jam dibayar</p>
          {error && <p role="alert" className="text-xs text-red-600">{error}</p>}
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>Batal</Button>
          <Button onClick={approve}>Setujui</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
