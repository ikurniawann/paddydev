"use client";

import { useState } from "react";
import { BanknotesIcon, PlusIcon } from "@heroicons/react/24/outline";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { formatRupiah } from "@/lib/format";
import { loanSummary } from "@/lib/hris/loans-view";
import { useDecideLoan } from "../mutations";
import { useLoans } from "../queries";
import { LoanFormDialog } from "./loan-form-dialog";
import { LoansTable } from "./loans-table";

/**
 * HRIS → Penggajian → Pinjaman (/dashboard/hris/loans):
 * pengajuan pinjaman/kasbon karyawan, approval, dan progres pelunasan.
 * Cicilan pinjaman approved otomatis terpotong di payroll (EPIC-008 Fase D);
 * saldo berkurang saat run payroll ditandai dibayar.
 */

const STATUS_FILTER_OPTIONS = [
  { value: "all", label: "Semua Status" },
  { value: "pending", label: "Menunggu" },
  { value: "manager_approved", label: "Disetujui Manager" },
  { value: "approved", label: "Berjalan" },
  { value: "paid_off", label: "Lunas" },
  { value: "rejected", label: "Ditolak" },
];

export function LoansPage() {
  const [statusFilter, setStatusFilter] = useState("all");
  const [dialogOpen, setDialogOpen] = useState(false);
  const { data: rows = [], isLoading } = useLoans(statusFilter);
  const decide = useDecideLoan();
  const { pendingCount, activeTotal } = loanSummary(rows);

  function handleDecide(id: string, approved: boolean) {
    let rejection_reason: string | undefined;
    if (!approved) {
      rejection_reason = window.prompt("Alasan penolakan?") ?? undefined;
      if (!rejection_reason?.trim()) return;
    }
    decide.mutate(
      { id, approved, rejection_reason },
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
          <h1 className="flex items-center gap-2 text-2xl font-bold text-gray-900">
            <BanknotesIcon className="h-6 w-6 text-pink-600" /> Pinjaman Karyawan
          </h1>
          <p className="text-sm text-gray-500">
            Kasbon & pinjaman — cicilan otomatis terpotong di payroll
            {pendingCount > 0 ? ` · ${pendingCount} menunggu approval` : ""}
            {activeTotal > 0 ? ` · total sisa berjalan ${formatRupiah(activeTotal)}` : ""}
          </p>
        </div>
        <Button className="bg-pink-600 hover:bg-pink-700" onClick={() => setDialogOpen(true)}>
          <PlusIcon className="mr-2 h-4 w-4" /> Pengajuan Baru
        </Button>
      </div>

      <div className="flex items-center gap-3">
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
      </div>

      <LoansTable
        rows={rows}
        loading={isLoading}
        decidingId={decide.isPending ? (decide.variables?.id ?? null) : null}
        onDecide={handleDecide}
      />

      <LoanFormDialog open={dialogOpen} onOpenChange={setDialogOpen} />
    </div>
  );
}
