"use client";

import { useMemo, useState } from "react";
import { toast } from "sonner";
import { AttendanceCalendar } from "@/components/hris/AttendanceCalendar";
import { Card, CardContent } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Combobox } from "@/components/ui/combobox";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Clock, Download, Lock, Pencil, Plus, Trash2 } from "lucide-react";
import { useStoreBranches } from "@/features/hris/placements/queries";
import { formatDate, formatTime } from "@/lib/format";
import { todayWib } from "@/lib/hris/attendance-period";
import { GeofenceBadge } from "./geofence-badge";
import { SelfiePhotoDialog } from "./selfie-photo-dialog";
import { ShortHoursBadge } from "./short-hours-badge";
import { AttendancePeriodBanner } from "./attendance-period-banner";
import { AttendanceAdjustDialog, STATUS_OPTIONS } from "./attendance-adjust-dialog";
import { AdjustmentBadge, DeleteAttendanceDialog } from "./attendance-adjustment-history";
import { exportAttendanceCsv, type AttendanceExportFormat } from "../api";
import { useActiveEmployees, useAttendanceList, useAttendancePeriods } from "../queries";
import type { AttendanceListRow } from "../types";

const ALL_STORES = "all";
const NO_STORE = "none";

/**
 * Tab "Rekap" absensi HRD: tabel absensi berpagination dengan filter
 * karyawan/periode absensi/keterlambatan/toko + export. Periode mengikuti
 * cut-off di pengaturan (bukan sekadar bulan kalender) dan menampilkan status
 * closing-nya. Selama periode terbuka HRD bisa menambah, mengoreksi, dan
 * menghapus absensi (wajib alasan, tercatat di riwayat); setelah closing
 * semua kontrol edit nonaktif. Saat satu karyawan dipilih, kalender bulanan
 * per-karyawan ikut ditampilkan.
 */

const PAGE_LIMIT = 20;

export function AttendanceRecapTab() {
  const [filterEmployee, setFilterEmployee] = useState("all");
  const [periodYear, setPeriodYear] = useState<number | undefined>(undefined);
  const [periodId, setPeriodId] = useState<string | null>(null);
  const [lateOnly, setLateOnly] = useState(false);
  const [store, setStore] = useState(ALL_STORES);
  const branches = useStoreBranches().data ?? [];
  const [page, setPage] = useState(1);
  const [exporting, setExporting] = useState<AttendanceExportFormat | null>(null);
  const [editing, setEditing] = useState<AttendanceListRow | "new" | null>(null);
  const [deleting, setDeleting] = useState<AttendanceListRow | null>(null);

  const employees = useActiveEmployees().data;
  const periodsQuery = useAttendancePeriods(periodYear);
  const periodList = periodsQuery.data;
  const periods = periodList?.periods ?? [];
  const period =
    periods.find((p) => p.id === periodId) ??
    periods.find((p) => p.id === periodList?.current.id) ??
    periods[0] ??
    null;
  const today = periodList?.today ?? todayWib();
  const canManage = periodList?.permissions.can_manage ?? false;
  const editable = canManage && period?.status === "open";
  const stats = period?.stats ?? null;
  const range = useMemo(
    () => ({ start: period?.start_date ?? "", end: period?.end_date ?? "" }),
    [period?.start_date, period?.end_date]
  );
  const shownYear = periodYear ?? periodList?.year ?? new Date().getFullYear();
  const years = Array.from({ length: 3 }, (_, i) => (periodList?.year ?? new Date().getFullYear()) - i);
  const listQuery = useAttendanceList({
    employee_id: filterEmployee === "all" ? undefined : filterEmployee,
    start_date: range.start,
    end_date: range.end,
    is_late: lateOnly || undefined,
    branch_id: store === ALL_STORES ? undefined : store,
    page,
    limit: PAGE_LIMIT,
  }, range.start !== "");
  const listEnabled = range.start !== "";
  const rows = listEnabled ? listQuery.data?.data ?? [] : [];
  const total = listQuery.data?.pagination.total ?? 0;
  const totalPages = Math.max(1, listQuery.data?.pagination.totalPages ?? 1);
  const loading = (listEnabled && listQuery.isLoading) || periodsQuery.isLoading;
  const error = listQuery.error ? listQuery.error.message || "Gagal memuat data absensi" : null;

  // Filter berubah → kembali ke halaman 1.
  const changeFilter = <T,>(setter: (value: T) => void) => (value: T) => {
    setter(value);
    setPage(1);
  };

  // Permintaan owner 2026-08-28: HR kesulitan membaca CSV — sediakan juga
  // Excel yang rapi dan PDF ber-foto selfie. Ketiganya mengikuti filter
  // karyawan + periode (bulan) yang sedang dipilih di halaman ini.
  async function handleExport(format: AttendanceExportFormat) {
    setExporting(format);
    try {
      const blob = await exportAttendanceCsv(
        {
          employee_id: filterEmployee,
          start_date: range.start,
          end_date: range.end,
        },
        format
      );
      const url = window.URL.createObjectURL(blob);
      const a = document.createElement("a");
      a.href = url;
      a.download = `rekap-absensi_${period ? `${period.period_year}-${String(period.period_month).padStart(2, "0")}` : "periode"}.${format}`;
      document.body.appendChild(a);
      a.click();
      window.URL.revokeObjectURL(url);
      document.body.removeChild(a);
    } catch (err) {
      toast.error("Export gagal: " + (err instanceof Error ? err.message : "unknown"));
    } finally {
      setExporting(null);
    }
  }

  const employeeOptions = useMemo(
    () => [
      { value: "all", label: "Semua Karyawan" },
      ...(employees ?? []).map((emp) => ({
        value: emp.id,
        label: emp.nip ? `${emp.full_name} (${emp.nip})` : emp.full_name,
      })),
    ],
    [employees]
  );

  return (
    <div className="space-y-4">
      {/* Filter */}
      <Card>
        <CardContent className="pt-6">
          <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-3">
            <div>
              <label className="text-sm font-medium text-gray-700 mb-2 block">Karyawan</label>
              <Combobox
                options={employeeOptions}
                value={filterEmployee}
                onChange={changeFilter(setFilterEmployee)}
                placeholder="Semua karyawan"
                searchPlaceholder="Cari nama/NIP…"
                emptyMessage="Karyawan tidak ditemukan"
                className="!w-full h-9 text-sm"
              />
            </div>
            <div>
              <label className="text-sm font-medium text-gray-700 mb-2 block">Periode absensi</label>
              <div className="flex gap-2">
                <Select
                  value={String(shownYear)}
                  onValueChange={(v) => {
                    setPeriodYear(Number(v));
                    setPeriodId(null);
                    setPage(1);
                  }}
                >
                  <SelectTrigger className="w-24 shrink-0" aria-label="Tahun periode">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {years.map((y) => (
                      <SelectItem key={y} value={String(y)}>
                        {y}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                <Select
                  value={period?.id ?? ""}
                  onValueChange={(v) => changeFilter(setPeriodId)(v ?? null)}
                  disabled={periods.length === 0}
                >
                  <SelectTrigger className="min-w-0 flex-1" aria-label="Periode absensi">
                    <SelectValue placeholder="Pilih periode">
                      {period ? `${period.name}${period.status === "closed" ? " 🔒" : ""}` : "Pilih periode"}
                    </SelectValue>
                  </SelectTrigger>
                  <SelectContent>
                    {periods.map((p) => (
                      <SelectItem key={p.id} value={p.id}>
                        {p.name} · {p.label}
                        {p.status === "closed" ? " · closed" : ""}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
            </div>
            <div>
              <label className="text-sm font-medium text-gray-700 mb-2 block">
                Keterlambatan
              </label>
              <Select
                value={lateOnly ? "late" : "all"}
                onValueChange={(value) => changeFilter(setLateOnly)(value === "late")}
              >
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="all">Semua Rekaman</SelectItem>
                  <SelectItem value="late">Hanya Terlambat</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div>
              <label className="text-sm font-medium text-gray-700 mb-2 block">Toko</label>
              <Select value={store} onValueChange={(value) => changeFilter(setStore)(value)}>
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value={ALL_STORES}>Semua toko</SelectItem>
                  {branches.map((b) => (
                    <SelectItem key={b.id} value={b.id}>
                      {b.name}
                    </SelectItem>
                  ))}
                  <SelectItem value={NO_STORE}>Tanpa toko</SelectItem>
                </SelectContent>
              </Select>
            </div>
          </div>
          <div className="mt-3 flex justify-end">
            <div className="flex gap-2">
              <Button
                variant="outline"
                onClick={() => handleExport("xlsx")}
                disabled={exporting !== null}
                title="Unduh rekap sebagai Excel yang rapi"
              >
                <Download className="w-4 h-4 mr-1.5" />
                {exporting === "xlsx" ? "Mengekspor…" : "Excel"}
              </Button>
              <Button
                variant="outline"
                onClick={() => handleExport("pdf")}
                disabled={exporting !== null}
                title="Unduh rekap sebagai PDF berikut foto selfie"
              >
                <Download className="w-4 h-4 mr-1.5" />
                {exporting === "pdf" ? "Mengekspor…" : "PDF + Foto"}
              </Button>
              <Button
                variant="ghost"
                className="flex-none px-3"
                onClick={() => handleExport("csv")}
                disabled={exporting !== null}
                title="Unduh data mentah CSV"
              >
                {exporting === "csv" ? "…" : "CSV"}
              </Button>
            </div>
          </div>
        </CardContent>
      </Card>

      {/* Status closing periode terpilih */}
      {period && (
        <AttendancePeriodBanner
          key={period.id}
          period={period}
          canManage={canManage}
          canReopen={periodList?.permissions.can_reopen ?? false}
          offsetDays={periodList?.settings.closing_offset_days}
        />
      )}

      {/* Statistik periode terpilih */}
      <div className="grid grid-cols-2 lg:grid-cols-4 gap-4">
        {[
          { label: "Rekaman Absensi", value: stats ? String(stats.rows) : "…" },
          { label: "Terlambat", value: stats ? String(stats.late) : "…" },
          { label: "Tanpa Jam Pulang", value: stats ? String(stats.without_clock_out) : "…" },
          { label: "Disesuaikan Manual", value: stats ? String(stats.adjusted) : "…" },
        ].map((item) => (
          <Card key={item.label}>
            <CardContent className="pt-4 pb-3">
              <p className="text-2xl font-bold text-gray-900">{item.value}</p>
              <p className="text-xs text-gray-500">{item.label}</p>
            </CardContent>
          </Card>
        ))}
      </div>

      {/* Kalender per-karyawan (hanya saat satu karyawan dipilih) */}
      {filterEmployee !== "all" && (
        <AttendanceCalendar employeeId={filterEmployee} />
      )}

      {/* Tabel */}
      <Card>
        <div className="flex flex-wrap items-center justify-between gap-2 border-b border-gray-100 px-4 py-3">
          <p className="text-sm text-gray-500">
            {period ? `${period.name} · ${period.label}` : "Periode absensi"}
            {period?.status === "closed" && " — terkunci"}
          </p>
          {canManage && (
            <Button
              size="sm"
              className="gap-1.5 bg-pink-600 text-white hover:bg-pink-700"
              disabled={!editable || !period}
              title={editable ? undefined : "Periode sudah di-closing — absensi tidak bisa ditambah"}
              onClick={() => setEditing("new")}
            >
              <Plus className="h-4 w-4" />
              Tambah absensi
            </Button>
          )}
        </div>
        <CardContent className="p-0">
          {loading ? (
            <div className="flex justify-center py-12">
              <div className="animate-spin w-6 h-6 border-2 border-gray-300 border-t-blue-500 rounded-full" />
            </div>
          ) : error ? (
            <div className="py-12 text-center text-sm text-red-500">{error}</div>
          ) : rows.length === 0 ? (
            <div className="py-12 text-center text-gray-400">
              <Clock className="w-8 h-8 mx-auto mb-2 text-gray-300" />
              Tidak ada rekaman absensi untuk filter ini.
            </div>
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b border-gray-100 bg-gray-50 text-left text-gray-600">
                    <th className="p-3">Tanggal</th>
                    <th className="p-3">Karyawan</th>
                    <th className="p-3">Toko</th>
                    <th className="p-3">Shift</th>
                    <th className="p-3">Masuk</th>
                    <th className="p-3">Pulang</th>
                    <th className="p-3">Jam Kerja</th>
                    <th className="p-3">Status</th>
                    <th className="p-3">Lokasi</th>
                    <th className="p-3">Selfie</th>
                    {canManage && <th className="p-3 text-right">Aksi</th>}
                  </tr>
                </thead>
                <tbody>
                  {rows.map((row) => (
                    <tr key={row.id} className="border-b border-gray-50">
                      <td className="p-3 whitespace-nowrap text-gray-700">
                        {formatDate(row.date)}
                      </td>
                      <td className="p-3">
                        <p className="font-medium text-gray-900">
                          {row.employee?.full_name ?? "—"}
                        </p>
                        {row.employee?.nip && (
                          <p className="text-xs text-gray-400">{row.employee.nip}</p>
                        )}
                      </td>
                      <td className="p-3">
                        {row.branch ? (
                          <span className="text-gray-700">{row.branch.name}</span>
                        ) : (
                          <span className="text-xs text-amber-700">Tanpa toko</span>
                        )}
                      </td>
                      <td className="p-3 text-gray-600">
                        {row.shift?.name ?? (
                          <span className="text-orange-600 text-xs">Di luar jadwal</span>
                        )}
                      </td>
                      <td className="p-3 text-gray-600">{formatTime(row.clock_in, "—")}</td>
                      <td className="p-3 text-gray-600">{formatTime(row.clock_out, "—")}</td>
                      <td className="p-3 text-gray-600">
                        {row.work_hours != null ? `${Number(row.work_hours).toFixed(1)} jam` : "—"}
                      </td>
                      <td className="p-3">
                        <div className="flex flex-wrap items-center gap-1">
                          {row.status === "absent" ? (
                            <Badge className="bg-red-100 text-red-700">Tidak hadir</Badge>
                          ) : row.is_late ? (
                            <Badge className="bg-yellow-100 text-yellow-700">
                              Terlambat +{row.late_minutes} mnt
                            </Badge>
                          ) : row.status !== "present" && row.status !== "late" ? (
                            <Badge className="bg-sky-100 text-sky-700">
                              {STATUS_OPTIONS.find((s) => s.value === row.status)?.label ?? row.status}
                            </Badge>
                          ) : (
                            <Badge className="bg-green-100 text-green-700">Hadir</Badge>
                          )}
                          {Number(row.overtime_hours ?? 0) > 0 && (
                            <Badge className="bg-indigo-50 text-indigo-700">
                              Lembur {Number(row.overtime_hours)} jam
                            </Badge>
                          )}
                          <AdjustmentBadge row={row} />
                          <ShortHoursBadge isShort={row.is_short_hours} minHours={row.min_work_hours} />
                        </div>
                      </td>
                      <td className="p-3">
                        <div className="flex flex-col items-start gap-1">
                          <GeofenceBadge status={row.clock_in_geofence} distanceM={row.clock_in_distance_m} prefix="Masuk" />
                          <GeofenceBadge status={row.clock_out_geofence} distanceM={row.clock_out_distance_m} prefix="Pulang" />
                          {!row.clock_in_geofence && !row.clock_out_geofence && (
                            <span className="text-xs text-gray-400">—</span>
                          )}
                          {row.notes && (row.clock_in_geofence === "invalid" || row.clock_out_geofence === "invalid") && (
                            <p className="max-w-[16rem] whitespace-pre-line text-xs text-gray-500">“{row.notes}”</p>
                          )}
                        </div>
                      </td>
                      <td className="p-3">
                        <div className="flex gap-2">
                          <SelfiePhotoDialog
                            path={row.clock_in_photo_url}
                            label="masuk"
                            employeeName={row.employee?.full_name ?? null}
                            time={formatTime(row.clock_in, "—")}
                          />
                          <SelfiePhotoDialog
                            path={row.clock_out_photo_url}
                            label="pulang"
                            employeeName={row.employee?.full_name ?? null}
                            time={formatTime(row.clock_out, "—")}
                          />
                        </div>
                      </td>
                      {canManage && (
                        <td className="p-3 text-right whitespace-nowrap">
                          {editable ? (
                            <>
                              <Button
                                size="icon"
                                variant="ghost"
                                className="h-8 w-8"
                                title="Sesuaikan absensi"
                                aria-label="Sesuaikan absensi"
                                onClick={() => setEditing(row)}
                              >
                                <Pencil className="h-4 w-4" />
                              </Button>
                              <Button
                                size="icon"
                                variant="ghost"
                                className="h-8 w-8 text-red-600 hover:text-red-700"
                                title="Hapus absensi"
                                aria-label="Hapus absensi"
                                onClick={() => setDeleting(row)}
                              >
                                <Trash2 className="h-4 w-4" />
                              </Button>
                            </>
                          ) : (
                            <Lock className="ml-auto h-4 w-4 text-gray-300" aria-label="Periode sudah di-closing" />
                          )}
                        </td>
                      )}
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </CardContent>
      </Card>

      {/* Pagination */}
      {!loading && !error && total > 0 && (
        <div className="flex items-center justify-between text-sm text-gray-500">
          <span>
            {total} rekaman · halaman {page} dari {totalPages}
          </span>
          <div className="flex gap-2">
            <Button
              variant="outline"
              size="sm"
              disabled={page <= 1}
              onClick={() => setPage((p) => p - 1)}
            >
              Sebelumnya
            </Button>
            <Button
              variant="outline"
              size="sm"
              disabled={page >= totalPages}
              onClick={() => setPage((p) => p + 1)}
            >
              Berikutnya
            </Button>
          </div>
        </div>
      )}

      {editing && period && (
        <AttendanceAdjustDialog
          key={editing === "new" ? "new" : editing.id}
          open
          onOpenChange={(open) => !open && setEditing(null)}
          row={editing === "new" ? null : editing}
          period={period}
          today={today}
        />
      )}
      <DeleteAttendanceDialog row={deleting} onOpenChange={(open) => !open && setDeleting(null)} />
    </div>
  );
}
