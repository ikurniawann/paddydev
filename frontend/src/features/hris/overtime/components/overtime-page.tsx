"use client";

import { useState } from "react";
import { PlusIcon } from "@heroicons/react/24/outline";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import type { OvertimeDecisionAction } from "@/lib/hris/overtime-rules";
import type { OvertimeDayType, OvertimeRow } from "../types";
import { countPendingEmployeeRequests, currentMonthValue } from "@/lib/hris/overtime-view";
import { useDecideOvertime } from "../mutations";
import { useOvertimeRequests } from "../queries";
import { OvertimeAssignDialog } from "./overtime-assign-dialog";
import { OvertimeCorrectionDialog } from "./overtime-correction-dialog";
import { OvertimeTable } from "./overtime-table";

/**
 * HRD → Kepegawaian → Lembur (/dashboard/hris/overtime):
 * kelola pengajuan lembur karyawan (approve/reject) dan buat penugasan
 * lembur atas nama perusahaan (dikonfirmasi karyawan via ESS).
 */

const STATUS_FILTER_OPTIONS = [
  { value: "all", label: "Semua Status" },
  { value: "pending", label: "Menunggu" },
  { value: "approved", label: "Disetujui" },
  { value: "rejected", label: "Ditolak" },
  { value: "cancelled", label: "Dibatalkan" },
];

export function OvertimePage({ canCorrect = false }: { canCorrect?: boolean }) {
  const [statusFilter, setStatusFilter] = useState("all");
  const [monthFilter, setMonthFilter] = useState(currentMonthValue);
  const [assignOpen, setAssignOpen] = useState(false);
  const [correcting, setCorrecting] = useState<OvertimeRow | null>(null);
  const { data: rows = [], isLoading } = useOvertimeRequests({ status: statusFilter, month: monthFilter });
  const decide = useDecideOvertime();
  const pendingEmployeeRequests = countPendingEmployeeRequests(rows);

  function handleDecide(id: string, action: OvertimeDecisionAction, day_type?: OvertimeDayType) {
    let rejection_reason: string | undefined;
    if (action === "reject") {
      rejection_reason = window.prompt("Alasan penolakan?") ?? undefined;
      if (!rejection_reason?.trim()) return;
    }
    decide.mutate(
      { overtime_id: id, action, rejection_reason, ...(action === "approve" && day_type ? { day_type } : {}) },
      {
        onSuccess: (res) => toast.success(res.message || "Berhasil diproses"),
        onError: (error) => toast.error(error.message || "Gagal memproses"),
      }
    );
  }

  return (
    <div className="space-y-6 pb-12">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-gray-900">Lembur</h1>
          <p className="text-sm text-gray-500">
            Persetujuan pengajuan lembur karyawan & penugasan lembur perusahaan
            {pendingEmployeeRequests > 0
              ? ` · ${pendingEmployeeRequests} pengajuan menunggu persetujuan`
              : ""}
          </p>
        </div>
        <Button className="bg-pink-600 hover:bg-pink-700" onClick={() => setAssignOpen(true)}>
          <PlusIcon className="mr-2 h-4 w-4" /> Tugaskan Lembur
        </Button>
      </div>

      <div className="flex flex-wrap items-center gap-3">
        <Select value={statusFilter} onValueChange={setStatusFilter}>
          <SelectTrigger className="w-44">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {STATUS_FILTER_OPTIONS.map((option) => (
              <SelectItem key={option.value} value={option.value}>
                {option.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <Input
          type="month"
          className="w-44"
          value={monthFilter}
          onChange={(e) => setMonthFilter(e.target.value)}
        />
      </div>

      <OvertimeTable
        rows={rows}
        loading={isLoading}
        decidingId={decide.isPending ? (decide.variables?.overtime_id ?? null) : null}
        onDecide={handleDecide}
        canCorrect={canCorrect}
        onCorrect={setCorrecting}
      />

      <OvertimeAssignDialog open={assignOpen} onOpenChange={setAssignOpen} />
      <OvertimeCorrectionDialog row={correcting} onClose={() => setCorrecting(null)} />
    </div>
  );
}
