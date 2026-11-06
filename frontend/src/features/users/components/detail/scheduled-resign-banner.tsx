"use client";

import { useState } from "react";
import { CalendarClock } from "lucide-react";
import { useQuery } from "@tanstack/react-query";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { ConfirmDialog } from "@/components/ui/confirm-dialog";
import { cancelScheduledResign, fetchScheduledResign } from "../../api";
import { RESIGN_TYPES, resignDateText } from "../resign-dialog";

export const scheduledResignKey = (employeeId: string) => ["hris", "employee-scheduled-resign", employeeId] as const;

/**
 * Profil karyawan: resign terjadwal yang belum dijalankan (EPIC-095 R8).
 * Tidak dirender bila tidak ada jadwal (atau selama memuat/gagal).
 */
export function ScheduledResignBanner({
  employeeId,
  canCancel,
  onChanged,
}: {
  employeeId: string;
  canCancel: boolean;
  onChanged: () => void;
}) {
  const query = useQuery({ queryKey: scheduledResignKey(employeeId), queryFn: () => fetchScheduledResign(employeeId) });
  const [confirm, setConfirm] = useState(false);
  const [busy, setBusy] = useState(false);
  const s = query.data;
  if (!s) return null;
  const type = RESIGN_TYPES.find((t) => t.value === s.resign_type)?.label ?? s.resign_type;

  async function cancel() {
    setBusy(true);
    try {
      const res = await cancelScheduledResign(employeeId, null);
      toast.success(res.message ?? "Jadwal resign dibatalkan");
      setConfirm(false);
      await query.refetch();
      onChanged();
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "Gagal membatalkan jadwal resign");
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="flex flex-wrap items-start gap-3 rounded-xl border border-sky-200 bg-sky-50 p-3 text-sm text-sky-900">
      <CalendarClock className="mt-0.5 h-4 w-4 shrink-0" />
      <div className="min-w-0 flex-1 space-y-0.5">
        <p className="font-medium">
          Resign terjadwal: {type} efektif {resignDateText(s.effective_date)} (hari terakhir kerja)
        </p>
        <p className="text-xs text-sky-800">
          Karyawan dinonaktifkan dan login diblokir otomatis keesokan harinya pukul 00.00 WIB. Hitung ulang payroll
          setelah tanggal itu.
          {s.created_by_name ? ` Dijadwalkan oleh ${s.created_by_name}.` : ""}
        </p>
      </div>
      {canCancel ? (
        <Button size="sm" variant="outline" onClick={() => setConfirm(true)}>
          Batalkan jadwal
        </Button>
      ) : null}
      <ConfirmDialog
        open={confirm}
        onOpenChange={setConfirm}
        title="Batalkan resign terjadwal?"
        description={`Karyawan tetap aktif dan jadwal resign ${resignDateText(s.effective_date)} dihapus dari antrean.`}
        confirmLabel="Ya, batalkan"
        loading={busy}
        onConfirm={cancel}
      />
    </div>
  );
}
