"use client";

import { useState } from "react";
import { CalendarDaysIcon } from "@heroicons/react/24/outline";
import { Card, CardContent } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { formatDays } from "@/features/hris/leave-balances/balance-math";
import { LeaveBalanceEditDialog } from "@/features/hris/leave-balances/components/leave-balance-edit-dialog";
import { useEditableBalanceYears } from "@/features/hris/leave-balances/editable-years";
import { useLeaveBalance, useLeaveBalanceAccess } from "@/features/hris/leave-balances/queries";
import type { LeaveBalanceView } from "@/features/hris/leave-balances/types";
import { formatDate } from "@/lib/format";
import { EmptyTabCard, TabSpinner } from "./tab-states";

function Figure({ label, value, testId, hint }: { label: string; value: number; testId: string; hint?: string }) {
  return (
    <div>
      <p className="text-xs font-medium uppercase tracking-wide text-gray-500">{label}</p>
      <p className="text-xl font-semibold text-gray-900" data-testid={testId}>
        {formatDays(value)}
      </p>
      {hint ? <p className="text-xs text-gray-400">{hint}</p> : null}
    </div>
  );
}

function carryNote(b: LeaveBalanceView): string | null {
  if (b.carry_over_days <= 0) return null;
  if (!b.carry_over_expires_on) return "Tanpa tanggal kedaluwarsa, sisa bawaan tidak berlaku.";
  return b.carry_over_active
    ? `Sisa bawaan berlaku sampai ${formatDate(b.carry_over_expires_on)}.`
    : `Sisa bawaan tidak berlaku lagi (kedaluwarsa ${formatDate(b.carry_over_expires_on)}).`;
}

/**
 * Saldo cuti tahunan satu karyawan (EPIC-068). Go membalas satu objek saldo
 * per tahun; tombol Ubah hanya untuk akun dengan aksi `update` dan tahun
 * berjalan/depan, atau tahun lalu sampai tanggal kedaluwarsa sisa bawaan (I3).
 */
export function LeaveBalanceTab({ employeeId }: { employeeId: string }) {
  const access = useLeaveBalanceAccess().data;
  const editable = useEditableBalanceYears(access?.update === true);
  const { thisYear } = editable;
  const [year, setYear] = useState(thisYear);
  const [editing, setEditing] = useState(false);
  const { data: balance, isLoading, isError, error } = useLeaveBalance(employeeId, year);
  const canEdit = editable.canEdit(year);

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <h3 className="font-semibold text-gray-700">Saldo Cuti Tahunan</h3>
        <div className="flex items-center gap-2">
          <label className="text-sm text-gray-500" htmlFor="lb-year">
            Tahun
          </label>
          <select
            id="lb-year"
            value={year}
            onChange={(e) => setYear(Number(e.target.value))}
            className="h-9 rounded-md border border-input bg-background px-3 text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring/50"
          >
            {[thisYear - 1, thisYear, thisYear + 1].map((y) => (
              <option key={y} value={y}>
                {y}
              </option>
            ))}
          </select>
          {canEdit && balance ? (
            <Button size="sm" onClick={() => setEditing(true)}>
              Ubah saldo
            </Button>
          ) : null}
        </div>
      </div>

      {isLoading ? (
        <TabSpinner />
      ) : isError || !balance ? (
        <EmptyTabCard icon={CalendarDaysIcon}>
          {error instanceof Error ? error.message : "Saldo cuti belum tersedia"}
        </EmptyTabCard>
      ) : (
        <Card>
          <CardContent className="space-y-4 p-4">
            <div className="flex items-end gap-2">
              <span className="text-4xl font-bold text-blue-600" data-testid="lb-available">
                {formatDays(balance.available)}
              </span>
              <span className="mb-1 text-sm text-gray-400">hari tersedia hari ini</span>
            </div>
            <div className="grid grid-cols-2 gap-4 sm:grid-cols-3 lg:grid-cols-5">
              <Figure label="Jatah" value={balance.annual_leave_total} testId="lb-total" />
              <Figure label="Penyesuaian" value={balance.annual_leave_adjustment} testId="lb-adjustment" />
              <Figure label="Terpakai" value={balance.annual_leave_used} testId="lb-used" />
              <Figure label="Sisa jatah" value={balance.annual_leave_remaining} testId="lb-remaining" />
              <Figure
                label="Sisa bawaan"
                value={balance.carry_over_remaining}
                testId="lb-carry"
                hint={balance.carry_over_days > 0 ? `dari ${formatDays(balance.carry_over_days)}` : undefined}
              />
            </div>
            {carryNote(balance) ? <p className="text-sm text-gray-500">{carryNote(balance)}</p> : null}
          </CardContent>
        </Card>
      )}

      {editing && balance ? <LeaveBalanceEditDialog open onOpenChange={setEditing} balance={balance} /> : null}
    </div>
  );
}
