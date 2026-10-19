"use client";

import { useState } from "react";
import Link from "next/link";
import { Loader2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Combobox } from "@/components/ui/combobox";
import { Input } from "@/components/ui/input";
import { useStoreBranches } from "@/features/hris/placements/queries";
import { todayWib } from "@/lib/dates";
import { scheduleTemplateUrl } from "../api";
import { useScheduleGrid } from "../queries";
import { defaultRange, rangeError } from "../schedule-range";
import { FixedScheduleDialog } from "./fixed-schedule-dialog";
import { ScheduleGrid } from "./schedule-grid";
import { ScheduleUploadDialog } from "./schedule-upload-dialog";

/** Jadwal Kerja per cabang (EPIC-056): lihat grid, unduh template, unggah. */
export function SchedulesPage() {
  const today = todayWib();
  const [branchId, setBranchId] = useState("");
  const [range, setRange] = useState(() => defaultRange(today));
  const [uploadOpen, setUploadOpen] = useState(false);
  const [fixedOpen, setFixedOpen] = useState(false);
  const branches = useStoreBranches();
  const params = { branch_id: branchId, from: range.from, to: range.to };
  const invalid = rangeError(range.from, range.to);
  const grid = useScheduleGrid(params);
  const branchName = branches.data?.find((b) => b.id === branchId)?.name ?? "";
  const ready = !!branchId && !invalid;

  return (
    <div className="space-y-5">
      <div className="border-b border-gray-200/70 pb-4">
        <h1 className="text-2xl font-bold text-gray-900">Jadwal Kerja</h1>
        <p className="text-sm text-gray-500">Atur jadwal shift per cabang untuk 7–45 hari</p>
      </div>

      <div className="flex flex-wrap items-end gap-3">
        <div className="w-64">
          <p className="mb-1 text-xs font-medium text-gray-600">Cabang</p>
          <Combobox
            options={(branches.data ?? []).map((b) => ({ value: b.id, label: b.name, description: b.code }))}
            value={branchId}
            onChange={setBranchId}
            placeholder="Pilih cabang"
            searchPlaceholder="Cari cabang..."
            emptyMessage="Cabang tidak ditemukan"
          />
        </div>
        <label className="text-xs font-medium text-gray-600">
          Dari
          <Input type="date" value={range.from} onChange={(e) => setRange((r) => ({ ...r, from: e.target.value }))} className="mt-1 w-40" />
        </label>
        <label className="text-xs font-medium text-gray-600">
          Sampai
          <Input type="date" value={range.to} onChange={(e) => setRange((r) => ({ ...r, to: e.target.value }))} className="mt-1 w-40" />
        </label>
        <div className="flex flex-wrap gap-2">
          {ready ? (
            <Button asChild variant="outline">
              <a href={scheduleTemplateUrl(params)}>Unduh template</a>
            </Button>
          ) : (
            <Button variant="outline" disabled>
              Unduh template
            </Button>
          )}
          <Button disabled={!ready} onClick={() => setUploadOpen(true)}>
            Unggah jadwal
          </Button>
          {grid.data && grid.data.non_shift_count > 0 ? (
            <Button variant="outline" onClick={() => setFixedOpen(true)}>
              Terapkan jadwal tetap (Non-Shift)
            </Button>
          ) : null}
        </div>
      </div>
      {invalid ? <p className="text-sm text-danger">{invalid}</p> : null}

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
            <ScheduleGrid grid={grid.data} />
          )}
        </div>
      ) : null}

      {uploadOpen ? (
        <ScheduleUploadDialog
          open
          onOpenChange={setUploadOpen}
          range={params}
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
