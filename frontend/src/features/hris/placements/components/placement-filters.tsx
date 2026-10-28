"use client";

import { Search } from "lucide-react";
import { Combobox } from "@/components/ui/combobox";
import { Input } from "@/components/ui/input";
import { cn } from "@/lib/utils";
import { useDepartmentList } from "@/features/master-data/departments/queries";
import { usePositionList } from "@/features/master-data/positions/queries";
import { useStoreBranches } from "../queries";
import type { PlacementSearchCounts, PlacementStatusFilter } from "../types";

const CHIPS: { status: PlacementStatusFilter; label: string; count: keyof PlacementSearchCounts; hint: string; warn?: boolean }[] = [
  { status: "", label: "Semua", count: "all", hint: "Semua karyawan aktif" },
  {
    status: "unplaced",
    label: "Belum ditempatkan",
    count: "unplaced",
    hint: "Belum punya toko. Tidak bisa dijadwalkan dan biaya gajinya masuk “Tanpa toko”.",
    warn: true,
  },
  {
    status: "no_schedule",
    label: "Belum ada jadwal",
    count: "no_schedule",
    hint: "Karyawan Shift tanpa jadwal untuk hari ini sampai 6 hari ke depan",
    warn: true,
  },
  { status: "multi", label: "Multi-toko", count: "multi", hint: "Bekerja di lebih dari satu toko" },
];

/** Chip "yang perlu diurus": angka mengikuti filter cari/toko/departemen/jabatan. */
export function PlacementStatusChips({
  status,
  counts,
  onChange,
}: {
  status: PlacementStatusFilter;
  counts: PlacementSearchCounts | undefined;
  onChange: (status: PlacementStatusFilter) => void;
}) {
  return (
    <div className="flex flex-wrap gap-2">
      {CHIPS.map((chip) => {
        const active = status === chip.status;
        const n = counts?.[chip.count];
        return (
          <button
            key={chip.count}
            type="button"
            title={chip.hint}
            aria-pressed={active}
            onClick={() => onChange(chip.status)}
            className={cn(
              "rounded-full border px-3 py-1.5 text-sm transition-colors",
              active
                ? "border-primary bg-primary/10 font-medium text-brand-text"
                : chip.warn && n
                  ? "border-amber-200 bg-amber-50 text-amber-800 hover:bg-amber-100"
                  : "border-gray-200 bg-white text-gray-700 hover:bg-gray-50"
            )}
          >
            {n === undefined ? chip.label : `${chip.label} · ${n}`}
          </button>
        );
      })}
    </div>
  );
}

export interface PlacementFilterValues {
  search: string;
  branchId: string;
  departmentId: string;
  positionId: string;
}

/** Filter 4 kolom: cari, toko, departemen, jabatan. */
export function PlacementFilters({
  values,
  onChange,
}: {
  values: PlacementFilterValues;
  onChange: (patch: Partial<PlacementFilterValues>) => void;
}) {
  const branches = useStoreBranches();
  const departments = useDepartmentList();
  const positions = usePositionList();

  return (
    <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-4">
      <div>
        <label htmlFor="placement-search" className="mb-1 block text-xs font-medium text-gray-600">
          Cari karyawan
        </label>
        <div className="relative">
          <Search className="absolute left-2.5 top-2.5 h-4 w-4 text-gray-400" />
          <Input
            id="placement-search"
            className="pl-8"
            placeholder="Nama atau NIP"
            value={values.search}
            onChange={(e) => onChange({ search: e.target.value })}
          />
        </div>
      </div>
      <FilterCombo
        label="Toko"
        placeholder="Semua toko"
        value={values.branchId}
        onChange={(branchId) => onChange({ branchId })}
        options={(branches.data ?? []).map((b) => ({ value: b.id, label: b.name }))}
      />
      <FilterCombo
        label="Departemen"
        placeholder="Semua departemen"
        value={values.departmentId}
        onChange={(departmentId) => onChange({ departmentId })}
        options={(departments.data ?? []).map((d) => ({ value: d.id, label: d.name }))}
      />
      <FilterCombo
        label="Jabatan"
        placeholder="Semua jabatan"
        value={values.positionId}
        onChange={(positionId) => onChange({ positionId })}
        options={(positions.data ?? []).map((p) => ({ value: p.id, label: p.title, description: p.department || undefined }))}
      />
    </div>
  );
}

function FilterCombo({
  label,
  placeholder,
  value,
  onChange,
  options,
}: {
  label: string;
  placeholder: string;
  value: string;
  onChange: (value: string) => void;
  options: { value: string; label: string; description?: string }[];
}) {
  return (
    <div>
      <p className="mb-1 text-xs font-medium text-gray-600">{label}</p>
      <Combobox
        options={options}
        value={value}
        onChange={onChange}
        placeholder={placeholder}
        searchPlaceholder={`Cari ${label.toLowerCase()}`}
        allowClear
      />
    </div>
  );
}
