"use client";

import { useQuery } from "@tanstack/react-query";
import { ArrowDownTrayIcon } from "@heroicons/react/24/outline";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { fetchOrgChartGap, fetchOrgChartValidation, orgChartGapXlsxUrl } from "../api";
import { orgChartQueryKeys } from "../query-keys";
import type { ChartGapSummary, ChartIssue } from "../types";

const SUMMARY: { key: keyof ChartGapSummary; label: string }[] = [
  { key: "employees", label: "Karyawan aktif" },
  { key: "matched", label: "Cocok ke satu posisi" },
  { key: "ambiguous", label: "Cocok ke beberapa posisi" },
  { key: "outside", label: "Jabatan di luar bagan" },
  { key: "manager_diff", label: "Atasan berbeda" },
  { key: "alias_review", label: "Alias perlu ditinjau" },
  { key: "empty_nodes", label: "Posisi tanpa pemegang" },
  { key: "stores_outside", label: "Toko di luar bagan" },
];

function Issues({ title, issues, tone }: { title: string; issues: ChartIssue[]; tone: "danger" | "warning" }) {
  if (issues.length === 0) return null;
  return (
    <div className="space-y-1.5">
      <p className={`text-sm font-semibold ${tone === "danger" ? "text-danger" : "text-warning"}`}>
        {issues.length} {title}
      </p>
      <ul className={`space-y-1 rounded-lg p-3 text-sm ${tone === "danger" ? "bg-danger-soft" : "bg-warning-soft"}`}>
        {issues.map((i, idx) => (
          <li key={idx} className="break-words">
            <span className="text-xs font-semibold text-gray-600">
              {i.rule}
              {i.code ? ` · ${i.code}` : ""}
            </span>{" "}
            <span>{i.message}</span>
          </li>
        ))}
      </ul>
    </div>
  );
}

/**
 * Selisih bagan vs data karyawan (EPIC-101): ringkasan, hasil validasi
 * struktur, dan laporan Excel lengkap untuk ditinjau HR. Tidak mengubah data.
 */
export function ChartGap({ version }: { version: string }) {
  const gap = useQuery({ queryKey: orgChartQueryKeys.gap(version), queryFn: () => fetchOrgChartGap(version) });
  const validation = useQuery({
    queryKey: orgChartQueryKeys.validation(version),
    queryFn: () => fetchOrgChartValidation(version),
  });
  const data = gap.data;

  return (
    <div className="space-y-4">
      <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
        <p className="text-sm text-gray-600">
          Perbandingan bagan {data?.version.label ?? ""} dengan jabatan, toko, dan atasan langsung karyawan aktif.
          Tidak ada data yang diubah.
        </p>
        <Button variant="outline" size="sm" asChild>
          <a href={orgChartGapXlsxUrl(version)} download>
            <ArrowDownTrayIcon className="mr-1.5 h-4 w-4" /> Unduh Excel
          </a>
        </Button>
      </div>

      {gap.isError ? (
        <p role="alert" className="rounded-lg bg-danger-soft p-3 text-sm text-danger">
          Laporan selisih tidak bisa dimuat
          {gap.error instanceof Error && gap.error.message ? ` — ${gap.error.message}` : ""}
        </p>
      ) : gap.isLoading ? (
        <div className="flex justify-center py-12">
          <div className="h-8 w-8 animate-spin rounded-full border-2 border-gray-300 border-t-blue-500" />
        </div>
      ) : data ? (
        <>
          <div className="grid grid-cols-2 gap-2 sm:grid-cols-4">
            {SUMMARY.map((s) => (
              <Card key={s.key}>
                <CardContent className="p-3">
                  <p className="text-xs text-gray-500">{s.label}</p>
                  <p className="text-xl font-semibold text-gray-900">{data.summary[s.key]}</p>
                </CardContent>
              </Card>
            ))}
          </div>
          {data.titles_outside.length > 0 ? (
            <div className="space-y-1.5">
              <p className="text-sm font-semibold text-gray-800">Jabatan HRIS di luar bagan</p>
              <ul className="space-y-0.5 rounded-lg border bg-white p-3 text-sm">
                {data.titles_outside.slice(0, 20).map((t) => (
                  <li key={t.title} className="flex justify-between gap-3">
                    <span className="break-words">{t.title}</span>
                    <span className="shrink-0 text-gray-500">{t.employees}</span>
                  </li>
                ))}
              </ul>
            </div>
          ) : null}
          {data.stores_outside.length > 0 ? (
            <div className="space-y-1.5">
              <p className="text-sm font-semibold text-gray-800">Toko aktif di luar bagan</p>
              <div className="flex flex-wrap gap-1.5">
                {data.stores_outside.map((s) => (
                  <span key={s} className="rounded bg-gray-100 px-2 py-0.5 text-xs text-gray-700">
                    {s}
                  </span>
                ))}
              </div>
            </div>
          ) : null}
        </>
      ) : null}

      {validation.isError ? (
        <p role="alert" className="rounded-lg bg-danger-soft p-3 text-sm text-danger">
          Validasi bagan tidak bisa dimuat
          {validation.error instanceof Error && validation.error.message ? ` — ${validation.error.message}` : ""}
        </p>
      ) : null}
      {validation.data ? (
        <div className="space-y-3">
          <p className="text-sm font-semibold text-gray-800">Validasi struktur bagan</p>
          {validation.data.errors.length === 0 && validation.data.warnings.length === 0 ? (
            <p className="text-sm text-gray-500">Tidak ada temuan.</p>
          ) : null}
          <Issues title="error" issues={validation.data.errors} tone="danger" />
          <Issues title="peringatan" issues={validation.data.warnings} tone="warning" />
        </div>
      ) : null}
    </div>
  );
}
