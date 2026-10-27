"use client";

import { useState } from "react";
import { Loader2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Combobox } from "@/components/ui/combobox";
import { Input } from "@/components/ui/input";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { useStoreBranches } from "@/features/hris/placements/queries";
import { useDebouncedValue } from "@/hooks/use-debounced-value";
import { formatDate } from "@/lib/format";
import { leaveBalanceTemplateUrl } from "../api";
import { formatDays } from "../balance-math";
import { useEditableBalanceYears } from "../editable-years";
import { useLeaveBalanceAccess, useLeaveBalances } from "../queries";
import type { LeaveBalanceView } from "../types";
import { LeaveBalanceEditDialog } from "./leave-balance-edit-dialog";
import { LeaveBalanceUploadDialog } from "./leave-balance-upload-dialog";
import { LeaveCarryReviewsCard } from "./leave-carry-reviews-card";
import { LeaveSettingsCard } from "./leave-settings-card";

const SOURCE_LABEL = { auto: "Otomatis", manual: "Manual" } as const;

function carryCell(b: LeaveBalanceView) {
  if (b.carry_over_days <= 0) return <span className="text-gray-400">0</span>;
  return (
    <div>
      <span>{formatDays(b.carry_over_remaining)}</span>
      <p className={`text-xs ${b.carry_over_active ? "text-gray-500" : "text-warning"}`}>
        {b.carry_over_expires_on
          ? b.carry_over_active
            ? `s/d ${formatDate(b.carry_over_expires_on)}`
            : `kedaluwarsa ${formatDate(b.carry_over_expires_on)}`
          : "tidak berlaku (tanpa kedaluwarsa)"}
      </p>
    </div>
  );
}

/**
 * Saldo Cuti (EPIC-068): daftar saldo tahunan semua karyawan aktif per cabang
 * dan tahun, ubah manual per karyawan, dan pengaturan sisa bawaan. Aksi
 * mengikuti hak akun di daun Saldo Cuti (read / update / import / export).
 */
export function LeaveBalancesPage() {
  const access = useLeaveBalanceAccess().data;
  const editable = useEditableBalanceYears(access?.update === true);
  const { thisYear, previousUntil } = editable;
  const [year, setYear] = useState(thisYear);
  const [branchId, setBranchId] = useState("");
  const [search, setSearch] = useState("");
  const [editing, setEditing] = useState<LeaveBalanceView | null>(null);
  const [uploadOpen, setUploadOpen] = useState(false);
  const q = useDebouncedValue(search, 300).trim();
  const branches = useStoreBranches();
  const list = useLeaveBalances({ year, branch_id: branchId, q });
  const rows = list.data ?? [];
  const canEdit = editable.canEdit(year);

  return (
    <div className="space-y-5">
      <div className="border-b border-gray-200/70 pb-4">
        <h1 className="text-2xl font-bold text-gray-900">Saldo Cuti</h1>
        <p className="text-sm text-gray-500">
          Jatah, penyesuaian, terpakai, dan sisa bawaan cuti tahunan per karyawan. Karyawan tanpa data tersimpan tampil
          dengan jatah default.
        </p>
      </div>

      {access?.read === true ? <LeaveCarryReviewsCard year={year} canUpdate={access.update} canCorrect={canEdit} /> : null}

      {access?.read === true ? <LeaveSettingsCard canUpdate={access.update} /> : null}

      <div className="flex flex-wrap items-end gap-3">
        <div className="w-64">
          <p className="mb-1 text-xs font-medium text-gray-600">Cabang</p>
          <Combobox
            options={(branches.data ?? []).map((b) => ({ value: b.id, label: b.name, description: b.code }))}
            value={branchId}
            onChange={setBranchId}
            allowClear
            placeholder="Semua cabang"
            searchPlaceholder="Cari cabang..."
            emptyMessage="Cabang tidak ditemukan"
          />
        </div>
        <div>
          <label className="mb-1 block text-xs font-medium text-gray-600" htmlFor="lbs-year">
            Tahun
          </label>
          <select
            id="lbs-year"
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
        </div>
        <div className="w-64">
          <label className="mb-1 block text-xs font-medium text-gray-600" htmlFor="lbs-search">
            Cari karyawan
          </label>
          <Input
            id="lbs-search"
            placeholder="Nama atau NIP"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
          />
        </div>
        {access?.import === true || access?.export === true ? (
          <div className="flex flex-wrap gap-2" data-testid="leave-balance-import-slot">
            {access.export ? (
              <Button asChild variant="outline">
                <a href={leaveBalanceTemplateUrl({ year, branch_id: branchId })}>Unduh template</a>
              </Button>
            ) : null}
            {access.import ? <Button onClick={() => setUploadOpen(true)}>Import Excel</Button> : null}
          </div>
        ) : null}
      </div>

      {canEdit && year === thisYear - 1 && previousUntil ? (
        <p className="text-sm text-gray-500">
          Saldo {year} masih bisa dikoreksi sampai {formatDate(previousUntil)} (kedaluwarsa sisa bawaan). Koreksi ikut
          menghitung ulang sisa bawaan {thisYear}.
        </p>
      ) : null}

      {list.isLoading ? (
        <div className="flex justify-center py-14">
          <Loader2 className="h-6 w-6 animate-spin text-gray-400" />
        </div>
      ) : list.isError ? (
        <p className="py-10 text-center text-sm text-danger">
          {list.error instanceof Error ? list.error.message : "Gagal memuat saldo cuti"}
        </p>
      ) : rows.length === 0 ? (
        <p className="py-10 text-center text-sm text-gray-400">Tidak ada karyawan yang cocok.</p>
      ) : (
        <div className="overflow-x-auto rounded-lg border border-gray-200 bg-white">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>NIP</TableHead>
                <TableHead>Nama</TableHead>
                <TableHead>Cabang</TableHead>
                <TableHead className="text-right">Jatah</TableHead>
                <TableHead className="text-right">Penyesuaian</TableHead>
                <TableHead className="text-right">Terpakai</TableHead>
                <TableHead className="text-right">Sisa jatah</TableHead>
                <TableHead>Sisa bawaan</TableHead>
                <TableHead>Sumber</TableHead>
                <TableHead className="text-right">Tersedia</TableHead>
                {canEdit ? <TableHead className="w-20" /> : null}
              </TableRow>
            </TableHeader>
            <TableBody>
              {rows.map((b) => (
                <TableRow key={b.employee_id}>
                  <TableCell className="text-gray-600">{b.nip ?? "-"}</TableCell>
                  <TableCell className="font-medium text-gray-900">
                    {b.full_name}
                    {b.exists ? null : (
                      <span
                        className="ml-2 rounded bg-gray-100 px-1.5 py-0.5 text-xs font-normal text-gray-500"
                        title="Belum tersimpan; memakai jatah default"
                      >
                        Default
                      </span>
                    )}
                  </TableCell>
                  <TableCell className="text-gray-600">{b.branch_name ?? "-"}</TableCell>
                  <TableCell className="text-right">{formatDays(b.annual_leave_total)}</TableCell>
                  <TableCell className="text-right">{formatDays(b.annual_leave_adjustment)}</TableCell>
                  <TableCell className="text-right">{formatDays(b.annual_leave_used)}</TableCell>
                  <TableCell className="text-right">{formatDays(b.annual_leave_remaining)}</TableCell>
                  <TableCell>{carryCell(b)}</TableCell>
                  <TableCell className="text-gray-600">{b.carry_over_days > 0 ? (SOURCE_LABEL[b.carry_over_source] ?? "–") : "–"}</TableCell>
                  <TableCell className="text-right font-semibold text-gray-900">{formatDays(b.available)}</TableCell>
                  {canEdit ? (
                    <TableCell className="text-right">
                      <Button size="sm" variant="outline" onClick={() => setEditing(b)}>
                        Ubah
                      </Button>
                    </TableCell>
                  ) : null}
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      )}
      {rows.length > 0 ? <p className="text-xs text-gray-400">{rows.length} karyawan</p> : null}

      {uploadOpen ? (
        <LeaveBalanceUploadDialog
          open
          onOpenChange={setUploadOpen}
          year={year}
          branchId={branchId}
          branchName={(branches.data ?? []).find((b) => b.id === branchId)?.name ?? "Semua cabang"}
        />
      ) : null}

      {editing ? (
        <LeaveBalanceEditDialog
          open
          onOpenChange={(o) => {
            if (!o) setEditing(null);
          }}
          balance={editing}
        />
      ) : null}
    </div>
  );
}
