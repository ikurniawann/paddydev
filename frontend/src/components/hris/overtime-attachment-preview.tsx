"use client";

import { overtimeAttachmentHref } from "@/lib/hris/overtime-rules";
import { AttachmentPreviewButtons } from "./attachment-preview";

/** Dokumentasi bukti lembur (EPIC-066): "Dokumentasi n" dengan pratinjau di dalam aplikasi. */
export function OvertimeAttachmentButtons(props: {
  paths: string[];
  className?: string;
  buttonClassName?: string;
  context?: string;
}) {
  return <AttachmentPreviewButtons {...props} hrefFor={overtimeAttachmentHref} />;
}
