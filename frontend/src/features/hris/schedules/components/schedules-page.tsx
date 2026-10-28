"use client";

import { useState } from "react";
import Link from "next/link";
import { ChevronLeft, ChevronRight, Loader2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Combobox } from "@/components/ui/combobox";
import { useStoreBranches } from "@/features/hris/placements/queries";
import { todayWib } from "@/lib/dates";
import { scheduleTemplateUrl } from "../api";
import { useScheduleGrid } from "../queries";
import { addDays, dayLabel, mondayOf, uploadRange } from "../schedule-range";
import { FixedScheduleDialog } from "./fixed-schedule-dialog";
import { ScheduleGrid } from "./schedule-grid";
import { ScheduleUploadDialog } from "./schedule-upload-dialog";

/** Pilihan lebar tampilan; semuanya memenuhi batas grid 7–45 hari. */
const VIEWS = [
  { days: 7, label: "1 minggu" },
  { days: 14, label: "2 minggu" },
  { days: 28, label: "4 minggu" },
] as const;

interface Props {
  /** Dari tautan Atur jadwal di Penempatan: `?branch_id=…&employees=a,b`. */
  initialBranchId?: string;
  initialEmployeeIds?: string[];
}

/** Jadwal Kerja per cabang (EPIC-056, EPIC-070): grid bisa diedit, pola, salin, unggah. */
export function SchedulesPage({ initialBranchId = "", initialEmployeeIds = [] }: Props) {
  const today = todayWib();
  const [branchId, setBranchId] = useState(initialBranchId);
  /** Cabang yang menerima centang awal; "" = cabang pertama yang dipilih, null = sudah dipakai. */
  const [prefillBranch, setPrefillBranch] = useState<string | null>(initialBranchId);
  const [start, setStart] = useState(() => mondayOf(today));
  const [days, setDays] = useState<number>(14);
  const [uploadOpen, setUploadOpen] = useState(false);
  const [fixedOpen, setFixedOpen] = useState(false);
  const branches = useStoreBranches();
  const params = { branch_id: branchId, from: start, to: addDays(start, days - 1) };
  // Template & unggah tidak boleh memuat tanggal lewat (server menolak).
  const fileRange = { branch_id: branchId, ...uploadRange(params.from, params.to, today) };
  const grid = useScheduleGrid(params);
  const branchName = branches.data?.find((b) => b.id === branchId)?.name ?? "";
  const ready = !!branchId;
  // Centang awal dipakai sekali: untuk cabang dari tautan (atau cabang pertama yang dipilih).
  const prefill = prefillBranch !== null && branchId === prefillBranch ? initialEmployeeIds : undefined;

  function changeBranch(next: string) {
    if (prefillBranch === "") setPrefillBranch(next);
    else if (prefillBranch !== null && next !== prefillBranch) setPrefillBranch(null);
    setBranchId(next);
  }

  return (
    <div className="space-y-5">
      <div className="border-b border-gray-200/70 pb-4">
        <h1 className="text-2xl font-bold text-gray-900">Jadwal Kerja</h1>
        <p className="text-sm text-gray-500">Atur jadwal shift per cabang: ubah per kotak, terapkan pola, atau unggah Excel</p>
      </div>

      <div className="flex flex-wrap items-end gap-3">
        <div className="w-64">
          <p className="mb-1 text-xs font-medium text-gray-600">Cabang</p>
          <Combobox
            options={(branches.data ?? []).map((b) => ({ value: b.id, label: b.name, description: b.code }))}
            value={branchId}
            onChange={changeBranch}
            placeholder="Pilih cabang"
            searchPlaceholder="Cari cabang..."
            emptyMessage="Cabang tidak ditemukan"
          />
        </div>
        <div>
          <p className="mb-1 text-xs font-medium text-gray-600">
            {dayLabel(params.from)} – {dayLabel(params.to, true)}
          </p>
          <div className="flex items-center gap-1">
            <Button variant="outline" size="icon" aria-label="Minggu sebelumnya" onClick={() => setStart((s) => addDays(s, -7))}>
              <ChevronLeft />
            </Button>
            <Button variant="outline" onClick={() => setStart(mondayOf(today))}>
              Minggu ini
            </Button>
            <Button variant="outline" size="icon" aria-label="Minggu berikutnya" onClick={() => setStart((s) => addDays(s, 7))}>
              <ChevronRight />
            </Button>
          </div>
        </div>
        <div>
          <p className="mb-1 text-xs font-medium text-gray-600">Tampilan</p>
          <div className="inline-flex h-10 items-stretch rounded-lg border border-gray-200 p-0.5" role="group" aria-label="Tampilan">
            {VIEWS.map((v) => (
              <button
                key={v.days}
                type="button"
                aria-pressed={days === v.days}
                onClick={() => setDays(v.days)}
                className={`rounded-md px-3 text-sm ${days === v.days ? "bg-gray-900 text-white" : "text-gray-600 hover:bg-gray-100"}`}
              >
                {v.label}
              </button>
            ))}
          </div>
        </div>
        <div className="ml-auto flex flex-wrap gap-2">
          {ready ? (
            <Button asChild variant="outline">
              <a href={scheduleTemplateUrl(fileRange)}>Unduh template</a>
            </Button>
          ) : (
            <Button variant="outline" disabled>
              Unduh template
            </Button>
          )}
          <Button variant="outline" disabled={!ready} onClick={() => setUploadOpen(true)}>
            Unggah jadwal
          </Button>
          {grid.data && grid.data.non_shift_count > 0 ? (
            <Button variant="outline" onClick={() => setFixedOpen(true)}>
              Terapkan jadwal tetap (Non-Shift)
            </Button>
          ) : null}
        </div>
      </div>

      {!branchId ? (
        <p className="py-10 text-center text-sm text-gray-400">Pilih cabang untuk melihat jadwal.</p>
      ) : grid.isLoading ? (
        <div className="flex justify-center py-14">
          <Loader2 className="h-6 w-6 animate-spin text-gray-400" />
        </div>
      ) : grid.isError ? (
        <p className="py-10 text-center text-sm text-danger">
          {grid.error instanceof Error ? grid.error.message : "Gagal memuat jadwal"}
        </p>
      ) : grid.data ? (
        <div className="space-y-3">
          {grid.data.non_shift_count > 0 ? (
            <p className="rounded-lg bg-info-soft px-3 py-2 text-sm text-info">
              {grid.data.non_shift_count} karyawan Non-Shift di cabang ini memakai jadwal tetap dan tidak tampil di sini. Lihat daftarnya di{" "}
              <Link href="/dashboard/employees" className="underline">
                Karyawan
              </Link>{" "}
              (filter Tipe Jadwal: Non-Shift).
            </p>
          ) : null}
          {grid.data.employees.length === 0 ? (
            <p className="py-10 text-center text-sm text-gray-400">
              Belum ada karyawan Shift yang ditempatkan di cabang ini.{" "}
              <Link href="/dashboard/hris/placements" className="underline">
                Atur penempatan karyawan
              </Link>
            </p>
          ) : (
            <ScheduleGrid
              key={branchId}
              grid={grid.data}
              branchId={branchId}
              today={today}
              initialChecked={prefill}
              loading={grid.isPlaceholderData}
            />
          )}
        </div>
      ) : null}

      {uploadOpen ? (
        <ScheduleUploadDialog
          open
          onOpenChange={setUploadOpen}
          range={fileRange}
          branchName={branchName}
          onSaved={() => void grid.refetch()}
        />
      ) : null}
      {fixedOpen ? (
        <FixedScheduleDialog
          open
          onOpenChange={setFixedOpen}
          branchId={branchId}
          branchName={branchName}
          today={today}
          onDone={() => void grid.refetch()}
        />
      ) : null}
    </div>
  );
}
