"use client";

import { useState } from "react";
import { Loader2 } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Combobox } from "@/components/ui/combobox";
import { Input } from "@/components/ui/input";
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  EMPTY_CONTRACT_FORM,
  contractFormFromRow,
  contractFormPayload,
  type ContractFormValues,
  type ContractType,
} from "@/lib/hris/employee-profile-contracts";
import type { EmployeeContractRow } from "../../api";
import { useContractAction, useCreateEmployeeContract } from "../../mutations";
import { ContractField, PkwtNotice } from "./contract-field";
import { CONTRACT_TYPE_LABELS, errorMessage } from "./contract-labels";

const TYPE_OPTIONS = [
  { value: "pkwt", label: CONTRACT_TYPE_LABELS.pkwt },
  { value: "pkwtt", label: CONTRACT_TYPE_LABELS.pkwtt },
];

interface ContractFormDialogProps {
  employeeId: string;
  /** null = buat kontrak baru; isi = edit draft. */
  contract: EmployeeContractRow | null;
  onClose: () => void;
  /** Tanpa akses Penggajian: kolom gaji pokok disembunyikan dan tidak dikirim. */
  canViewSalary: boolean;
}

/** Buat kontrak baru atau edit isi draft. Dipasang ulang (key) tiap dibuka. */
export function ContractFormDialog({ employeeId, contract, onClose, canViewSalary }: ContractFormDialogProps) {
  const createMutation = useCreateEmployeeContract(employeeId);
  const actionMutation = useContractAction(employeeId);
  const [form, setForm] = useState<ContractFormValues>(() =>
    contract ? contractFormFromRow(contract) : EMPTY_CONTRACT_FORM
  );
  const isPkwt = form.contract_type === "pkwt";
  const pending = createMutation.isPending || actionMutation.isPending;

  const setField = (patch: Partial<ContractFormValues>) => setForm((f) => ({ ...f, ...patch }));

  async function handleSubmit() {
    if (!form.start_date) {
      toast.error("Tanggal mulai wajib diisi");
      return;
    }
    const { base_salary, ...rest } = contractFormPayload(form);
    const payload = canViewSalary ? { ...rest, base_salary } : rest;
    try {
      if (contract) {
        const res = await actionMutation.mutateAsync({
          contractId: contract.id,
          action: "edit",
          ...payload,
        });
        toast.success(res.message ?? "Draft kontrak diperbarui");
      } else {
        const res = await createMutation.mutateAsync({
          contract_type: form.contract_type,
          ...payload,
        });
        toast.success(res.message ?? "Draft kontrak dibuat");
      }
      onClose();
    } catch (error) {
      toast.error(errorMessage(error, "Gagal menyimpan kontrak"));
    }
  }

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle>
            {contract ? `Edit Draft ${contract.contract_number}` : "Buat Kontrak Baru"}
          </DialogTitle>
        </DialogHeader>
        <div className="space-y-3">
          <ContractField label="Tipe kontrak">
            {contract ? (
              <p className="rounded-lg border border-gray-200 bg-gray-50 px-3 py-2 text-sm text-gray-600">
                {CONTRACT_TYPE_LABELS[form.contract_type]} — tipe tidak bisa diubah (hapus draft
                lalu buat ulang bila salah tipe)
              </p>
            ) : (
              <Combobox
                options={TYPE_OPTIONS}
                value={form.contract_type}
                onChange={(value) => setField({ contract_type: value as ContractType })}
                placeholder="Pilih tipe"
              />
            )}
          </ContractField>
          <div className="grid grid-cols-2 gap-3">
            <ContractField label="Tanggal mulai">
              <Input
                type="date"
                value={form.start_date}
                onChange={(e) => setField({ start_date: e.target.value })}
              />
            </ContractField>
            {isPkwt ? (
              <ContractField label="Tanggal berakhir">
                <Input
                  type="date"
                  value={form.end_date}
                  onChange={(e) => setField({ end_date: e.target.value })}
                />
              </ContractField>
            ) : (
              <ContractField label="Akhir masa percobaan (ops.)">
                <Input
                  type="date"
                  value={form.probation_end_date}
                  onChange={(e) => setField({ probation_end_date: e.target.value })}
                />
              </ContractField>
            )}
          </div>
          <div className="grid grid-cols-2 gap-3">
            {canViewSalary && (
              <ContractField label="Gaji pokok (ops. — default gaji aktif)">
                <Input
                  type="number"
                  placeholder="cth. 4500000"
                  value={form.base_salary}
                  onChange={(e) => setField({ base_salary: e.target.value })}
                />
              </ContractField>
            )}
            <ContractField label="Lokasi kerja (ops.)">
              <Input
                placeholder="cth. Kantor Pusat Jakarta"
                value={form.work_location}
                onChange={(e) => setField({ work_location: e.target.value })}
              />
            </ContractField>
          </div>
          <ContractField label="Catatan (ops.)">
            <Input value={form.notes} onChange={(e) => setField({ notes: e.target.value })} />
          </ContractField>
          {isPkwt && (
            <PkwtNotice>
              PKWT wajib punya tanggal berakhir, tidak boleh ada masa percobaan, dan total seluruh
              PKWT karyawan ini maksimal 5 tahun — sistem menolak otomatis bila terlampaui.
            </PkwtNotice>
          )}
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={onClose}>
            Batal
          </Button>
          <Button onClick={handleSubmit} disabled={pending}>
            {pending ? (
              <Loader2 className="h-4 w-4 animate-spin" />
            ) : contract ? (
              "Simpan Perubahan"
            ) : (
              "Simpan Draft"
            )}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
