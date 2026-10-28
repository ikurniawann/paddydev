"use client";

import { useState } from "react";
import { Loader2 } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Combobox } from "@/components/ui/combobox";
import { Input } from "@/components/ui/input";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { todayWib } from "@/lib/dates";
import { shortDateId } from "@/lib/hris/placements";
import { cn } from "@/lib/utils";
import { useBulkEndPlacement, useBulkPlacement } from "../mutations";
import { useStoreBranches } from "../queries";
import type { BulkOutcome, BulkPlacementResponse, BulkPlacementResultRow, BulkStep } from "../types";

export type BulkPlacementAction = "move" | "add" | "end";

/** Batas karyawan per aksi massal (sama dengan server). */
export const BULK_LIMIT = 500;
export const BULK_LIMIT_MESSAGE = `Maksimal ${BULK_LIMIT} karyawan per aksi`;

interface BulkPlacementDialogProps {
  open: boolean;
  mode: BulkPlacementAction;
  employeeIds: string[];
  onOpenChange: (open: boolean) => void;
  /** Dipanggil setelah tersimpan: tutup dialog dan kosongkan pilihan. */
  onDone: () => void;
}

const COPY: Record<BulkPlacementAction, { title: (n: number) => string; description: string }> = {
  move: {
    title: (n) => `Pindahkan ${n} karyawan ke toko`,
    description:
      "Penempatan yang sedang berjalan diakhiri sehari sebelum tanggal pindah, lalu toko baru menjadi toko utama.",
  },
  add: {
    title: (n) => `Tambah toko untuk ${n} karyawan`,
    description: "Toko lama tetap berjalan. Toko baru dicatat sebagai toko tambahan, kecuali dijadikan toko utama.",
  },
  end: {
    title: (n) => `Akhiri penempatan ${n} karyawan`,
    description:
      "Penempatan yang sedang berjalan berakhir pada tanggal ini. Karyawan tanpa penempatan tidak bisa dijadwalkan.",
  },
};

const OUTCOME: Record<BulkOutcome, { label: string; summary: string; className: string }> = {
  moved: { label: "Dipindah", summary: "dipindah", className: "bg-blue-50 text-blue-700" },
  created: { label: "Ditambah", summary: "ditambah", className: "bg-green-50 text-green-700" },
  ended: { label: "Diakhiri", summary: "diakhiri", className: "bg-gray-100 text-gray-700" },
  skipped: { label: "Dilewati", summary: "dilewati", className: "bg-amber-50 text-amber-800" },
};

/**
 * Aksi massal halaman Penempatan Karyawan: Pindahkan ke toko, Tambah toko,
 * atau Akhiri penempatan. Selalu lewat pratinjau dulu, lalu Simpan.
 */
export function BulkPlacementDialog({ open, mode, employeeIds, onOpenChange, onDone }: BulkPlacementDialogProps) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-2xl">
        {open && (
          <BulkPlacementForm
            key={mode}
            mode={mode}
            employeeIds={employeeIds}
            onCancel={() => onOpenChange(false)}
            onDone={onDone}
          />
        )}
      </DialogContent>
    </Dialog>
  );
}

function BulkPlacementForm({
  mode,
  employeeIds,
  onCancel,
  onDone,
}: {
  mode: BulkPlacementAction;
  employeeIds: string[];
  onCancel: () => void;
  onDone: () => void;
}) {
  const branches = useStoreBranches();
  const bulk = useBulkPlacement();
  const bulkEnd = useBulkEndPlacement();
  const [branchId, setBranchId] = useState("");
  const [date, setDate] = useState(() => todayWib());
  const [primary, setPrimary] = useState(false);
  const [notes, setNotes] = useState("");
  const [preview, setPreview] = useState<BulkPlacementResponse["data"] | null>(null);
  const pending = bulk.isPending || bulkEnd.isPending;
  const tooMany = employeeIds.length > BULK_LIMIT;
  const copy = COPY[mode];
  const storeOptions = (branches.data ?? []).map((b) => ({ value: b.id, label: b.name }));

  // Isian berubah → pratinjau lama tidak berlaku lagi.
  const edit =
    <T,>(set: (v: T) => void) =>
    (v: T) => {
      set(v);
      setPreview(null);
    };

  function run(step: BulkStep) {
    if (mode !== "end" && !branchId) {
      toast.error("Pilih toko tujuan dulu");
      return;
    }
    if (!date) {
      toast.error("Isi tanggal dulu");
      return;
    }
    const handlers = {
      onSuccess: (res: BulkPlacementResponse) => {
        if (step === "preview") {
          setPreview(res.data);
          return;
        }
        toast.success(res.message || "Penempatan disimpan");
        onDone();
      },
      onError: (error: Error) =>
        toast.error(error.message || (step === "preview" ? "Gagal memuat pratinjau" : "Gagal menyimpan penempatan")),
    };
    if (mode === "end") {
      bulkEnd.mutate({ employee_ids: employeeIds, end_date: date, branch_id: branchId || null, step }, handlers);
      return;
    }
    bulk.mutate(
      {
        mode,
        step,
        branch_id: branchId,
        employee_ids: employeeIds,
        start_date: date,
        is_primary: mode === "move" ? true : primary,
        notes: notes.trim() || null,
      },
      handlers
    );
  }

  const results = preview?.results ?? [];
  const willChange = results.some((r) => r.outcome !== "skipped");

  return (
    <>
      <DialogHeader>
        <DialogTitle>{copy.title(employeeIds.length)}</DialogTitle>
        <DialogDescription>{copy.description}</DialogDescription>
      </DialogHeader>

      {preview ? (
        <div className="space-y-3">
          <p className="text-sm font-medium text-gray-900">{previewSummary(results)}</p>
          <div className="max-h-80 overflow-y-auto rounded-lg border border-gray-100">
            <table className="w-full text-sm">
              <thead className="sticky top-0 bg-gray-50">
                <tr className="text-left text-xs text-gray-500">
                  <th className="p-2.5">Karyawan</th>
                  <th className="p-2.5">Hasil</th>
                  <th className="p-2.5">Keterangan</th>
                </tr>
              </thead>
              <tbody>
                {results.map((r) => (
                  <tr key={r.employee_id} className="border-t border-gray-50 align-top">
                    <td className="p-2.5">
                      <p className="font-medium text-gray-900">{r.name ?? "—"}</p>
                      {r.nip && <p className="text-xs text-gray-400">{r.nip}</p>}
                    </td>
                    <td className="p-2.5">
                      <span className={cn("rounded-full px-2 py-0.5 text-xs font-medium", OUTCOME[r.outcome].className)}>
                        {OUTCOME[r.outcome].label}
                      </span>
                    </td>
                    <td className="p-2.5 text-xs text-gray-600">
                      <ResultNote row={r} />
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          {!willChange && (
            <p className="text-xs text-amber-700">Semua karyawan dilewati, tidak ada yang bisa disimpan.</p>
          )}
        </div>
      ) : (
        <div className="space-y-3">
          {mode !== "end" ? (
            <div>
              <p className="mb-1 text-xs font-medium text-gray-600">{mode === "move" ? "Toko tujuan" : "Toko"}</p>
              <Combobox
                options={storeOptions}
                value={branchId}
                onChange={edit(setBranchId)}
                placeholder="Pilih toko"
                searchPlaceholder="Cari kode atau nama toko"
              />
            </div>
          ) : (
            <div>
              <p className="mb-1 text-xs font-medium text-gray-600">Toko yang diakhiri</p>
              <Combobox
                options={storeOptions}
                value={branchId}
                onChange={edit(setBranchId)}
                placeholder="Semua toko"
                searchPlaceholder="Cari kode atau nama toko"
                allowClear
              />
              <p className="mt-1 text-xs text-gray-400">Biarkan “Semua toko” untuk mengakhiri semua penempatan yang berjalan.</p>
            </div>
          )}
          <div>
            <label htmlFor="bulk-placement-date" className="mb-1 block text-xs font-medium text-gray-600">
              {mode === "move" ? "Tanggal pindah" : mode === "add" ? "Tanggal mulai" : "Tanggal terakhir bekerja"}
            </label>
            <Input
              id="bulk-placement-date"
              type="date"
              value={date}
              onChange={(e) => edit(setDate)(e.target.value)}
            />
          </div>
          {mode === "add" && (
            <label className="flex items-start gap-2 text-sm text-gray-700">
              <input
                type="checkbox"
                className="mt-0.5"
                checked={primary}
                onChange={(e) => edit(setPrimary)(e.target.checked)}
              />
              <span>
                Jadikan toko utama
                <span className="block text-xs text-gray-400">
                  Toko utama lama tetap berjalan sebagai toko tambahan.
                </span>
              </span>
            </label>
          )}
          {mode !== "end" && (
            <div>
              <label htmlFor="bulk-placement-notes" className="mb-1 block text-xs font-medium text-gray-600">
                Catatan (opsional)
              </label>
              <Input
                id="bulk-placement-notes"
                maxLength={500}
                placeholder="Mis. rotasi akhir tahun"
                value={notes}
                onChange={(e) => edit(setNotes)(e.target.value)}
              />
            </div>
          )}
          {tooMany && <p className="text-sm font-medium text-red-600">{BULK_LIMIT_MESSAGE}</p>}
        </div>
      )}

      <DialogFooter>
        {preview ? (
          <>
            <Button variant="outline" onClick={() => setPreview(null)} disabled={pending}>
              Ubah isian
            </Button>
            <Button onClick={() => run("commit")} disabled={pending || !willChange}>
              {pending ? <Loader2 className="h-4 w-4 animate-spin" aria-label="Menyimpan" /> : "Simpan"}
            </Button>
          </>
        ) : (
          <>
            <Button variant="outline" onClick={onCancel} disabled={pending}>
              Batal
            </Button>
            <Button onClick={() => run("preview")} disabled={pending || tooMany}>
              {pending ? <Loader2 className="h-4 w-4 animate-spin" aria-label="Memuat pratinjau" /> : "Lihat pratinjau"}
            </Button>
          </>
        )}
      </DialogFooter>
    </>
  );
}

/** "3 dipindah · 1 dilewati" dari hasil pratinjau. */
function previewSummary(results: BulkPlacementResultRow[]): string {
  const counts = new Map<BulkOutcome, number>();
  for (const r of results) counts.set(r.outcome, (counts.get(r.outcome) ?? 0) + 1);
  const order: BulkOutcome[] = ["moved", "created", "ended", "skipped"];
  const parts = order.filter((o) => counts.get(o)).map((o) => `${counts.get(o)} ${OUTCOME[o].summary}`);
  return parts.length > 0 ? parts.join(" · ") : "Tidak ada karyawan yang diproses";
}

function ResultNote({ row }: { row: BulkPlacementResultRow }) {
  if (row.reason) return <>{row.reason}</>;
  if (row.ended.length === 0) return <>—</>;
  return (
    <>
      {row.ended.map((e, i) => (
        <p key={i}>{`${e.branch_name} diakhiri ${shortDateId(e.end_date)}`}</p>
      ))}
    </>
  );
}
