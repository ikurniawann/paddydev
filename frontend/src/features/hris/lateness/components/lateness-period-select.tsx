"use client";

import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import type { LatenessPeriodOption } from "../types";

/** Pemilih periode absensi (periode berjalan + 5 sebelumnya). */
export function LatenessPeriodSelect({
  periods,
  value,
  onChange,
  label = "Pilih periode",
}: {
  periods: LatenessPeriodOption[];
  value: number;
  onChange: (back: number) => void;
  label?: string;
}) {
  if (periods.length === 0) return null;
  return (
    <Select
      value={String(value)}
      onValueChange={(v) => onChange(Number(v))}
      items={periods.map((p) => ({ value: String(p.back), label: p.name }))}
    >
      <SelectTrigger aria-label={label} className="w-full sm:w-56">
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        {periods.map((p) => (
          <SelectItem key={p.back} value={String(p.back)}>
            <span className="flex min-w-0 flex-col">
              <span>{p.name}</span>
              <span className="text-xs text-gray-500">{p.label}</span>
            </span>
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}
