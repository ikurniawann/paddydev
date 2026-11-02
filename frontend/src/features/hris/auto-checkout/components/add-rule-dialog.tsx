"use client";

import { useMemo, useState } from "react";
import { Loader2 } from "lucide-react";
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
import { useStoreErrorText } from "@/features/hris/schedule-access/access-guard";
import { useCreateAutoCheckoutRule } from "../mutations";
import { useAutoCheckoutOptions } from "../queries";

const ALL = "all";

interface Props {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

/** Dialog "Tambah aturan": jabatan + lokasi. Pengatur toko hanya melihat tokonya sendiri (tanpa "Semua lokasi"). */
export function AddRuleDialog({ open, onOpenChange }: Props) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogPanel size="sm">
        <DialogPanelHeader>
          <DialogPanelTitle>Tambah aturan</DialogPanelTitle>
        </DialogPanelHeader>
        {open && <AddRuleBody onOpenChange={onOpenChange} />}
      </DialogPanel>
    </Dialog>
  );
}

function AddRuleBody({ onOpenChange }: Pick<Props, "onOpenChange">) {
  const options = useAutoCheckoutOptions(true);
  const create = useCreateAutoCheckoutRule();
  const errorText = useStoreErrorText();
  const [positionId, setPositionId] = useState("");
  const [picked, setPicked] = useState("");
  const [error, setError] = useState("");

  const data = options.data;
  const positionOptions = useMemo<ComboboxOption[]>(
    () => (data?.positions ?? []).map((p) => ({ value: p.id, label: p.title })),
    [data]
  );
  const locationOptions = useMemo<ComboboxOption[]>(() => {
    const stores = (data?.branches ?? []).map((b) => ({ value: b.id, label: b.name.startsWith(b.code) ? b.name : `${b.code} - ${b.name}` }));
    return data?.all_locations ? [{ value: ALL, label: "Semua lokasi" }, ...stores] : stores;
  }, [data]);
  // Pengatur dengan satu toko: tokonya langsung terpilih.
  const locationId = picked || (locationOptions.length === 1 ? locationOptions[0].value : "");

  function handleSave() {
    if (!positionId) return setError("Pilih jabatan");
    if (!locationId) return setError("Pilih lokasi");
    setError("");
    create.mutate(
      { position_id: positionId, branch_id: locationId === ALL ? null : locationId },
      {
        onSuccess: () => {
          toast.success("Aturan checkout otomatis ditambahkan");
          onOpenChange(false);
        },
        onError: (e: Error) => setError(errorText.fail(e, "Gagal menyimpan aturan")),
      }
    );
  }

  return (
    <>
      <DialogPanelBody className="min-w-0 space-y-4">
        {options.isLoading ? (
          <div className="flex justify-center py-6">
            <Loader2 className="h-5 w-5 animate-spin text-gray-400" aria-label="Memuat" />
          </div>
        ) : options.isError ? (
          <p className="text-sm text-red-600">{errorText.text(options.error, "Gagal memuat pilihan")}</p>
        ) : (
          <>
            <div>
              <label className="mb-1 block text-xs font-medium text-gray-600">Jabatan</label>
              <Combobox
                options={positionOptions}
                value={positionId}
                onChange={(v) => {
                  setPositionId(v);
                  setError("");
                }}
                placeholder="Pilih jabatan"
                searchPlaceholder="Cari jabatan..."
                emptyMessage="Jabatan tidak ditemukan."
              />
            </div>
            <div>
              <label className="mb-1 block text-xs font-medium text-gray-600">Lokasi</label>
              <Combobox
                options={locationOptions}
                value={locationId}
                onChange={(v) => {
                  setPicked(v);
                  setError("");
                }}
                placeholder="Pilih lokasi"
                searchPlaceholder="Cari lokasi..."
                emptyMessage="Lokasi tidak ditemukan."
              />
            </div>
            <p className="text-xs text-gray-500">
              Absen karyawan dengan jabatan ini yang belum clock-out ditutup otomatis di jam selesai shift.
            </p>
          </>
        )}
        {error && (
          <p role="alert" className="rounded-lg border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700">
            {error}
          </p>
        )}
      </DialogPanelBody>
      <DialogFooter>
        <Button variant="outline" onClick={() => onOpenChange(false)} disabled={create.isPending}>
          Batal
        </Button>
        <Button onClick={handleSave} disabled={create.isPending || !data}>
          {create.isPending ? "Menyimpan..." : "Simpan"}
        </Button>
      </DialogFooter>
    </>
  );
}
