"use client";

import { useState } from "react";
import { Loader2 } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { useStoreErrorText } from "@/features/hris/schedule-access/access-guard";
import { fetchScheduleGrid } from "../api";
import { CELLS_EMPLOYEE_LIMIT, CELLS_EMPLOYEE_LIMIT_MESSAGE, CELLS_LIMIT, useScheduleCellsSave } from "../mutations";
import { coverChunks, expandPattern, mergeGrids } from "../schedule-cells";
import { addDays, dayDiff } from "../schedule-range";
import type { ScheduleShift } from "../types";
import { CellsReviewPanel } from "./cells-confirm-dialog";
import { hhmm } from "./shift-picker";

const DAYS = ["Senin", "Selasa", "Rabu", "Kamis", "Jumat", "Sabtu", "Minggu"];
/** Rentang terpanjang yang diterima server per simpan. */
export const PATTERN_MAX_DAYS = 62;
const CLEAR = "__clear";

interface Props {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  branchId: string;
  employees: { id: string; name: string }[];
  shifts: ScheduleShift[];
  /** Tanggal mulai bawaan (awal minggu yang tampil, tidak sebelum hari ini). */
  defaultFrom: string;
  today: string;
  onSaved: () => void;
}

/** Jam kerja bersih satu shift dalam menit: durasi (lewat tengah malam bila selesai <= mulai) dikurangi istirahat. */
function netMinutes(s: ScheduleShift): number {
  const toMin = (t: string) => {
    const [h, m] = t.split(":").map(Number);
    return h * 60 + m;
  };
  const start = toMin(s.start_time);
  let end = toMin(s.end_time);
  if (end <= start) end += 24 * 60;
  return end - start - (s.break_minutes ?? 0);
}

function rangeProblem(from: string, to: string, today: string): string | null {
  if (!from || !to) return "Pilih tanggal mulai dan selesai";
  if (from < today) return "Tanggal mulai tidak boleh sebelum hari ini";
  const span = dayDiff(from, to) + 1;
  if (span < 1) return "Tanggal selesai harus sesudah tanggal mulai";
  if (span > PATTERN_MAX_DAYS) return `Rentang maksimal ${PATTERN_MAX_DAYS} hari`;
  return null;
}

/**
 * Terapkan pola mingguan (Senin–Minggu) ke karyawan yang dicentang untuk satu
 * rentang tanggal. Kotak yang tidak ditempatkan dilewati; hasilnya selalu
 * dipratinjau dulu sebelum disimpan.
 */
export function PatternDialog({ open, onOpenChange, branchId, employees, shifts, defaultFrom, today, onSaved }: Props) {
  const errorText = useStoreErrorText();
  const [pattern, setPattern] = useState<string[]>(() => DAYS.map(() => ""));
  const [from, setFrom] = useState(defaultFrom);
  const [to, setTo] = useState(() => addDays(defaultFrom, 27));
  const [loading, setLoading] = useState(false);
  const saver = useScheduleCellsSave(branchId, () => {
    onSaved();
    onOpenChange(false);
  });

  const problem = rangeProblem(from, to, today);
  const tooMany = employees.length > CELLS_EMPLOYEE_LIMIT;
  const busy = loading || saver.busy;
  // Perkiraan atas (sebelum kotak "-" dilewati) supaya ditolak sebelum memuat apa pun.
  const cellCount = problem ? 0 : employees.length * (dayDiff(from, to) + 1);
  const tooManyCells = cellCount > CELLS_LIMIT;
  const ready = !problem && !tooMany && !tooManyCells && pattern.every(Boolean) && !busy;
  // Dialog ini hanya dibuka dari grid, yang hanya memuat staf shift.
  const byCode = new Map(shifts.map((s) => [s.code, s]));
  const complete = pattern.every(Boolean);
  const hasClear = pattern.includes(CLEAR); // hari ikut pola mingguan: jamnya tidak diketahui
  const weekMinutes = pattern.reduce((sum, v) => sum + (byCode.has(v) ? netMinutes(byCode.get(v)!) : 0), 0);
  const noDayOff = complete && pattern.every((v) => byCode.has(v));
  const names = employees.slice(0, 5).map((e) => e.name).join(", ") + (employees.length > 5 ? `, dan ${employees.length - 5} lainnya` : "");

  async function apply() {
    if (!ready) return;
    setLoading(true);
    try {
      const grids = await Promise.all(
        coverChunks(from, to).map((c) => fetchScheduleGrid({ branch_id: branchId, ...c }).then((res) => res.data))
      );
      const codes = pattern.map((v) => (v === CLEAR ? "" : v));
      const cells = expandPattern(
        employees.map((e) => e.id),
        codes,
        from,
        to,
        mergeGrids(grids)
      );
      await saver.save(cells, { confirm: true });
    } catch (err) {
      toast.error(errorText.fail(err, "Gagal memuat jadwal"));
    } finally {
      setLoading(false);
    }
  }

  return (
    <Dialog open={open} onOpenChange={(next) => (!busy ? onOpenChange(next) : undefined)}>
      <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-xl">
        <DialogHeader>
          <DialogTitle>Terapkan pola mingguan</DialogTitle>
          <DialogDescription>
            Untuk {employees.length} karyawan: {names}
          </DialogDescription>
        </DialogHeader>

        {saver.review ? (
          <CellsReviewPanel
            preview={saver.review.preview}
            ack={saver.ack}
            onAck={saver.setAck}
            reason={saver.reason}
            onReason={saver.setReason}
          />
        ) : (
          <div className="space-y-4 text-sm">
            {tooMany ? <p className="font-medium text-danger">{CELLS_EMPLOYEE_LIMIT_MESSAGE}</p> : null}
            <div className="grid grid-cols-2 gap-x-4 gap-y-2 sm:grid-cols-[auto_1fr]">
              {DAYS.map((day, i) => (
                <label key={day} className="contents">
                  <span className="self-center text-gray-700">{day}</span>
                  <select
                    aria-label={day}
                    value={pattern[i]}
                    onChange={(e) => setPattern((prev) => prev.map((v, idx) => (idx === i ? e.target.value : v)))}
                    className="h-9 rounded-lg border border-input bg-transparent px-2 text-sm"
                  >
                    <option value="">Pilih…</option>
                    {shifts.map((s) => (
                      <option key={s.code} value={s.code}>
                        {`${s.code} · ${s.name} ${hhmm(s.start_time)}–${hhmm(s.end_time)}`}
                      </option>
                    ))}
                    <option value="L">L · Libur</option>
                    <option value={CLEAR}>Kosongkan (ikut pola mingguan)</option>
                  </select>
                </label>
              ))}
            </div>
            <div className="flex flex-wrap gap-3">
              <label className="text-xs font-medium text-gray-600">
                Dari
                <Input
                  type="date"
                  aria-label="Dari tanggal"
                  min={today}
                  value={from}
                  onChange={(e) => setFrom(e.target.value)}
                  className="mt-1 w-40"
                />
              </label>
              <label className="text-xs font-medium text-gray-600">
                Sampai
                <Input
                  type="date"
                  aria-label="Sampai tanggal"
                  min={from}
                  max={from ? addDays(from, PATTERN_MAX_DAYS - 1) : undefined}
                  value={to}
                  onChange={(e) => setTo(e.target.value)}
                  className="mt-1 w-40"
                />
              </label>
            </div>
            {complete && !hasClear ? (
              <p className="text-xs text-gray-600">{`Total ±${Math.round(weekMinutes / 60)} jam kerja/minggu`}</p>
            ) : null}
            {noDayOff ? (
              <p className="rounded-lg border border-amber-200 bg-amber-50 px-3 py-2 text-xs text-amber-800">
                Belum ada hari libur — sistem shift wajib libur 1 hari setiap minggu
              </p>
            ) : null}
            {problem ? <p className="text-sm text-danger">{problem}</p> : null}
            {!tooMany && tooManyCells ? (
              <p className="text-sm text-danger">
                {`${employees.length} karyawan × ${dayDiff(from, to) + 1} hari = ${cellCount} kotak, maksimal ${CELLS_LIMIT} sekali simpan. Perkecil rentang tanggal atau jumlah karyawan.`}
              </p>
            ) : null}
            <p className="text-xs text-gray-500">
              Tanggal saat karyawan tidak ditempatkan di cabang ini dilewati. Maksimal {PATTERN_MAX_DAYS} hari sekali simpan.
            </p>
          </div>
        )}

        <DialogFooter>
          {saver.review ? (
            <>
              <Button variant="outline" onClick={saver.cancel} disabled={saver.busy}>
                Ubah pola
              </Button>
              <Button disabled={!saver.canSave} onClick={() => void saver.confirm()}>
                {saver.busy ? <Loader2 className="h-4 w-4 animate-spin" /> : null}
                Simpan jadwal
              </Button>
            </>
          ) : (
            <>
              <Button variant="outline" onClick={() => onOpenChange(false)} disabled={busy}>
                Batal
              </Button>
              <Button disabled={!ready} onClick={() => void apply()}>
                {busy ? <Loader2 className="h-4 w-4 animate-spin" /> : null}
                Terapkan
              </Button>
            </>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
