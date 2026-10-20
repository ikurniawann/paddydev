"use client";

import { useState } from "react";
import { Loader2 } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Combobox } from "@/components/ui/combobox";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter } from "@/components/ui/dialog";
import { formatRupiah } from "@/lib/format";
import {
  EMPTY_LOAN_FORM,
  LOAN_TYPE_OPTIONS,
  kasbonTenorOptions,
  loanPayload,
  previewInstallment,
  validateLoanForm,
} from "@/lib/hris/loans-view";
import { useCreateLoan } from "../mutations";
import { useEmployeeOptions } from "@/features/hris/shared/use-employee-options";

interface LoanFormDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

export function LoanFormDialog({ open, onOpenChange }: LoanFormDialogProps) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-md">
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
            onChange={(value) => setForm((f) => ({ ...f, employee_id: value }))}
            placeholder="Pilih karyawan"
          />
        </div>
        <div>
          <label className="mb-1 block text-xs font-medium text-gray-600">Jenis</label>
          <Combobox
            options={LOAN_TYPE_OPTIONS}
            value={form.loan_type}
            onChange={(value) =>
              setForm((f) => ({
                ...f,
                loan_type: value,
                tenor_months: value === "kasbon" && !["1", "2"].includes(f.tenor_months) ? "2" : f.tenor_months,
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
        {installment !== null && (
          <p className="rounded-lg bg-sky-50 px-3 py-2 text-xs text-sky-700">
            Perkiraan cicilan: <strong>{formatRupiah(installment)}</strong>
            /bulan × {form.tenor_months} bulan. Cicilan mulai terpotong di
            payroll bulan setelah approval. Batas cicilan mengikuti
            pengaturan payroll (default 30% gaji pokok).
          </p>
        )}
      </div>
      <DialogFooter>
        <Button variant="outline" onClick={onClose}>
          Batal
        </Button>
        <Button onClick={handleSubmit} disabled={create.isPending}>
          {create.isPending ? <Loader2 className="h-4 w-4 animate-spin" /> : "Ajukan"}
        </Button>
      </DialogFooter>
    </>
  );
}
