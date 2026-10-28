"use client";

import { CalendarDays, Loader2, LogOut, MoveRight, Plus, X } from "lucide-react";
import { Button } from "@/components/ui/button";
import { BULK_LIMIT, BULK_LIMIT_MESSAGE, type BulkPlacementAction } from "./bulk-placement-dialog";

interface BulkActionBarProps {
  count: number;
  /** Jumlah hasil filter saat ini. */
  total: number;
  selectingAll: boolean;
  onSelectAll: () => void;
  onClear: () => void;
  onAction: (action: BulkPlacementAction) => void;
  onSchedule: () => void;
}

/** Bilah aksi massal; muncul begitu ada karyawan yang dicentang. */
export function BulkActionBar({ count, total, selectingAll, onSelectAll, onClear, onAction, onSchedule }: BulkActionBarProps) {
  const tooMany = count > BULK_LIMIT;
  return (
    <div className="sticky bottom-4 z-20 flex flex-wrap items-center gap-x-3 gap-y-2 rounded-xl border border-gray-200 bg-white px-4 py-3 shadow-lg">
      <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-sm">
        <span className="font-medium text-gray-900">{count} karyawan dipilih</span>
        {total > count && (
          <button
            type="button"
            className="text-brand-text underline-offset-2 hover:underline disabled:opacity-50"
            onClick={onSelectAll}
            disabled={selectingAll}
          >
            {selectingAll && <Loader2 className="mr-1 inline h-3.5 w-3.5 animate-spin" />}
            Pilih semua hasil filter ({total})
          </button>
        )}
        <button type="button" className="inline-flex items-center gap-1 text-gray-500 hover:text-gray-800" onClick={onClear}>
          <X className="h-3.5 w-3.5" /> Hapus pilihan
        </button>
        {tooMany && <span className="text-xs font-medium text-red-600">{BULK_LIMIT_MESSAGE}</span>}
      </div>
      <div className="ml-auto flex flex-wrap gap-2">
        <Button size="sm" onClick={() => onAction("move")} disabled={tooMany}>
          <MoveRight /> Pindahkan ke toko
        </Button>
        <Button size="sm" variant="outline" onClick={() => onAction("add")} disabled={tooMany}>
          <Plus /> Tambah toko
        </Button>
        <Button size="sm" variant="outline" onClick={() => onAction("end")} disabled={tooMany}>
          <LogOut /> Akhiri penempatan
        </Button>
        <Button size="sm" variant="outline" onClick={onSchedule}>
          <CalendarDays /> Atur jadwal
        </Button>
      </div>
    </div>
  );
}
