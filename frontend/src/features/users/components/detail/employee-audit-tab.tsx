"use client";

import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Loader2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { apiGet } from "@/lib/api-client";
import { auditActionLabel, auditFieldLabel, auditValue } from "@/lib/hris/employee-audit";

interface AuditEntry {
  id: string;
  created_at: string;
  actor_name: string | null;
  action: string;
  before: Record<string, unknown> | null;
  after: Record<string, unknown> | null;
  reason: string | null;
}

interface AuditPage {
  data: AuditEntry[];
  meta: { total: number; totalPages: number };
}

/** Kolom ID (uuid) tidak ditampilkan nilainya — cukup "diubah". */
const ID_FIELDS = new Set(["department_id", "section_id", "job_title_id", "reporting_to", "batch_id", "released_to"]);

function when(iso: string): string {
  const d = new Date(new Date(iso).getTime() + 7 * 3_600_000);
  const p = (n: number) => String(n).padStart(2, "0");
  return `${p(d.getUTCDate())}/${p(d.getUTCMonth() + 1)}/${d.getUTCFullYear()} ${p(d.getUTCHours())}:${p(d.getUTCMinutes())}`;
}

/** Tab "Riwayat Perubahan" detail karyawan (audit EPIC-056). */
export function EmployeeAuditTab({ employeeId }: { employeeId: string }) {
  const [page, setPage] = useState(1);
  const query = useQuery({
    queryKey: ["hris", "employee-audit", employeeId, page],
    queryFn: () => apiGet<AuditPage>(`/api/hris/employees/${employeeId}/audit?page=${page}`),
  });

  if (query.isLoading) {
    return (
      <div className="flex justify-center py-10">
        <Loader2 className="h-6 w-6 animate-spin text-gray-400" />
      </div>
    );
  }
  const entries = query.data?.data ?? [];
  const totalPages = query.data?.meta.totalPages ?? 1;
  if (entries.length === 0) {
    return <p className="py-10 text-center text-sm text-gray-400">Belum ada perubahan tercatat.</p>;
  }

  return (
    <div className="space-y-3">
      {entries.map((e) => {
        const after = e.after ?? {};
        const before = e.before ?? {};
        const fields = Object.keys(after);
        return (
          <div key={e.id} className="rounded-xl border border-gray-200/70 bg-white p-4">
            <div className="flex flex-wrap items-baseline justify-between gap-2">
              <p className="font-semibold text-gray-900">{auditActionLabel(e.action)}</p>
              <p className="text-xs text-gray-500">
                {e.actor_name ?? "Sistem"} · {when(e.created_at)}
              </p>
            </div>
            {fields.length > 0 ? (
              <table className="mt-2 w-full text-xs">
                <thead className="text-left text-gray-500">
                  <tr>
                    <th className="py-1 pr-3 font-medium">Data</th>
                    <th className="py-1 pr-3 font-medium">Sebelum</th>
                    <th className="py-1 font-medium">Sesudah</th>
                  </tr>
                </thead>
                <tbody>
                  {fields.map((f) => (
                    <tr key={f} className="border-t border-gray-100">
                      <td className="py-1 pr-3 text-gray-600">{auditFieldLabel(f)}</td>
                      <td className="py-1 pr-3 text-gray-500">
                        {ID_FIELDS.has(f) ? (f in before ? "—" : "") : f in before ? auditValue(f, before[f]) : "—"}
                      </td>
                      <td className="py-1 text-gray-900">{ID_FIELDS.has(f) ? "diubah" : auditValue(f, after[f])}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            ) : null}
            {e.reason ? <p className="mt-2 text-xs text-gray-500">Alasan: {e.reason}</p> : null}
          </div>
        );
      })}
      {totalPages > 1 ? (
        <div className="flex items-center justify-between text-sm text-gray-600">
          <span>
            Halaman {page} dari {totalPages}
          </span>
          <div className="flex gap-2">
            <Button size="sm" variant="outline" disabled={page <= 1} onClick={() => setPage(page - 1)}>
              Sebelumnya
            </Button>
            <Button size="sm" variant="outline" disabled={page >= totalPages} onClick={() => setPage(page + 1)}>
              Berikutnya
            </Button>
          </div>
        </div>
      ) : null}
    </div>
  );
}
