"use client";

import { useState } from "react";
import { AlarmClock, CheckCircle2, ChevronDown } from "lucide-react";
import { cn } from "@/lib/utils";
import { LATENESS_CODES, LATENESS_DISCLAIMER, LATENESS_SHORT, LATENESS_TONE } from "@/lib/hris/lateness-view";
import { useTeamLateness } from "../queries";
import type { LatenessCode, TeamLatenessMember } from "../types";
import { LatenessDaysList } from "./lateness-days-list";
import { LatenessLevelBadge } from "./lateness-level-badge";
import { LatenessPeriodSelect } from "./lateness-period-select";

type Filter = "late" | LatenessCode;

function MemberRow({ member }: { member: TeamLatenessMember }) {
  const [open, setOpen] = useState(false);
  const late = member.count > 0;
  const sub = [member.position_title, member.direct ? null : `via ${member.reporting_to_name}`].filter(Boolean).join(" · ");
  return (
    <li className="py-1">
      <button
        type="button"
        disabled={!late}
        aria-expanded={late ? open : undefined}
        aria-label={late ? `Hari terlambat ${member.full_name}` : undefined}
        onClick={() => setOpen((v) => !v)}
        className="flex min-h-12 w-full items-center gap-3 rounded-xl px-2 py-2 text-left transition enabled:hover:bg-gray-50 disabled:cursor-default"
      >
        <span className="min-w-0 flex-1">
          <span className="block break-words text-sm font-semibold text-gray-900">{member.full_name}</span>
          {sub ? <span className="block break-words text-xs text-gray-500">{sub}</span> : null}
        </span>
        <span className="flex shrink-0 flex-col items-end gap-1">
          <span className={cn("text-sm font-bold tabular-nums", LATENESS_TONE[member.level.code].text)}>
            {member.count}x
          </span>
          <LatenessLevelBadge level={member.level} short />
        </span>
        {late ? (
          <ChevronDown className={cn("h-4 w-4 shrink-0 text-gray-400 transition-transform", open && "rotate-180")} />
        ) : (
          <span className="w-4 shrink-0" aria-hidden />
        )}
      </button>
      {late && open ? (
        <div className="px-2 pb-2">
          <LatenessDaysList days={member.days} />
        </div>
      ) : null}
    </li>
  );
}

/**
 * ESS → Absensi Tim: "Keterlambatan Tim" (EPIC-077 R10) — jumlah terlambat
 * tiap anggota tim (langsung & tidak langsung) per periode absensi, chip
 * ringkasan per tingkat sebagai filter. Tidak dirender tanpa anggota tim.
 */
export function TeamLatenessCard() {
  const [back, setBack] = useState(0);
  const [filter, setFilter] = useState<Filter>("late");
  const data = useTeamLateness(back).data;
  if (!data || data.employees.length === 0) return null;

  const lateCount = data.employees.filter((m) => m.count > 0).length;
  const shown = data.employees.filter((m) => (filter === "late" ? m.count > 0 : m.level.code === filter));
  const chips: { key: Filter; label: string; count: number; tone: string }[] = [
    { key: "late", label: "Terlambat", count: lateCount, tone: "bg-gray-900 text-white ring-gray-900" },
    ...LATENESS_CODES.map((code) => ({
      key: code as Filter,
      label: LATENESS_SHORT[code],
      count: data.summary[code] ?? 0,
      tone: LATENESS_TONE[code].badge,
    })),
  ];

  return (
    <section
      aria-labelledby="keterlambatan-tim-title"
      className="space-y-4 rounded-2xl border border-gray-200/70 bg-white p-4 shadow-sm sm:p-5"
    >
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="min-w-0">
          <h2 id="keterlambatan-tim-title" className="flex items-center gap-2 text-base font-semibold text-gray-900">
            <AlarmClock className="h-5 w-5 shrink-0 text-pink-600" />
            Keterlambatan Tim
          </h2>
          <p className="mt-0.5 text-xs text-gray-500">
            Periode absensi {data.period.label} · {data.employees.length} anggota
          </p>
        </div>
        <LatenessPeriodSelect periods={data.periods} value={back} onChange={setBack} label="Pilih periode tim" />
      </div>

      <div className="flex flex-wrap gap-2" role="group" aria-label="Saring tingkat">
        {chips.map((chip) => (
          <button
            key={chip.key}
            type="button"
            aria-pressed={filter === chip.key}
            onClick={() => setFilter(chip.key)}
            className={cn(
              "inline-flex min-h-10 items-center gap-1 rounded-full px-3 text-xs font-semibold ring-1 transition",
              chip.tone,
              filter === chip.key ? "ring-2 ring-offset-1 ring-offset-white" : "opacity-80 hover:opacity-100"
            )}
          >
            {chip.label} · <span className="tabular-nums">{chip.count}</span>
          </button>
        ))}
      </div>

      {shown.length > 0 ? (
        <ul className="divide-y divide-gray-100">
          {shown.map((m) => (
            <MemberRow key={m.employee_id} member={m} />
          ))}
        </ul>
      ) : (
        <p className="flex items-start gap-2 rounded-xl bg-emerald-50 p-3 text-sm text-emerald-800">
          <CheckCircle2 className="mt-0.5 h-4 w-4 shrink-0" />
          {filter === "late" ? "Semua anggota tepat waktu di periode ini." : "Tidak ada anggota di tingkat ini."}
        </p>
      )}

      <p className="text-xs text-gray-500">{LATENESS_DISCLAIMER}</p>
    </section>
  );
}
