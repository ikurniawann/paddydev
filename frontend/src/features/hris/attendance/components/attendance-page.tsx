"use client";

import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { BusinessTripsTab } from "@/features/hris/business-trips";
import { useAuth } from "@/hooks/use-auth";
import { canManageTrips } from "@/lib/hris/business-trips";
import { AttendanceMonitoringTab } from "./attendance-monitoring-tab";
import { AttendanceRecapTab } from "./attendance-recap-tab";
import { AttendancePeriodsTab } from "./attendance-periods-tab";

/**
 * Halaman Absensi HRD/super admin — empat tab:
 * - Monitoring: roster harian per shift (siapa sudah/belum absen, cuti,
 *   libur, tanpa jadwal) dari /api/hris/attendance/daily-roster.
 * - Rekap: tabel absensi per periode absensi (cut-off) + filter + export +
 *   kalender per-karyawan; HRD bisa menambah/mengoreksi absensi sebelum closing.
 * - Periode: pengaturan cut-off & closing, daftar periode, closing/buka ulang.
 * - Perjalanan Dinas: pengajuan dinas karyawan (EPIC-062), HRD bisa memutus dan mencatat.
 * Absen dilakukan karyawan via ESS (/dashboard/me/absensi) dengan selfie +
 * GPS — halaman ini murni monitoring & rekap utk HRD.
 */
export function AttendancePage() {
  // Kuasa HR Perjalanan Dinas hanya super_admin/admin/hrd (bukan hiring_manager);
  // server tetap menolak aksi di luar itu.
  const { user } = useAuth();
  const showTrips = canManageTrips(user?.role);
  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-xl sm:text-2xl font-bold text-gray-900">Absensi & Timesheet</h1>
        <p className="text-sm text-gray-500 mt-1">
          Monitoring kehadiran per shift dan rekap absensi karyawan
        </p>
      </div>

      <Tabs defaultValue="monitoring" className="w-full flex-col">
        <TabsList className={`grid h-9 w-full max-w-xl ${showTrips ? "grid-cols-4" : "grid-cols-3"}`}>
          <TabsTrigger value="monitoring">Monitoring</TabsTrigger>
          <TabsTrigger value="rekap">Rekap</TabsTrigger>
          <TabsTrigger value="periode">Periode Absensi</TabsTrigger>
          {showTrips && <TabsTrigger value="dinas">Perjalanan Dinas</TabsTrigger>}
        </TabsList>
        <TabsContent value="monitoring" className="mt-4">
          <AttendanceMonitoringTab />
        </TabsContent>
        <TabsContent value="rekap" className="mt-4">
          <AttendanceRecapTab />
        </TabsContent>
        <TabsContent value="periode" className="mt-4">
          <AttendancePeriodsTab />
        </TabsContent>
        {showTrips && (
          <TabsContent value="dinas" className="mt-4">
            <BusinessTripsTab />
          </TabsContent>
        )}
      </Tabs>
    </div>
  );
}
