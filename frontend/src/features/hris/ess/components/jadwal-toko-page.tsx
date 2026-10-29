"use client";

import { useCallback, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { CalendarDays, Loader2, Store } from "lucide-react";

import { Card, CardContent } from "@/components/ui/card";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { PlacementsPage } from "@/features/hris/placements/components/placements-page";
import { NOT_DELEGATE_MESSAGE, StoreForbiddenProvider } from "@/features/hris/schedule-access/access-guard";
import { useScheduleAccess } from "@/features/hris/schedule-access/queries";
import { SCHEDULE_ACCESS_KEY } from "@/features/hris/schedule-access/query-keys";
import { SchedulesPage } from "@/features/hris/schedules/components/schedules-page";

type Tab = "jadwal" | "penempatan";

/** Toko + karyawan yang dibawa dari "Atur jadwal" di tab Penempatan; `n` memasang ulang grid. */
interface ScheduleTarget {
  n: number;
  branchId: string;
  employeeIds: string[];
}

/**
 * Area Karyawan → Jadwal Toko (EPIC-071 R8): pengatur jadwal toko yang
 * ditunjuk HR mengatur jadwal kerja (dan, bila boleh, penempatan) karyawan
 * di toko wilayahnya. Memakai halaman Jadwal Kerja & Penempatan HR dalam
 * mode toko; server yang membatasi wilayahnya.
 */
export function EssJadwalTokoPage() {
  const qc = useQueryClient();
  const access = useScheduleAccess();
  const [tab, setTab] = useState<Tab>("jadwal");
  const [target, setTarget] = useState<ScheduleTarget>({ n: 0, branchId: "", employeeIds: [] });
  /** Saat API mode toko menjawab "bukan pengatur" (penunjukan dicabut / karyawan dinonaktifkan). */
  const [lostAt, setLostAt] = useState<number | null>(null);

  // 403 apa pun di mode toko: muat ulang akses (tab Penempatan hilang bila hak penempatan dicabut).
  const onForbidden = useCallback(
    (lost: boolean) => {
      if (lost) setLostAt(Date.now());
      void qc.invalidateQueries({ queryKey: SCHEDULE_ACCESS_KEY });
    },
    [qc]
  );

  function openSchedule(branchId: string, employeeIds: string[]) {
    setTarget((prev) => ({ n: prev.n + 1, branchId, employeeIds }));
    setTab("jadwal");
  }

  const data = access.data;
  // Kartu bukan pengatur sampai akses yang dimuat sesudah penolakan berkata lain (gagal muat = tetap kartu).
  const lost = lostAt !== null && access.dataUpdatedAt <= lostAt;
  return (
    <div className="space-y-5">
      <div>
        <h1 className="flex items-center gap-2 text-2xl font-bold text-gray-900">
          <CalendarDays className="h-6 w-6 text-brand-text" />
          Jadwal Toko
        </h1>
        <p className="mt-1 text-sm text-gray-500">
          Atur jadwal kerja karyawan di toko Anda. Perubahan langsung berlaku ke absensi mereka.
        </p>
      </div>

      {/* Gagal muat ulang di latar tidak boleh mengganti halaman yang sedang dipakai (suntingan grid hilang). */}
      {!data && access.isError ? (
        <Card>
          <CardContent className="p-6 text-sm text-red-600">
            {access.error instanceof Error && access.error.message ? access.error.message : "Gagal memuat akses Jadwal Toko"}
          </CardContent>
        </Card>
      ) : !data ? (
        <Card>
          <CardContent className="flex items-center gap-2 p-6 text-sm text-gray-500">
            <Loader2 className="h-4 w-4 animate-spin" /> Memuat…
          </CardContent>
        </Card>
      ) : !data.is_delegate || lost ? (
        <Card>
          <CardContent className="flex flex-col items-center gap-2 p-10 text-center">
            <Store className="h-8 w-8 text-gray-300" />
            <p className="max-w-md text-sm font-medium text-gray-700">{NOT_DELEGATE_MESSAGE}</p>
          </CardContent>
        </Card>
      ) : (
        <StoreForbiddenProvider value={onForbidden}>
          <Tabs value={tab} onValueChange={(value) => setTab(value as Tab)}>
            <TabsList>
              <TabsTrigger value="jadwal">Jadwal</TabsTrigger>
              {data.can_place ? <TabsTrigger value="penempatan">Penempatan</TabsTrigger> : null}
            </TabsList>
            <TabsContent value="jadwal" className="pt-2">
              <SchedulesPage
                key={target.n}
                mode="store"
                initialBranchId={target.branchId}
                initialEmployeeIds={target.employeeIds}
                onOpenPlacements={data.can_place ? () => setTab("penempatan") : undefined}
              />
            </TabsContent>
            {data.can_place ? (
              <TabsContent value="penempatan" className="pt-2">
                <PlacementsPage mode="store" onOpenSchedule={openSchedule} />
              </TabsContent>
            ) : null}
          </Tabs>
        </StoreForbiddenProvider>
      )}
    </div>
  );
}
