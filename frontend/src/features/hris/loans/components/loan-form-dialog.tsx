"use client";

import { useState } from "react";
import { Loader2 } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Combobox } from "@/components/ui/combobox";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter } from "@/components/ui/dialog";
import { formatRupiah } from "@/lib/format";
import { todayWib } from "@/lib/dates";
import {
  EMPTY_LOAN_FORM,
  LOAN_TYPE_OPTIONS,
  loanPayload,
  loanPayrollPeriodLabel,
  loanTenorOptions,
  loanTermsText,
  previewInstallment,
  tenorForType,
  validateLoanForm,
} from "@/lib/hris/loans-view";
import { useCreateLoan } from "../mutations";
import { useEmployeeOptions } from "@/features/hris/shared/use-employee-options";
import { LoanAttachmentsFields } from "./loan-attachments-fields";

interface LoanFormDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

export function LoanFormDialog({ open, onOpenChange }: LoanFormDialogProps) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[90dvh] overflow-y-auto sm:max-w-md">
        <DialogHeader>
          <DialogTitle>Pengajuan Pinjaman / Kasbon</DialogTitle>
        </DialogHeader>
        <LoanFormBody onClose={() => onOpenChange(false)} />
      </DialogContent>
    </Dialog>
  );
}

function LoanFormBody({ onClose }: { onClose: () => void }) {
  const [form, setForm] = useState(EMPTY_LOAN_FORM);
  const [uploading, setUploading] = useState(false);
  const { data: employeeOptions = [] } = useEmployeeOptions();
  const create = useCreateLoan();
  const installment = previewInstallment(form);

  function handleSubmit() {
    const invalid = validateLoanForm(form);
    if (invalid) {
      toast.error(invalid);
      return;
    }
    create.mutate(loanPayload(form), {
      onSuccess: (res) => {
        toast.success(res.message || "Pengajuan pinjaman dibuat");
        onClose();
      },
      onError: (error) => toast.error(error.message || "Gagal membuat pengajuan"),
    });
  }

  return (
    <>
      <div className="space-y-3">
        <div>
          <label className="mb-1 block text-xs font-medium text-gray-600">Karyawan</label>
          <Combobox
            options={employeeOptions}
            value={form.employee_id}
            // Lampiran disimpan di folder karyawan: ganti karyawan = unggah ulang.
            onChange={(value) =>
              setForm((f) => (value === f.employee_id ? f : { ...f, employee_id: value, attachments: [], collateral_attachments: [] }))
            }
            placeholder="Pilih karyawan"
          />
        </div>
        <div>
          <label className="mb-1 block text-xs font-medium text-gray-600">Jenis</label>
          <Combobox
            options={LOAN_TYPE_OPTIONS}
            value={form.loan_type}
            onChange={(value) =>
              setForm((f) => ({ ...f, loan_type: value, tenor_months: tenorForType(value, f.tenor_months) }))
            }
            placeholder="Pilih jenis"
          />
        </div>
        <p className="rounded-lg bg-amber-50 px-3 py-2 text-xs text-amber-800">{loanTermsText(form.loan_type)}</p>
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
            <label className="mb-1 block text-xs font-medium text-gray-600">Cicilan</label>
            <Combobox
              options={loanTenorOptions(form.loan_type)}
              value={form.tenor_months}
              onChange={(value) => setForm((f) => ({ ...f, tenor_months: value }))}
              placeholder="Pilih cicilan"
            />
          </div>
        </div>
        <div>
          <label className="mb-1 block text-xs font-medium text-gray-600">
            Bunga flat (%/bulan — 0 untuk kasbon)
          </label>
          <Input
            type="number"
            min="0"
            step="0.5"
            value={form.interest_rate}
            onChange={(e) => setForm((f) => ({ ...f, interest_rate: e.target.value }))}
          />
        </div>
        <div>
          <label className="mb-1 block text-xs font-medium text-gray-600">Keperluan</label>
          <Input
            placeholder="cth. biaya sekolah anak"
            value={form.purpose}
            onChange={(e) => setForm((f) => ({ ...f, purpose: e.target.value }))}
          />
        </div>
        {form.employee_id ? (
          <LoanAttachmentsFields
            key={form.employee_id}
            loanType={form.loan_type}
            attachments={form.attachments}
            collateral={form.collateral_attachments}
            onAttachmentsChange={(paths) => setForm((f) => ({ ...f, attachments: paths }))}
            onCollateralChange={(paths) => setForm((f) => ({ ...f, collateral_attachments: paths }))}
            onBusyChange={setUploading}
            employeeId={form.employee_id}
          />
        ) : (
          <p className="text-xs text-gray-500">Pilih karyawan dulu untuk melampirkan dokumen.</p>
        )}
        <p className="rounded-lg bg-sky-50 px-3 py-2 text-xs text-sky-700">
          {installment !== null && (
            <>
              Perkiraan cicilan: <strong>{formatRupiah(installment)}</strong> × {form.tenor_months} kali.{" "}
            </>
          )}
          Dipotong mulai payroll <strong>{loanPayrollPeriodLabel(todayWib())}</strong> setelah disetujui (pengajuan tgl
          1–20 → payroll bulan ini; tgl 21–akhir bulan → payroll bulan berikutnya).
        </p>
      </div>
      <DialogFooter>
        <Button variant="outline" onClick={onClose}>
          Batal
        </Button>
        <Button onClick={handleSubmit} disabled={create.isPending || uploading}>
          {create.isPending ? <Loader2 className="h-4 w-4 animate-spin" /> : "Ajukan"}
        </Button>
      </DialogFooter>
    </>
  );
}
