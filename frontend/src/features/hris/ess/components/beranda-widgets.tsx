"use client";

import Link from "next/link";
import {
  MegaphoneIcon,
  CalendarDaysIcon,
  BanknotesIcon,
  ClockIcon,
  ArrowRightIcon,
  BriefcaseIcon,
  ChevronRightIcon,
  SparklesIcon,
  WalletIcon,
} from "@heroicons/react/24/outline";
import { requestStatusBadge } from "@/lib/hris/ess-view";
import { leaveTypeLabel } from "@/lib/hris/leave-types";
import type { EssBeranda, EssRecentRequest } from "../types";

/** Kartu & daftar kecil di Beranda Karyawan. */

const DAY_LABELS = ["", "Sen", "Sel", "Rab", "Kam", "Jum", "Sab", "Min"];
const KIND_META: Record<EssRecentRequest["kind"], { label: string; cls: string }> = {
  cuti: { label: "Cuti", cls: "bg-indigo-100 text-indigo-700" },
  lembur: { label: "Lembur", cls: "bg-sky-100 text-sky-700" },
  pinjaman: { label: "Pinjaman", cls: "bg-emerald-100 text-emerald-700" },
};

export function StatCard({
  href, icon, label, value, sub, accent,
}: {
  href: string; icon: React.ReactNode; label: string; value: string; sub?: string; accent: string;
}) {
  return (
    <Link
      href={href}
      className="group flex flex-col gap-2 rounded-xl border border-gray-200/70 bg-white p-4 shadow-sm transition hover:border-pink-300 hover:shadow"
    >
      <div className="flex items-center justify-between">
        <span className={`flex h-9 w-9 items-center justify-center rounded-lg ${accent}`}>{icon}</span>
        <ChevronRightIcon className="h-4 w-4 text-gray-300 transition group-hover:text-pink-400" />
      </div>
      <div>
        <p className="text-2xl font-bold text-gray-900">{value}</p>
        <p className="text-xs font-medium text-gray-500">{label}</p>
        {sub && <p className="mt-0.5 text-[11px] text-gray-400">{sub}</p>}
      </div>
    </Link>
  );
}

function QuickAction({ href, label, icon }: { href: string; label: string; icon: React.ReactNode }) {
  return (
    <Link
      href={href}
      className="flex flex-col items-center gap-1.5 rounded-xl border border-gray-200/70 bg-white px-3 py-3 text-center shadow-sm transition hover:border-pink-300 hover:bg-pink-50/40"
    >
      <span className="text-pink-600">{icon}</span>
      <span className="text-[11px] font-semibold text-gray-600">{label}</span>
    </Link>
  );
}

export function WeekScheduleCard({ days }: { days: EssBeranda["week_schedule"] }) {
  return (
    <div className="rounded-2xl border border-gray-200/70 bg-white p-5 shadow-sm">
      <h2 className="mb-3 flex items-center gap-2 text-base font-semibold text-gray-900">
        <CalendarDaysIcon className="h-5 w-5 text-pink-600" /> Jadwal Minggu Ini
      </h2>
      <div className="grid grid-cols-7 gap-2">
        {days.map((d) => (
          <div
            key={d.date}
            className={`flex flex-col items-center gap-1 rounded-lg border p-2 text-center ${
              d.is_today ? "border-pink-300 bg-pink-50" : "border-gray-200/70 bg-gray-50/50"
            }`}
          >
            <span className={`text-[11px] font-semibold ${d.is_today ? "text-pink-600" : "text-gray-400"}`}>
              {DAY_LABELS[d.day_of_week]}
            </span>
            <span className="text-sm font-bold text-gray-700">{d.date.slice(8, 10)}</span>
            {d.status === "shift" ? (
              <span className="text-[9px] font-medium leading-tight text-gray-500">
                {d.start_time?.slice(0, 5)}
              </span>
            ) : (
              <span className="text-[9px] font-medium leading-tight text-gray-300">
                {d.status === "libur" ? "Libur" : "—"}
              </span>
            )}
          </div>
        ))}
      </div>
    </div>
  );
}

export function AnnouncementsCard({ announcements }: { announcements: EssBeranda["announcements"] }) {
  return (
    <div className="rounded-2xl border border-gray-200/70 bg-white p-5 shadow-sm">
      <div className="mb-3 flex items-center justify-between">
        <h2 className="flex items-center gap-2 text-base font-semibold text-gray-900">
          <MegaphoneIcon className="h-5 w-5 text-pink-600" /> Pengumuman
          {announcements.unread > 0 && (
            <span className="rounded-full bg-pink-600 px-1.5 text-[11px] font-bold text-white">
              {announcements.unread}
            </span>
          )}
        </h2>
        <Link href="/dashboard/me/pengumuman" className="flex items-center gap-1 text-xs font-semibold text-pink-600 hover:underline">
          Lihat semua <ArrowRightIcon className="h-3.5 w-3.5" />
        </Link>
      </div>
      {announcements.items.length === 0 ? (
        <p className="py-6 text-center text-sm text-gray-400">Belum ada pengumuman.</p>
      ) : (
        <ul className="divide-y divide-gray-100">
          {announcements.items.map((a) => (
            <li key={a.id}>
              <Link href="/dashboard/me/pengumuman" className="flex items-center gap-3 py-2.5 transition hover:opacity-80">
                {!a.is_read && <span className="h-2 w-2 shrink-0 rounded-full bg-pink-600" />}
                <span className={`min-w-0 flex-1 truncate text-sm ${a.is_read ? "text-gray-500" : "font-semibold text-gray-900"}`}>
                  {a.is_pinned && "📌 "}
                  {a.title}
                </span>
                {!a.is_read && (
                  <span className="shrink-0 rounded-full bg-pink-100 px-1.5 py-0.5 text-[10px] font-bold text-pink-600">Baru</span>
                )}
              </Link>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

export function RecentRequestsCard({ requests }: { requests: EssRecentRequest[] }) {
  return (
    <div className="rounded-2xl border border-gray-200/70 bg-white p-5 shadow-sm">
      <h2 className="mb-3 flex items-center gap-2 text-base font-semibold text-gray-900">
        <BriefcaseIcon className="h-5 w-5 text-pink-600" /> Status Pengajuan Saya
      </h2>
      {requests.length === 0 ? (
        <p className="py-6 text-center text-sm text-gray-400">Belum ada pengajuan.</p>
      ) : (
        <ul className="divide-y divide-gray-100">
          {requests.map((r) => {
            const badge = requestStatusBadge(r.status);
            const kind = KIND_META[r.kind];
            return (
              <li key={`${r.kind}-${r.id}`}>
                <Link href={r.href} className="flex items-center gap-2 py-2.5 transition hover:opacity-80">
                  <span className={`shrink-0 rounded px-1.5 py-0.5 text-[10px] font-bold ${kind.cls}`}>{kind.label}</span>
                  <span className="min-w-0 flex-1">
                    <span className="block truncate text-sm font-medium capitalize text-gray-800">
                      {leaveTypeLabel(r.label)}
                    </span>
                    <span className="block truncate text-[11px] text-gray-400">{r.detail}</span>
                  </span>
                  <span className={`shrink-0 rounded-full px-2 py-0.5 text-[10px] font-semibold ${badge.cls}`}>{badge.label}</span>
                </Link>
              </li>
            );
          })}
        </ul>
      )}
    </div>
  );
}

export function KpiSummaryCard({ kpi }: { kpi: NonNullable<EssBeranda["kpi"]> }) {
  return (
    <div className="rounded-2xl border border-gray-200/70 bg-white p-5 shadow-sm">
      <h2 className="mb-3 flex items-center gap-2 text-base font-semibold text-gray-900">
        <BriefcaseIcon className="h-5 w-5 text-pink-600" /> Kinerja (KPI) Terakhir
      </h2>
      <div className="flex flex-wrap items-center gap-6">
        <div>
          <p className="text-3xl font-bold text-gray-900">
            {kpi.avg_achievement !== null ? `${kpi.avg_achievement}%` : "-"}
          </p>
          <p className="text-xs text-gray-500">Rata-rata pencapaian</p>
        </div>
        <div className="h-10 w-px bg-gray-200" />
        <div>
          <p className="text-3xl font-bold text-gray-900">{kpi.avg_score ?? "-"}</p>
          <p className="text-xs text-gray-500">Skor rata-rata (1–5)</p>
        </div>
        <div className="h-10 w-px bg-gray-200" />
        <div>
          <p className="text-3xl font-bold text-gray-900">{kpi.count}</p>
          <p className="text-xs text-gray-500">Indikator dinilai</p>
        </div>
      </div>
      {kpi.avg_achievement !== null && (
        <div className="mt-3 h-2 w-full overflow-hidden rounded-full bg-gray-100">
          <div
            className="h-full rounded-full bg-gradient-to-r from-pink-500 to-indigo-500"
            style={{ width: `${Math.min(100, kpi.avg_achievement)}%` }}
          />
        </div>
      )}
    </div>
  );
}

/** `therapist`: tambah pintasan Tugas Terapis & Komisi (akun terapis spa). */
export function QuickActions({ therapist = false }: { therapist?: boolean }) {
  return (
    <div>
      <h2 className="mb-3 text-sm font-semibold text-gray-500">Aksi Cepat</h2>
      <div className={therapist ? "grid grid-cols-4 gap-3 lg:grid-cols-8" : "grid grid-cols-3 gap-3 sm:grid-cols-6"}>
        {therapist && (
          <>
            <QuickAction href="/dashboard/me/spa" label="Tugas Terapis" icon={<SparklesIcon className="h-6 w-6" />} />
            <QuickAction href="/dashboard/me/spa#komisi" label="Komisi" icon={<WalletIcon className="h-6 w-6" />} />
          </>
        )}
        <QuickAction href="/dashboard/me/absensi" label="Absensi" icon={<ClockIcon className="h-6 w-6" />} />
        <QuickAction href="/dashboard/me/cuti" label="Ajukan Cuti" icon={<CalendarDaysIcon className="h-6 w-6" />} />
        <QuickAction href="/dashboard/me/lembur" label="Lembur" icon={<ClockIcon className="h-6 w-6" />} />
        <QuickAction href="/dashboard/me/pinjaman" label="Pinjaman" icon={<BanknotesIcon className="h-6 w-6" />} />
        <QuickAction href="/dashboard/me/slip-gaji" label="Slip Gaji" icon={<BanknotesIcon className="h-6 w-6" />} />
        <QuickAction href="/dashboard/me/pengumuman" label="Pengumuman" icon={<MegaphoneIcon className="h-6 w-6" />} />
      </div>
    </div>
  );
}
