"use client";

import { ArrowDownTrayIcon } from "@heroicons/react/24/outline";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from "@/components/ui/dialog";
import { formatRupiah } from "@/lib/format";
import { monthYearLabel } from "@/lib/hris/month-label";
import { loanInstallmentLabel } from "@/lib/payroll/loans";
import { formatShareOfGross } from "@/lib/payroll/share";
import { prorateNote } from "@/lib/payroll/prorate-note";
import { SlipStoreBreakdown } from "@/features/hris/payroll/components/store-breakdown";
import type { EssPayslip } from "../types";

export function payslipPeriodLabel(slip: EssPayslip): string {
  const run = slip.payroll_run;
  if (!run) return "—";
  const period = monthYearLabel(run.period_month ?? 1, run.period_year);
  return run.company_name ? `${period} · ${run.company_name}` : period;
}

interface RowProps {
  label: string;
  amount: number | undefined;
  bold?: boolean;
  negative?: boolean;
  /** Porsi terhadap bruto, mis. "1,8%". Kosong = tidak ditampilkan. */
  share?: string | null;
}

function AmountRow({ label, amount, bold, negative, share }: RowProps) {
  if (!bold && !(Number(amount) > 0)) return null;
  return (
    <div className={`flex items-baseline justify-between gap-2 py-1 text-sm ${bold ? "font-semibold" : ""}`}>
      <span className="text-gray-600">{label}</span>
      <span className="flex items-baseline gap-2">
        {/* Porsi terhadap bruto — makna yang sama untuk semua potongan. */}
        {share && <span className="text-xs font-normal text-gray-400">{share}</span>}
        <span className={negative ? "text-red-600" : "text-gray-900"}>
          {negative ? "− " : ""}
          {formatRupiah(amount)}
        </span>
      </span>
    </div>
  );
}

/** Rincian slip gaji milik sendiri + tautan unduh PDF. */
export function PayslipDetailDialog({
  selected,
  onClose,
}: {
  selected: EssPayslip | null;
  onClose: () => void;
}) {
  const selectedProrateNote = selected
    ? prorateNote({
        factor: selected.prorate_factor,
        fullBase: selected.full_base_salary,
        paidBase: selected.base_salary,
      })
    : null;

  /**
   * Porsi potongan terhadap bruto slip yang sedang dibuka. Makna yang sama
   * dipakai untuk semua baris, termasuk yang tidak punya tarif resmi.
   */
  const bagian = (amount: number | undefined) =>
    formatShareOfGross(Number(amount) || 0, Number(selected?.gross_salary) || 0);

  return (
      <Dialog open={selected !== null} onOpenChange={(open) => !open && onClose()}>
        <DialogContent className="max-h-[85vh] max-w-md overflow-y-auto">
          <DialogHeader>
            <DialogTitle>Slip Gaji — {selected ? payslipPeriodLabel(selected) : ""}</DialogTitle>
          </DialogHeader>
          {selected && (
            <div className="space-y-4">
              <div>
                <p className="mb-1 text-xs font-semibold uppercase tracking-wide text-gray-400">
                  Penghasilan
                </p>
                <AmountRow label="Gaji Pokok" amount={selected.base_salary} />
                {selectedProrateNote && (
                  <p className="text-xs italic text-amber-700">
                    {selectedProrateNote}
                  </p>
                )}
                <AmountRow label="Tunjangan Tetap" amount={selected.fixed_allowance} />
                <AmountRow label="Tunjangan Variabel" amount={selected.variable_allowance} />
                <AmountRow label="Tunjangan Transport" amount={selected.transport_allowance} />
                <AmountRow label="Tunjangan Makan" amount={selected.meal_allowance} />
                <AmountRow label="Tunjangan Perumahan" amount={selected.housing_allowance} />
                <AmountRow label="Tunjangan Jabatan" amount={Number(selected.position_allowance ?? 0)} />
                <AmountRow label="Subsidi BPJS Kesehatan" amount={Number(selected.health_allowance ?? 0)} />
                <AmountRow label="Lembur" amount={selected.overtime_pay} />
                <AmountRow label="THR" amount={selected.thr} />
                <AmountRow label="Bonus" amount={selected.bonus} />
                <div className="border-t pt-1">
                  <AmountRow label="Total Bruto" amount={selected.gross_salary} bold />
                </div>
                <AmountRow label="Penghasilan Tidak Kena Pajak" amount={Number(selected.other_earning_nontax ?? 0)} />
              </div>

              <div>
                <div className="mb-1 flex items-baseline justify-between">
                  <p className="text-xs font-semibold uppercase tracking-wide text-gray-400">
                    Potongan
                  </p>
                  <p className="text-[11px] text-gray-400">% dari bruto</p>
                </div>
                <AmountRow label="BPJS TK (JHT)" amount={selected.bpjs_tk_jht_deduction} share={bagian(selected.bpjs_tk_jht_deduction)} negative />
                <AmountRow label="BPJS TK (JP)" amount={selected.bpjs_tk_jp_deduction} share={bagian(selected.bpjs_tk_jp_deduction)} negative />
                <AmountRow label="BPJS Kesehatan" amount={selected.bpjs_kes_deduction} share={bagian(selected.bpjs_kes_deduction)} negative />
                <AmountRow label="Tapera" amount={selected.tapera_deduction} share={bagian(selected.tapera_deduction)} negative />
                <AmountRow label="PPh 21" amount={selected.pph21_deduction} share={bagian(selected.pph21_deduction)} negative />
                <AmountRow label="Cuti Tanpa Bayaran" amount={selected.unpaid_leave_deduction} share={bagian(selected.unpaid_leave_deduction)} negative />
                <AmountRow label="Potongan Keterlambatan" amount={selected.late_deduction} share={bagian(selected.late_deduction)} negative />
                <AmountRow label="Potongan Izin Pribadi" amount={Number(selected.personal_leave_deduction ?? 0)} share={bagian(Number(selected.personal_leave_deduction ?? 0))} negative />
                <AmountRow label="Potongan Sakit & Cuti Melahirkan" amount={Number(selected.sick_leave_deduction ?? 0)} share={bagian(Number(selected.sick_leave_deduction ?? 0))} negative />
                {(selected.loan_details?.length ?? 0) > 0 ? (
                  selected.loan_details?.map((loan) => (
                    <AmountRow
                      key={loan.loan_id}
                      label={loanInstallmentLabel(loan)}
                      amount={loan.amount}
                      share={bagian(loan.amount)}
                      negative
                    />
                  ))
                ) : (
                  <AmountRow label="Cicilan Pinjaman" amount={selected.loan_deduction} share={bagian(selected.loan_deduction)} negative />
                )}
                <AmountRow label="Potongan Lain" amount={selected.other_deduction} share={bagian(selected.other_deduction)} negative />
                <div className="border-t pt-1">
                  <AmountRow
                    label="Total Potongan"
                    amount={selected.total_deductions}
                    share={bagian(selected.total_deductions)}
                    bold
                    negative
                  />
                </div>
              </div>

              <div className="rounded-lg bg-pink-50 px-4 py-3">
                <div className="flex items-center justify-between">
                  <span className="text-sm font-semibold text-gray-700">
                    Gaji Diterima (Take Home Pay)
                  </span>
                  <span className="text-lg font-bold text-pink-700">
                    {formatRupiah(selected.net_salary)}
                  </span>
                </div>
              </div>

              <SlipStoreBreakdown allocations={selected.allocations} />

              <p className="text-xs text-gray-400">
                Kehadiran: {selected.present_days}/{selected.working_days} hari
                {selected.late_days > 0 ? ` · terlambat ${selected.late_days}×` : ""}
                {Number(selected.overtime_hours) > 0
                  ? ` · lembur ${Number(selected.overtime_hours)} jam`
                  : ""}
                . Ada pertanyaan tentang slip ini? Hubungi HRD.
              </p>
            </div>
          )}
          <DialogFooter>
            {selected && (
              /* Tautan unduh, bukan <Button asChild> — Button di repo ini
                 tidak mendukung asChild, sehingga <a> akan bersarang di dalam
                 <button> dan tampilannya rusak. Gaya ditulis eksplisit agar
                 tidak bergantung pada buttonVariants, yang tipenya bermasalah
                 di seluruh repo. */
              <a
                href={`/api/hris/payslips/${selected.id}/pdf`}
                download
                className="inline-flex h-9 shrink-0 cursor-pointer items-center justify-center gap-1.5 rounded-lg bg-primary px-3 text-sm font-medium text-primary-foreground transition-colors hover:bg-primary/90 active:translate-y-px"
              >
                <ArrowDownTrayIcon className="h-4 w-4" /> Unduh PDF
              </a>
            )}
            <Button variant="outline" onClick={onClose}>
              Tutup
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
  );
}
