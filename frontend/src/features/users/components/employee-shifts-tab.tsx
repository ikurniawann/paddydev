"use client";

import { useState } from "react";
import { Loader2 } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Combobox } from "@/components/ui/combobox";
import { Input } from "@/components/ui/input";
import { todayWib } from "@/lib/dates";
import {
  SHIFT_DAY_NAMES,
  SHIFT_OFF,
  currentShiftPattern,
  shiftPatternPayload,
  summarizeShiftHistory,
  type ShiftPatternDraft,
} from "@/lib/hris/employee-profile-shifts";
import type { ScheduleType } from "@/lib/hris/schedule-type";
import { applyFixedSchedule } from "../api";
import { useSaveEmployeeShiftPattern } from "../mutations";
import { useActiveShiftOptions, useEmployeeShiftSchedule } from "../queries";

/**
 * Tab "Jadwal Shift" di detail karyawan: HRD/Super Admin mengatur pola
 * shift mingguan (Senin–Minggu) per karyawan, berlaku sejak tanggal
 * tertentu. Absensi karyawan dinilai (terlambat/tidak) terhadap jadwal ini.
 */

export function EmployeeShiftsTab({
  employeeId,
  scheduleType = "shift",
}: {
  employeeId: string;
  /** Non-Shift: pola dikunci ke jadwal tetap (EPIC-056). */
  scheduleType?: ScheduleType;
}) {
  const locked = scheduleType === "non_shift";
  const [applying, setApplying] = useState(false);
  const shiftsQuery = useActiveShiftOptions();
  const scheduleQuery = useEmployeeShiftSchedule(employeeId);
  const saveMutation = useSaveEmployeeShiftPattern(employeeId);
  const [effectiveFrom, setEffectiveFrom] = useState(() => todayWib());
  // pilihan yang diubah pengguna, ditimpakan ke pola berjalan dari server
  const [edits, setEdits] = useState<ShiftPatternDraft>({});

  if (shiftsQuery.isLoading || scheduleQuery.isLoading) {
    return (
      <div className="flex justify-center py-10">
        <Loader2 className="h-6 w-6 animate-spin text-gray-400" />
      </div>
    );
  }

  const history = scheduleQuery.data ?? [];
  const pattern = { ...currentShiftPattern(history, todayWib()), ...edits };
  const shiftOptions = [
    { value: SHIFT_OFF, label: "Libur" },
    ...(shiftsQuery.data ?? []).map((s) => ({
      value: s.id,
      label: `${s.name} (${s.start_time.slice(0, 5)}–${s.end_time.slice(0, 5)})`,
    })),
  ];

  async function handleApplyFixed() {
    setApplying(true);
    try {
      const res = await applyFixedSchedule({ employee_ids: [employeeId], effective_from: effectiveFrom });
      toast.success(res.message ?? "Jadwal tetap diterapkan");
      await scheduleQuery.refetch?.();
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "Gagal menerapkan jadwal tetap");
    } finally {
      setApplying(false);
    }
  }

  async function handleSave() {
    try {
      const res = await saveMutation.mutateAsync({
        effective_from: effectiveFrom,
        days: shiftPatternPayload(pattern),
      });
      toast.success(res.message ?? "Jadwal disimpan");
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "Gagal menyimpan");
    }
  }

  return (
    <div className="space-y-5">
      <div>
        <h3 className="text-sm font-semibold text-gray-800">
          {locked ? "Jadwal tetap (Non-Shift)" : "Pola Shift Mingguan"}
        </h3>
        {locked ? (
          <p className="text-xs text-gray-500">
            Senin–Jumat 09.00–18.00, Sabtu–Minggu libur. Ubah tipe jadwal ke Shift untuk mengatur shift.
          </p>
        ) : (
          <p className="text-xs text-gray-500">
            Absensi karyawan dinilai terhadap jadwal ini (terlambat dihitung dari jam mulai shift +
            toleransi). Perubahan disimpan sebagai pola baru — riwayat lama tetap tercatat.
          </p>
        )}
        {(shiftsQuery.isError || scheduleQuery.isError) && (
          <p className="mt-1 text-xs text-red-600">Gagal memuat jadwal</p>
        )}
      </div>

      <div className="grid gap-2 sm:grid-cols-2">
        {SHIFT_DAY_NAMES.map((name, index) => {
          const day = index + 1;
          return (
            <div
              key={day}
              className="flex items-center justify-between gap-3 rounded-lg border border-gray-200/70 bg-white px-3 py-2"
            >
              <span className="w-16 text-sm font-medium text-gray-700">{name}</span>
              <Combobox
                options={shiftOptions}
                value={pattern[day]}
                onChange={(value) => setEdits((prev) => ({ ...prev, [day]: value }))}
                placeholder="Pilih shift"
                className="h-11 flex-1"
                disabled={locked}
              />
            </div>
          );
        })}
      </div>

      <div className="flex flex-wrap items-end gap-3">
        <div>
          <label className="mb-1 block text-xs font-medium text-gray-600">Berlaku mulai</label>
          <Input
            type="date"
            value={effectiveFrom}
            onChange={(e) => setEffectiveFrom(e.target.value)}
            className="w-44"
          />
        </div>
        {locked ? (
          <Button onClick={handleApplyFixed} disabled={applying}>
            {applying ? <Loader2 className="h-4 w-4 animate-spin" /> : "Terapkan jadwal tetap"}
          </Button>
        ) : (
          <Button onClick={handleSave} disabled={saveMutation.isPending}>
            {saveMutation.isPending ? <Loader2 className="h-4 w-4 animate-spin" /> : "Simpan Jadwal"}
          </Button>
        )}
      </div>

      {history.length > 0 && (
        <div>
          <h4 className="mb-1 text-xs font-semibold uppercase tracking-wide text-gray-500">
            Riwayat pola
          </h4>
          <ul className="space-y-0.5 text-xs text-gray-500">
            {summarizeShiftHistory(history).map(({ from, to, summary }) => (
              <li key={from}>
                {from} {to ? `s.d. ${to}` : "(berjalan)"} — {summary}
              </li>
            ))}
          </ul>
        </div>
      )}
    </div>
  );
}
