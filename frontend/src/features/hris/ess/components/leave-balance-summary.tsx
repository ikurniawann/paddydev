import { formatDate } from "@/lib/format";
import { formatDays } from "@/features/hris/leave-balances/balance-math";
import type { EssLeaveBalance } from "../types";

const days = (n: number) => `${formatDays(n)} hari`;

/**
 * Rincian saldo cuti tahunan karyawan (EPIC-068): jatah, penyesuaian (bila
 * tidak nol), terpakai, dan sisa bawaan beserta kedaluwarsanya (hanya bila
 * masih berlaku). Angka berasal dari server; tidak ada nilai bawaan di sini.
 */
export function LeaveBalanceBreakdown({ balance }: { balance: EssLeaveBalance }) {
  const rows: { label: string; value: string }[] = [{ label: "Jatah", value: days(balance.annual_leave_total) }];
  if (balance.annual_leave_adjustment !== 0) {
    rows.push({ label: "Penyesuaian", value: days(balance.annual_leave_adjustment) });
  }
  rows.push({ label: "Terpakai", value: days(balance.annual_leave_used) });
  if (balance.carry_over_active) {
    rows.push({
      label: "Sisa bawaan",
      value: `${days(balance.carry_over_remaining)}, berlaku s/d ${formatDate(balance.carry_over_expires_on)}`,
    });
  }
  return (
    <dl className="mt-3 space-y-1 text-xs text-gray-600">
      {rows.map((r) => (
        <div key={r.label} className="flex justify-between gap-3">
          <dt>{r.label}</dt>
          <dd className="text-right font-medium text-gray-800">{r.value}</dd>
        </div>
      ))}
    </dl>
  );
}

/** Satu baris ringkas untuk kartu Beranda. */
export function balanceSubtitle(balance: EssLeaveBalance): string {
  const base = `Jatah ${days(balance.annual_leave_total)} · terpakai ${days(balance.annual_leave_used)}`;
  return balance.carry_over_active ? `${base} · termasuk ${days(balance.carry_over_remaining)} sisa bawaan` : base;
}
