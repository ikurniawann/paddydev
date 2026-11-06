"use client";

import { useState } from "react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { resignEmployee } from "../api";

/** Jenis resign; dipakai juga banner resign terjadwal (EPIC-095). */
export const RESIGN_TYPES = [
  { value: "resign", label: "Resign (mengundurkan diri)" },
  { value: "end_of_contract", label: "Kontrak berakhir" },
  { value: "terminated", label: "Diberhentikan (PHK)" },
] as const;
const TYPES = RESIGN_TYPES;

/** Paling jauh 180 hari dari hari ini (EPIC-095 R2). */
const MAX_AHEAD_DAYS = 180;

const shiftDays = (iso: string, days: number) => {
  const d = new Date(`${iso}T00:00:00Z`);
  d.setUTCDate(d.getUTCDate() + days);
  return d.toISOString().slice(0, 10);
};

/** dd-mm-yyyy, sama dengan pesan server. */
export const resignDateText = (iso: string) => iso.split("-").reverse().join("-");

interface Props {
  open: boolean;
  employeeId: string;
  employeeName: string;
  joinDate: string;
  today: string;
  onOpenChange: (open: boolean) => void;
  onDone: () => void;
}

export function ResignDialog({ open, employeeId, employeeName, joinDate, today, onOpenChange, onDone }: Props) {
  const [date, setDate] = useState(today);
  const [type, setType] = useState<(typeof TYPES)[number]["value"]>("resign");
  const [reason, setReason] = useState("");
  const [saving, setSaving] = useState(false);
  const maxDate = shiftDays(today, MAX_AHEAD_DAYS);
  // Tanggal efektif = hari terakhir kerja; setelah hari ini → resign terjadwal (EPIC-095).
  const scheduled = !!date && date > today;
  const dateError = !date
    ? "Tanggal efektif wajib diisi"
    : date < joinDate
      ? "Tidak boleh sebelum tanggal masuk"
      : date > maxDate
        ? "Tanggal efektif paling jauh 180 hari dari hari ini"
        : null;

  async function submit() {
    if (dateError) return;
    setSaving(true);
    try {
      const res = await resignEmployee(employeeId, { effective_date: date, resign_type: type, reason: reason.trim() || null });
      toast.success(res.message ?? "Karyawan diresign");
      onDone();
      onOpenChange(false);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Gagal memproses resign");
    } finally {
      setSaving(false);
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle>Resign — {employeeName}</DialogTitle>
        </DialogHeader>
        <div className="space-y-4 text-sm">
          {scheduled && !dateError ? (
            <p className="rounded-lg bg-sky-50 p-3 text-sky-800">
              Karyawan tetap aktif sampai {resignDateText(date)}. Akun dinonaktifkan dan login diblokir otomatis pada{" "}
              {resignDateText(shiftDays(date, 1))} 00.00 WIB.
            </p>
          ) : (
            <p className="rounded-lg bg-danger-soft p-3 text-danger">
              Akun login {employeeName} akan diblokir, penempatan dan jadwal ditutup per tanggal efektif. Data lama tetap tersimpan.
            </p>
          )}
          <label className="block">
            Tanggal efektif *
            <Input type="date" min={joinDate} max={maxDate} value={date} onChange={(e) => setDate(e.target.value)} className="mt-1" />
            {dateError ? <span className="text-danger">{dateError}</span> : null}
          </label>
          <fieldset className="space-y-1">
            <legend>Jenis *</legend>
            {TYPES.map((t) => (
              <label key={t.value} className="flex items-center gap-2">
                <input type="radio" name="resign_type" checked={type === t.value} onChange={() => setType(t.value)} />
                {t.label}
              </label>
            ))}
          </fieldset>
          <label className="block">
            Alasan
            <textarea aria-label="Alasan" value={reason} maxLength={500} onChange={(e) => setReason(e.target.value)} className="mt-1 w-full rounded-lg border p-2" rows={3} />
          </label>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>Batal</Button>
          <Button variant="destructive" disabled={!!dateError || saving} onClick={submit}>
            {scheduled ? "Jadwalkan resign" : "Resign"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
