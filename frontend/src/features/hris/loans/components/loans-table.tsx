import { useState } from "react";
import { Loader2 } from "lucide-react";
import { RequestDetailDialog } from "@/components/hris/request-detail-dialog";
import { Button } from "@/components/ui/button";
import { formatDateTime, formatRupiah } from "@/lib/format";
import { isAwaitingHr, loanPaidPercent, loanTypeLabel } from "@/lib/hris/loans-view";
import type { LoanRow } from "../types";

const STATUS_BADGES: Record<string, { label: string; className: string }> = {
  pending: { label: "Menunggu", className: "bg-amber-100 text-amber-700" },
  manager_approved: { label: "Disetujui Manager", className: "bg-indigo-100 text-indigo-700" },
  approved: { label: "Berjalan", className: "bg-green-100 text-green-700" },
  rejected: { label: "Ditolak", className: "bg-red-100 text-red-700" },
  paid_off: { label: "Lunas", className: "bg-sky-100 text-sky-700" },
};

interface LoansTableProps {
  rows: LoanRow[];
  loading: boolean;
  decidingId: string | null;
  onDecide: (id: string, approved: boolean) => void;
}

export function LoansTable({ rows, loading, decidingId, onDecide }: LoansTableProps) {
  return (
    <div className="overflow-x-auto rounded-xl border border-gray-200/70 bg-white shadow-sm">
      <table className="w-full text-sm">
        <thead>
          <tr className="border-b bg-gray-50 text-left text-xs uppercase tracking-wide text-gray-500">
            <th className="px-4 py-3">Karyawan</th>
            <th className="px-4 py-3">Departemen</th>
            <th className="px-4 py-3">Jenis</th>
            <th className="px-4 py-3">Pokok</th>
            <th className="px-4 py-3">Cicilan/Bulan</th>
            <th className="px-4 py-3">Tenor</th>
            <th className="px-4 py-3">Pelunasan</th>
            <th className="px-4 py-3">Status</th>
            <th className="px-4 py-3 text-right">Aksi</th>
          </tr>
        </thead>
        <tbody>
          {loading ? (
            <tr>
              <td colSpan={9} className="py-10 text-center text-gray-400">
                <Loader2 className="mx-auto h-6 w-6 animate-spin" />
              </td>
            </tr>
          ) : rows.length === 0 ? (
            <tr>
              <td colSpan={9} className="py-10 text-center text-gray-400">
                Belum ada pinjaman pada filter ini.
              </td>
            </tr>
          ) : (
            rows.map((row) => (
              <LoanTableRow key={row.id} row={row} deciding={decidingId === row.id} onDecide={onDecide} />
            ))
          )}
        </tbody>
      </table>
    </div>
  );
}

interface LoanTableRowProps {
  row: LoanRow;
  deciding: boolean;
  onDecide: (id: string, approved: boolean) => void;
}

function LoanTableRow({ row, deciding, onDecide }: LoanTableRowProps) {
  const badge = STATUS_BADGES[row.status] ?? STATUS_BADGES.pending;
  const paidPct = loanPaidPercent(row);
  const [showDetail, setShowDetail] = useState(false);
  const awaitingHr = isAwaitingHr(row);
  return (
    <tr className="border-b last:border-0 hover:bg-gray-50/60">
      <td className="px-4 py-3 font-medium text-gray-900">
        {row.employee
          ? `${row.employee.full_name}${row.employee.nip ? ` - [${row.employee.nip}]` : ""}`
          : "—"}
        {row.purpose ? (
          <p className="line-clamp-2 max-w-[220px] break-words text-xs font-normal text-gray-400" title={row.purpose}>
            {row.purpose}
          </p>
        ) : null}
      </td>
      <td className="px-4 py-3">{row.employee?.department?.name ?? "—"}</td>
      <td className="px-4 py-3">{loanTypeLabel(row.loan_type)}</td>
      <td className="px-4 py-3">{formatRupiah(row.principal_amount)}</td>
      <td className="px-4 py-3">{formatRupiah(row.monthly_installment)}</td>
      <td className="px-4 py-3">
        {row.tenor_months} bln
        {row.first_installment_month
          ? ` · mulai ${row.first_installment_month}/${row.first_installment_year}`
          : ""}
      </td>
      <td className="px-4 py-3">
        {row.status === "approved" || row.status === "paid_off" ? (
          <div className="min-w-[140px]">
            <div className="mb-1 flex justify-between text-xs text-gray-500">
              <span>{formatRupiah(row.paid_amount)}</span>
              <span>{paidPct}%</span>
            </div>
            <div className="h-1.5 w-full rounded-full bg-gray-100">
              <div className="h-1.5 rounded-full bg-green-500" style={{ width: `${paidPct}%` }} />
            </div>
            <p className="mt-1 text-xs text-gray-400">Sisa {formatRupiah(row.remaining_balance)}</p>
          </div>
        ) : row.status === "rejected" && row.rejection_reason ? (
          <span className="text-xs text-gray-400">{row.rejection_reason}</span>
        ) : (
          "—"
        )}
      </td>
      <td className="px-4 py-3">
        <span className={`rounded-full px-2 py-0.5 text-xs font-medium ${badge.className}`}>{badge.label}</span>
      </td>
      <td className="px-4 py-3 text-right">
        <Button
          size="sm"
          variant="ghost"
          className="mb-1 text-xs"
          aria-label={`Detail pinjaman ${row.employee?.full_name ?? ""}`}
          onClick={() => setShowDetail(true)}
        >
          Detail
        </Button>
        {awaitingHr ? (
          <div className="flex justify-end gap-2">
            <Button size="sm" disabled={deciding} onClick={() => onDecide(row.id, true)}>
              Setujui
            </Button>
            <Button size="sm" variant="outline" disabled={deciding} onClick={() => onDecide(row.id, false)}>
              Tolak
            </Button>
          </div>
        ) : row.status === "pending" ? (
          <span className="block text-xs text-gray-400">Menunggu Manager</span>
        ) : null}
        {showDetail && (
          <RequestDetailDialog
            open
            onOpenChange={setShowDetail}
            title="Detail pinjaman"
            employee={{
              name: row.employee?.full_name ?? "—",
              nip: row.employee?.nip,
              subtitle: row.employee?.department?.name,
            }}
            status={<span className={`rounded-full px-2 py-0.5 text-xs font-medium ${badge.className}`}>{badge.label}</span>}
            fields={[
              { label: "Jenis", value: loanTypeLabel(row.loan_type) },
              { label: "Pokok", value: formatRupiah(row.principal_amount) },
              { label: "Cicilan/bulan", value: formatRupiah(row.monthly_installment) },
              { label: "Tenor", value: `${row.tenor_months} bulan` },
              { label: "Diajukan", value: formatDateTime(row.created_at, "—") },
            ]}
            reason={{ label: "Keperluan", text: row.purpose }}
            notes={
              row.notes?.trim() || (row.status === "rejected" && row.rejection_reason) ? (
                <div className="space-y-2">
                  {row.notes?.trim() && (
                    <div>
                      <p className="mb-1 text-xs font-medium text-gray-600">Catatan</p>
                      <p className="whitespace-pre-line break-words text-sm text-gray-900">{row.notes}</p>
                    </div>
                  )}
                  {row.status === "rejected" && row.rejection_reason && (
                    <p className="whitespace-pre-line break-words text-xs text-red-600">
                      Alasan penolakan: {row.rejection_reason}
                    </p>
                  )}
                </div>
              ) : undefined
            }
            actions={
              awaitingHr ? (
                <>
                  <Button variant="outline" disabled={deciding} onClick={() => {
                    setShowDetail(false);
                    onDecide(row.id, false);
                  }}>
                    Tolak
                  </Button>
                  <Button disabled={deciding} onClick={() => {
                    setShowDetail(false);
                    onDecide(row.id, true);
                  }}>
                    Setujui
                  </Button>
                </>
              ) : undefined
            }
          />
        )}
      </td>
    </tr>
  );
}
