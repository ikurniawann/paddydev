"use client";

import { useState } from "react";
import { toast } from "sonner";
import { BanknotesIcon, PlusIcon } from "@heroicons/react/24/outline";
import { Loader2 } from "lucide-react";
import { RequestDetailDialog } from "@/components/hris/request-detail-dialog";
import { Button } from "@/components/ui/button";
import { ConfirmDialog } from "@/components/ui/confirm-dialog";
import { Textarea } from "@/components/ui/textarea";
import { Input } from "@/components/ui/input";
import { Combobox } from "@/components/ui/combobox";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from "@/components/ui/dialog";
import { formatDateTime, formatRupiah } from "@/lib/format";
import { installmentPreview, loanPaidPercent } from "@/lib/hris/ess-view";
import { LOAN_TYPE_OPTIONS, kasbonTenorOptions, loanTypeLabel } from "@/lib/hris/loans-view";
import { useEssMe, useMyLoans, useTeamLoanApprovals } from "../queries";
import { useDecideTeamLoan, useSubmitLoan } from "../mutations";
import type { EssLoanRow } from "../types";
import { EssLoading, EssNotLinked } from "./ess-states";

/**
 * ESS → Pinjaman (/dashboard/me/pinjaman): karyawan mengajukan
 * pinjaman/kasbon utk dirinya sendiri + memantau status & pelunasan.
 * Server memaksa employee_id = diri sendiri, bunga 0, status pending.
 */

const STATUS_BADGES: Record<string, { label: string; className: string }> = {
  pending: { label: "Menunggu", className: "bg-amber-100 text-amber-700" },
  manager_approved: { label: "Disetujui Manager", className: "bg-indigo-100 text-indigo-700" },
  approved: { label: "Berjalan", className: "bg-green-100 text-green-700" },
  rejected: { label: "Ditolak", className: "bg-red-100 text-red-700" },
  paid_off: { label: "Lunas", className: "bg-sky-100 text-sky-700" },
};

const EMPTY_FORM = {
  loan_type: "kasbon",
  principal_amount: "",
  tenor_months: "2",
  purpose: "",
};

export function EssPinjamanPage() {
  const { data: me, isLoading } = useEssMe();
  const loans = useMyLoans().data ?? [];
  const submitMutation = useSubmitLoan();
  const teamLoans = useTeamLoanApprovals().data ?? [];
  const decideTeamLoan = useDecideTeamLoan();
  const [dialog, setDialog] = useState(false);
  const [form, setForm] = useState(EMPTY_FORM);
  // Kasbon tim (EPIC-072): detail sebelum memutus; tolak wajib alasan.
  const [detail, setDetail] = useState<EssLoanRow | null>(null);
  const [rejecting, setRejecting] = useState<EssLoanRow | null>(null);
  const [rejectReason, setRejectReason] = useState("");
  const previewInstallment = installmentPreview(form.principal_amount, form.tenor_months);

  function handleSubmit() {
    if (!Number(form.principal_amount) || !Number(form.tenor_months)) {
      toast.error("Jumlah pinjaman dan tenor wajib diisi");
      return;
    }
    if (form.loan_type === "kasbon" && !["1", "2"].includes(form.tenor_months)) {
      toast.error("Tenor kasbon hanya 1 atau 2 bulan");
      return;
    }
    submitMutation.mutate(
      {
        loan_type: form.loan_type,
        principal_amount: Number(form.principal_amount),
        tenor_months: Number(form.tenor_months),
        purpose: form.purpose || undefined,
      },
      {
        onSuccess: () => {
          toast.success("Pengajuan pinjaman terkirim — menunggu persetujuan");
          setDialog(false);
          setForm(EMPTY_FORM);
        },
        onError: (error) => toast.error(error.message || "Gagal mengajukan pinjaman"),
      }
    );
  }

  function askRejectTeam(loan: EssLoanRow) {
    setDetail(null);
    setRejectReason("");
    setRejecting(loan);
  }

  function confirmRejectTeam() {
    if (!rejecting) return;
    const text = rejectReason.trim();
    if (!text) {
      toast.error("Alasan penolakan wajib diisi");
      return;
    }
    const id = rejecting.id;
    setRejecting(null);
    handleDecideTeam(id, false, text);
  }

  function handleDecideTeam(id: string, approved: boolean, rejectionReason?: string) {
    setDetail(null);
    decideTeamLoan.mutate(
      { id, approved, ...(rejectionReason ? { rejectionReason } : {}) },
      {
        onSuccess: (res) => toast.success(res.message || (approved ? "Kasbon disetujui" : "Kasbon ditolak")),
        onError: (error) => toast.error(error.message || "Gagal memproses kasbon"),
      }
    );
  }

  if (isLoading) return <EssLoading />;
  if (!me?.employee) return <EssNotLinked feature="Pengajuan pinjaman" />;

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between border-b border-gray-200/70 pb-4">
        <div>
          <h1 className="flex items-center gap-2 text-2xl font-bold text-gray-900">
            <BanknotesIcon className="h-6 w-6 text-pink-600" /> Pinjaman
          </h1>
          <p className="text-sm text-gray-500">
            Ajukan pinjaman/kasbon dan pantau cicilannya
          </p>
        </div>
        <Button className="gap-2" onClick={() => setDialog(true)}>
          <PlusIcon className="h-4 w-4" /> Ajukan Pinjaman
        </Button>
      </div>

      <div className="rounded-xl border border-gray-200/70 bg-white p-5 shadow-sm">
        <h3 className="text-sm font-semibold text-gray-800">Riwayat Pinjaman</h3>
        {loans.length === 0 ? (
          <p className="mt-4 py-6 text-center text-sm text-gray-400">
            Belum ada pengajuan pinjaman.
          </p>
        ) : (
          <ul className="mt-3 divide-y divide-gray-100">
            {loans.map((loan) => {
              const badge = STATUS_BADGES[loan.status] ?? STATUS_BADGES.pending;
              const paidPct = loanPaidPercent(loan.paid_amount, loan.remaining_balance);
              return (
                <li key={loan.id} className="py-3">
                  <div className="flex items-start justify-between gap-3">
                    <div className="min-w-0">
                      <p className="text-sm font-medium text-gray-900">
                        {loanTypeLabel(loan.loan_type)} · {formatRupiah(loan.principal_amount)}
                      </p>
                      <p className="text-xs text-gray-500">
                        Cicilan {formatRupiah(loan.monthly_installment)}/bln × {loan.tenor_months} bln
                        {loan.purpose ? ` · ${loan.purpose}` : ""}
                      </p>
                      {loan.status === "rejected" && loan.rejection_reason && (
                        <p className="mt-0.5 text-xs text-red-500">
                          Alasan ditolak: {loan.rejection_reason}
                        </p>
                      )}
                    </div>
                    <span
                      className={`shrink-0 rounded-full px-2 py-0.5 text-xs font-medium ${badge.className}`}
                    >
                      {badge.label}
                    </span>
                  </div>
                  {(loan.status === "approved" || loan.status === "paid_off") && (
                    <div className="mt-2">
                      <div className="mb-1 flex justify-between text-xs text-gray-500">
                        <span>Terbayar {formatRupiah(loan.paid_amount)}</span>
                        <span>Sisa {formatRupiah(loan.remaining_balance)}</span>
                      </div>
                      <div className="h-1.5 w-full rounded-full bg-gray-100">
                        <div
                          className="h-1.5 rounded-full bg-green-500"
                          style={{ width: `${paidPct}%` }}
                        />
                      </div>
                    </div>
                  )}
                </li>
              );
            })}
          </ul>
        )}
      </div>

      {teamLoans.length > 0 && (
        <div className="rounded-xl border border-gray-200/70 bg-white p-5 shadow-sm">
          <h3 className="text-sm font-semibold text-gray-800">Persetujuan Kasbon Tim</h3>
          <ul className="mt-3 divide-y divide-gray-100">
            {teamLoans.map((loan) => (
              <li key={loan.id} className="flex items-start justify-between gap-3 py-3">
                <div className="min-w-0">
                  <p className="text-sm font-medium text-gray-900">
                    {loan.employee?.full_name ?? "—"}
                  </p>
                  <p className="text-xs text-gray-500">
                    {loanTypeLabel(loan.loan_type)} · {formatRupiah(loan.principal_amount)} ·{" "}
                    {loan.tenor_months} bln
                  </p>
                  {loan.purpose && <p className="line-clamp-2 break-words text-xs text-gray-500">{loan.purpose}</p>}
                </div>
                <div className="flex shrink-0 gap-2">
                  <Button
                    size="sm"
                    variant="ghost"
                    aria-label={`Detail kasbon ${loan.employee?.full_name ?? ""}`}
                    onClick={() => setDetail(loan)}
                  >
                    Detail
                  </Button>
                  <Button
                    size="sm"
                    disabled={decideTeamLoan.isPending}
                    onClick={() => handleDecideTeam(loan.id, true)}
                  >
                    Setujui
                  </Button>
                  <Button
                    size="sm"
                    variant="outline"
                    disabled={decideTeamLoan.isPending}
                    aria-label={`Tolak kasbon ${loan.employee?.full_name ?? ""}`}
                    onClick={() => askRejectTeam(loan)}
                  >
                    Tolak
                  </Button>
                </div>
              </li>
            ))}
          </ul>
        </div>
      )}

      {detail && (
        <RequestDetailDialog
          open
          onOpenChange={(o) => !o && setDetail(null)}
          title="Detail kasbon"
          employee={{ name: detail.employee?.full_name ?? "—", nip: detail.employee?.nip }}
          fields={[
            { label: "Jenis", value: loanTypeLabel(detail.loan_type) },
            { label: "Nominal", value: formatRupiah(detail.principal_amount) },
            { label: "Tenor", value: `${detail.tenor_months} bulan` },
            ...(detail.created_at ? [{ label: "Diajukan", value: formatDateTime(detail.created_at, "—") }] : []),
          ]}
          reason={{ label: "Keperluan", text: detail.purpose }}
          notes={
            detail.notes?.trim() ? (
              <div>
                <p className="mb-1 text-xs font-medium text-gray-600">Catatan</p>
                <p className="whitespace-pre-line break-words text-sm text-gray-900">{detail.notes}</p>
              </div>
            ) : undefined
          }
          actions={
            <>
              <Button variant="outline" disabled={decideTeamLoan.isPending} onClick={() => askRejectTeam(detail)}>
                Tolak
              </Button>
              <Button disabled={decideTeamLoan.isPending} onClick={() => handleDecideTeam(detail.id, true)}>
                Setujui
              </Button>
            </>
          }
        />
      )}

      <ConfirmDialog
        open={rejecting !== null}
        onOpenChange={(o) => !o && setRejecting(null)}
        title={`Tolak kasbon ${rejecting?.employee?.full_name ?? ""}?`}
        confirmLabel="Tolak pengajuan"
        cancelLabel="Kembali"
        onConfirm={confirmRejectTeam}
      >
        <div className="space-y-1.5">
          <label htmlFor="team-loan-rejection-reason" className="block text-xs font-medium text-gray-600">
            Alasan penolakan
          </label>
          <Textarea
            id="team-loan-rejection-reason"
            rows={3}
            maxLength={500}
            value={rejectReason}
            onChange={(e) => setRejectReason(e.target.value)}
          />
        </div>
      </ConfirmDialog>

      <Dialog open={dialog} onOpenChange={setDialog}>
        <DialogContent className="max-w-md">
          <DialogHeader>
            <DialogTitle>Ajukan Pinjaman / Kasbon</DialogTitle>
          </DialogHeader>
          <div className="space-y-3">
            <div>
              <label className="mb-1 block text-xs font-medium text-gray-600">Jenis</label>
              <Combobox
                options={LOAN_TYPE_OPTIONS}
                value={form.loan_type}
                onChange={(value) =>
                  setForm((f) => ({
                    ...f,
                    loan_type: value,
                    tenor_months:
                      value === "kasbon" && !["1", "2"].includes(f.tenor_months) ? "2" : f.tenor_months,
                  }))
                }
                placeholder="Pilih jenis"
              />
            </div>
            <div className="grid grid-cols-2 gap-3">
              <div>
                <label className="mb-1 block text-xs font-medium text-gray-600">Jumlah (Rp)</label>
                <Input
                  type="number"
                  min="0"
                  value={form.principal_amount}
                  onChange={(e) => setForm((f) => ({ ...f, principal_amount: e.target.value }))}
                />
              </div>
              <div>
                <label className="mb-1 block text-xs font-medium text-gray-600">Tenor (bulan)</label>
                {form.loan_type === "kasbon" ? (
                  <Combobox
                    options={kasbonTenorOptions}
                    value={form.tenor_months}
                    onChange={(value) => setForm((f) => ({ ...f, tenor_months: value }))}
                    placeholder="Pilih tenor"
                  />
                ) : (
                  <Input
                    type="number"
                    min="1"
                    max="60"
                    value={form.tenor_months}
                    onChange={(e) => setForm((f) => ({ ...f, tenor_months: e.target.value }))}
                  />
                )}
              </div>
            </div>
            <div>
              <label className="mb-1 block text-xs font-medium text-gray-600">Keperluan</label>
              <Input
                placeholder="cth. biaya sekolah anak"
                value={form.purpose}
                onChange={(e) => setForm((f) => ({ ...f, purpose: e.target.value }))}
              />
            </div>
            {previewInstallment !== null && (
              <p className="rounded-lg bg-sky-50 px-3 py-2 text-xs text-sky-700">
                Perkiraan cicilan: <strong>{formatRupiah(previewInstallment)}</strong>/bulan ×{" "}
                {form.tenor_months} bulan (tanpa bunga). Cicilan otomatis
                terpotong dari gaji setelah disetujui HRD.
              </p>
            )}
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
