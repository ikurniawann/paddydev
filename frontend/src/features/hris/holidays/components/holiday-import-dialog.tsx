"use client";

import { useState } from "react";
import { Loader2 } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter } from "@/components/ui/dialog";
import { defaultImportSelection, toggleInSet } from "@/lib/hris/holidays-view";
import { useImportHolidays } from "../mutations";
import { useHolidayImportPreview } from "../queries";
import type { HolidayImportRow } from "../types";
import { HolidayBadges } from "./holiday-badges";

interface HolidayImportDialogProps {
  open: boolean;
  year: number;
  onOpenChange: (open: boolean) => void;
}

/**
 * Impor kalender (EPIC-036 Fase E): tarik → preview bercentang → simpan.
 * Tidak ada yang tersimpan sebelum HRD menekan Simpan; entri yang bukan
 * tanggal merah datang dalam keadaan tidak tercentang beserta alasannya.
 */
export function HolidayImportDialog({ open, year, onOpenChange }: HolidayImportDialogProps) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>Impor Kalender Hari Libur {year}</DialogTitle>
        </DialogHeader>
        <HolidayImportBody key={year} year={year} onClose={() => onOpenChange(false)} />
      </DialogContent>
    </Dialog>
  );
}

const NO_ROWS: HolidayImportRow[] = [];

function HolidayImportBody({ year, onClose }: { year: number; onClose: () => void }) {
  const preview = useHolidayImportPreview(year);
  const save = useImportHolidays();
  const rows = preview.data ?? NO_ROWS;
  // Centang awal diturunkan dari data; pilihan HRD disimpan bersama baris sumbernya.
  const [picked, setPicked] = useState<{ rows: HolidayImportRow[]; checked: Set<string> } | null>(null);
  const checked = picked?.rows === rows ? picked.checked : defaultImportSelection(rows);

  function handleSave() {
    const items = rows.filter((row) => checked.has(row.source_ref));
    if (items.length === 0) {
      toast.error("Centang minimal satu hari libur");
      return;
    }
    save.mutate(
      items.map(({ source_ref, holiday_date, name, type, deducts_leave, status }) => ({
        source_ref,
        holiday_date,
        name,
        type,
        deducts_leave,
        status,
      })),
      {
        onSuccess: (res) => {
          toast.success(res.message ?? "Impor selesai");
          onClose();
        },
        onError: (error) => toast.error(error.message || "Gagal menyimpan impor"),
      }
    );
  }

  return (
    <>
      {preview.isLoading ? (
        <div className="flex justify-center py-12">
          <Loader2 className="h-6 w-6 animate-spin text-gray-400" />
        </div>
      ) : preview.isError ? (
        <div className="rounded-lg border border-amber-200 bg-amber-50 p-4 text-sm text-amber-800">
          <p className="font-semibold">Impor tidak tersedia</p>
          <p className="mt-1">{preview.error.message || "Gagal mengambil kalender"}</p>
        </div>
      ) : rows.length === 0 ? (
        <p className="py-12 text-center text-sm text-gray-400">
          Kalender sumber tidak memuat entri untuk {year}.
        </p>
      ) : (
        <>
          <p className="text-xs text-gray-500">
            Centang yang benar-benar tanggal merah menurut SKB 3 Menteri. Entri
            yang bukan hari libur sudah tidak tercentang beserta alasannya, dan
            tanggal yang ditandai belum pasti masuk sebagai draft.
          </p>
          <div className="max-h-[52vh] space-y-1.5 overflow-y-auto pr-1">
            {rows.map((row) => (
              <label
                key={row.source_ref}
                className={`flex items-start gap-2.5 rounded-lg border p-2.5 ${
                  row.already_imported ? "border-gray-100 bg-gray-50/60" : "border-gray-200 hover:bg-slate-50"
                }`}
              >
                <input
                  type="checkbox"
                  className="mt-1"
                  checked={checked.has(row.source_ref)}
                  onChange={() => setPicked({ rows, checked: toggleInSet(checked, row.source_ref) })}
                />
                <div className="min-w-0 flex-1">
                  <p className="text-sm font-medium text-gray-900">
                    <span className="mr-2 font-mono text-xs text-gray-400">{row.holiday_date}</span>
                    {row.name}
                  </p>
                  <HolidayBadges type={row.type} deductsLeave={row.deducts_leave} isDayOff={false} status={row.status}>
                    {row.already_imported && (
                      <span className="rounded-full bg-sky-100 px-1.5 py-0.5 text-[10px] font-medium text-sky-700">
                        Sudah ada
                      </span>
                    )}
                  </HolidayBadges>
                  {row.reason && (
                    <p className="mt-1 text-[11px] leading-snug text-gray-400">{row.reason}</p>
                  )}
                </div>
              </label>
            ))}
          </div>
        </>
      )}

      <DialogFooter>
        <Button variant="outline" onClick={onClose}>
          Batal
        </Button>
        <Button onClick={handleSave} disabled={save.isPending || preview.isLoading || checked.size === 0}>
          {save.isPending ? <Loader2 className="h-4 w-4 animate-spin" /> : `Simpan ${checked.size} libur`}
        </Button>
      </DialogFooter>
    </>
  );
}
