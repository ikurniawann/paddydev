"use client";

import { useMemo, useState } from "react";
import { Loader2, X } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Combobox, type ComboboxOption } from "@/components/ui/combobox";
import {
  Dialog,
  DialogFooter,
  DialogPanel,
  DialogPanelBody,
  DialogPanelHeader,
  DialogPanelTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { useStoreBranches } from "@/features/hris/placements/queries";
import { mainPlacement } from "@/features/hris/placements/components/placements-table";
import type { PlacementSearchRow } from "@/features/hris/placements/types";
import { useDebouncedValue } from "@/hooks/use-debounced-value";
import { todayWib } from "@/lib/dates";
import { useSaveDelegate } from "../mutations";
import { useDelegateEmployeeSearch, useDelegateSuggest, useScheduleDelegates } from "../queries";
import type { ScheduleDelegate } from "../types";

interface DelegateFormDialogProps {
  open: boolean;
  /** null = tambah pengatur baru */
  delegate: ScheduleDelegate | null;
  onOpenChange: (open: boolean) => void;
}

export function DelegateFormDialog(props: DelegateFormDialogProps) {
  const { open, delegate, onOpenChange } = props;
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogPanel size="sm">
        <DialogPanelHeader>
          <DialogPanelTitle>{delegate ? "Ubah pengatur jadwal toko" : "Tambah pengatur jadwal toko"}</DialogPanelTitle>
        </DialogPanelHeader>
        {open && <DelegateFormBody key={delegate?.employee_id ?? "new"} {...props} />}
      </DialogPanel>
    </Dialog>
  );
}

/** Nama toko sudah memuat kodenya ("PVJ - Paddy PVJ"); bila belum, kode ditambahkan di depan. */
const storeText = (b: { code: string; name: string }) => (b.name.startsWith(b.code) ? b.name : `${b.code} - ${b.name}`);

const employeeOption = (row: PlacementSearchRow, today: string): ComboboxOption => {
  const main = mainPlacement(row, today);
  const detail = [row.position_title, main?.branch_name].filter(Boolean).join(" · ");
  return {
    value: row.employee_id,
    label: row.nip ? `${row.full_name} · ${row.nip}` : row.full_name,
    description: detail || undefined,
  };
};

function DelegateFormBody({ delegate, onOpenChange }: DelegateFormDialogProps) {
  const editing = delegate !== null;
  const today = todayWib();
  const [employeeId, setEmployeeId] = useState(delegate?.employee_id ?? "");
  const [picked, setPicked] = useState<ComboboxOption | null>(null);
  const [searchText, setSearchText] = useState("");
  const q = useDebouncedValue(searchText.trim(), 300);
  // null = belum diubah HR: toko mengikuti baris pengatur yang ada, lalu saran server (karyawan baru).
  const [chosen, setChosen] = useState<string[] | null>(null);
  const [canPlaceChoice, setCanPlaceChoice] = useState<boolean | null>(null);
  const [storeFilter, setStoreFilter] = useState("");
  const [error, setError] = useState("");

  const branches = useStoreBranches();
  const employees = useDelegateEmployeeSearch(q, !editing);
  const delegates = useScheduleDelegates();
  // Karyawan yang sudah menjadi pengatur: nilai barisnya dipakai, bukan saran (simpan akan menggantikannya).
  const existing = editing ? undefined : (delegates.data ?? []).find((d) => d.employee_id === employeeId);
  const base = delegate ?? existing;
  const suggest = useDelegateSuggest(editing || existing || delegates.isLoading ? undefined : employeeId || undefined);
  const save = useSaveDelegate();

  const selectedIds = chosen ?? base?.branches.map((b) => b.id) ?? suggest.data?.branch_ids ?? [];
  const canPlace = canPlaceChoice ?? base?.can_place ?? false;
  const selected = new Set(selectedIds);

  const options = useMemo(() => {
    const list = (employees.data?.rows ?? []).map((r) => employeeOption(r, today));
    // Karyawan terpilih tetap ada di daftar walau hasil pencarian berikutnya tidak memuatnya.
    if (picked && !list.some((o) => o.value === picked.value)) list.unshift(picked);
    return list;
  }, [employees.data, picked, today]);

  const storeLabel = useMemo(() => {
    const map = new Map<string, string>();
    for (const b of delegate?.branches ?? []) map.set(b.id, b.code);
    for (const b of branches.data ?? []) map.set(b.id, b.code);
    return map;
  }, [branches.data, delegate]);

  const stores = (branches.data ?? []).filter((b) => {
    const needle = storeFilter.trim().toLowerCase();
    return !needle || `${b.code} ${b.name}`.toLowerCase().includes(needle);
  });

  function pickEmployee(id: string) {
    setEmployeeId(id);
    setPicked(options.find((o) => o.value === id) ?? null);
    setChosen(null);
    setCanPlaceChoice(null);
    setError("");
  }

  function toggleStore(id: string) {
    setChosen(selected.has(id) ? selectedIds.filter((x) => x !== id) : [...selectedIds, id]);
    setError("");
  }

  function handleSave() {
    if (!employeeId) {
      setError("Pilih karyawan");
      return;
    }
    if (selectedIds.length === 0) {
      setError("Pilih minimal satu toko");
      return;
    }
    setError("");
    save.mutate(
      { employeeId, payload: { branch_ids: selectedIds, can_place: canPlace } },
      {
        onSuccess: () => {
          toast.success(editing ? "Pengatur jadwal toko diperbarui" : "Pengatur jadwal toko ditambahkan");
          onOpenChange(false);
        },
        onError: (e: Error) => setError(e.message || "Gagal menyimpan pengatur jadwal toko"),
      }
    );
  }

  const suggesting = !editing && !!employeeId && chosen === null && !base && suggest.isLoading;
  const suggestFailed = !editing && !!employeeId && !base && suggest.isError;
  // Tanpa daftar pengatur, karyawan yang sudah jadi pengatur tidak dikenali: simpan bisa menimpa wilayahnya diam-diam.
  const listFailed = !editing && delegates.isError;
  const noSuggestion = !editing && !!employeeId && chosen === null && !base && suggest.isSuccess && selectedIds.length === 0;

  return (
    <>
      <DialogPanelBody className="min-w-0 space-y-4">
        <div>
          <label className="mb-1 block text-xs font-medium text-gray-600">Karyawan</label>
          {editing ? (
            <div className="min-w-0 rounded-lg border border-gray-200/80 bg-gray-50 px-3 py-2 text-sm">
              <p className="break-words font-medium text-gray-900">{delegate.full_name}</p>
              <p className="break-words text-xs text-gray-500">
                {[delegate.nip, delegate.position_title].filter(Boolean).join(" · ") || "—"}
              </p>
            </div>
          ) : (
            <Combobox
              options={options}
              value={employeeId}
              onChange={pickEmployee}
              onSearchChange={setSearchText}
              placeholder="Pilih karyawan"
              searchPlaceholder="Cari nama atau NIP..."
              emptyMessage={employees.isFetching ? "Mencari..." : "Karyawan tidak ditemukan."}
            />
          )}
        </div>

        <div>
          <div className="mb-1 flex items-center justify-between">
            <label className="text-xs font-medium text-gray-600">Toko yang diatur</label>
            {suggesting && <Loader2 className="h-3.5 w-3.5 animate-spin text-gray-400" aria-label="Memuat saran toko" />}
          </div>
          {selectedIds.length > 0 && (
            <div className="mb-2 flex flex-wrap gap-1.5">
              {selectedIds.map((id) => (
                <span
                  key={id}
                  className="inline-flex items-center gap-1 rounded-full border border-gray-200 bg-gray-50 px-2 py-0.5 text-xs text-gray-700"
                >
                  {storeLabel.get(id) ?? "Toko"}
                  <button
                    type="button"
                    aria-label={`Hapus ${storeLabel.get(id) ?? "toko"}`}
                    className="text-gray-400 hover:text-gray-700"
                    onClick={() => toggleStore(id)}
                  >
                    <X className="h-3 w-3" />
                  </button>
                </span>
              ))}
            </div>
          )}
          <Input
            placeholder="Cari toko"
            aria-label="Cari toko"
            value={storeFilter}
            onChange={(e) => setStoreFilter(e.target.value)}
          />
          <div className="mt-2 max-h-48 divide-y divide-gray-50 overflow-y-auto rounded-lg border border-gray-200/80">
            {branches.isLoading ? (
              <p className="p-3 text-sm text-gray-400">Memuat toko...</p>
            ) : stores.length === 0 ? (
              <p className="p-3 text-sm text-gray-400">Toko tidak ditemukan.</p>
            ) : (
              stores.map((b) => (
                <label key={b.id} className="flex cursor-pointer items-start gap-2 px-3 py-2 text-sm hover:bg-gray-50">
                  <input
                    type="checkbox"
                    className="mt-0.5 shrink-0"
                    aria-label={storeText(b)}
                    checked={selected.has(b.id)}
                    onChange={() => toggleStore(b.id)}
                  />
                  <span className="min-w-0 break-words text-gray-900">{storeText(b)}</span>
                </label>
              ))
            )}
          </div>
          {existing && (
            <p className="mt-1 text-xs text-amber-700">
              Karyawan ini sudah menjadi pengatur — perubahan akan menggantikan toko sebelumnya.
            </p>
          )}
          {suggestFailed && (
            <p className="mt-1 text-xs text-amber-700">Saran toko gagal dimuat — pilih toko secara manual.</p>
          )}
          {noSuggestion && (
            <p className="mt-1 text-xs text-gray-500">
              Karyawan ini belum punya penempatan atau bawahan di toko mana pun. Pilih tokonya sendiri.
            </p>
          )}
        </div>

        <label className="flex items-start gap-2 text-sm text-gray-700">
          <input
            type="checkbox"
            className="mt-0.5 shrink-0"
            aria-label="Boleh atur penempatan karyawan"
            checked={canPlace}
            onChange={(e) => setCanPlaceChoice(e.target.checked)}
          />
          <span>Boleh atur penempatan karyawan</span>
        </label>
        <p className="text-xs text-gray-500">Pengatur hanya bisa mengubah jadwal dan penempatan di toko yang dipilih</p>

        {listFailed && (
          <p className="text-sm text-red-600">Daftar pengatur gagal dimuat. Tutup lalu buka lagi dialog ini.</p>
        )}
        {error && (
          <p role="alert" className="rounded-lg border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700">
            {error}
          </p>
        )}
      </DialogPanelBody>
      <DialogFooter>
        <Button variant="outline" onClick={() => onOpenChange(false)} disabled={save.isPending}>
          Batal
        </Button>
        <Button onClick={handleSave} disabled={save.isPending || listFailed}>
          {save.isPending ? <Loader2 className="h-4 w-4 animate-spin" /> : "Simpan"}
        </Button>
      </DialogFooter>
    </>
  );
}
