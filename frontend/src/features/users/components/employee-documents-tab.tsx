"use client";

import { useState } from "react";
import {
  ArrowDownTrayIcon,
  ArrowUpTrayIcon,
  BriefcaseIcon,
  CheckCircleIcon,
  DocumentTextIcon,
  IdentificationIcon,
  TrashIcon,
} from "@heroicons/react/24/outline";
import { toast } from "sonner";
import { Card, CardContent } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { ConfirmDialog } from "@/components/ui/confirm-dialog";
import { formatDate } from "@/lib/format";
import {
  useCanViewContractSalary,
  useEmployeeContracts,
  useEmployeeDocuments,
  useEmployeeRecruitmentDocs,
} from "../queries";
import { useDeleteEmployeeDocument } from "../mutations";
import {
  CONTRACT_STATUS_BADGES,
  contractDocumentUrl,
  contractSignedDocumentUrl,
} from "./contracts/contract-labels";
import { DocumentRow } from "./documents/document-row";
import { DOC_TYPE_LABELS, DocumentUploadDialog } from "./documents/document-upload-dialog";
import { RecruitmentDocumentsSection } from "./documents/recruitment-documents-section";

/**
 * Tab "Dokumen" di detail karyawan (HRD/super admin): SEMUA lampiran milik
 * karyawan dalam satu tempat: dokumen asal rekrutmen (CV, Laporan Pipeline),
 * dokumen kontrak (draft PDF + kontrak bertanda tangan per kontrak), dan
 * dokumen kepegawaian yang diupload manual.
 */

const CONTRACT_TYPE_LABELS: Record<string, string> = {
  pkwtt: "PKWTT",
  pkwt: "PKWT",
};

export function EmployeeDocumentsTab({ employeeId }: { employeeId: string }) {
  const { data: documents = [], isLoading: documentsLoading } = useEmployeeDocuments(employeeId);
  const { data: contracts = [], isLoading: contractsLoading } = useEmployeeContracts(employeeId);
  // PDF & scan kontrak memuat upah: hanya akses Penggajian (EPIC-057).
  const canViewSalary = useCanViewContractSalary(employeeId);
  const { data: recruitmentDocs, isLoading: recruitmentLoading } =
    useEmployeeRecruitmentDocs(employeeId);
  const deleteDocumentMutation = useDeleteEmployeeDocument(employeeId);

  const [uploadOpen, setUploadOpen] = useState(false);
  const [deleteDocId, setDeleteDocId] = useState<string | null>(null);

  const loading = documentsLoading || contractsLoading || recruitmentLoading;

  async function handleDeleteDocument(docId: string) {
    try {
      await deleteDocumentMutation.mutateAsync(docId);
      toast.success("Dokumen dihapus");
      setDeleteDocId(null);
    } catch {
      toast.error("Gagal menghapus dokumen");
    }
  }

  if (loading) {
    return (
      <div className="flex justify-center py-12">
        <div className="animate-spin w-6 h-6 border-2 border-gray-300 border-t-blue-500 rounded-full" />
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <RecruitmentDocumentsSection docs={recruitmentDocs ?? null} />

      {/* ── Dokumen kontrak: draft PDF + kontrak bertanda tangan ── */}
      <div className="space-y-3">
        <h3 className="flex items-center gap-2 font-semibold text-gray-700">
          <BriefcaseIcon className="w-4 h-4 text-gray-400" /> Dokumen Kontrak
        </h3>
        {contracts.length === 0 ? (
          <p className="text-sm text-gray-400">Belum ada kontrak untuk karyawan ini.</p>
        ) : (
          <div className="space-y-2">
            {contracts.map((contract) => {
              const statusBadge = CONTRACT_STATUS_BADGES[contract.status] ?? {
                label: contract.status,
                className: "bg-gray-100 text-gray-600",
              };
              return (
                <div key={contract.id} className="space-y-2">
                  <DocumentRow
                    icon={<DocumentTextIcon className="w-7 h-7 shrink-0 text-amber-600" />}
                    title={`${contract.status === "draft" ? "Draft Kontrak" : "Surat Perjanjian Kerja"} — ${contract.contract_number}`}
                    subtitle={
                      <span className="flex flex-wrap items-center gap-1.5">
                        {CONTRACT_TYPE_LABELS[contract.contract_type] ?? contract.contract_type}
                        <Badge className={statusBadge.className}>{statusBadge.label}</Badge>
                        {`Mulai ${formatDate(contract.start_date)}`}
                        {contract.end_date ? ` s.d. ${formatDate(contract.end_date)}` : ""}
                      </span>
                    }
                    action={
                      canViewSalary ? (
                        <Button
                          size="sm"
                          variant="outline"
                          onClick={() => window.open(contractDocumentUrl(contract.id), "_blank")}
                          className="gap-1"
                        >
                          <ArrowDownTrayIcon className="w-4 h-4" /> PDF
                        </Button>
                      ) : undefined
                    }
                  />
                  {contract.signed_document_url ? (
                    <DocumentRow
                      icon={<CheckCircleIcon className="w-7 h-7 shrink-0 text-green-600" />}
                      title={`Kontrak Bertanda Tangan — ${contract.contract_number}`}
                      subtitle={
                        contract.signed_at
                          ? `Ditandatangani ${formatDate(contract.signed_at)}`
                          : "Dokumen hasil scan yang diupload"
                      }
                      action={
                        canViewSalary ? (
                          <Button
                            size="sm"
                            variant="outline"
                            onClick={() =>
                              window.open(contractSignedDocumentUrl(contract.id), "_blank")
                            }
                            className="gap-1"
                          >
                            <ArrowDownTrayIcon className="w-4 h-4" /> Lihat
                          </Button>
                        ) : undefined
                      }
                    />
                  ) : (
                    <p className="pl-2 text-xs text-gray-400">
                      Dokumen bertanda tangan belum diunggah — kelola lewat tab Kontrak.
                    </p>
                  )}
                </div>
              );
            })}
          </div>
        )}
      </div>

      {/* ── Dokumen kepegawaian (upload manual) ── */}
      <div className="space-y-3">
        <div className="flex justify-between items-center">
          <h3 className="flex items-center gap-2 font-semibold text-gray-700">
            <IdentificationIcon className="w-4 h-4 text-gray-400" /> Dokumen Kepegawaian
          </h3>
          <Button size="sm" onClick={() => setUploadOpen(true)} className="gap-1">
            <ArrowUpTrayIcon className="w-4 h-4" /> Upload Dokumen
          </Button>
        </div>
        {documents.length === 0 ? (
          <Card>
            <CardContent className="py-12 text-center text-gray-400">
              <DocumentTextIcon className="w-8 h-8 mx-auto mb-2 text-gray-300" />
              Belum ada dokumen. Klik &quot;Upload Dokumen&quot; untuk menambahkan.
            </CardContent>
          </Card>
        ) : (
          <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
            {documents.map((doc) => (
              <Card key={doc.id} className="hover:shadow-md transition-shadow">
                <CardContent className="p-4">
                  <div className="flex items-start justify-between gap-2">
                    <div className="flex-1 min-w-0">
                      <p className="text-xs font-medium text-blue-600 uppercase tracking-wide">
                        {DOC_TYPE_LABELS[doc.document_type] || doc.document_type}
                      </p>
                      <p className="font-medium text-gray-900 text-sm mt-0.5 truncate">
                        {doc.document_name}
                      </p>
                      {doc.issue_date && (
                        <p className="text-xs text-gray-400 mt-1">
                          Terbit: {formatDate(doc.issue_date)}
                        </p>
                      )}
                      {doc.expiry_date && (
                        <p
                          className={`text-xs mt-0.5 ${new Date(doc.expiry_date) < new Date() ? "text-red-500" : "text-gray-400"}`}
                        >
                          Kedaluwarsa: {formatDate(doc.expiry_date)}
                        </p>
                      )}
                    </div>
                    <div className="flex gap-1 shrink-0">
                      <Button
                        size="sm"
                        variant="ghost"
                        onClick={() => window.open(doc.file_url, "_blank")}
                        className="text-blue-600 hover:bg-blue-50 p-1.5"
                        title="Lihat dokumen"
                      >
                        <IdentificationIcon className="w-4 h-4" />
                      </Button>
                      <Button
                        size="sm"
                        variant="ghost"
                        onClick={() => setDeleteDocId(doc.id)}
                        className="text-red-500 hover:bg-red-50 p-1.5"
                        title="Hapus"
                      >
                        <TrashIcon className="w-4 h-4" />
                      </Button>
                    </div>
                  </div>
                  {doc.is_verified && (
                    <div className="flex items-center gap-1 mt-2 text-xs text-green-600">
                      <CheckCircleIcon className="w-3.5 h-3.5" /> Terverifikasi
                    </div>
                  )}
                </CardContent>
              </Card>
            ))}
          </div>
        )}
      </div>

      {uploadOpen && (
        <DocumentUploadDialog employeeId={employeeId} onClose={() => setUploadOpen(false)} />
      )}
      <ConfirmDialog
        open={deleteDocId !== null}
        onOpenChange={(open) => !open && setDeleteDocId(null)}
        title="Hapus dokumen ini?"
        confirmLabel="Hapus"
        loading={deleteDocumentMutation.isPending}
        onConfirm={() => deleteDocId && handleDeleteDocument(deleteDocId)}
      />
    </div>
  );
}
