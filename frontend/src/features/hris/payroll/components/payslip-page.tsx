"use client";

import { useParams, useRouter } from "next/navigation";
import { Card, CardContent, CardHeader } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { ArrowDownTrayIcon, PrinterIcon, ArrowLeftIcon } from "@heroicons/react/24/outline";
import { formatDateLong, formatRupiah } from "@/lib/format";
import { monthName } from "@/lib/hris/month-label";
import { loanInstallmentLabel } from "@/lib/payroll/loans";
import { formatShareOfGross } from "@/lib/payroll/share";
import { prorateNote } from "@/lib/payroll/prorate-note";
import { usePayslip } from "../queries";
import { SlipStoreBreakdown } from "./store-breakdown";

export function PayslipPage() {
  const params = useParams();
  const router = useRouter();

  const payrollRunId = params.id as string;
  const employeeId = params.employeeId as string;

  const payslipQuery = usePayslip(payrollRunId, employeeId);
  const detail = payslipQuery.data ?? null;
  const slipProrateNote = detail
    ? prorateNote({
        factor: detail.prorate_factor,
        fullBase: detail.full_base_salary,
        paidBase: detail.base_salary,
      })
    : null;
  const loading = payslipQuery.isLoading;

  function handlePrint() {
    window.print();
  }

  /** Porsi potongan terhadap bruto — makna sama untuk semua baris. */
  const bagian = (amount: number | undefined) =>
    formatShareOfGross(Number(amount) || 0, Number(detail?.gross_salary) || 0);

  if (loading) {
    return (
      <div className="flex items-center justify-center min-h-screen">
        <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-pink-600" />
      </div>
    );
  }

  if (!detail) {
    return (
      <div className="container mx-auto py-8">
        <Card>
          <CardContent className="py-8 text-center">
            <p className="text-gray-500 mb-4">Slip gaji tidak ditemukan</p>
            <Button onClick={() => router.push(`/dashboard/hris/payroll/${payrollRunId}`)}>
              Kembali ke Payroll
            </Button>
          </CardContent>
        </Card>
      </div>
    );
  }

  const employee = detail.employee;
  const payrollRun = detail.payroll_run;

  return (
    <div className="container mx-auto py-8 max-w-4xl print:max-w-none">
      {/* Header Actions - Hidden on Print */}
      <div className="mb-6 flex items-center justify-between no-print">
        <Button
          variant="outline"
          onClick={() => router.push(`/dashboard/hris/payroll/${payrollRunId}`)}
        >
          <ArrowLeftIcon className="w-4 h-4 mr-2" />
          Kembali
        </Button>
        <div className="flex gap-2">
          {/* PDF dibuat server (pdfkit) — hasilnya seragam di semua perangkat,
              tidak seperti cetak browser. Memakai <a> + buttonVariants karena
              Button di repo ini tidak mendukung asChild. */}
          <a
            href={`/api/hris/payslips/${detail.id}/pdf`}
            download
            className="inline-flex h-9 shrink-0 cursor-pointer items-center justify-center gap-1.5 rounded-lg bg-primary px-3 text-sm font-medium text-primary-foreground transition-colors hover:bg-primary/90 active:translate-y-px"
          >
            <ArrowDownTrayIcon className="w-4 h-4 mr-2" />
            Unduh PDF
          </a>
          <Button variant="outline" onClick={handlePrint}>
            <PrinterIcon className="w-4 h-4 mr-2" />
            Cetak
          </Button>
        </div>
      </div>

      {/* Payslip Content */}
      <Card className="print:shadow-none print:border-0">
        <CardHeader className="border-b pb-4">
          <div className="flex items-center justify-between">
            <div>
              <h1 className="text-2xl font-bold text-gray-900">Slip Gaji</h1>
              <p className="text-sm text-gray-500 mt-1">
                Periode: {monthName(payrollRun?.period_month)} {payrollRun?.period_year}
              </p>
            </div>
            <Badge className="bg-blue-100 text-blue-700">
              {payrollRun?.company_name ? `${payrollRun.run_name} · ${payrollRun.company_name}` : payrollRun?.run_name}
            </Badge>
          </div>
        </CardHeader>

        <CardContent className="pt-6 space-y-6">
          {/* Employee Info */}
          <div className="grid grid-cols-2 gap-4">
            <div>
              <p className="text-sm text-gray-500">Nama Karyawan</p>
              <p className="font-semibold">{employee?.full_name}</p>
              <p className="text-sm text-gray-500">NIP</p>
              <p>{employee?.nip}</p>
            </div>
            <div>
              <p className="text-sm text-gray-500">Jabatan</p>
              <p>{employee?.position?.title || "-"}</p>
              <p className="text-sm text-gray-500">Departemen</p>
              <p>{employee?.department?.name || "-"}</p>
            </div>
          </div>

          {/* Earnings */}
          <div>
            <h3 className="font-semibold text-lg mb-3 text-gray-700">Penghasilan</h3>
            <div className="space-y-2">
              <EarningRow label="Gaji Pokok" amount={detail.base_salary} />
              {slipProrateNote && (
                <p className="text-xs text-amber-700 italic -mt-1">
                  {slipProrateNote}
                </p>
              )}
              <EarningRow label="Tunjangan Tetap" amount={detail.fixed_allowance} />
              {detail.variable_allowance > 0 && (
                <EarningRow label="Tunjangan Variabel" amount={detail.variable_allowance} />
              )}
              {detail.transport_allowance > 0 && (
                <EarningRow label="Tunjangan Transport" amount={detail.transport_allowance} />
              )}
              {detail.meal_allowance > 0 && (
                <EarningRow label="Tunjangan Makan" amount={detail.meal_allowance} />
              )}
              {detail.housing_allowance > 0 && (
                <EarningRow label="Tunjangan Rumah" amount={detail.housing_allowance} />
              )}
              {Number(detail.position_allowance ?? 0) > 0 && (
                <EarningRow label="Tunjangan Jabatan" amount={Number(detail.position_allowance)} />
              )}
              {Number(detail.health_allowance ?? 0) > 0 && (
                <EarningRow label="Subsidi BPJS Kesehatan" amount={Number(detail.health_allowance)} />
              )}
              {detail.overtime_pay > 0 && (
                <EarningRow label="Lembur" amount={detail.overtime_pay} />
              )}
              {detail.thr > 0 && (
                <EarningRow label="THR" amount={detail.thr} />
              )}
              {detail.bonus > 0 && (
                <EarningRow label="Bonus" amount={detail.bonus} />
              )}
              {detail.other_earning > 0 && (
                <EarningRow label="Penghasilan Lain" amount={detail.other_earning} />
              )}
              <div className="border-t pt-2 mt-2">
                <EarningRow 
                  label="Total Penghasilan" 
                  amount={detail.gross_salary} 
                  bold 
                />
              </div>
              {Number(detail.other_earning_nontax ?? 0) > 0 && (
                <EarningRow label="Penghasilan Tidak Kena Pajak" amount={Number(detail.other_earning_nontax)} />
              )}
            </div>
          </div>

          {/* Deductions */}
          <div>
            <h3 className="font-semibold text-lg mb-3 text-red-700">Potongan</h3>
            <div className="space-y-2">
              <DeductionRow label="BPJS TK (JHT)" amount={detail.bpjs_tk_jht_deduction} share={bagian(detail.bpjs_tk_jht_deduction)} />
              <DeductionRow label="BPJS TK (JP)" amount={detail.bpjs_tk_jp_deduction} share={bagian(detail.bpjs_tk_jp_deduction)} />
              <DeductionRow label="BPJS Kesehatan" amount={detail.bpjs_kes_deduction} share={bagian(detail.bpjs_kes_deduction)} />
              <DeductionRow label="Tapera" amount={detail.tapera_deduction} share={bagian(detail.tapera_deduction)} />
              <DeductionRow label="PPh 21" amount={detail.pph21_deduction} share={bagian(detail.pph21_deduction)} />
              {detail.pph21_method === "ter" && (
                <p className="-mt-1 text-xs text-gray-500">
                  {detail.ter_rate !== null && detail.ter_rate !== undefined
                    ? `TER kategori ${detail.ter_category} ${(Number(detail.ter_rate ?? 0) * 100).toLocaleString("id-ID")}% × DPP ${formatRupiah(Number(detail.pph21_dpp ?? 0))}`
                    : `Koreksi Desember (Pasal 17 setahun) — DPP bulan ini ${formatRupiah(Number(detail.pph21_dpp ?? 0))}`}
                </p>
              )}
              {detail.unpaid_leave_deduction > 0 && (
                <DeductionRow label="Cuti Tanpa Bayaran" amount={detail.unpaid_leave_deduction} share={bagian(detail.unpaid_leave_deduction)} />
              )}
              {(detail.late_deduction ?? 0) > 0 && (
                <DeductionRow label="Potongan Keterlambatan" amount={detail.late_deduction ?? 0} share={bagian(detail.late_deduction ?? 0)} />
              )}
              {Number(detail.personal_leave_deduction ?? 0) > 0 && (
                <DeductionRow label="Potongan Izin Pribadi" amount={Number(detail.personal_leave_deduction)} share={bagian(Number(detail.personal_leave_deduction))} />
              )}
              {Number(detail.sick_leave_deduction ?? 0) > 0 && (
                <DeductionRow label="Potongan Sakit & Cuti Melahirkan" amount={Number(detail.sick_leave_deduction)} share={bagian(Number(detail.sick_leave_deduction))} />
              )}
              {(detail.loan_details?.length ?? 0) > 0
                ? detail.loan_details?.map((loan) => (
                    <DeductionRow
                      key={loan.loan_id}
                      label={loanInstallmentLabel(loan)}
                      amount={loan.amount}
                    />
                  ))
                : (detail.loan_deduction ?? 0) > 0 && (
                    <DeductionRow label="Cicilan Pinjaman" amount={detail.loan_deduction ?? 0} share={bagian(detail.loan_deduction ?? 0)} />
                  )}
              {detail.other_deduction > 0 && (
                <DeductionRow label="Potongan Lain" amount={detail.other_deduction} share={bagian(detail.other_deduction)} />
              )}
              <div className="border-t pt-2 mt-2">
                <DeductionRow 
                  label="Total Potongan" 
                  amount={detail.total_deductions} 
                  bold 
                />
              </div>
            </div>
          </div>

          {/* Net Salary */}
          <div className="bg-green-50 border border-green-200 rounded-lg p-4">
            <div className="flex items-center justify-between">
              <div>
                <p className="text-sm text-gray-600">Take Home Pay (Gaji Bersih)</p>
                <p className="text-2xl font-bold text-green-700">
                  {formatRupiah(detail.net_salary)}
                </p>
              </div>
              <div className="text-right text-sm text-gray-500">
                <p>Tanggal Cetak</p>
                <p>{formatDateLong(new Date())}</p>
              </div>
            </div>
          </div>

          <SlipStoreBreakdown allocations={detail.allocations} />

          {/* Attendance Summary */}
          <div className="grid grid-cols-4 gap-4 text-center">
            <div className="bg-gray-50 rounded-lg p-3">
              <p className="text-xs text-gray-500">Hari Kerja</p>
              <p className="text-lg font-semibold">{detail.working_days}</p>
            </div>
            <div className="bg-green-50 rounded-lg p-3">
              <p className="text-xs text-gray-500">Hadir</p>
              <p className="text-lg font-semibold text-green-700">{detail.present_days}</p>
            </div>
            <div className="bg-yellow-50 rounded-lg p-3">
              <p className="text-xs text-gray-500">Terlambat</p>
              <p className="text-lg font-semibold text-yellow-700">{detail.late_days}</p>
            </div>
            <div className="bg-red-50 rounded-lg p-3">
              <p className="text-xs text-gray-500">Cuti (Unpaid)</p>
              <p className="text-lg font-semibold text-red-700">{detail.unpaid_leave_days}</p>
            </div>
          </div>

          {/* Footer Note */}
          <div className="border-t pt-4 mt-6 text-center text-sm text-gray-500">
            <p>Dokumen ini dicetak secara otomatis oleh sistem.</p>
            <p>Untuk pertanyaan mengenai slip gaji ini, silakan hubungi HRD.</p>
          </div>
        </CardContent>
      </Card>

      {/* Print Styles */}
      <style jsx global>{`
        @media print {
          .no-print {
            display: none !important;
          }
          body {
            background: white;
          }
        }
      `}</style>
    </div>
  );
}

function EarningRow({ label, amount, bold = false }: { label: string; amount: number; bold?: boolean }) {
  return (
    <div className="flex items-center justify-between">
      <span className={`text-gray-700 ${bold ? 'font-semibold' : ''}`}>{label}</span>
      <span className={`text-gray-900 ${bold ? 'font-bold' : ''}`}>
        {amount > 0 ? formatRupiah(amount) : '-'}
      </span>
    </div>
  );
}

function DeductionRow({
  label,
  amount,
  bold = false,
  share,
}: {
  label: string;
  amount: number;
  bold?: boolean;
  /** Porsi terhadap bruto, mis. "1,8%". Kosong = tidak ditampilkan. */
  share?: string | null;
}) {
  return (
    <div className="flex items-baseline justify-between gap-2">
      <span className={`text-gray-700 ${bold ? 'font-semibold' : ''}`}>{label}</span>
      <span className="flex items-baseline gap-2">
        {share && <span className="text-xs font-normal text-gray-400">{share}</span>}
        <span className={`text-red-600 ${bold ? 'font-bold' : ''}`}>
          {amount > 0 ? `- ${formatRupiah(amount)}` : '-'}
        </span>
      </span>
    </div>
  );
}
