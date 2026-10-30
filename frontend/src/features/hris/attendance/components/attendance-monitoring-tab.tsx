"use client";

import { useMemo, useState } from "react";
import { Card, CardContent } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { CalendarOff, Clock, RefreshCw, Users } from "lucide-react";
import { todayWib } from "@/lib/dates";
import { shiftClock } from "@/lib/hris/attendance-calendar";
import { useDailyRoster, useTeamAttendance } from "../queries";
import type { RosterStatus } from "../types";
import { RosterRow, STATUS_META } from "./roster-row";

/**
 * Tab "Monitoring" absensi HRD: roster satu hari, siapa sudah/belum absen
 * per shift, plus cuti/libur/tanpa jadwal. Karyawan terjadwal yang melewati
 * jam mulai + toleransi tanpa clock-in ditonjolkan (is_overdue).
 */

export function AttendanceMonitoringTab({ scope = "hr" }: {
  /** "team" = Absensi Tim atasan di Area Karyawan (EPIC-074): tim sendiri, tanpa foto. */
  scope?: "hr" | "team";
} = {}) {
  const team = scope === "team";
  const [date, setDate] = useState(() => todayWib());
  const hrQuery = useDailyRoster(date, !team);
  const teamQuery = useTeamAttendance(date, team);
  const rosterQuery = team ? teamQuery : hrQuery;
  const data = rosterQuery.data ?? null;
  const loading = rosterQuery.isLoading || rosterQuery.isFetching;
  const error = rosterQuery.error ? rosterQuery.error.message || "Gagal memuat roster" : null;
  const [filterStatus, setFilterStatus] = useState<"all" | RosterStatus>("all");
  const [filterShift, setFilterShift] = useState<string>("all");
  const [search, setSearch] = useState("");

  const filtered = useMemo(() => {
    if (!data) return [];
    const term = search.trim().toLowerCase();
    return data.employees.filter((emp) => {
      if (filterStatus !== "all" && emp.status !== filterStatus) return false;
      if (filterShift === "off" && emp.shift !== null) return false;
      if (filterShift !== "all" && filterShift !== "off" && emp.shift?.id !== filterShift)
        return false;
      if (term && !`${emp.full_name} ${emp.nip ?? ""}`.toLowerCase().includes(term))
        return false;
      return true;
    });
  }, [data, filterStatus, filterShift, search]);

  const groups = useMemo(() => {
    if (!data) return [];
    const byShift = data.shifts.map((shift) => ({
      key: shift.id,
      title: `${shift.name} (${shiftClock(shift.start_time)}–${shiftClock(shift.end_time)}${shift.is_overnight ? " +1 hari" : ""})`,
      employees: filtered.filter((emp) => emp.shift?.id === shift.id),
    }));
    const offSchedule = {
      key: "off",
      title: "Tanpa Shift Terjadwal (libur / cuti / di luar jadwal)",
      employees: filtered.filter((emp) => emp.shift === null),
    };
    return [...byShift, offSchedule].filter((group) => group.employees.length > 0);
  }, [data, filtered]);

  const summaryChips: { status: RosterStatus; count: number }[] = useMemo(() => {
    if (!data) return [];
    return (Object.keys(STATUS_META) as RosterStatus[])
      .map((status) => ({ status, count: data.summary[status] ?? 0 }))
      .filter(
        (chip) =>
          chip.count > 0 || ["hadir", "terlambat", "belum_absen"].includes(chip.status)
      )
      .filter((chip) => (data.is_today ? chip.status !== "absen" : chip.status !== "belum_absen"));
  }, [data]);

  return (
    <div className="space-y-4">
      {/* Kontrol: tanggal + filter */}
      <Card>
        <CardContent className="pt-6">
          <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-3">
            <div>
              <label className="text-sm font-medium text-gray-700 mb-2 block">Tanggal</label>
              <div className="flex gap-2">
                <Input type="date" value={date} onChange={(e) => setDate(e.target.value)} />
                <Button
                  variant="outline"
                  size="icon"
                  onClick={() => void rosterQuery.refetch()}
                  title="Muat ulang"
                >
                  <RefreshCw className={`w-4 h-4 ${loading ? "animate-spin" : ""}`} />
                </Button>
              </div>
            </div>
            <div>
              <label className="text-sm font-medium text-gray-700 mb-2 block">Shift</label>
              <Select value={filterShift} onValueChange={setFilterShift}>
                <SelectTrigger>
                  <SelectValue placeholder="Semua shift" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="all">Semua Shift</SelectItem>
                  {(data?.shifts ?? []).map((shift) => (
                    <SelectItem key={shift.id} value={shift.id}>
                      {shift.name}
                    </SelectItem>
                  ))}
                  <SelectItem value="off">Tanpa Shift</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div>
              <label className="text-sm font-medium text-gray-700 mb-2 block">Status</label>
              <Select
                value={filterStatus}
                onValueChange={(value) => setFilterStatus(value as "all" | RosterStatus)}
              >
                <SelectTrigger>
                  <SelectValue placeholder="Semua status" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="all">Semua Status</SelectItem>
                  {(Object.keys(STATUS_META) as RosterStatus[]).map((status) => (
                    <SelectItem key={status} value={status}>
                      {STATUS_META[status].label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div>
              <label className="text-sm font-medium text-gray-700 mb-2 block">Cari</label>
              <Input
                placeholder="Nama / NIP…"
                value={search}
                onChange={(e) => setSearch(e.target.value)}
              />
            </div>
          </div>
        </CardContent>
      </Card>

      {/* Hari libur — jelaskan kenapa roster hari ini sepi (EPIC-036) */}
      {data && data.holidays.length > 0 && (
        <div className="flex items-start gap-2 rounded-lg border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700">
          <CalendarOff className="mt-0.5 h-4 w-4 shrink-0" />
          <p>
            <span className="font-semibold">
              {data.holidays.map((holiday) => holiday.name).join(" · ")}
            </span>{" "}
            — hari libur, karyawan terjadwal tidak dihitung mangkir.
          </p>
        </div>
      )}

      {/* Ringkasan */}
      {data && (
        <div className="flex flex-wrap items-center gap-2">
          <Badge className="bg-indigo-100 text-indigo-700">
            <Users className="w-3 h-3" /> Terjadwal {data.summary.scheduled}
          </Badge>
          {summaryChips.map(({ status, count }) => (
            <button
              key={status}
              onClick={() => setFilterStatus(filterStatus === status ? "all" : status)}
              className={`rounded-full transition-opacity ${
                filterStatus !== "all" && filterStatus !== status ? "opacity-40" : ""
              }`}
              title={`Filter ${STATUS_META[status].label}`}
            >
              <Badge className={STATUS_META[status].className}>
                {STATUS_META[status].label} {count}
              </Badge>
            </button>
          ))}
        </div>
      )}

      {/* Isi roster */}
      {rosterQuery.isLoading ? (
        <div className="flex justify-center py-12">
          <div className="animate-spin w-6 h-6 border-2 border-gray-300 border-t-blue-500 rounded-full" />
        </div>
      ) : error ? (
        <Card>
          <CardContent className="py-12 text-center text-red-500 text-sm">{error}</CardContent>
        </Card>
      ) : team && data && data.employees.length === 0 ? (
        <Card>
          <CardContent className="py-12 text-center text-gray-400">
            <Users className="w-8 h-8 mx-auto mb-2 text-gray-300" />
            Belum ada anggota tim.
          </CardContent>
        </Card>
      ) : groups.length === 0 ? (
        <Card>
          <CardContent className="py-12 text-center text-gray-400">
            <Clock className="w-8 h-8 mx-auto mb-2 text-gray-300" />
            Tidak ada data yang cocok dengan filter.
          </CardContent>
        </Card>
      ) : (
        <div className="space-y-5">
          {groups.map((group) => (
            <div key={group.key} className="space-y-2">
              <div className="flex items-center justify-between">
                <h3 className="text-sm font-semibold text-gray-700">{group.title}</h3>
                <span className="text-xs text-gray-400">{group.employees.length} karyawan</span>
              </div>
              <div className="space-y-2">
                {group.employees.map((emp) => (
                  <RosterRow
                    key={emp.employee_id}
                    emp={emp}
                    isToday={data?.is_today ?? false}
                  />
                ))}
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
