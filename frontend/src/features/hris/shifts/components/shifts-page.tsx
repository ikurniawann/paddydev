"use client";

import { useEffect, useMemo, useState } from "react";
import { PlusIcon, PencilIcon, TrashIcon, MoonIcon, MagnifyingGlassIcon } from "@heroicons/react/24/outline";
import { Loader2 } from "lucide-react";
import { toast } from "sonner";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { ConfirmDialog } from "@/components/ui/confirm-dialog";
import { Input } from "@/components/ui/input";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { hhmm } from "@/lib/hris/shifts-view";
import { useDeleteShift, useSaveShift } from "../mutations";
import { useShifts } from "../queries";
import { filterShifts, groupShifts, isProtectedShiftCode } from "../shift-list";
import type { ShiftRow } from "../types";
import { ShiftFormDialog } from "./shift-form-dialog";

const COLUMNS = ["Kode", "Nama", "Jam", "Istirahat", "Toleransi", "Dipakai", "Status", "Aksi"];

/**
 * HRIS → Kepegawaian → Shift Kerja: master shift (jam kerja + toleransi
 * terlambat) yang menjadi acuan jadwal per karyawan dan perhitungan
 * keterlambatan absensi. Ditampilkan sebagai tabel ringkas per awalan kode.
 */
export function ShiftsPage() {
  const { data: shifts = [], isLoading, isError } = useShifts();
  const remove = useDeleteShift();
  const save = useSaveShift();
  const [dialogOpen, setDialogOpen] = useState(false);
  const [editing, setEditing] = useState<ShiftRow | null>(null);
  const [deactivating, setDeactivating] = useState<ShiftRow | null>(null);
  const [keyword, setKeyword] = useState("");

  useEffect(() => {
    if (isError) toast.error("Gagal memuat shift");
  }, [isError]);

  const visible = useMemo(() => filterShifts(shifts, keyword), [shifts, keyword]);
  const groups = useMemo(() => groupShifts(visible), [visible]);

  function openDialog(shift: ShiftRow | null) {
    setEditing(shift);
    setDialogOpen(true);
  }

  function handleDelete(shift: ShiftRow) {
    if (!confirm(`Hapus shift "${shift.name}"?`)) return;
    remove.mutate(shift.id, {
      onSuccess: () => toast.success("Shift dihapus / dinonaktifkan"),
      onError: (error) => toast.error(error.message || "Gagal menghapus"),
    });
  }

  function setActive(shift: ShiftRow, isActive: boolean, onDone?: () => void) {
    save.mutate(
      { payload: { is_active: isActive }, id: shift.id },
      {
        onSuccess: () => {
          toast.success(isActive ? `Shift ${shift.code} diaktifkan` : `Shift ${shift.code} dinonaktifkan`);
          onDone?.();
        },
        onError: (error) => {
          toast.error(error.message || "Gagal menyimpan");
          onDone?.();
        },
      }
    );
  }

  return (
    <div className="space-y-5">
      <div className="flex items-center justify-between border-b border-gray-200/70 pb-4">
        <div>
          <h1 className="text-2xl font-bold text-gray-900">Shift Kerja</h1>
          <p className="text-sm text-gray-500">
            Daftar jam kerja yang dipilih saat mengatur jadwal karyawan dan menghitung keterlambatan
          </p>
        </div>
        <Button className="gap-2" onClick={() => openDialog(null)}>
          <PlusIcon className="h-4 w-4" /> Tambah shift
        </Button>
      </div>

      <div className="relative max-w-sm">
        <MagnifyingGlassIcon className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-gray-400" />
        <Input
          className="pl-9"
          placeholder="Cari kode, nama, atau jam (cth. 11:00)"
          aria-label="Cari shift"
          value={keyword}
          onChange={(e) => setKeyword(e.target.value)}
        />
      </div>

      {isLoading ? (
        <div className="flex justify-center py-14">
          <Loader2 className="h-6 w-6 animate-spin text-gray-400" />
        </div>
      ) : (
        <div className="overflow-x-auto rounded-xl border border-gray-200/70 bg-white shadow-sm">
          <Table>
            <TableHeader>
              <TableRow>
                {COLUMNS.map((c) => (
                  <TableHead key={c} className={c === "Aksi" ? "text-right" : undefined}>
                    {c}
                  </TableHead>
                ))}
              </TableRow>
            </TableHeader>
            <TableBody>
              {groups.flatMap((group) => [
                <TableRow key={`g-${group.name}`} className="bg-gray-50/80 hover:bg-gray-50/80">
                  <TableCell colSpan={COLUMNS.length} className="py-1.5">
                    <span data-testid="shift-group" className="text-xs font-semibold uppercase tracking-wide text-gray-600">
                      {group.name} · {group.shifts.length} shift
                    </span>
                  </TableCell>
                </TableRow>,
                ...group.shifts.map((shift) => (
                  <ShiftTableRow
                    key={shift.id}
                    shift={shift}
                    onEdit={() => openDialog(shift)}
                    onDelete={() => handleDelete(shift)}
                    onDeactivate={() => setDeactivating(shift)}
                    onActivate={() => setActive(shift, true)}
                  />
                )),
              ])}
              {groups.length === 0 && (
                <TableRow>
                  <TableCell colSpan={COLUMNS.length} className="py-10 text-center text-sm text-gray-400">
                    {shifts.length === 0
                      ? "Belum ada shift — tambahkan dulu untuk mengatur jadwal karyawan."
                      : "Tidak ada shift yang cocok dengan pencarian."}
                  </TableCell>
                </TableRow>
              )}
            </TableBody>
          </Table>
        </div>
      )}

      <ShiftFormDialog open={dialogOpen} shift={editing} onOpenChange={setDialogOpen} />
      <ConfirmDialog
        open={!!deactivating}
        onOpenChange={(open) => !open && setDeactivating(null)}
        title={deactivating ? `Nonaktifkan shift ${deactivating.code}?` : "Nonaktifkan shift?"}
        description="Shift yang nonaktif tidak muncul lagi di pilihan jadwal. Bisa diaktifkan kembali kapan saja."
        confirmLabel="Ya, nonaktifkan"
        loading={save.isPending}
        onConfirm={() => deactivating && setActive(deactivating, false, () => setDeactivating(null))}
      />
    </div>
  );
}

interface ShiftTableRowProps {
  shift: ShiftRow;
  onEdit: () => void;
  onDelete: () => void;
  onDeactivate: () => void;
  onActivate: () => void;
}

function ShiftTableRow({ shift, onEdit, onDelete, onDeactivate, onActivate }: ShiftTableRowProps) {
  const used = shift.employees_using ?? 0;
  const locked = isProtectedShiftCode(shift.code);
  return (
    <TableRow className={shift.is_active ? undefined : "text-gray-400"}>
      <TableCell>
        <span
          data-testid="shift-code"
          className="rounded-md bg-gray-100 px-1.5 py-0.5 font-mono text-xs text-gray-700"
        >
          {shift.code}
        </span>
      </TableCell>
      <TableCell className="max-w-56 truncate font-medium text-gray-900">{shift.name}</TableCell>
      <TableCell className="whitespace-nowrap tabular-nums">
        <span className="inline-flex items-center gap-1">
          {hhmm(shift.start_time)}–{hhmm(shift.end_time)}
          {shift.is_overnight && (
            <MoonIcon className="h-4 w-4 text-indigo-500" aria-label="Lewat tengah malam" role="img" />
          )}
        </span>
      </TableCell>
      <TableCell className="whitespace-nowrap">{shift.break_minutes} mnt</TableCell>
      <TableCell className="whitespace-nowrap">{shift.late_tolerance_minutes} mnt</TableCell>
      <TableCell className="whitespace-nowrap">
        {used > 0 ? `${used} karyawan` : <span className="text-gray-400">Belum dipakai</span>}
      </TableCell>
      <TableCell>
        <Badge variant={shift.is_active ? "success" : "muted"}>{shift.is_active ? "Aktif" : "Nonaktif"}</Badge>
      </TableCell>
      <TableCell>
        <div className="flex justify-end gap-1">
          <Button size="sm" variant="ghost" aria-label={`Edit ${shift.code}`} title="Edit" onClick={onEdit}>
            <PencilIcon className="h-4 w-4" />
          </Button>
          {!locked && shift.is_active && used > 0 && (
            <Button size="sm" variant="ghost" onClick={onDeactivate}>
              Nonaktifkan
            </Button>
          )}
          {!locked && !shift.is_active && (
            <Button size="sm" variant="ghost" onClick={onActivate}>
              Aktifkan
            </Button>
          )}
          {!locked && used === 0 && (
            <Button
              size="sm"
              variant="ghost"
              className="text-red-600"
              aria-label={`Hapus ${shift.code}`}
              title="Hapus"
              onClick={onDelete}
            >
              <TrashIcon className="h-4 w-4" />
            </Button>
          )}
        </div>
      </TableCell>
    </TableRow>
  );
}
