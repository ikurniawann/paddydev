"use client";

import {
  CalendarDaysIcon,
  BanknotesIcon,
  ClockIcon,
  BriefcaseIcon,
} from "@heroicons/react/24/outline";
import { Loader2 } from "lucide-react";
import { formatRupiah } from "@/lib/format";
import { greeting, initials, tenure } from "@/lib/hris/ess-view";
import { monthName, monthYearLabel } from "@/lib/hris/month-label";
import { TherapistBerandaCard } from "@/features/spa/components/therapist-beranda-card";
import { useSpaMe } from "@/features/spa/queries";
import { formatDays } from "@/features/hris/leave-balances/balance-math";
import { useScheduleAccess } from "@/features/hris/schedule-access/queries";
import { useEssBeranda } from "../queries";
import { EssClockPanel } from "./ess-clock-panel";
import { JadwalTokoShortcut } from "./jadwal-toko-shortcut";
import { balanceSubtitle } from "./leave-balance-summary";
import {
  AnnouncementsCard,
  KpiSummaryCard,
  QuickActions,
  RecentRequestsCard,
  StatCard,
  WeekScheduleCard,
} from "./beranda-widgets";

/**
 * Beranda Karyawan (/dashboard/me): ringkasan "hari saya": profil, absensi
 * hari ini + clock, saldo cuti/kehadiran/slip/pinjaman, jadwal minggu ini,
 * pengumuman terbaru, status pengajuan, aksi cepat, ringkas KPI. Semua data
 * dari satu endpoint agregasi /api/hris/me/beranda.
 */
export function EssBerandaPage() {
  const { data, isLoading } = useEssBeranda();
  // Dimuat paralel dengan beranda agar kartu Jadwal Toko tidak muncul belakangan (EPIC-073).
  useScheduleAccess();
  const spaMe = useSpaMe();
  const isTherapist = !!spaMe.data?.therapist?.is_active;

  if (isLoading) {
    return (
      <div className="flex justify-center py-24">
        <Loader2 className="h-8 w-8 animate-spin text-gray-400" />
      </div>
    );
  }

  // Akun tak tertaut record karyawan (mis. super admin murni)
  if (!data || !data.employee) {
    return (
      <div className="rounded-xl border border-amber-200 bg-amber-50 p-8 text-center">
        <BriefcaseIcon className="mx-auto h-10 w-10 text-amber-400" />
        <p className="mt-3 font-semibold text-amber-800">Akun belum terhubung ke data karyawan</p>
        <p className="mt-1 text-sm text-amber-600">
          Beranda karyawan hanya tersedia untuk akun yang tertaut ke record kepegawaian.
        </p>
      </div>
    );
  }

  const emp = data.employee;
  const { attendance, today_shift, latest_payslip, active_loans, kpi } = data;
  const leaveAvailable = data.leave_balance ? data.leave_balance.available : null;

  return (
    <div className="space-y-6">
      {/* ── Header sapaan + profil ── */}
      <div className="overflow-hidden rounded-2xl bg-gradient-to-br from-pink-600 via-pink-500 to-indigo-500 p-5 text-white shadow-md sm:p-6">
        <div className="flex flex-wrap items-center gap-4">
          <div className="flex h-14 w-14 items-center justify-center rounded-full bg-white/20 text-lg font-bold ring-2 ring-white/40 backdrop-blur-sm">
            {initials(emp.full_name)}
          </div>
          <div className="min-w-0 flex-1">
            <p className="text-sm text-white/80">{greeting()},</p>
            <h1 className="truncate text-xl font-bold sm:text-2xl">{emp.full_name}</h1>
            <p className="mt-0.5 truncate text-sm text-white/85">
              {[emp.position_title, emp.department_name].filter(Boolean).join(" · ") || "Karyawan"}
            </p>
          </div>
          <div className="flex flex-col items-end gap-1 text-right">
            <span className="rounded-full bg-white/20 px-2.5 py-1 text-xs font-semibold capitalize backdrop-blur-sm">
              {emp.employment_status?.replace(/_/g, " ") || "-"}
            </span>
            <span className="text-[11px] text-white/75">
              {emp.nip ? `NIP ${emp.nip} · ` : ""}Masa kerja {tenure(emp.join_date)}
            </span>
          </div>
        </div>
      </div>

      {/* ── Absensi hari ini + clock ── */}
      <div className="rounded-2xl border border-gray-200/70 bg-white p-5 shadow-sm">
        <div className="mb-3 flex flex-wrap items-center justify-between gap-2">
          <h2 className="flex items-center gap-2 text-base font-semibold text-gray-900">
            <ClockIcon className="h-5 w-5 text-pink-600" /> Absensi Hari Ini
          </h2>
          {today_shift ? (
            <span className="text-xs text-gray-500">
              Shift <b className="text-gray-700">{today_shift.name}</b>
              {today_shift.start_time && today_shift.end_time
                ? ` · ${today_shift.start_time.slice(0, 5)}–${today_shift.end_time.slice(0, 5)}`
                : ""}
              {today_shift.late_tolerance_minutes
                ? ` · toleransi ${today_shift.late_tolerance_minutes} mnt`
                : ""}
            </span>
          ) : (
            <span className="rounded-full bg-gray-100 px-2.5 py-1 text-xs font-medium text-gray-500">
              {data.has_schedule ? "Libur / tanpa shift hari ini" : "Belum ada jadwal shift"}
            </span>
          )}
        </div>
        <EssClockPanel />
      </div>

      {/* ── Pengatur jadwal toko / HR Jadwal Kerja: pintasan Jadwal Toko (EPIC-073) ── */}
      <JadwalTokoShortcut />

      {/* ── Terapis spa: booking berikutnya & komisi (hanya akun terapis) ── */}
      <TherapistBerandaCard />

      {/* ── Ringkasan cepat ── */}
      <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
        <StatCard
          href="/dashboard/me/cuti"
          icon={<CalendarDaysIcon className="h-5 w-5 text-indigo-600" />}
          accent="bg-indigo-50"
          label="Sisa Cuti Tahunan"
          value={leaveAvailable !== null ? `${formatDays(leaveAvailable)} hari` : "-"}
          sub={data.leave_balance ? balanceSubtitle(data.leave_balance) : "Belum ada kuota"}
        />
        <StatCard
          href="/dashboard/me/absensi"
          icon={<ClockIcon className="h-5 w-5 text-pink-600" />}
          accent="bg-pink-50"
          label={`Kehadiran ${monthName(attendance.month)}`}
          value={`${attendance.present} hari`}
          sub={`${attendance.late} terlambat${attendance.avg_work_hours ? ` · avg ${attendance.avg_work_hours} jam` : ""}`}
        />
        <StatCard
          href="/dashboard/me/slip-gaji"
          icon={<BanknotesIcon className="h-5 w-5 text-emerald-600" />}
          accent="bg-emerald-50"
          label="Slip Gaji Terakhir"
          value={latest_payslip ? formatRupiah(latest_payslip.net_salary) : "-"}
          sub={
            latest_payslip
              ? monthYearLabel(latest_payslip.period_month, latest_payslip.period_year)
              : "Belum ada slip dibayar"
          }
        />
        <StatCard
          href="/dashboard/me/pinjaman"
          icon={<BanknotesIcon className="h-5 w-5 text-amber-600" />}
          accent="bg-amber-50"
          label="Pinjaman Aktif"
          value={active_loans.count > 0 ? formatRupiah(active_loans.total_remaining) : "Tidak ada"}
          sub={
            active_loans.count > 0
              ? `${active_loans.count} pinjaman · cicilan ${formatRupiah(active_loans.monthly_installment)}/bln`
              : "Tidak ada tanggungan"
          }
        />
      </div>

      {data.week_schedule?.length > 0 && <WeekScheduleCard days={data.week_schedule} />}

      <div className="grid gap-4 lg:grid-cols-2">
        <AnnouncementsCard announcements={data.announcements} />
        <RecentRequestsCard requests={data.recent_requests} />
      </div>

      {kpi && <KpiSummaryCard kpi={kpi} />}

      <QuickActions therapist={isTherapist} />
    </div>
  );
}
