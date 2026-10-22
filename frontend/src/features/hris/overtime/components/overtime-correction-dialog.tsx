"use client";

import { useState } from "react";
import { toast } from "sonner";
import { Loader2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { OVERTIME_DAY_TYPE_OPTIONS } from "@/lib/hris/overtime-rules";
import { hhmm } from "@/lib/hris/shifts-view";
import { useCorrectOvertime } from "../mutations";
import type { OvertimeDayType, OvertimeRow } from "../types";

interface OvertimeCorrectionDialogProps {
  row: OvertimeRow | null;
  onClose: () => void;
}

/**
 * Koreksi lembur oleh AdminHRIS (MOM AP-04, EPIC-063 R6): tanggal, jam,
 * istirahat (EPIC-066), dan jenis; jam dihitung ulang server, alasan wajib dan
 * tercatat di audit log.
 */
export function OvertimeCorrectionDialog({ row, onClose }: OvertimeCorrectionDialogProps) {
  return (
    <Dialog open={row !== null} onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="max-w-md">{row && <CorrectionForm key={row.id} row={row} onClose={onClose} />}</DialogContent>
    </Dialog>
  );
}

function CorrectionForm({ row, onClose }: { row: OvertimeRow; onClose: () => void }) {
  const correct = useCorrectOvertime();
  const [date, setDate] = useState(row.date.slice(0, 10));
  const [start, setStart] = useState(hhmm(row.start_time, ""));
  const [end, setEnd] = useState(hhmm(row.end_time, ""));
  const storedBreak = Number(row.break_minutes ?? 0);
  const [breakMinutes, setBreakMinutes] = useState(String(storedBreak));
  const [dayType, setDayType] = useState<OvertimeDayType>(row.day_type ?? "regular");
  const [reason, setReason] = useState("");

  function submit() {
    const text = reason.trim();
    if (text.length < 5) {
      toast.error("Alasan koreksi minimal 5 karakter");
      return;
    }
    const brk = Number(breakMinutes || 0);
    if (!Number.isInteger(brk) || brk < 0 || brk > 720) {
      toast.error("Istirahat 0–720 menit");
      return;
    }
    correct.mutate(
      {
        id: row.id,
        date,
        start_time: start,
        end_time: end,
        ...(brk !== storedBreak && { break_minutes: brk }),
        day_type: dayType,
        reason: text,
      },
      {
        onSuccess: (res) => {
          toast.success(res.message || "Lembur dikoreksi");
          onClose();
        },
        onError: (e) => toast.error(e.message || "Gagal mengoreksi lembur"),
      }
    );
  }

  return (
    <>
      <DialogHeader>
        <DialogTitle>Koreksi Lembur — {row.employee?.full_name ?? "Karyawan"}</DialogTitle>
      </DialogHeader>
      <div className="space-y-3">
        <div>
          <label htmlFor="otc-date" className="mb-1 block text-xs font-medium text-gray-600">Tanggal</label>
          <Input id="otc-date" type="date" value={date} onChange={(e) => setDate(e.target.value)} />
        </div>
        <div className="grid grid-cols-2 gap-3">
          <div>
            <label htmlFor="otc-start" className="mb-1 block text-xs font-medium text-gray-600">Jam Mulai</label>
            <Input id="otc-start" type="time" value={start} onChange={(e) => setStart(e.target.value)} />
          </div>
          <div>
            <label htmlFor="otc-end" className="mb-1 block text-xs font-medium text-gray-600">Jam Selesai</label>
            <Input id="otc-end" type="time" value={end} onChange={(e) => setEnd(e.target.value)} />
          </div>
        </div>
        <div>
          <label htmlFor="otc-break" className="mb-1 block text-xs font-medium text-gray-600">Istirahat (menit)</label>
          <Input id="otc-break" type="number" min={0} max={720} step={5} value={breakMinutes}
            onChange={(e) => setBreakMinutes(e.target.value)} />
        </div>
        <div>
          <label htmlFor="otc-day-type" className="mb-1 block text-xs font-medium text-gray-600">Jenis lembur</label>
          <select
            id="otc-day-type"
            value={dayType}
            onChange={(e) => setDayType(e.target.value as OvertimeDayType)}
            className="h-9 w-full rounded-md border border-gray-200 bg-white px-2 text-sm"
          >
            {OVERTIME_DAY_TYPE_OPTIONS.map((option) => (
              <option key={option.value} value={option.value}>
                {option.label}
              </option>
            ))}
          </select>
        </div>
        <div>
          <label htmlFor="otc-reason" className="mb-1 block text-xs font-medium text-gray-600">Alasan koreksi</label>
          <Textarea id="otc-reason" rows={3} maxLength={500} value={reason} onChange={(e) => setReason(e.target.value)} />
        </div>
      </div>
      <DialogFooter>
        <Button variant="outline" onClick={onClose}>
          Batal
        </Button>
        <Button onClick={submit} disabled={correct.isPending}>
          {correct.isPending ? <Loader2 className="h-4 w-4 animate-spin" /> : "Simpan koreksi"}
        </Button>
      </DialogFooter>
    </>
  );
}
