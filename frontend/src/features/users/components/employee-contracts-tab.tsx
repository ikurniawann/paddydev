"use client";

import { useState } from "react";
import { PlusIcon } from "@heroicons/react/24/outline";
import { Loader2 } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { ConfirmDialog } from "@/components/ui/confirm-dialog";
import { contractActionMessage } from "@/lib/hris/employee-profile-contracts";
import type { EmployeeContractRow } from "../api";
import { useContractAction, useDeleteEmployeeContract } from "../mutations";
import { useCanViewContractSalary, useEmployeeContracts } from "../queries";
import { ContractAdminDialog } from "./contracts/contract-admin-dialog";
import { ContractCard, type DirectContractAction } from "./contracts/contract-card";
import { ContractFormDialog } from "./contracts/contract-form-dialog";
import { errorMessage } from "./contracts/contract-labels";
import { ContractRenewDialog } from "./contracts/contract-renew-dialog";
import { ContractTerminateDialog } from "./contracts/contract-terminate-dialog";

/**
 * Tab "Kontrak" di detail karyawan: daftar kontrak PKWTT/PKWT + aksi siklus
 * hidup (aktifkan, akhiri, putus, konversi). Aturan compliance (batas PKWT
 * 5 tahun, larangan probation PKWT) ditegakkan server-side; UI menampilkan
 * pesan errornya apa adanya.
 */

type OpenDialog =
  | { kind: "form"; contract: EmployeeContractRow | null }
  | { kind: "renew" | "terminate" | "delete"; contract: EmployeeContractRow }
  | { kind: "admin"; contractId: string };

export function EmployeeContractsTab({ employeeId }: { employeeId: string }) {
  const { data: contracts = [], isLoading } = useEmployeeContracts(employeeId);
  const canViewSalary = useCanViewContractSalary(employeeId);
  const actionMutation = useContractAction(employeeId);
  const deleteMutation = useDeleteEmployeeContract(employeeId);
  const [dialog, setDialog] = useState<OpenDialog | null>(null);
  const close = () => setDialog(null);

  const adminContract =
    dialog?.kind === "admin" ? contracts.find((c) => c.id === dialog.contractId) : undefined;

  async function handleAction(contract: EmployeeContractRow, action: DirectContractAction) {
    try {
      const res = await actionMutation.mutateAsync({
        contractId: contract.id,
        action,
      });
      toast.success(contractActionMessage(res));
    } catch (error) {
      toast.error(errorMessage(error, "Aksi gagal"));
    }
  }

  async function handleDelete(contract: EmployeeContractRow) {
    try {
      await deleteMutation.mutateAsync(contract.id);
      toast.success("Draft kontrak dihapus");
      close();
    } catch (error) {
      toast.error(errorMessage(error, "Gagal menghapus"));
    }
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <div>
          <h3 className="text-sm font-semibold text-gray-800">Kontrak Kerja</h3>
          <p className="text-xs text-gray-500">
            PKWT maksimal total 5 tahun & tanpa masa percobaan; PKWTT probation maks 3 bulan (UU
            13/2003 jo. PP 35/2021).
          </p>
        </div>
        <Button
          size="sm"
          className="gap-1.5"
          onClick={() => setDialog({ kind: "form", contract: null })}
        >
          <PlusIcon className="h-4 w-4" /> Buat Kontrak
        </Button>
      </div>

      {isLoading ? (
        <div className="flex justify-center py-10">
          <Loader2 className="h-6 w-6 animate-spin text-gray-400" />
        </div>
      ) : contracts.length === 0 ? (
        <p className="rounded-lg border border-dashed border-gray-200 py-10 text-center text-sm text-gray-400">
          Belum ada kontrak. Buat kontrak pertama untuk karyawan ini.
        </p>
      ) : (
        <div className="space-y-3">
          {contracts.map((contract) => (
            <ContractCard
              key={contract.id}
              contract={contract}
              actionPending={actionMutation.isPending}
              deletePending={deleteMutation.isPending}
              onAction={(action) => handleAction(contract, action)}
              onEdit={() => setDialog({ kind: "form", contract })}
              onAdmin={() => setDialog({ kind: "admin", contractId: contract.id })}
              onRenew={() => setDialog({ kind: "renew", contract })}
              onTerminate={() => setDialog({ kind: "terminate", contract })}
              onDelete={() => setDialog({ kind: "delete", contract })}
              canViewSalary={canViewSalary}
            />
          ))}
        </div>
      )}

      {dialog?.kind === "form" && (
        <ContractFormDialog
          key={dialog.contract?.id ?? "new"}
          employeeId={employeeId}
          contract={dialog.contract}
          onClose={close}
          canViewSalary={canViewSalary}
        />
      )}
      {dialog?.kind === "renew" && (
        <ContractRenewDialog employeeId={employeeId} contract={dialog.contract} onClose={close} />
      )}
      {dialog?.kind === "terminate" && (
        <ContractTerminateDialog
          employeeId={employeeId}
          contract={dialog.contract}
          onClose={close}
        />
      )}
      {adminContract && (
        <ContractAdminDialog
          employeeId={employeeId}
          contract={adminContract}
          onClose={close}
          canViewSalary={canViewSalary}
        />
      )}
      <ConfirmDialog
        open={dialog?.kind === "delete"}
        onOpenChange={(open) => !open && close()}
        title={
          dialog?.kind === "delete" ? `Hapus draft kontrak ${dialog.contract.contract_number}?` : ""
        }
        confirmLabel="Hapus"
        loading={deleteMutation.isPending}
        onConfirm={() => dialog?.kind === "delete" && handleDelete(dialog.contract)}
      />
    </div>
  );
}
