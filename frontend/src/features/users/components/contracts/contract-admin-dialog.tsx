"use client";

import { useRef, useState } from "react";
import { Loader2 } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { ConfirmDialog } from "@/components/ui/confirm-dialog";
import { Input } from "@/components/ui/input";
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  contractAdminFormFromRow,
  contractAdminPayload,
  type ContractAdminFormValues,
} from "@/lib/hris/employee-profile-contracts";
import type { EmployeeContractRow } from "../../api";
import {
  useContractAction,
  useDeleteContractSignedDocument,
  useUploadContractSignedDocument,
} from "../../mutations";
import { ContractField, PkwtNotice } from "./contract-field";
import { contractSignedDocumentUrl, errorMessage } from "./contract-labels";

interface ContractAdminDialogProps {
  employeeId: string;
  /** Versi live dari cache query: status upload/hapus dokumen selalu segar. */
  contract: EmployeeContractRow;
  onClose: () => void;
  /** Scan bertanda tangan memuat upah: hanya akses Penggajian yang bisa membukanya. */
  canViewSalary: boolean;
}

/** Dokumen bertanda tangan dan tanggal administrasi (ttd, Kemnaker, kompensasi). */
export function ContractAdminDialog({ employeeId, contract, onClose, canViewSalary }: ContractAdminDialogProps) {
  const actionMutation = useContractAction(employeeId);
  const uploadMutation = useUploadContractSignedDocument(employeeId);
  const deleteSignedMutation = useDeleteContractSignedDocument(employeeId);
  const [form, setForm] = useState<ContractAdminFormValues>(() =>
    contractAdminFormFromRow(contract)
  );
  const [confirmDeleteOpen, setConfirmDeleteOpen] = useState(false);
  const fileInputRef = useRef<HTMLInputElement>(null);

  const setField = (patch: Partial<ContractAdminFormValues>) =>
    setForm((f) => ({ ...f, ...patch }));

  async function handleUpload(file: File) {
    try {
      const res = await uploadMutation.mutateAsync({
        contractId: contract.id,
        file,
      });
      toast.success(res.message ?? "Dokumen bertanda tangan tersimpan");
    } catch (error) {
      toast.error(errorMessage(error, "Upload gagal"));
    } finally {
      if (fileInputRef.current) fileInputRef.current.value = "";
    }
  }

  async function handleDeleteSigned() {
    try {
      await deleteSignedMutation.mutateAsync(contract.id);
      toast.success("Dokumen bertanda tangan dihapus");
      setConfirmDeleteOpen(false);
    } catch (error) {
      toast.error(errorMessage(error, "Gagal menghapus"));
    }
  }

  async function handleSave() {
    try {
      const res = await actionMutation.mutateAsync({
        contractId: contract.id,
        action: "update",
        ...contractAdminPayload(form),
      });
      toast.success(res.message ?? "Kontrak diperbarui");
      onClose();
    } catch (error) {
      toast.error(errorMessage(error, "Gagal menyimpan"));
    }
  }

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle>Administrasi Kontrak</DialogTitle>
        </DialogHeader>
        <div className="space-y-4">
          <p className="text-sm text-gray-600">
            Kontrak <span className="font-mono">{contract.contract_number}</span> — dokumen bertanda
            tangan &amp; tanggal administrasi.
          </p>

          <ContractField label="Dokumen bertanda tangan (PDF/JPG/PNG/WebP, maks 10 MB)">
            <input
              ref={fileInputRef}
              type="file"
              accept="application/pdf,image/jpeg,image/png,image/webp"
              className="hidden"
              onChange={(e) => {
                const file = e.target.files?.[0];
                if (file) void handleUpload(file);
              }}
            />
            <div className="flex flex-wrap gap-1.5">
              <Button
                size="sm"
                variant="outline"
                disabled={uploadMutation.isPending}
                onClick={() => fileInputRef.current?.click()}
              >
                {uploadMutation.isPending ? (
                  <Loader2 className="h-4 w-4 animate-spin" />
                ) : contract.signed_document_url ? (
                  "Ganti Dokumen"
                ) : (
                  "Upload Dokumen"
                )}
              </Button>
              {contract.signed_document_url && (
                <>
                  {canViewSalary && (
                    <Button
                      size="sm"
                      variant="ghost"
                      onClick={() => window.open(contractSignedDocumentUrl(contract.id), "_blank")}
                    >
                      Lihat
                    </Button>
                  )}
                  <Button
                    size="sm"
                    variant="ghost"
                    className="text-red-600"
                    disabled={deleteSignedMutation.isPending}
                    onClick={() => setConfirmDeleteOpen(true)}
                  >
                    Hapus
                  </Button>
                </>
              )}
            </div>
          </ContractField>

          <div className="grid grid-cols-2 gap-3">
            <ContractField label="Tanggal tanda tangan">
              <Input
                type="date"
                value={form.signed_at}
                onChange={(e) => setField({ signed_at: e.target.value })}
              />
            </ContractField>
            <ContractField label="Dicatatkan ke Kemnaker">
              <Input
                type="date"
                value={form.kemnaker_registered_at}
                onChange={(e) => setField({ kemnaker_registered_at: e.target.value })}
              />
            </ContractField>
            {contract.compensation_amount && (
              <ContractField label="Kompensasi dibayar tanggal">
                <Input
                  type="date"
                  value={form.compensation_paid_at}
                  onChange={(e) => setField({ compensation_paid_at: e.target.value })}
                />
              </ContractField>
            )}
          </div>
          {contract.contract_type === "pkwt" && (
            <PkwtNotice>
              PKWT wajib dicatatkan ke Kementerian Ketenagakerjaan paling lambat 3 hari kerja sejak
              penandatanganan (daring via wajiblapor.kemnaker.go.id).
            </PkwtNotice>
          )}
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={onClose}>
            Tutup
          </Button>
          <Button onClick={handleSave} disabled={actionMutation.isPending}>
            {actionMutation.isPending ? (
              <Loader2 className="h-4 w-4 animate-spin" />
            ) : (
              "Simpan Tanggal"
            )}
          </Button>
        </DialogFooter>
      </DialogContent>

      <ConfirmDialog
        open={confirmDeleteOpen}
        onOpenChange={setConfirmDeleteOpen}
        title="Hapus dokumen bertanda tangan kontrak ini?"
        confirmLabel="Hapus"
        loading={deleteSignedMutation.isPending}
        onConfirm={handleDeleteSigned}
      />
    </Dialog>
  );
}
