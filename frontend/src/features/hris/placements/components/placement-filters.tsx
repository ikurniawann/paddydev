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
  includeEnded: boolean;
}

/**
 * Filter 4 kolom: cari, toko, departemen, jabatan. Dengan `storeBranches`
 * (pengatur jadwal toko, EPIC-071) hanya cari + toko wilayah: master
 * departemen/jabatan khusus HR, jadi tidak diminta sama sekali.
 */
export function PlacementFilters({
  values,
  onChange,
  storeBranches,
}: FieldProps & { storeBranches?: { id: string; name: string }[] }) {
  if (storeBranches) {
    return (
      <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
        <SearchField values={values} onChange={onChange} />
        <FilterCombo
          label="Toko"
          placeholder="Semua toko"
          value={values.branchId}
          onChange={(branchId) => onChange({ branchId })}
          options={storeBranches.map((b) => ({ value: b.id, label: b.name }))}
        />
        <IncludeEnded values={values} onChange={onChange} className="sm:col-span-2" />
      </div>
    );
  }
  return <HrPlacementFilters values={values} onChange={onChange} />;
}

function HrPlacementFilters({ values, onChange }: FieldProps) {
  const branches = useStoreBranches();
  const departments = useDepartmentList();
  const positions = usePositionList();

  return (
    <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-4">
      <SearchField values={values} onChange={onChange} />
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
      <IncludeEnded values={values} onChange={onChange} className="sm:col-span-2 lg:col-span-4" />
    </div>
  );
}

interface FieldProps {
  values: PlacementFilterValues;
  onChange: (patch: Partial<PlacementFilterValues>) => void;
}

function SearchField({ values, onChange }: FieldProps) {
  return (
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
  );
}

function IncludeEnded({ values, onChange, className }: FieldProps & { className: string }) {
  return (
    <label className={cn("flex items-center gap-2 text-sm text-gray-600", className)}>
      <input
        type="checkbox"
        checked={values.includeEnded}
        onChange={(e) => onChange({ includeEnded: e.target.checked })}
      />
      Tampilkan penempatan yang sudah berakhir
    </label>
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
