"use client";

import type { ReactNode } from "react";
import { Card, CardContent } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import {
  ArrowRightStartOnRectangleIcon,
  BriefcaseIcon,
  CheckCircleIcon,
  ClipboardDocumentCheckIcon,
  KeyIcon,
  RocketLaunchIcon,
  UserPlusIcon,
} from "@heroicons/react/24/outline";
import { formatDate } from "@/lib/format";
import { STATUS_LABELS } from "../constants";
import { useEmployeeLifecycle } from "../queries";
import type { EmployeeLifecycleData } from "../api";

const HISTORY_TYPE_LABELS: Record<string, string> = {
  hire: "Direkrut",
  promotion: "Promosi",
  transfer: "Mutasi",
  demotion: "Demosi",
  status_change: "Perubahan Status",
  salary_change: "Perubahan Gaji",
  resign: "Resign",
  terminated: "Diberhentikan",
  end_of_contract: "Kontrak Berakhir",
  contract_activated: "Kontrak Diaktifkan",
  rehire: "Diaktifkan Kembali",
};

const SOURCE_LABELS: Record<string, string> = {
  portal: "Portal Karier",
  internal: "Internal",
  referral: "Rekomendasi",
  jobstreet: "JobStreet",
  instagram: "Instagram",
  jobfair: "Job Fair",
  other: "Lainnya",
};

type PhaseStatus = "done" | "current" | "upcoming" | "attention";

interface Phase {
  key: string;
  icon: ReactNode;
  title: string;
  date: string | null;
  status: PhaseStatus;
  lines: ReactNode[];
}

const PHASE_DOT: Record<PhaseStatus, string> = {
  done: "bg-emerald-500",
  current: "bg-blue-500 ring-4 ring-blue-100",
  upcoming: "bg-gray-300",
  attention: "bg-amber-500",
};

/** Susun fase lifecycle dari data agregat server. */
function buildPhases(data: EmployeeLifecycleData): Phase[] {
  const phases: Phase[] = [];
  const { employee, recruitment, account, onboarding, history, offboarding } = data;
  const isEnded =
    Boolean(employee.end_date) || ["resigned", "terminated"].includes(employee.employment_status);

  // 1. Rekrutmen
  phases.push({
    key: "recruitment",
    icon: <UserPlusIcon className="w-4 h-4" />,
    title: "Rekrutmen",
    date: recruitment?.applied_at ?? null,
    status: recruitment ? "done" : "upcoming",
    lines: recruitment
      ? [
          <span key="a">
            Melamar {formatDate(recruitment.applied_at)}
            {recruitment.source
              ? ` via ${SOURCE_LABELS[recruitment.source] ?? recruitment.source}`
              : ""}
            {recruitment.position_title ? ` — posisi ${recruitment.position_title}` : ""}
          </span>,
          recruitment.offer_accepted_at ? (
            <span key="b">Menerima offer {formatDate(recruitment.offer_accepted_at)}</span>
          ) : null,
          recruitment.promoted_at ? (
            <span key="c">Dipromosikan jadi karyawan {formatDate(recruitment.promoted_at)}</span>
          ) : null,
        ].filter(Boolean)
      : [<span key="none">Tidak melalui pipeline rekrutmen (input langsung)</span>],
  });

  // 2. Bergabung
  phases.push({
    key: "join",
    icon: <RocketLaunchIcon className="w-4 h-4" />,
    title: "Bergabung",
    date: employee.join_date,
    status: employee.join_date ? "done" : "upcoming",
    lines: [
      <span key="a">
        Mulai bekerja {formatDate(employee.join_date, "—")} · status awal{" "}
        {STATUS_LABELS[
          history.find((h) => h.change_type === "hire")?.new_employment_status ?? ""
        ] ??
          STATUS_LABELS[employee.employment_status] ??
          employee.employment_status}
      </span>,
    ],
  });

  // 3. Onboarding
  const onboardingDone = onboarding.total > 0 && onboarding.completed === onboarding.total;
  phases.push({
    key: "onboarding",
    icon: <ClipboardDocumentCheckIcon className="w-4 h-4" />,
    title: "Onboarding",
    date: onboardingDone ? onboarding.last_completed_at : null,
    status: onboarding.total === 0 ? "upcoming" : onboardingDone ? "done" : "attention",
    lines: [
      onboarding.total === 0 ? (
        <span key="a">Checklist onboarding belum dibuat</span>
      ) : (
        <span key="a">
          {onboarding.completed}/{onboarding.total} tugas selesai
          {onboardingDone && onboarding.last_completed_at
            ? ` · tuntas ${formatDate(onboarding.last_completed_at)}`
            : ""}
        </span>
      ),
    ],
  });

  // 4. Akun aplikasi
  phases.push({
    key: "account",
    icon: <KeyIcon className="w-4 h-4" />,
    title: "Akun Aplikasi",
    date: account?.created_at ?? null,
    status: account ? "done" : "attention",
    lines: account
      ? [
          <span key="a">
            Akun {account.email} dibuat {formatDate(account.created_at)}
            {account.last_sign_in_at
              ? ` · login terakhir ${formatDate(account.last_sign_in_at)}`
              : " · belum pernah login"}
          </span>,
        ]
      : [
          <span key="a">
            Belum punya akun login — buat lewat tombol &quot;Buat Akun Login&quot;
          </span>,
        ],
  });

  // 5. Perjalanan kepegawaian (selain hire)
  const journey = history.filter((h) => h.change_type !== "hire");
  phases.push({
    key: "journey",
    icon: <BriefcaseIcon className="w-4 h-4" />,
    title: "Perjalanan Kepegawaian",
    date: journey.length > 0 ? journey[journey.length - 1].effective_date : null,
    status: journey.length > 0 ? "done" : isEnded ? "done" : "current",
    lines:
      journey.length > 0
        ? journey.map((h) => (
            <span key={h.id}>
              {formatDate(h.effective_date)} — {HISTORY_TYPE_LABELS[h.change_type] ?? h.change_type}
              {h.new_job_title ? `: ${h.prev_job_title ?? "—"} → ${h.new_job_title}` : ""}
              {h.new_employment_status
                ? ` (${STATUS_LABELS[h.prev_employment_status ?? ""] ?? "—"} → ${STATUS_LABELS[h.new_employment_status] ?? h.new_employment_status})`
                : ""}
              {h.new_department_name && h.new_department_name !== h.prev_department_name
                ? ` · dept ${h.prev_department_name ?? "—"} → ${h.new_department_name}`
                : ""}
            </span>
          ))
        : [
            <span key="a">
              Belum ada perubahan — status saat ini{" "}
              {STATUS_LABELS[employee.employment_status] ?? employee.employment_status}
            </span>,
          ],
  });

  // 6. Offboarding / berakhir
  if (offboarding || isEnded) {
    const clearances = offboarding
      ? [
          ["HRD", offboarding.clearance_hrd],
          ["IT", offboarding.clearance_it],
          ["Finance", offboarding.clearance_finance],
          ["Manager", offboarding.clearance_manager],
        ]
      : [];
    phases.push({
      key: "offboarding",
      icon: <ArrowRightStartOnRectangleIcon className="w-4 h-4" />,
      title: "Offboarding",
      date: offboarding?.last_working_day ?? employee.end_date,
      status: offboarding?.completed_at || isEnded ? "done" : "attention",
      lines: [
        offboarding ? (
          <span key="a">
            {offboarding.resignation_type ?? "Berakhir"} · pengajuan{" "}
            {formatDate(offboarding.resignation_date, "—")} · hari terakhir{" "}
            {formatDate(offboarding.last_working_day ?? employee.end_date, "—")}
          </span>
        ) : (
          <span key="a">Berakhir {formatDate(employee.end_date, "—")}</span>
        ),
        clearances.length > 0 ? (
          <span key="b" className="flex flex-wrap gap-1.5">
            {clearances.map(([label, done]) => (
              <Badge
                key={String(label)}
                className={done ? "bg-emerald-100 text-emerald-700" : "bg-gray-100 text-gray-500"}
              >
                {done ? "✓" : "•"} Clearance {label}
              </Badge>
            ))}
          </span>
        ) : null,
      ].filter(Boolean),
    });
  }

  // fase "current": fase pertama yang belum done (bila belum berakhir)
  if (!isEnded) {
    const firstPending = phases.find((p) => p.status === "upcoming" || p.status === "attention");
    if (firstPending && firstPending.status === "upcoming") firstPending.status = "current";
  }

  return phases;
}

/** Tab Lifecycle: perjalanan karyawan dari rekrutmen sampai offboarding. */
export function EmployeeLifecycleTab({ employeeId }: { employeeId: string }) {
  const { data, isLoading } = useEmployeeLifecycle(employeeId);

  if (isLoading) {
    return (
      <div className="flex justify-center py-12">
        <div className="animate-spin w-6 h-6 border-2 border-gray-300 border-t-blue-500 rounded-full" />
      </div>
    );
  }
  if (!data) {
    return (
      <Card>
        <CardContent className="py-12 text-center text-gray-400">
          Data lifecycle tidak tersedia
        </CardContent>
      </Card>
    );
  }

  const phases = buildPhases(data);

  return (
    <div className="space-y-4">
      <h3 className="font-semibold text-gray-700">Employee Lifecycle</h3>
      <div className="relative">
        <div className="absolute left-5 top-2 bottom-2 w-0.5 bg-gray-200" />
        <div className="space-y-4">
          {phases.map((phase) => (
            <div key={phase.key} className="relative pl-12">
              <div
                className={`absolute left-3.5 top-4 w-3.5 h-3.5 rounded-full border-2 border-white shadow ${PHASE_DOT[phase.status]}`}
              />
              <Card className={phase.status === "current" ? "border-blue-200" : ""}>
                <CardContent className="p-4">
                  <div className="flex flex-wrap items-center gap-2">
                    <span className="text-gray-400">{phase.icon}</span>
                    <span className="text-sm font-semibold text-gray-800">{phase.title}</span>
                    {phase.status === "done" && (
                      <CheckCircleIcon className="w-4 h-4 text-emerald-500" />
                    )}
                    {phase.status === "current" && (
                      <Badge className="bg-blue-100 text-blue-700">berjalan</Badge>
                    )}
                    {phase.status === "attention" && (
                      <Badge className="bg-amber-100 text-amber-700">perlu perhatian</Badge>
                    )}
                    {phase.date && (
                      <span className="ml-auto text-xs text-gray-400">
                        {formatDate(phase.date)}
                      </span>
                    )}
                  </div>
                  <div className="mt-1.5 space-y-1 text-sm text-gray-600">
                    {phase.lines.map((line, i) => (
                      <div key={i}>{line}</div>
                    ))}
                  </div>
                </CardContent>
              </Card>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}
