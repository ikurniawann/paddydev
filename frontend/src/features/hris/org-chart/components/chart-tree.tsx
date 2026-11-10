"use client";

import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { ChevronDownIcon, ChevronRightIcon } from "@heroicons/react/24/outline";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent } from "@/components/ui/card";
import { fetchOrgChartTree } from "../api";
import { orgChartQueryKeys } from "../query-keys";
import type { ChartNode } from "../types";

const STATUS: Record<ChartNode["status"], { label: string; color: string }> = {
  active: { label: "Aktif", color: "bg-green-100 text-green-700" },
  planned: { label: "Rencana", color: "bg-blue-100 text-blue-700" },
  vacant: { label: "Kosong", color: "bg-gray-100 text-gray-600" },
  frozen: { label: "Dibekukan", color: "bg-gray-200 text-gray-700" },
};
const CONFIRM = { label: "Perlu konfirmasi", color: "bg-yellow-100 text-yellow-800" };

function holderText(n: ChartNode) {
  const names = n.holders.map((h) => h.name);
  if (names.length <= 3) return names.join(", ");
  return `${names.slice(0, 3).join(", ")}, dan ${names.length - 3} lainnya`;
}

function TreeItem({ node, kids }: { node: ChartNode; kids: Map<string, ChartNode[]> }) {
  const children = kids.get(node.code) ?? [];
  const [open, setOpen] = useState(true);
  const status = STATUS[node.status] ?? STATUS.active;
  return (
    <li role="treeitem" aria-expanded={children.length > 0 ? open : undefined} aria-selected={false} className="min-w-0">
      <div className="flex min-w-0 items-start gap-1.5 py-1">
        {children.length > 0 ? (
          <button
            type="button"
            className="mt-0.5 shrink-0 rounded p-0.5 text-gray-500 hover:bg-gray-100 focus-visible:outline focus-visible:outline-2 focus-visible:outline-blue-500"
            aria-label={`${open ? "Lipat" : "Buka"} ${node.title}`}
            onClick={() => setOpen((v) => !v)}
          >
            {open ? <ChevronDownIcon className="h-4 w-4" /> : <ChevronRightIcon className="h-4 w-4" />}
          </button>
        ) : (
          <span className="w-5 shrink-0" aria-hidden />
        )}
        <div className="min-w-0 flex-1 rounded-lg border border-gray-200 bg-white px-3 py-2 text-sm">
          <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
            <span className="font-semibold text-gray-900 break-words">{node.title}</span>
            <span className="text-xs text-gray-400">{node.code}</span>
            {node.job_level ? <span className="text-xs text-gray-500">{node.job_level}</span> : null}
            {node.is_position ? <Badge className={`px-1.5 py-0 text-xs ${status.color}`}>{status.label}</Badge> : null}
            {node.needs_confirm ? <Badge className={`px-1.5 py-0 text-xs ${CONFIRM.color}`}>{CONFIRM.label}</Badge> : null}
          </div>
          {node.store_codes.length > 0 || node.region ? (
            <p className="mt-0.5 flex flex-wrap gap-x-2 text-xs text-gray-500">
              {node.region ? <span>{node.region}</span> : null}
              {node.store_codes.length > 0 ? <span className="font-medium">{node.store_codes.join(" · ")}</span> : null}
            </p>
          ) : null}
          {node.holders.length > 0 ? <p className="mt-0.5 text-xs text-gray-700 break-words">{holderText(node)}</p> : null}
        </div>
      </div>
      {children.length > 0 && open ? (
        <ul role="group" className="ml-3 border-l border-gray-200 pl-2 sm:ml-5 sm:pl-3">
          {children.map((c) => (
            <TreeItem key={c.code} node={c} kids={kids} />
          ))}
        </ul>
      ) : null}
    </li>
  );
}

/**
 * Bagan Draft 6 (EPIC-101): pohon slot jabatan dari bagan klien, dengan
 * status, tanda perlu konfirmasi, toko, dan karyawan yang cocok. Hanya baca.
 */
export function ChartTree() {
  const [division, setDivision] = useState("");
  const [region, setRegion] = useState("");
  const all = useQuery({
    queryKey: orgChartQueryKeys.tree("", ""),
    queryFn: () => fetchOrgChartTree({ division: "", region: "" }),
  });
  const filtered = useQuery({
    queryKey: orgChartQueryKeys.tree(division, region),
    queryFn: () => fetchOrgChartTree({ division, region }),
  });
  const allNodes = all.data?.nodes ?? [];
  const divisions = [...new Map(allNodes.map((n) => [n.division_code, n.division_name])).entries()];
  const regions = [
    ...new Set(allNodes.filter((n) => !division || n.division_code === division).map((n) => n.region).filter(Boolean)),
  ] as string[];

  const nodes = filtered.data?.nodes ?? [];
  const codes = new Set(nodes.map((n) => n.code));
  const kids = new Map<string, ChartNode[]>();
  const roots: ChartNode[] = [];
  for (const n of nodes) {
    if (n.parent_code && codes.has(n.parent_code)) kids.set(n.parent_code, [...(kids.get(n.parent_code) ?? []), n]);
    else roots.push(n);
  }

  return (
    <div className="space-y-4">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-end">
        <label className="block text-sm">
          <span className="mb-1 block text-xs font-medium text-gray-600">Divisi</span>
          <select
            aria-label="Divisi"
            className="h-9 w-full rounded-md border border-gray-300 bg-white px-2 text-sm sm:w-56"
            value={division}
            onChange={(e) => {
              setDivision(e.target.value);
              setRegion("");
            }}
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
            className="h-9 w-full rounded-md border border-gray-300 bg-white px-2 text-sm sm:w-56"
            value={region}
            onChange={(e) => setRegion(e.target.value)}
          >
            <option value="">Semua region</option>
            {regions.map((r) => (
              <option key={r} value={r}>
                {r}
              </option>
            ))}
          </select>
        </label>
        {filtered.data ? <p className="text-xs text-gray-500 sm:pb-2">{filtered.data.version.label} · {nodes.length} posisi</p> : null}
      </div>

      <div aria-label="Legenda bagan" className="flex flex-wrap gap-2 text-xs">
        {[STATUS.active, STATUS.planned, STATUS.vacant, CONFIRM].map((s) => (
          <Badge key={s.label} className={s.color}>
            {s.label}
          </Badge>
        ))}
        <span className="text-gray-400">· Status &quot;Rencana&quot; dan &quot;Perlu konfirmasi&quot; menunggu jawaban klien</span>
      </div>

      {filtered.isError ? (
        <p role="alert" className="rounded-lg bg-danger-soft p-3 text-sm text-danger">
          Bagan tidak bisa dimuat
          {filtered.error instanceof Error && filtered.error.message ? ` — ${filtered.error.message}` : ""}
        </p>
      ) : filtered.isLoading ? (
        <div className="flex justify-center py-12">
          <div className="h-8 w-8 animate-spin rounded-full border-2 border-gray-300 border-t-blue-500" />
        </div>
      ) : nodes.length === 0 ? (
        <Card>
          <CardContent className="py-12 text-center text-sm text-gray-400">Tidak ada posisi untuk filter ini</CardContent>
        </Card>
      ) : (
        <ul role="tree" aria-label="Bagan organisasi" className="rounded-xl bg-gray-50 p-3 sm:p-4">
          {roots.map((n) => (
            <TreeItem key={n.code} node={n} kids={kids} />
          ))}
        </ul>
      )}
    </div>
  );
}
