"use client";

import { useMemo, useState, useSyncExternalStore } from "react";
import { CalendarRange, ChevronDown, Copy, Eraser, Loader2, MousePointerClick, PaintBucket, X } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { useStoreErrorText } from "@/features/hris/schedule-access/access-guard";
import { fetchScheduleGrid } from "../api";
import { CELLS_EMPLOYEE_LIMIT, CELLS_EMPLOYEE_LIMIT_MESSAGE, useScheduleCellsSave } from "../mutations";
import { cellKey, copyWeek, fillCells, parseCellKey, rowRange, shiftTone } from "../schedule-cells";
import { addDays, dm, weekdayIndex } from "../schedule-range";
import type { ScheduleGrid as Grid } from "../types";
import { CellsConfirmDialog } from "./cells-confirm-dialog";
import { PatternDialog } from "./pattern-dialog";
import { ShiftPicker, hhmm } from "./shift-picker";

const HATCH = { backgroundImage: "repeating-linear-gradient(135deg, rgb(229 231 235) 0 3px, transparent 3px 7px)" };

interface Props {
  grid: Grid;
  branchId: string;
  today: string;
  /** Karyawan yang dicentang dari halaman Penempatan (yang tidak ada di grid diabaikan). */
  initialChecked?: string[];
  /** Grid minggu baru sedang dimuat (data lama masih tampil): kotak dikunci. */
  loading?: boolean;
}

const WIDE_QUERY = "(min-width: 640px)";

/** Server render, hidrasi, dan peramban tanpa matchMedia dianggap layar lebar (legenda terbuka). */
function useWideScreen(): boolean {
  return useSyncExternalStore(
    (onChange) => {
      if (typeof window.matchMedia !== "function") return () => {};
      const list = window.matchMedia(WIDE_QUERY);
      list.addEventListener("change", onChange);
      return () => list.removeEventListener("change", onChange);
    },
    () => (typeof window.matchMedia === "function" ? window.matchMedia(WIDE_QUERY).matches : true),
    () => true
  );
}

interface Picker {
  anchor: Element;
  title: string;
  keys: string[];
}

/**
 * Grid Jadwal Kerja yang bisa diedit (EPIC-070): klik kotak untuk ubah satu
 * hari; Ctrl/Cmd-klik dan Shift-klik, atau tombol "Pilih beberapa kotak"
 * (untuk layar sentuh), untuk memilih banyak kotak; centang karyawan untuk
 * Terapkan pola. Tanggal lewat dan kotak "-" tidak bisa diubah.
 */
export function ScheduleGrid({ grid, branchId, today, initialChecked, loading = false }: Props) {
  const errorText = useStoreErrorText();
  const [checked, setChecked] = useState(() => new Set(initialChecked ?? []));
  const [selected, setSelected] = useState<Set<string>>(() => new Set());
  const [anchorKey, setAnchorKey] = useState<string | null>(null);
  const [picker, setPicker] = useState<Picker | null>(null);
  const [patternOpen, setPatternOpen] = useState(false);
  const [copying, setCopying] = useState(false);
  /** Mode sentuh: ketuk kotak = pilih/batal pilih (setara Ctrl-klik). */
  const [multiSelect, setMultiSelect] = useState(false);
  /** Legenda kode shift: bawaan ikut ukuran layar (terbuka di ≥640px, tertutup di HP); ketukan pengguna menimpanya. */
  const wide = useWideScreen();
  const [legendChoice, setLegendChoice] = useState<boolean | null>(null);
  const legendOpen = legendChoice ?? wide;
  const [reviewTitle, setReviewTitle] = useState("Periksa perubahan jadwal");
  const saver = useScheduleCellsSave(branchId, () => setSelected(new Set()));

  const dates = useMemo(() => grid.dates.map((d) => d.date), [grid.dates]);
  /** Kotak yang bisa diubah, berurutan per baris lalu tanggal. */
  const editableKeys = useMemo(() => {
    const keys: string[] = [];
    for (const e of grid.employees) for (const c of e.cells) if (c.code !== "-" && c.date >= today) keys.push(cellKey(e.id, c.date));
    return keys;
  }, [grid.employees, today]);
  const editable = useMemo(() => new Set(editableKeys), [editableKeys]);
  const selectedKeys = editableKeys.filter((k) => selected.has(k));
  const checkedRows = grid.employees.filter((e) => checked.has(e.id));
  const allChecked = grid.employees.length > 0 && checkedRows.length === grid.employees.length;
  const missing = (initialChecked ?? []).filter((id) => !grid.employees.some((e) => e.id === id));
  const shiftByCode = new Map(grid.shifts.map((s) => [s.code, s]));
  const busy = saver.busy || copying || loading;

  function toggleRow(id: string) {
    setChecked((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }

  function toggleAll() {
    setChecked(allChecked ? new Set() : new Set(grid.employees.map((e) => e.id)));
  }

  function clearSelection() {
    setSelected(new Set());
    setChecked(new Set());
    setAnchorKey(null);
  }

  function toggleMultiSelect() {
    if (multiSelect) {
      setSelected(new Set());
      setAnchorKey(null);
    }
    setMultiSelect(!multiSelect);
  }

  function save(cells: Parameters<typeof saver.save>[0], title: string, confirm = false) {
    setReviewTitle(title);
    void saver.save(cells, { confirm });
  }

  function onCell(e: React.MouseEvent<HTMLButtonElement>, employee: Grid["employees"][number], date: string) {
    const key = cellKey(employee.id, date);
    const additive = e.ctrlKey || e.metaKey || multiSelect;
    if (e.shiftKey && anchorKey && parseCellKey(anchorKey).employee_id === employee.id) {
      const range = rowRange(employee.id, dates, parseCellKey(anchorKey).date, date).filter((k) => editable.has(k));
      setSelected((prev) => new Set([...(additive ? prev : []), ...range]));
      return;
    }
    setAnchorKey(key);
    if (additive || e.shiftKey) {
      setSelected((prev) => {
        const next = new Set(prev);
        if (next.has(key)) next.delete(key);
        else next.add(key);
        return next;
      });
      return;
    }
    setSelected(new Set());
    setPicker({ anchor: e.currentTarget, title: `${employee.name} · ${dm(date)}`, keys: [key] });
  }

  function pick(code: string) {
    if (!picker) return;
    const many = picker.keys.length > 1;
    setPicker(null);
    save(fillCells(picker.keys, code), many ? `Isi ${picker.keys.length} kotak` : "Ubah jadwal");
  }

  async function copyLastWeek() {
    const from = dates[0];
    setCopying(true);
    try {
      const prev = await fetchScheduleGrid({ branch_id: branchId, from: addDays(from, -7), to: addDays(from, -1) });
      const cells = copyWeek(prev.data, grid, today);
      if (cells.length === 0) {
        toast.info("Tidak ada jadwal minggu lalu yang perlu disalin");
        return;
      }
      setReviewTitle(`Salin jadwal ${dm(addDays(from, -7))}–${dm(addDays(from, -1))}`);
      await saver.save(cells, { confirm: true });
    } catch (err) {
      toast.error(errorText.fail(err, "Gagal memuat jadwal minggu lalu"));
    } finally {
      setCopying(false);
    }
  }

  const offDay = (d: Grid["dates"][number]) => d.holiday !== null || weekdayIndex(d.date) === 6;
  const tooMany = checkedRows.length > CELLS_EMPLOYEE_LIMIT;
  const hasSelection = checkedRows.length > 0 || selectedKeys.length > 0;

  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0 flex-1 space-y-2">
          <button
            type="button"
            aria-expanded={legendOpen}
            aria-controls="schedule-legend"
            onClick={() => setLegendChoice(!legendOpen)}
            className="flex items-center gap-1 text-xs font-medium text-gray-700 hover:text-gray-900"
          >
            <ChevronDown className={`h-3.5 w-3.5 transition-transform ${legendOpen ? "" : "-rotate-90"}`} aria-hidden />
            Kode shift ({grid.shifts.length})
          </button>
          {legendOpen ? (
            <ul id="schedule-legend" className="flex flex-wrap items-center gap-x-3 gap-y-1.5 text-xs text-gray-600">
              {grid.shifts.map((s) => (
                <li key={s.code} className="flex items-center gap-1.5">
                  <span className={`rounded px-1.5 py-0.5 font-mono font-semibold ${shiftTone(s.code)}`}>{s.code}</span>
                  {s.name} {hhmm(s.start_time)}–{hhmm(s.end_time)}
                </li>
              ))}
              <li className="flex items-center gap-1.5">
                <span className={`rounded px-1.5 py-0.5 font-mono font-semibold ${shiftTone("L")}`}>L</span>Libur
              </li>
              <li className="flex items-center gap-1.5">
                <span className={`rounded px-1.5 py-0.5 ${shiftTone("")}`}>Belum ada</span>belum dijadwalkan
              </li>
              <li className="flex items-center gap-1.5">
                <span className="inline-block h-4 w-6 rounded border border-gray-200" style={HATCH} />
                tidak ditempatkan di cabang ini
              </li>
              <li className="flex items-center gap-1.5">
                <span className="underline decoration-dotted">P</span>dari pola mingguan
              </li>
            </ul>
          ) : null}
        </div>
        <div className="flex flex-wrap gap-2">
          <Button variant={multiSelect ? "default" : "outline"} size="sm" aria-pressed={multiSelect} onClick={toggleMultiSelect}>
            <MousePointerClick /> Pilih beberapa kotak
          </Button>
          <Button variant="outline" size="sm" onClick={() => void copyLastWeek()} disabled={busy}>
            {copying ? <Loader2 className="animate-spin" /> : <Copy />} Salin minggu lalu
          </Button>
        </div>
      </div>

      <p className="flex items-center gap-1.5 text-xs text-gray-500">
        {loading ? <Loader2 className="h-3.5 w-3.5 animate-spin" aria-label="Memuat jadwal" /> : null}
        Klik kotak untuk mengubah satu hari. Tahan Ctrl (Mac: Cmd) untuk memilih beberapa kotak, atau Shift untuk memilih rentang dalam satu baris.
      </p>
      <p className="text-xs text-gray-500 sm:hidden">Ketuk Pilih beberapa kotak untuk mengisi banyak hari sekaligus.</p>
      {missing.length > 0 ? (
        <p className="rounded-lg bg-warning-soft px-3 py-2 text-sm text-warning">
          {missing.length} karyawan yang dipilih dari Penempatan tidak tampil karena tidak ditempatkan di cabang ini pada tanggal ini.
        </p>
      ) : null}

      <div className="overflow-x-auto rounded-xl border border-gray-200/70 bg-white">
        <table className="w-full border-separate border-spacing-0 text-xs">
          <thead>
            <tr className="bg-gray-50 text-gray-500">
              <th className="sticky left-0 z-10 min-w-32 border-b border-gray-200 bg-gray-50 sm:min-w-56 px-3 py-2 text-left">
                <label className="flex items-center gap-2 font-medium">
                  <input type="checkbox" aria-label="Pilih semua karyawan" checked={allChecked} onChange={toggleAll} />
                  Karyawan
                </label>
              </th>
              {grid.dates.map((d) => (
                <th
                  key={d.date}
                  title={d.holiday ?? undefined}
                  className={`whitespace-nowrap border-b border-gray-200 px-1 py-2 text-center font-medium ${
                    offDay(d) ? "bg-danger-soft text-danger" : ""
                  } ${d.date < today ? "opacity-50" : ""} ${d.date === today ? "text-gray-900 underline underline-offset-4" : ""}`}
                >
                  {d.label}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {grid.employees.map((e) => (
              <tr key={e.id}>
                <td className="sticky left-0 z-10 min-w-32 border-b border-gray-100 bg-white px-3 py-1.5 sm:min-w-56">
                  <label className="flex max-w-32 items-center gap-2 sm:max-w-none">
                    <input type="checkbox" aria-label={`Pilih ${e.name}`} checked={checked.has(e.id)} onChange={() => toggleRow(e.id)} />
                    <span className="min-w-0">
                      <span className="block truncate font-medium text-gray-800" title={e.name}>
                        {e.name}
                      </span>
                      <span className="block truncate text-[11px] text-gray-400">
                        {e.nip}
                        {e.position ? ` · ${e.position}` : ""}
                      </span>
                    </span>
                  </label>
                </td>
                {e.cells.map((c) => {
                  const key = cellKey(e.id, c.date);
                  const shift = shiftByCode.get(c.code);
                  const meaning =
                    c.code === "" ? "Belum ada jadwal" : c.code === "L" ? "Libur" : shift ? `${shift.name} ${hhmm(shift.start_time)}–${hhmm(shift.end_time)}` : c.code;
                  const title = `${meaning}${c.source === "weekly" ? " (dari pola mingguan)" : ""}`;
                  const content = c.code === "" ? <span className="text-[10px] font-sans">Belum ada</span> : c.code;
                  const base = "flex h-8 w-full min-w-11 items-center justify-center rounded-md font-mono text-xs font-semibold";
                  const weekly = c.source === "weekly" ? "underline decoration-dotted underline-offset-2" : "";
                  return (
                    <td key={c.date} className="border-b border-gray-100 p-0.5">
                      {c.code === "-" ? (
                        <span title="Tidak ditempatkan di cabang ini" className={`${base} text-transparent`} style={HATCH}>
                          -
                        </span>
                      ) : !editable.has(key) ? (
                        <span title={`${title} · tanggal sudah lewat`} className={`${base} ${shiftTone(c.code)} ${weekly} opacity-45`}>
                          {content}
                        </span>
                      ) : (
                        <button
                          type="button"
                          aria-label={`${e.name} ${dm(c.date)}`}
                          aria-pressed={selected.has(key)}
                          title={title}
                          disabled={busy}
                          onClick={(ev) => onCell(ev, e, c.date)}
                          className={`${base} ${shiftTone(c.code)} ${weekly} cursor-pointer transition hover:ring-2 hover:ring-gray-300 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-gray-400 ${
                            selected.has(key) ? "ring-2 ring-gray-900 ring-offset-1" : ""
                          }`}
                        >
                          {content}
                        </button>
                      )}
                    </td>
                  );
                })}
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      {hasSelection ? (
        <div className="sticky bottom-4 z-20 flex flex-wrap items-center gap-x-3 gap-y-2 rounded-xl border border-gray-200 bg-white px-4 py-3 shadow-lg">
          <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-sm">
            {checkedRows.length > 0 ? <span className="font-medium text-gray-900">{checkedRows.length} karyawan dicentang</span> : null}
            {selectedKeys.length > 0 ? <span className="font-medium text-gray-900">{selectedKeys.length} kotak dipilih</span> : null}
            {tooMany ? <span className="text-xs font-medium text-red-600">{CELLS_EMPLOYEE_LIMIT_MESSAGE}</span> : null}
          </div>
          <div className="ml-auto flex flex-wrap gap-2">
            <Button size="sm" onClick={() => setPatternOpen(true)} disabled={checkedRows.length === 0 || tooMany || busy}>
              <CalendarRange /> Terapkan pola
            </Button>
            <Button
              size="sm"
              variant="outline"
              disabled={selectedKeys.length === 0 || busy}
              onClick={(ev) => setPicker({ anchor: ev.currentTarget, title: `Isi ${selectedKeys.length} kotak dengan`, keys: selectedKeys })}
            >
              <PaintBucket /> Isi kotak terpilih
            </Button>
            <Button
              size="sm"
              variant="outline"
              disabled={selectedKeys.length === 0 || busy}
              onClick={() => save(fillCells(selectedKeys, ""), `Kosongkan ${selectedKeys.length} kotak`)}
            >
              <Eraser /> Kosongkan
            </Button>
            <Button size="sm" variant="ghost" onClick={clearSelection}>
              <X /> Batal pilih
            </Button>
          </div>
        </div>
      ) : null}

      <ShiftPicker
        anchor={picker?.anchor ?? null}
        title={picker?.title ?? ""}
        shifts={grid.shifts}
        onPick={pick}
        onClose={() => setPicker(null)}
      />
      <CellsConfirmDialog saver={saver} title={reviewTitle} />
      {patternOpen ? (
        <PatternDialog
          open
          onOpenChange={setPatternOpen}
          branchId={branchId}
          employees={checkedRows.map((e) => ({ id: e.id, name: e.name }))}
          shifts={grid.shifts}
          defaultFrom={dates[0] > today ? dates[0] : today}
          today={today}
          onSaved={() => setChecked(new Set())}
        />
      ) : null}
    </div>
  );
}
