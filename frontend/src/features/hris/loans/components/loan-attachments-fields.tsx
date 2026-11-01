"use client";

import { PrivateAttachmentsField } from "@/components/hris/private-attachments-field";
import { MAX_LOAN_ATTACHMENTS, loanAttachmentHref } from "@/lib/hris/loans-view";
import { uploadLoanAttachment } from "../api";

interface LoanAttachmentsFieldsProps {
  loanType: string;
  attachments: string[];
  collateral: string[];
  onAttachmentsChange: (paths: string[]) => void;
  onCollateralChange: (paths: string[]) => void;
  onBusyChange?: (busy: boolean) => void;
  /** HR mengajukan untuk karyawan lain: file disimpan di folder karyawan itu. */
  employeeId?: string;
  disabled?: boolean;
}

/**
 * Lampiran pengajuan (EPIC-076 R8): kasbon wajib dokumen pendukung; Pinjaman
 * Karyawan wajib jaminan (surat berharga), dokumen pendukung opsional.
 */
export function LoanAttachmentsFields({
  loanType,
  attachments,
  collateral,
  onAttachmentsChange,
  onCollateralChange,
  onBusyChange,
  employeeId,
  disabled,
}: LoanAttachmentsFieldsProps) {
  const upload = (file: File) => uploadLoanAttachment(file, employeeId);
  const kasbon = loanType === "kasbon";
  return (
    <div className="space-y-3">
      {!kasbon && (
        <PrivateAttachmentsField
          value={collateral}
          onChange={onCollateralChange}
          onBusyChange={onBusyChange}
          upload={upload}
          label="Jaminan / surat berharga (wajib, 1–3 file PDF/foto)"
          noun="jaminan"
          inputLabel="Pilih file jaminan"
          tooManyMessage={`Maksimal ${MAX_LOAN_ATTACHMENTS} file jaminan`}
          max={MAX_LOAN_ATTACHMENTS}
          disabled={disabled}
        />
      )}
      <PrivateAttachmentsField
        value={attachments}
        onChange={onAttachmentsChange}
        onBusyChange={onBusyChange}
        upload={upload}
        label={kasbon ? "Dokumen pendukung (wajib, 1–3 file PDF/foto)" : "Dokumen pendukung (opsional, maks 3 file)"}
        noun="dokumen pendukung"
        inputLabel="Pilih file dokumen pendukung"
        tooManyMessage={`Maksimal ${MAX_LOAN_ATTACHMENTS} dokumen pendukung`}
        max={MAX_LOAN_ATTACHMENTS}
        disabled={disabled}
      />
    </div>
  );
}

/** Kelompok lampiran untuk RequestDetailDialog: jaminan lalu dokumen pendukung. */
export function loanAttachmentGroups(row: { attachments?: string[] | null; collateral_attachments?: string[] | null }) {
  return [
    { title: "Jaminan (surat berharga)", label: "Jaminan", paths: row.collateral_attachments ?? [], hrefFor: loanAttachmentHref },
    { title: "Dokumen pendukung", label: "Dokumen", paths: row.attachments ?? [], hrefFor: loanAttachmentHref },
  ];
}
