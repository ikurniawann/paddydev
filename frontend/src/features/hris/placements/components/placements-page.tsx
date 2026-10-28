"use client";

import { useRef, useState } from "react";
import { useRouter } from "next/navigation";
import { ChevronLeft, ChevronRight, Loader2, MapPin } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { PageHeader } from "@/components/ui/page-header";
import { Dialog, DialogContent, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { useIamAccess } from "@/components/iam/iam-access-provider";
import { useDebouncedValue } from "@/hooks/use-debounced-value";
import { todayWib } from "@/lib/dates";
import { fetchPlacementIds } from "../api";
import { usePlacementSearch, useStoreBranches } from "../queries";
import type { PlacementSearchFilter, PlacementSearchRow, PlacementStatusFilter } from "../types";
import { BranchLocationDialog } from "./branch-location-dialog";
import { BulkActionBar } from "./bulk-action-bar";
import { BulkPlacementDialog, type BulkPlacementAction } from "./bulk-placement-dialog";
import { EmployeePlacementsTab, PLACEMENT_MANAGERS } from "./employee-placements-tab";
import { PlacementFilters, PlacementStatusChips, type PlacementFilterValues } from "./placement-filters";
import { mainPlacement, PlacementsTable } from "./placements-table";

const PAGE_SIZES = [25, 50, 100] as const;
type PageSize = (typeof PAGE_SIZES)[number];
/** Batas karyawan yang dibawa ke Jadwal Kerja lewat URL. */
const SCHEDULE_LINK_LIMIT = 50;

/**
 * HRIS → Master Data → Penempatan Karyawan (EPIC-070): siapa bekerja di toko
 * mana. Chip "yang perlu diurus", filter, halaman dari server, dan aksi massal
 * (pindah/tambah/akhiri dengan pratinjau, atur jadwal). Pilihan bertahan
 * lintas halaman dan filter sampai dihapus.
 */
export function PlacementsPage() {
  const router = useRouter();
  const canManage = useIamAccess().hasPrefix(PLACEMENT_MANAGERS);
  const [status, setStatus] = useState<PlacementStatusFilter>("");
  const [filters, setFilters] = useState<PlacementFilterValues>({ search: "", branchId: "", departmentId: "", positionId: "" });
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState<PageSize>(25);
  const [selected, setSelected] = useState<Set<string>>(() => new Set());
  const [selectingAll, setSelectingAll] = useState(false);
  const [action, setAction] = useState<BulkPlacementAction | null>(null);
  const [locationOpen, setLocationOpen] = useState(false);
  const [managing, setManaging] = useState<PlacementSearchRow | null>(null);
  // Toko utama karyawan yang pernah dicentang, untuk tujuan "Atur jadwal".
  const branchOf = useRef(new Map<string, string>());
  const search = useDebouncedValue(filters.search.trim(), 300);
  const today = todayWib();
  const branches = useStoreBranches();
  const missingCoords = (branches.data ?? []).filter((b) => b.lat === null || b.lng === null).length;

  const baseFilter: PlacementSearchFilter = {
    q: search || undefined,
    branch_id: filters.branchId || undefined,
    department_id: filters.departmentId || undefined,
    position_id: filters.positionId || undefined,
    status: status || undefined,
  };
  const result = usePlacementSearch({ ...baseFilter, page, page_size: pageSize });
  const data = result.data;
  const rows = data?.rows ?? [];
  const total = data?.total ?? 0;
  const shownPage = data?.page ?? page;
  const shownSize = data?.page_size ?? pageSize;
  const from = total === 0 ? 0 : (shownPage - 1) * shownSize + 1;
  const to = Math.min(shownPage * shownSize, total);
  const lastPage = Math.max(1, Math.ceil(total / shownSize));

  const changeFilters = (patch: Partial<PlacementFilterValues>) => {
    setFilters((prev) => ({ ...prev, ...patch }));
    setPage(1);
  };
  const changeStatus = (next: PlacementStatusFilter) => {
    setStatus(next);
    setPage(1);
  };

  const remember = (row: PlacementSearchRow) => {
    const main = mainPlacement(row, today);
    if (main) branchOf.current.set(row.employee_id, main.branch_id);
  };
  const toggle = (row: PlacementSearchRow) => {
    remember(row);
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(row.employee_id)) next.delete(row.employee_id);
      else next.add(row.employee_id);
      return next;
    });
  };
  const togglePage = () => {
    const all = rows.every((r) => selected.has(r.employee_id));
    rows.forEach(remember);
    setSelected((prev) => {
      const next = new Set(prev);
      for (const r of rows) {
        if (all) next.delete(r.employee_id);
        else next.add(r.employee_id);
      }
      return next;
    });
  };

  async function selectAllFiltered() {
    setSelectingAll(true);
    try {
      const res = await fetchPlacementIds(baseFilter);
      setSelected(new Set(res.ids));
      if (res.truncated) toast.info(`Hanya ${res.ids.length} karyawan pertama yang dipilih`);
    } catch (error) {
      toast.error(error instanceof Error && error.message ? error.message : "Gagal memilih semua hasil filter");
    } finally {
      setSelectingAll(false);
    }
  }

  function openSchedules() {
    const ids = [...selected];
    const tally = new Map<string, number>();
    for (const id of ids) {
      const branch = branchOf.current.get(id);
      if (branch) tally.set(branch, (tally.get(branch) ?? 0) + 1);
    }
    let branch = filters.branchId;
    let best = 0;
    for (const [id, n] of tally) {
      if (n > best) [branch, best] = [id, n];
    }
    if (ids.length > SCHEDULE_LINK_LIMIT) {
      toast.info(`Jadwal Kerja dibuka untuk ${SCHEDULE_LINK_LIMIT} karyawan pertama yang dipilih`);
    }
    const params = [branch ? `branch_id=${encodeURIComponent(branch)}` : "", `employees=${ids.slice(0, SCHEDULE_LINK_LIMIT).join(",")}`];
    router.push(`/dashboard/hris/schedules?${params.filter(Boolean).join("&")}`);
  }

  return (
    <div className="space-y-5">
      <PageHeader
        kicker="HRIS · Master Data"
        title="Penempatan Karyawan"
        description="Toko tempat setiap karyawan bekerja. Dipakai untuk pilihan toko saat clock-in, jadwal kerja, dan pembagian biaya gaji per toko."
        actions={
          canManage ? (
            <Button variant="outline" onClick={() => setLocationOpen(true)}>
              <MapPin className="mr-1.5 h-4 w-4" /> Lokasi Toko
              {missingCoords > 0 ? ` (${missingCoords} belum diisi)` : ""}
            </Button>
          ) : null
        }
      />

      <Card>
        <CardContent className="space-y-4 pt-6">
          <PlacementStatusChips status={status} counts={data?.counts} onChange={changeStatus} />
          <PlacementFilters values={filters} onChange={changeFilters} />
        </CardContent>
      </Card>

      <Card>
        <CardContent className="p-0">
          {result.isLoading ? (
            <div className="flex justify-center py-12">
              <Loader2 className="h-6 w-6 animate-spin text-gray-400" aria-label="Memuat" />
            </div>
          ) : result.isError ? (
            <div className="py-12 text-center text-sm text-red-500">Gagal memuat penempatan karyawan</div>
          ) : rows.length === 0 ? (
            <div className="py-12 text-center text-sm text-gray-400">Tidak ada karyawan untuk filter ini.</div>
          ) : (
            <PlacementsTable
              rows={rows}
              today={today}
              canManage={canManage}
              selected={selected}
              onToggle={toggle}
              onTogglePage={togglePage}
              onManage={setManaging}
            />
          )}
          {total > 0 && (
            <div className="flex flex-wrap items-center justify-between gap-3 border-t border-gray-100 px-4 py-3 text-sm text-gray-600">
              <span>
                {from}–{to} dari {total}
              </span>
              <div className="flex items-center gap-3">
                <label className="flex items-center gap-2">
                  <span className="text-xs text-gray-500">Baris per halaman</span>
                  <select
                    aria-label="Baris per halaman"
                    className="h-8 rounded-md border border-gray-200 bg-white px-2 text-sm"
                    value={pageSize}
                    onChange={(e) => {
                      setPageSize(Number(e.target.value) as PageSize);
                      setPage(1);
                    }}
                  >
                    {PAGE_SIZES.map((n) => (
                      <option key={n} value={n}>
                        {n}
                      </option>
                    ))}
                  </select>
                </label>
                <span className="text-xs text-gray-500">
                  Halaman {shownPage} dari {lastPage}
                </span>
                <Button
                  size="icon-sm"
                  variant="outline"
                  aria-label="Halaman sebelumnya"
                  disabled={page <= 1}
                  onClick={() => setPage((p) => Math.max(1, p - 1))}
                >
                  <ChevronLeft />
                </Button>
                <Button
                  size="icon-sm"
                  variant="outline"
                  aria-label="Halaman berikutnya"
                  disabled={page >= lastPage}
                  onClick={() => setPage((p) => p + 1)}
                >
                  <ChevronRight />
                </Button>
              </div>
            </div>
          )}
        </CardContent>
      </Card>

      {canManage && selected.size > 0 && (
        <BulkActionBar
          count={selected.size}
          total={total}
          selectingAll={selectingAll}
          onSelectAll={selectAllFiltered}
          onClear={() => setSelected(new Set())}
          onAction={setAction}
          onSchedule={openSchedules}
        />
      )}

      <BulkPlacementDialog
        open={action !== null}
        mode={action ?? "move"}
        employeeIds={[...selected]}
        onOpenChange={(open) => !open && setAction(null)}
        onDone={() => {
          setAction(null);
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
