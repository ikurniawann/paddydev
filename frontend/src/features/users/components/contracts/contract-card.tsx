"use client";

import type { ReactNode } from "react";
import {
  ArrowDownTrayIcon,
  ClipboardDocumentCheckIcon,
  PencilSquareIcon,
  TrashIcon,
} from "@heroicons/react/24/outline";
import { Button } from "@/components/ui/button";
import { formatDate, formatRupiah } from "@/lib/format";
import type { EmployeeContractRow } from "../../api";
import {
  CONTRACT_STATUS_BADGES,
  CONTRACT_TYPE_LABELS,
  contractDocumentUrl,
  contractSignedDocumentUrl,
} from "./contract-labels";

export type DirectContractAction = "activate" | "end" | "convert";

interface ContractCardProps {
  contract: EmployeeContractRow;
  actionPending: boolean;
  deletePending: boolean;
  onAction: (action: DirectContractAction) => void;
  onEdit: () => void;
  onAdmin: () => void;
  onRenew: () => void;
  onTerminate: () => void;
  onDelete: () => void;
  /** Akses Penggajian: gaji pokok, kompensasi, dan PDF (memuat upah). */
  canViewSalary: boolean;
}

const rupiahOrDash = (value: string | null) => (value ? formatRupiah(value) : "-");

function Field({ label, children, wide }: { label: string; children: ReactNode; wide?: boolean }) {
  return (
    <div className={wide ? "col-span-2" : undefined}>
      <p className="text-xs text-gray-500">{label}</p>
      {children}
    </div>
  );
}

export function ContractCard({
  contract,
  actionPending,
  deletePending,
  onAction,
  onEdit,
  onAdmin,
  onRenew,
  onTerminate,
  onDelete,
  canViewSalary,
}: ContractCardProps) {
  const badge = CONTRACT_STATUS_BADGES[contract.status] ?? CONTRACT_STATUS_BADGES.draft;
  const isPkwt = contract.contract_type === "pkwt";

  return (
    <div className="rounded-xl border border-gray-200/70 bg-white p-4 shadow-sm">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div className="flex items-center gap-2">
          <span className="font-mono text-sm font-semibold text-gray-900">
            {contract.contract_number}
          </span>
          <span className={`rounded-full px-2 py-0.5 text-xs font-medium ${badge.className}`}>
            {badge.label}
          </span>
        </div>
        <div className="flex gap-1.5">
          {canViewSalary && (
            <Button
              size="sm"
              variant="ghost"
              className="gap-1"
              title="Unduh PDF surat perjanjian kerja"
              onClick={() => window.open(contractDocumentUrl(contract.id), "_blank")}
            >
              <ArrowDownTrayIcon className="h-4 w-4" /> PDF
            </Button>
          )}
          <Button
            size="sm"
            variant="ghost"
            className="gap-1"
            title="Administrasi: dokumen bertanda tangan, pencatatan Kemnaker"
            onClick={onAdmin}
          >
            <ClipboardDocumentCheckIcon className="h-4 w-4" /> Administrasi
          </Button>
          {contract.status === "draft" && (
            <>
              <Button
                size="sm"
                variant="ghost"
                className="gap-1"
                title="Edit isi draft kontrak"
                onClick={onEdit}
              >
                <PencilSquareIcon className="h-4 w-4" /> Edit
              </Button>
              <Button
                size="sm"
                variant="outline"
                disabled={actionPending}
                onClick={() => onAction("activate")}
              >
                Aktifkan
              </Button>
              <Button
                size="sm"
                variant="ghost"
                className="text-red-600"
                disabled={deletePending}
                onClick={onDelete}
              >
                <TrashIcon className="h-4 w-4" />
              </Button>
            </>
          )}
          {contract.status === "active" && (
            <>
              <Button
                size="sm"
                variant="outline"
                disabled={actionPending}
                onClick={() => onAction("end")}
              >
                Akhiri
              </Button>
              {isPkwt && (
                <>
                  <Button size="sm" variant="outline" disabled={actionPending} onClick={onRenew}>
                    Perpanjang
                  </Button>
                  <Button
                    size="sm"
                    variant="outline"
                    disabled={actionPending}
                    onClick={() => onAction("convert")}
                  >
                    Konversi ke Tetap
                  </Button>
                </>
              )}
              <Button
                size="sm"
                variant="ghost"
                className="text-red-600"
                disabled={actionPending}
                onClick={onTerminate}
              >
                Putus
              </Button>
            </>
          )}
        </div>
      </div>

      <div className="mt-3 grid grid-cols-2 gap-x-6 gap-y-1.5 text-sm sm:grid-cols-3">
        <Field label="Tipe">
          <p className="font-medium">{CONTRACT_TYPE_LABELS[contract.contract_type]}</p>
        </Field>
        <Field label="Periode">
          <p className="font-medium">
            {formatDate(contract.start_date)}
            {" — "}
            {isPkwt ? formatDate(contract.end_date) : "tanpa batas"}
          </p>
        </Field>
        {canViewSalary && (
          <Field label="Gaji pokok">
            <p className="font-medium">{rupiahOrDash(contract.base_salary ?? null)}</p>
          </Field>
        )}
        {contract.probation_end_date && (
          <Field label="Masa percobaan s.d.">
            <p className="font-medium">{formatDate(contract.probation_end_date)}</p>
          </Field>
        )}
        {contract.position_title && (
          <Field label="Jabatan">
            <p className="font-medium">{contract.position_title}</p>
          </Field>
        )}
        {canViewSalary && contract.compensation_amount && (
          <Field label="Uang kompensasi (PP 35/2021)">
            <p className="font-medium">
              {rupiahOrDash(contract.compensation_amount)}
              {contract.compensation_paid_at
                ? ` — dibayar ${formatDate(contract.compensation_paid_at)}`
                : " — belum dibayar"}
            </p>
          </Field>
        )}
        <Field label="Dokumen bertanda tangan">
          {contract.signed_document_url ? (
            <button
              type="button"
              className="font-medium text-blue-600 hover:underline"
              onClick={() => window.open(contractSignedDocumentUrl(contract.id), "_blank")}
            >
              Lihat dokumen
              {contract.signed_at ? ` (ttd ${formatDate(contract.signed_at)})` : ""}
            </button>
          ) : (
            <p className="font-medium text-gray-400">Belum diunggah</p>
          )}
        </Field>
        <Field label="Pencatatan Kemnaker">
          <p className="font-medium">
            {contract.kemnaker_registered_at
              ? formatDate(contract.kemnaker_registered_at)
              : isPkwt
                ? "Belum dicatatkan"
                : "-"}
          </p>
        </Field>
        {contract.terminated_reason && (
          <Field label="Alasan pemutusan" wide>
            <p className="font-medium">{contract.terminated_reason}</p>
          </Field>
        )}
      </div>
    </div>
  );
}
