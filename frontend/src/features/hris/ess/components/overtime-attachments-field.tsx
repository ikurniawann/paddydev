"use client";

import { PrivateAttachmentsField } from "@/components/hris/private-attachments-field";
import { MAX_OVERTIME_ATTACHMENTS } from "@/lib/hris/overtime-rules";
import { uploadOvertimeAttachment } from "../api";

interface OvertimeAttachmentsFieldProps {
  value: string[];
  onChange: (paths: string[]) => void;
  /** Dipanggil saat unggahan mulai/selesai agar form tidak dikirim selagi mengunggah. */
  onBusyChange?: (busy: boolean) => void;
}

/** Dokumentasi lembur (EPIC-066): 1–3 file PDF/JPG/PNG/WebP ≤ 5 MB, diunggah satu per satu. */
export function OvertimeAttachmentsField({ value, onChange, onBusyChange }: OvertimeAttachmentsFieldProps) {
  return (
    <PrivateAttachmentsField
      value={value}
      onChange={onChange}
      onBusyChange={onBusyChange}
      upload={uploadOvertimeAttachment}
      label="Dokumentasi (wajib, 1–3 file PDF/foto)"
      noun="dokumentasi"
      inputLabel="Pilih file dokumentasi lembur"
      tooManyMessage={`Maksimal ${MAX_OVERTIME_ATTACHMENTS} dokumentasi lembur`}
      max={MAX_OVERTIME_ATTACHMENTS}
    />
  );
}
