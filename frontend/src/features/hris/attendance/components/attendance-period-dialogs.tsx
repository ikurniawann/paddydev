"use client";

import { useState } from "react";
import { toast } from "sonner";
import { AlertTriangle, Loader2, Lock } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import {
  Dialog,
  DialogFooter,
  DialogPanel,
  DialogPanelBody,
  DialogPanelDescription,
  DialogPanelHeader,
  DialogPanelTitle,
} from "@/components/ui/dialog";
import { addDays, shortDateId, todayWib } from "@/lib/hris/attendance-period";
import { useAttendancePeriod } from "../queries";
import {
  useCloseAttendancePeriod,
  useReopenAttendancePeriod,
  useUpdateAttendancePeriodClosing,
} from "../mutations";
import type { AttendancePeriod } from "../types";

const RUN_STATUS: Record<string, string> = {
  draft: "Draft",
  processing: "Diproses",
  completed: "Disetujui",
  paid: "Dibayar",
};

function Stat({ label, value, warn }: { label: string; value: number | undefined; warn?: boolean }) {
  return (
    <div className={`rounded-lg border p-3 ${warn ? "border-amber-200 bg-amber-50" : "border-gray-200 bg-gray-50"}`}>
      <p className={`text-xl font-bold ${warn ? "text-amber-700" : "text-gray-900"}`}>{value ?? "…"}</p>
      <p className="text-xs text-gray-500">{label}</p>
    </div>
  );
}

/** Konfirmasi "Closing sekarang" dengan ringkasan absensi periode. */
export function ClosePeriodDialog({
  period,
  open,
  onOpenChange,
}: {
  period: AttendancePeriod;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const detail = useAttendancePeriod(period.id, open);
  const stats = detail.data?.stats ?? period.stats;
  const close = useCloseAttendancePeriod();

  function confirm() {
    close.mutate(period.id, {
      onSuccess: (res) => {
        toast.success(res.message || "Periode absensi sudah di-closing");
        onOpenChange(false);
      },
      onError: (err) => toast.error(err.message || "Gagal closing periode"),
    });
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogPanel size="sm">
        <DialogPanelHeader>
          <DialogPanelTitle>Closing periode {period.name}</DialogPanelTitle>
          <DialogPanelDescription>
            {period.label}. Setelah closing, absensi di periode ini tidak bisa ditambah, diubah, atau dihapus —
            termasuk clock-in/out karyawan. Buka ulang hanya oleh super admin/HR manager.
          </DialogPanelDescription>
        </DialogPanelHeader>
        <DialogPanelBody className="space-y-3">
          <div className="grid grid-cols-2 gap-2 sm:grid-cols-3">
            <Stat label="Karyawan" value={stats?.employees} />
            <Stat label="Baris absensi" value={stats?.rows} />
            <Stat label="Terlambat" value={stats?.late} />
            <Stat label="Tanpa jam pulang" value={stats?.without_clock_out} warn={(stats?.without_clock_out ?? 0) > 0} />
            <Stat label="Belum divalidasi" value={stats?.unvalidated} />
            <Stat label="Disesuaikan manual" value={stats?.adjusted} />
            <Stat
              label="Pengajuan absensi menunggu"
              value={stats?.pending_requests ?? 0}
              warn={(stats?.pending_requests ?? 0) > 0}
            />
          </div>
          {(stats?.without_clock_out ?? 0) > 0 && (
            <p className="flex items-start gap-2 rounded-lg border border-amber-200 bg-amber-50 p-3 text-xs text-amber-800">
              <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0" />
              Ada {stats?.without_clock_out} absensi tanpa jam pulang — jam kerjanya tidak terhitung. Lengkapi dulu
              lewat Rekap bila perlu.
            </p>
          )}
          {(stats?.pending_requests ?? 0) > 0 && (
            <p className="flex items-start gap-2 rounded-lg border border-amber-200 bg-amber-50 p-3 text-xs text-amber-800">
              <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0" />
              Ada {stats?.pending_requests} pengajuan absensi karyawan yang belum diputus — putuskan dulu di tab
              Pengajuan Absensi; setelah periode ditutup pengajuan tidak bisa disetujui.
            </p>
          )}
          <p className="text-xs text-gray-500">
            Closing otomatis dijadwalkan {shortDateId(period.closing_date)}. Payroll {period.name} baru bisa diproses
            setelah periode ini di-closing.
          </p>
        </DialogPanelBody>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)} disabled={close.isPending}>
            Batal
          </Button>
          <Button onClick={confirm} disabled={close.isPending} className="gap-2 bg-pink-600 text-white hover:bg-pink-700">
            {close.isPending ? <Loader2 className="h-4 w-4 animate-spin" /> : <Lock className="h-4 w-4" />}
            Closing sekarang
          </Button>
        </DialogFooter>
      </DialogPanel>
    </Dialog>
  );
}

/** Buka ulang periode yang sudah di-closing: alasan wajib + tanggal closing baru. */
export function ReopenPeriodDialog({
  period,
  open,
  onOpenChange,
  offsetDays = 3,
}: {
  period: AttendancePeriod;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  offsetDays?: number;
}) {
  const today = todayWib();
  const [reason, setReason] = useState("");
  const [closing, setClosing] = useState(() => addDays(today, Math.max(1, offsetDays)));
  const detail = useAttendancePeriod(period.id, open);
  const blocking = detail.data?.blocking_payroll_runs ?? [];
  const reopen = useReopenAttendancePeriod();
  const tooShort = reason.trim().length < 5;

  function confirm() {
    reopen.mutate(
      { id: period.id, reason: reason.trim(), closingDate: closing },
      {
        onSuccess: (res) => {
          toast.success(res.message || "Periode dibuka ulang");
          setReason("");
          onOpenChange(false);
        },
        onError: (err) => toast.error(err.message || "Gagal membuka ulang periode"),
      }
    );
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogPanel size="sm">
        <DialogPanelHeader>
          <DialogPanelTitle>Buka ulang periode {period.name}</DialogPanelTitle>
          <DialogPanelDescription>
            {period.label} — di-closing {period.close_mode === "auto" ? "otomatis oleh sistem" : `oleh ${period.closed_by_name ?? "-"}`}.
            Pembukaan ulang dicatat di audit trail.
          </DialogPanelDescription>
        </DialogPanelHeader>
        <DialogPanelBody className="space-y-4">
          {blocking.length > 0 && (
            <div className="rounded-lg border border-red-200 bg-red-50 p-3 text-sm text-red-800">
              Tidak bisa dibuka ulang: payroll{" "}
              {blocking.map((r) => `"${r.run_name}" (${RUN_STATUS[r.status] ?? r.status})`).join(", ")} sudah diproses.
              Payroll yang sudah diproses/disetujui/dibayar harus tetap memakai absensi yang sudah di-closing.
            </div>
          )}
          <div>
            <label className="mb-1 block text-sm font-medium text-gray-700" htmlFor="reopen-reason">
              Alasan buka ulang <span className="text-red-500">*</span>
            </label>
            <Textarea
              id="reopen-reason"
              value={reason}
              onChange={(e) => setReason(e.target.value)}
              placeholder="Mis. koreksi absensi crew PVJ yang mesin absennya error tgl 12"
              rows={3}
            />
          </div>
          <div>
            <label className="mb-1 block text-sm font-medium text-gray-700" htmlFor="reopen-closing">
              Closing otomatis berikutnya
            </label>
            <Input id="reopen-closing" type="date" min={today} value={closing} onChange={(e) => setClosing(e.target.value)} />
            <p className="mt-1 text-xs text-gray-500">Setelah tanggal ini periode terkunci lagi secara otomatis.</p>
          </div>
        </DialogPanelBody>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)} disabled={reopen.isPending}>
            Batal
          </Button>
          <Button
            onClick={confirm}
            disabled={reopen.isPending || tooShort || blocking.length > 0 || !closing}
            className="gap-2 bg-pink-600 text-white hover:bg-pink-700"
          >
            {reopen.isPending && <Loader2 className="h-4 w-4 animate-spin" />}
            Buka ulang
          </Button>
        </DialogFooter>
      </DialogPanel>
    </Dialog>
  );
}

/** Ubah tanggal closing otomatis periode yang masih terbuka. */
export function ClosingDateDialog({
  period,
  open,
  onOpenChange,
}: {
  period: AttendancePeriod;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const today = todayWib();
  const [closing, setClosing] = useState(period.closing_date);
  const [reason, setReason] = useState("");
  const update = useUpdateAttendancePeriodClosing();
  const min = today > period.end_date ? today : period.end_date;

  function confirm() {
    update.mutate(
      { id: period.id, closingDate: closing, reason: reason.trim() || undefined },
      {
        onSuccess: () => {
          toast.success("Tanggal closing diperbarui");
          onOpenChange(false);
        },
        onError: (err) => toast.error(err.message || "Gagal mengubah tanggal closing"),
      }
    );
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogPanel size="xs">
        <DialogPanelHeader>
          <DialogPanelTitle>Tanggal closing {period.name}</DialogPanelTitle>
          <DialogPanelDescription>
            Sampai tanggal ini HRD masih bisa menyesuaikan absensi {period.label}.
          </DialogPanelDescription>
        </DialogPanelHeader>
        <DialogPanelBody className="space-y-3">
          <Input type="date" min={min} value={closing} onChange={(e) => setClosing(e.target.value)} aria-label="Tanggal closing" />
          <Textarea value={reason} onChange={(e) => setReason(e.target.value)} placeholder="Catatan (opsional)" rows={2} />
        </DialogPanelBody>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)} disabled={update.isPending}>
            Batal
          </Button>
          <Button
            onClick={confirm}
            disabled={update.isPending || !closing || closing < min}
            className="bg-pink-600 text-white hover:bg-pink-700"
          >
            Simpan
          </Button>
        </DialogFooter>
      </DialogPanel>
    </Dialog>
  );
}
