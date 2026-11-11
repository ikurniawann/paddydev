"use client";

import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { ArrowDownTrayIcon } from "@heroicons/react/24/outline";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { fetchOrgChartVacancy, orgChartVacancyXlsxUrl } from "../api";
import { orgChartQueryKeys } from "../query-keys";
import { NO_REGION, type ChartVacancyCounts, type ChartVacancyFilter, type VacancyState } from "../types";

export const VACANCY_STATE: Record<VacancyState, { label: string; color: string }> = {
  empty: { label: "Kosong", color: "bg-danger-soft text-danger" },
  maybe: { label: "Mungkin terisi", color: "bg-warning-soft text-warning" },
  planned: { label: "Rencana", color: "bg-blue-100 text-blue-700" },
  frozen: { label: "Dibekukan", color: "bg-gray-200 text-gray-700" },
};

const TILES: { key: keyof ChartVacancyCounts; label: string }[] = [
  { key: "positions", label: "Posisi" },
  { key: "filled", label: "Terisi" },
  { key: "empty", label: "Kosong" },
  { key: "maybe", label: "Mungkin terisi" },
  { key: "planned", label: "Rencana" },
  { key: "frozen", label: "Dibekukan" },
];

const selectClass = "h-9 w-full rounded-md border border-gray-300 bg-white px-2 text-sm sm:w-48";
const regionText = (r: string) => r || "Tanpa region";

/**
 * Posisi Kosong (EPIC-102): slot bagan yang belum ada pemegangnya, per divisi
 * dan region. "Mungkin terisi" = ada karyawan berjabatan sama yang belum bisa
 * dipastikan slotnya; namanya ikut ditampilkan. Hanya baca.
 */
export function ChartVacancy({ version }: { version: string }) {
  const [filter, setFilter] = useState<ChartVacancyFilter>({ division: "", region: "", state: "" });
  const base = useQuery({
    queryKey: orgChartQueryKeys.vacancy(version, "", "", ""),
    queryFn: () => fetchOrgChartVacancy(version, { division: "", region: "", state: "" }),
  });
  const res = useQuery({
    queryKey: orgChartQueryKeys.vacancy(version, filter.division, filter.region, filter.state),
    queryFn: () => fetchOrgChartVacancy(version, filter),
  });
  const allGroups = base.data?.groups ?? [];
  const divisions = [...new Map(allGroups.map((g) => [g.division, g.division_name || g.division])).entries()];
  const inDivision = allGroups.filter((g) => !filter.division || g.division === filter.division);
  const regions = [...new Set(inDivision.map((g) => g.region).filter(Boolean))];
  const hasNoRegion = inDivision.some((g) => !g.region);
  const data = res.data;

  return (
    <div className="space-y-4">
      <div className="flex flex-col gap-3 sm:flex-row sm:flex-wrap sm:items-end">
        <label className="block text-sm">
          <span className="mb-1 block text-xs font-medium text-gray-600">Divisi</span>
          <select
            aria-label="Divisi"
            className={selectClass}
            value={filter.division}
            onChange={(e) => setFilter((f) => ({ ...f, division: e.target.value, region: "" }))}
          >
            <option value="">Semua divisi</option>
            {divisions.map(([code, name]) => (
              <option key={code} value={code}>
                {name}
              </option>
            ))}
          </select>
        </label>
        <label className="block text-sm">
          <span className="mb-1 block text-xs font-medium text-gray-600">Region</span>
          <select
            aria-label="Region"
            className={selectClass}
            value={filter.region}
            onChange={(e) => setFilter((f) => ({ ...f, region: e.target.value }))}
          >
            <option value="">Semua region</option>
            {hasNoRegion ? <option value={NO_REGION}>Tanpa region</option> : null}
            {regions.map((r) => (
              <option key={r} value={r}>
                {r}
              </option>
            ))}
          </select>
        </label>
        <label className="block text-sm">
          <span className="mb-1 block text-xs font-medium text-gray-600">Keadaan</span>
          <select
            aria-label="Keadaan"
            className={selectClass}
            value={filter.state}
            onChange={(e) => setFilter((f) => ({ ...f, state: e.target.value }))}
          >
            <option value="">Semua belum terisi</option>
            {(Object.keys(VACANCY_STATE) as VacancyState[]).map((s) => (
              <option key={s} value={s}>
                {VACANCY_STATE[s].label}
              </option>
            ))}
          </select>
        </label>
        <Button variant="outline" size="sm" className="sm:ml-auto" asChild>
          <a href={orgChartVacancyXlsxUrl(version, filter)} download>
            <ArrowDownTrayIcon className="mr-1.5 h-4 w-4" /> Unduh Excel
          </a>
        </Button>
      </div>

      {res.isError ? (
        <p role="alert" className="rounded-lg bg-danger-soft p-3 text-sm text-danger">
          Posisi kosong tidak bisa dimuat
          {res.error instanceof Error && res.error.message ? ` — ${res.error.message}` : ""}
        </p>
      ) : res.isLoading ? (
        <div className="flex justify-center py-12">
          <div className="h-8 w-8 animate-spin rounded-full border-2 border-gray-300 border-t-blue-500" />
        </div>
      ) : data ? (
        <>
          <div aria-label="Ringkasan posisi" className="grid grid-cols-2 gap-2 sm:grid-cols-3 lg:grid-cols-6">
            {TILES.map((t) => (
              <Card key={t.key}>
                <CardContent className="p-3">
                  <p className="text-xs text-gray-500">{t.label}</p>
                  <p className="text-xl font-semibold text-gray-900">{data.summary[t.key]}</p>
                </CardContent>
              </Card>
            ))}
          </div>
          <p className="text-xs text-gray-500">
            {data.version.label} · keadaan dihitung dari jabatan dan toko karyawan aktif; tidak ada data yang diubah.
          </p>

          <div className="overflow-x-auto rounded-lg border bg-white">
            <table aria-label="Ringkasan per divisi dan region" className="w-full min-w-[640px] text-sm">
              <thead className="bg-gray-50 text-left text-xs text-gray-600">
                <tr>
                  <th className="px-3 py-2">Divisi</th>
                  <th className="px-3 py-2">Region</th>
                  <th className="px-3 py-2 text-right">Posisi</th>
                  <th className="px-3 py-2 text-right">Terisi</th>
                  <th className="px-3 py-2 text-right">Kosong</th>
                  <th className="px-3 py-2 text-right">Mungkin terisi</th>
                  <th className="px-3 py-2 text-right">Rencana</th>
                  <th className="px-3 py-2 text-right">Dibekukan</th>
                </tr>
              </thead>
              <tbody>
                {data.groups.map((g) => (
                  <tr key={`${g.division}|${g.region}`} className="border-t">
                    <td className="px-3 py-1.5">{g.division_name || g.division}</td>
                    <td className="px-3 py-1.5">{regionText(g.region)}</td>
                    <td className="px-3 py-1.5 text-right">{g.positions}</td>
                    <td className="px-3 py-1.5 text-right">{g.filled}</td>
                    <td className="px-3 py-1.5 text-right">{g.empty}</td>
                    <td className="px-3 py-1.5 text-right">{g.maybe}</td>
                    <td className="px-3 py-1.5 text-right">{g.planned}</td>
                    <td className="px-3 py-1.5 text-right">{g.frozen}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>

          {data.rows.length === 0 ? (
            <Card>
              <CardContent className="py-12 text-center text-sm text-gray-400">
                {filter.state ? "Tidak ada posisi dengan keadaan ini" : "Semua posisi untuk filter ini sudah terisi"}
              </CardContent>
            </Card>
          ) : (
            <div className="overflow-x-auto rounded-lg border bg-white">
              <table aria-label="Daftar posisi belum terisi" className="w-full min-w-[760px] text-sm">
                <thead className="bg-gray-50 text-left text-xs text-gray-600">
                  <tr>
                    <th className="px-3 py-2">Kode</th>
                    <th className="px-3 py-2">Jabatan</th>
                    <th className="px-3 py-2">Divisi · Region</th>
                    <th className="px-3 py-2">Toko</th>
                    <th className="px-3 py-2">Atasan Bagan</th>
                    <th className="px-3 py-2">Keadaan</th>
                  </tr>
                </thead>
                <tbody>
                  {data.rows.map((r) => {
                    const st = VACANCY_STATE[r.state] ?? VACANCY_STATE.empty;
                    return (
                      <tr key={r.code} className="border-t align-top">
                        <td className="px-3 py-1.5 text-xs text-gray-500">{r.code}</td>
                        <td className="px-3 py-1.5 font-medium text-gray-900">{r.title}</td>
                        <td className="px-3 py-1.5">
                          {r.division_name}
                          {r.region ? <span className="text-gray-500"> · {r.region}</span> : null}
                        </td>
                        <td className="px-3 py-1.5 text-xs">{r.store_codes.join(" · ")}</td>
                        <td className="px-3 py-1.5 text-xs text-gray-600">{r.parent_title}</td>
                        <td className="px-3 py-1.5">
                          <Badge className={`px-1.5 py-0 text-xs ${st.color}`}>{st.label}</Badge>
                          {r.maybe_holders.length > 0 ? (
                            <p className="mt-0.5 text-xs text-gray-700 break-words">{r.maybe_holders.join(", ")}</p>
                          ) : null}
                        </td>
                      </tr>
                    );
                  })}
                </tbody>
              </table>
            </div>
          )}
        </>
      ) : null}
    </div>
  );
}
