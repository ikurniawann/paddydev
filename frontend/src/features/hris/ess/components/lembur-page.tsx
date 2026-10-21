"use client";

import { useState } from "react";
import { toast } from "sonner";
import { ClockIcon, PaperAirplaneIcon, BellAlertIcon } from "@heroicons/react/24/outline";
import { Loader2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from "@/components/ui/dialog";
import { todayWib } from "@/lib/dates";
import { formatDate } from "@/lib/format";
import {
  OVERTIME_DAY_TYPE_OPTIONS,
  OVERTIME_TOO_LATE,
  overtimeDayTypeLabel,
  overtimeStartsInPast,
  type OvertimeDayType,
} from "@/lib/hris/overtime-rules";
import { splitOvertime } from "@/lib/hris/ess-view";
import { useEssMe, useMyOvertime, useOvertimeDayType } from "../queries";
import { useDecideOvertime, useSubmitOvertime } from "../mutations";
import type { EssOvertimeDecision, EssOvertimeForm } from "../types";
import { EssLoading, EssNotLinked } from "./ess-states";

/**
 * ESS → Lembur (/dashboard/me/lembur): pengajuan lembur mandiri +
 * konfirmasi penugasan lembur dari perusahaan (dibuat HRD).
 * Jam lembur approved yang terealisasi (ada clock-out) otomatis masuk payroll.
 */

const STATUS_BADGES: Record<string, { label: string; className: string }> = {
  pending: { label: "Menunggu", className: "bg-amber-100 text-amber-700" },
  approved: { label: "Disetujui", className: "bg-green-100 text-green-700" },
  rejected: { label: "Ditolak", className: "bg-red-100 text-red-700" },
  cancelled: { label: "Dibatalkan", className: "bg-gray-100 text-gray-600" },
};

/** "17:30:00" → "17:30" */
function clockLabel(value: string): string {
  return value?.slice(0, 5) ?? "—";
}

const EMPTY_FORM: EssOvertimeForm = { date: "", start_time: "", end_time: "", reason: "", day_type: "regular" };

export function EssLemburPage() {
  const { data: me, isLoading } = useEssMe();
  const rows = useMyOvertime().data ?? [];
  const submitMutation = useSubmitOvertime();
  const decideMutation = useDecideOvertime();
  const [dialog, setDialog] = useState(false);
  const [form, setForm] = useState(EMPTY_FORM);
  const decidingId = decideMutation.isPending ? decideMutation.variables?.id : null;
  // K-13 (EPIC-063 R5): jenis dikunci ke saran jadwal bila server mengetahuinya.
  const hint = useOvertimeDayType(form.date).data ?? null;
  const lockedType = hint?.day_type ?? null;
  const dayType: OvertimeDayType = lockedType ?? form.day_type;

  function handleSubmit() {
    if (!form.date || !form.start_time || !form.end_time) {
      toast.error("Tanggal dan jam lembur wajib diisi");
      return;
    }
    if (form.reason.trim().length < 5) {
      toast.error("Alasan minimal 5 karakter");
      return;
    }
    if (overtimeStartsInPast(form.date, form.start_time)) {
      toast.error(OVERTIME_TOO_LATE);
      return;
    }
    submitMutation.mutate({ ...form, day_type: dayType }, {
      onSuccess: () => {
        toast.success("Pengajuan lembur terkirim — menunggu persetujuan");
        setDialog(false);
        setForm(EMPTY_FORM);
      },
      onError: (error) => toast.error(error.message || "Gagal mengajukan lembur"),
    });
  }

  function handleDecide(id: string, action: EssOvertimeDecision) {
    let rejectionReason: string | undefined;
    if (action === "reject") {
      rejectionReason = window.prompt("Alasan menolak penugasan ini?") ?? undefined;
      if (!rejectionReason?.trim()) return;
    }
    decideMutation.mutate(
      { id, action, rejectionReason },
      {
        onSuccess: (json) => toast.success(json.message || "Berhasil diproses"),
        onError: (error) => toast.error(error.message || "Gagal memproses"),
      }
    );
  }

  if (isLoading) return <EssLoading />;
  if (!me?.employee) return <EssNotLinked feature="Pengajuan lembur" />;

  const { assignments: companyAssignments, history } = splitOvertime(rows);

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between border-b border-gray-200/70 pb-4">
        <div>
          <h1 className="text-2xl font-bold text-gray-900">Lembur</h1>
          <p className="text-sm text-gray-500">
            Ajukan lembur atau konfirmasi penugasan lembur dari perusahaan
          </p>
        </div>
        <Button className="gap-2" onClick={() => setDialog(true)}>
          <PaperAirplaneIcon className="h-4 w-4" /> Ajukan Lembur
        </Button>
      </div>

      {companyAssignments.length > 0 && (
        <div className="rounded-xl border border-amber-200 bg-amber-50/70 p-5 shadow-sm">
          <h3 className="flex items-center gap-2 text-sm font-semibold text-amber-800">
            <BellAlertIcon className="h-4 w-4" /> Penugasan Lembur dari Perusahaan —
            butuh konfirmasi Anda
          </h3>
          <ul className="mt-3 divide-y divide-amber-100">
            {companyAssignments.map((row) => (
              <li key={row.id} className="flex flex-wrap items-center justify-between gap-3 py-2.5">
                <div className="min-w-0">
                  <p className="text-sm font-medium text-gray-900">
                    {formatDate(row.date, "—")} · {clockLabel(row.start_time)}–{clockLabel(row.end_time)}{" "}
                    ({Number(row.hours)} jam)
                  </p>
                  <p className="truncate text-xs text-gray-600">
                    {row.reason || "—"}
                    {row.requester?.full_name ? ` · Ditugaskan oleh ${row.requester.full_name}` : ""}
                  </p>
                </div>
                <div className="flex shrink-0 gap-2">
                  <Button
                    size="sm"
                    disabled={decidingId === row.id}
                    onClick={() => handleDecide(row.id, "approve")}
                  >
                    Konfirmasi
                  </Button>
                  <Button
                    size="sm"
                    variant="outline"
                    disabled={decidingId === row.id}
                    onClick={() => handleDecide(row.id, "reject")}
                  >
                    Tolak
                  </Button>
                </div>
              </li>
            ))}
          </ul>
        </div>
      )}

      <div className="rounded-xl border border-gray-200/70 bg-white p-5 shadow-sm">
        <h3 className="flex items-center gap-2 text-sm font-semibold text-gray-800">
          <ClockIcon className="h-4 w-4 text-pink-500" /> Riwayat Lembur
        </h3>
        {history.length === 0 ? (
          <p className="mt-4 py-6 text-center text-sm text-gray-400">Belum ada pengajuan lembur.</p>
        ) : (
          <ul className="mt-3 divide-y divide-gray-100">
            {history.map((row) => {
              const badge = STATUS_BADGES[row.status] ?? STATUS_BADGES.pending;
              return (
                <li key={row.id} className="flex items-center justify-between gap-3 py-2.5">
                  <div className="min-w-0">
                    <p className="text-sm font-medium text-gray-900">
                      {formatDate(row.date, "—")} · {clockLabel(row.start_time)}–{clockLabel(row.end_time)}{" "}
                      ({Number(row.hours)} jam)
                      {row.source === "company" && (
                        <span className="ml-2 rounded-full bg-sky-100 px-2 py-0.5 text-xs font-medium text-sky-700">
                          Penugasan
                        </span>
                      )}
                      {row.day_type === "rest_day" && (
                        <span className="ml-2 rounded-full bg-violet-100 px-2 py-0.5 text-xs font-medium text-violet-700">
                          {overtimeDayTypeLabel(row.day_type)}
                        </span>
                      )}
                    </p>
                    <p className="truncate text-xs text-gray-500">
                      {row.reason || "—"}
                      {row.status === "rejected" && row.rejection_reason
                        ? ` · Alasan ditolak: ${row.rejection_reason}`
                        : ""}
                    </p>
                  </div>
                  <div className="flex shrink-0 items-center gap-2">
                    {row.source === "employee" && row.status === "pending" && (
                      <Button
                        size="sm"
                        variant="ghost"
                        className="text-xs text-gray-500"
                        disabled={decidingId === row.id}
                        onClick={() => handleDecide(row.id, "cancel")}
                      >
                        Batalkan
                      </Button>
                    )}
                    <span
                      className={`rounded-full px-2 py-0.5 text-xs font-medium ${badge.className}`}
                    >
                      {badge.label}
                    </span>
                  </div>
                </li>
              );
            })}
          </ul>
        )}
      </div>

      <Dialog open={dialog} onOpenChange={setDialog}>
        <DialogContent className="max-w-md">
          <DialogHeader>
            <DialogTitle>Ajukan Lembur</DialogTitle>
          </DialogHeader>
          <div className="space-y-3">
            <div>
              <label htmlFor="ot-date" className="mb-1 block text-xs font-medium text-gray-600">Tanggal</label>
              <Input
                id="ot-date"
                type="date"
                min={todayWib()}
                value={form.date}
                onChange={(e) => setForm((f) => ({ ...f, date: e.target.value }))}
              />
            </div>
            <div className="grid grid-cols-2 gap-3">
              <div>
                <label htmlFor="ot-start" className="mb-1 block text-xs font-medium text-gray-600">Jam Mulai</label>
                <Input
                  id="ot-start"
                  type="time"
                  value={form.start_time}
                  onChange={(e) => setForm((f) => ({ ...f, start_time: e.target.value }))}
                />
              </div>
              <div>
                <label htmlFor="ot-end" className="mb-1 block text-xs font-medium text-gray-600">Jam Selesai</label>
                <Input
                  id="ot-end"
                  type="time"
                  value={form.end_time}
                  onChange={(e) => setForm((f) => ({ ...f, end_time: e.target.value }))}
                />
              </div>
            </div>
            <div>
              <label htmlFor="ot-day-type" className="mb-1 block text-xs font-medium text-gray-600">Jenis lembur</label>
              <select
                id="ot-day-type"
                value={dayType}
                disabled={lockedType !== null}
                onChange={(e) => setForm((f) => ({ ...f, day_type: e.target.value as OvertimeDayType }))}
                className="h-9 w-full rounded-md border border-gray-200 bg-white px-2 text-sm disabled:bg-gray-50"
              >
                {OVERTIME_DAY_TYPE_OPTIONS.map((option) => (
                  <option key={option.value} value={option.value}>
                    {option.label}
                  </option>
                ))}
              </select>
              {form.date && (
                <p className="mt-1 text-xs text-gray-500">
                  {lockedType
                    ? `Sesuai jadwal Anda: ${overtimeDayTypeLabel(lockedType)}`
                    : "Jadwal tanggal ini belum ada — pilih sesuai hari lemburnya"}
                </p>
              )}
            </div>
            <div>
              <label htmlFor="ot-reason" className="mb-1 block text-xs font-medium text-gray-600">
                Alasan / pekerjaan yang dilembur (min. 5 karakter)
              </label>
              <Input
                id="ot-reason"
                placeholder="cth. closing stok bulanan"
                value={form.reason}
                onChange={(e) => setForm((f) => ({ ...f, reason: e.target.value }))}
              />
            </div>
            <p className="rounded-lg bg-sky-50 px-3 py-2 text-xs text-sky-700">
              Lembur wajib diajukan sebelum jam mulai. Lembur yang disetujui dibayar sesuai jam yang
              terealisasi (wajib absen clock-in/out pada hari tersebut).
            </p>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setDialog(false)}>
              Batal
            </Button>
            <Button onClick={handleSubmit} disabled={submitMutation.isPending}>
              {submitMutation.isPending ? <Loader2 className="h-4 w-4 animate-spin" /> : "Kirim Pengajuan"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
