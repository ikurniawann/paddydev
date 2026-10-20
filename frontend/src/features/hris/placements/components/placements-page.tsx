"use client";

import { useState } from "react";
import Link from "next/link";
import { Building2, Loader2, MapPin, Search, Store, Users } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { PageHeader } from "@/components/ui/page-header";
import { StatCard } from "@/components/ui/stat-card";
import { Dialog, DialogContent, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { useIamAccess } from "@/components/iam/iam-access-provider";
import { todayWib } from "@/lib/dates";
import { placementStatus } from "@/lib/hris/placements";
import { usePlacementOverview, useStoreBranches } from "../queries";
import type { PlacementOverviewRow } from "../types";
import { BranchLocationDialog } from "./branch-location-dialog";
import { BulkPlacementDialog } from "./bulk-placement-dialog";
import { EmployeePlacementsTab, PLACEMENT_MANAGERS } from "./employee-placements-tab";
import { PlacementBadges } from "./placement-badges";

const ALL = "all";
const UNPLACED = "none";

/**
 * HRIS → Master Data → Penempatan Karyawan: siapa bekerja di toko mana.
 * Filter per toko, cari karyawan tanpa penempatan, dan tempatkan beberapa
 * karyawan sekaligus. Detail per karyawan sama dengan tab "Penempatan".
 */
export function PlacementsPage() {
  const canManage = useIamAccess().hasPrefix(PLACEMENT_MANAGERS);
  const [store, setStore] = useState(ALL);
  const [search, setSearch] = useState("");
  const [includeEnded, setIncludeEnded] = useState(false);
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [bulkOpen, setBulkOpen] = useState(false);
  const [locationOpen, setLocationOpen] = useState(false);
  const [managing, setManaging] = useState<PlacementOverviewRow | null>(null);
  const branches = useStoreBranches();
  const overview = usePlacementOverview({ include_ended: includeEnded });
  const today = todayWib();
  const all = overview.data ?? [];
  const missingCoords = (branches.data ?? []).filter((b) => b.lat === null || b.lng === null).length;

  const running = (row: PlacementOverviewRow) => row.placements.filter((p) => placementStatus(p, today) !== "berakhir");
  const stats = (() => {
    const perStore = new Map<string, number>();
    let unplaced = 0;
    let multi = 0;
    for (const row of all) {
      const active = row.placements.filter((p) => placementStatus(p, today) !== "berakhir");
      if (active.length === 0) unplaced++;
      if (new Set(active.map((p) => p.branch_id)).size > 1) multi++;
      for (const id of new Set(active.map((p) => p.branch_id))) perStore.set(id, (perStore.get(id) ?? 0) + 1);
    }
    return { perStore, unplaced, multi };
  })();

  const q = search.trim().toLowerCase();
  const rows = all.filter((row) => {
      if (q && !row.full_name.toLowerCase().includes(q) && !(row.nip ?? "").toLowerCase().includes(q)) return false;
      const active = row.placements.filter((p) => placementStatus(p, today) !== "berakhir");
      if (store === UNPLACED) return active.length === 0;
      if (store !== ALL) return active.some((p) => p.branch_id === store);
      return true;
  });

  const toggle = (id: string) =>
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  const allVisibleSelected = rows.length > 0 && rows.every((r) => selected.has(r.employee_id));

  return (
    <div className="space-y-5">
      <PageHeader
        kicker="HRIS · Master Data"
        title="Penempatan Karyawan"
        description="Toko tempat setiap karyawan bekerja. Dipakai untuk pilihan toko saat clock-in dan pembagian biaya gaji per toko."
        actions={
          canManage ? (
            <div className="flex flex-wrap gap-2">
              <Button variant="outline" onClick={() => setLocationOpen(true)}>
                <MapPin className="mr-1.5 h-4 w-4" /> Lokasi Toko
                {missingCoords > 0 ? ` (${missingCoords} belum diisi)` : ""}
              </Button>
              {selected.size > 0 && (
                <Button onClick={() => setBulkOpen(true)}>
                  <Store className="mr-1.5 h-4 w-4" /> Tempatkan {selected.size} karyawan…
                </Button>
              )}
            </div>
          ) : null
        }
      />

      <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
        {(branches.data ?? []).map((b) => (
          <StatCard
            key={b.id}
            label={b.name}
            value={stats.perStore.get(b.id) ?? 0}
            unit="karyawan"
            icon={<Building2 className="h-4 w-4" />}
          />
        ))}
        <StatCard label="Multi-toko" value={stats.multi} unit="karyawan" tone="info" icon={<Users className="h-4 w-4" />} />
        <StatCard
          label="Tanpa penempatan"
          value={stats.unplaced}
          unit="karyawan"
          tone={stats.unplaced > 0 ? "warning" : "success"}
          hint={stats.unplaced > 0 ? "Biaya gajinya masuk “Tanpa toko”" : undefined}
        />
      </div>

      <Card>
        <CardContent className="flex flex-wrap items-end gap-3 pt-6">
          <div className="w-full sm:w-56">
            <label className="mb-1 block text-xs font-medium text-gray-600">Toko</label>
            <Select value={store} onValueChange={setStore}>
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value={ALL}>Semua toko</SelectItem>
                {(branches.data ?? []).map((b) => (
                  <SelectItem key={b.id} value={b.id}>
                    {b.name}
                  </SelectItem>
                ))}
                <SelectItem value={UNPLACED}>Tanpa penempatan</SelectItem>
              </SelectContent>
            </Select>
          </div>
          <div className="w-full sm:w-72">
            <label className="mb-1 block text-xs font-medium text-gray-600">Cari karyawan</label>
            <div className="relative">
              <Search className="absolute left-2.5 top-2.5 h-4 w-4 text-gray-400" />
              <Input
                className="pl-8"
                placeholder="Nama atau NIP"
                value={search}
                onChange={(e) => setSearch(e.target.value)}
              />
            </div>
          </div>
          <label className="flex items-center gap-2 pb-2 text-sm text-gray-600">
            <input type="checkbox" checked={includeEnded} onChange={(e) => setIncludeEnded(e.target.checked)} />
            Tampilkan penempatan yang sudah berakhir
          </label>
        </CardContent>
      </Card>

      <Card>
        <CardContent className="p-0">
          {overview.isLoading ? (
            <div className="flex justify-center py-12">
              <Loader2 className="h-6 w-6 animate-spin text-gray-400" />
            </div>
          ) : overview.isError ? (
            <div className="py-12 text-center text-sm text-red-500">Gagal memuat penempatan karyawan</div>
          ) : rows.length === 0 ? (
            <div className="py-12 text-center text-sm text-gray-400">Tidak ada karyawan untuk filter ini.</div>
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b border-gray-100 bg-gray-50 text-left text-xs text-gray-500">
                    {canManage && (
                      <th className="w-10 p-3">
                        <input
                          type="checkbox"
                          aria-label="Pilih semua"
                          checked={allVisibleSelected}
                          onChange={() =>
                            setSelected((prev) => {
                              const next = new Set(prev);
                              for (const r of rows) {
                                if (allVisibleSelected) next.delete(r.employee_id);
                                else next.add(r.employee_id);
                              }
                              return next;
                            })
                          }
                        />
                      </th>
                    )}
                    <th className="p-3">Karyawan</th>
                    <th className="p-3">Departemen / Jabatan</th>
                    <th className="p-3">Toko</th>
                    <th className="p-3 text-right">Aksi</th>
                  </tr>
                </thead>
                <tbody>
                  {rows.map((row) => (
                    <tr key={row.employee_id} className="border-b border-gray-50 align-top last:border-0">
                      {canManage && (
                        <td className="p-3">
                          <input
                            type="checkbox"
                            aria-label={`Pilih ${row.full_name}`}
                            checked={selected.has(row.employee_id)}
                            onChange={() => toggle(row.employee_id)}
                          />
                        </td>
                      )}
                      <td className="p-3">
                        <Link
                          href={`/dashboard/employees/${row.employee_id}?tab=placements`}
                          className="font-medium text-gray-900 hover:underline"
                        >
                          {row.full_name}
                        </Link>
                        <p className="text-xs text-gray-400">{row.nip ?? "—"}</p>
                      </td>
                      <td className="p-3 text-gray-600">
                        <p>{row.department_name ?? "—"}</p>
                        <p className="text-xs text-gray-400">{row.position_title ?? ""}</p>
                      </td>
                      <td className="p-3">
                        <PlacementBadges placements={includeEnded ? row.placements : running(row)} />
                      </td>
                      <td className="p-3 text-right">
                        <Button size="sm" variant="outline" onClick={() => setManaging(row)}>
                          {canManage ? "Kelola" : "Lihat"}
                        </Button>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </CardContent>
      </Card>
      {!overview.isLoading && rows.length > 0 && (
        <p className="text-xs text-gray-500">
          {rows.length} karyawan ditampilkan{selected.size > 0 ? ` · ${selected.size} dipilih` : ""}
        </p>
      )}

      <BulkPlacementDialog
        open={bulkOpen}
        employeeIds={[...selected]}
        onOpenChange={setBulkOpen}
        onDone={() => {
          setBulkOpen(false);
          setSelected(new Set());
        }}
      />
      {canManage && (
        <BranchLocationDialog open={locationOpen} onOpenChange={setLocationOpen} branches={branches.data ?? []} />
      )}
      <Dialog open={managing !== null} onOpenChange={(open) => !open && setManaging(null)}>
        <DialogContent className="sm:max-w-3xl">
          <DialogHeader>
            <DialogTitle>{managing?.full_name}</DialogTitle>
          </DialogHeader>
          {managing && <EmployeePlacementsTab employeeId={managing.employee_id} employeeName={managing.full_name} />}
        </DialogContent>
      </Dialog>
    </div>
  );
}
