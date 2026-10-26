"use client";

import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { AttendanceRequestsTab } from "@/features/hris/attendance-requests";
import { BusinessTripsTab } from "@/features/hris/business-trips";
import { useHrApproveAccess } from "@/features/hris/approve-access";
import { AttendanceGapSummaryCard } from "./attendance-gap-summary-card";
import { AttendanceMonitoringTab } from "./attendance-monitoring-tab";
import { AttendanceRecapTab } from "./attendance-recap-tab";
import { AttendancePeriodsTab } from "./attendance-periods-tab";

/**
 * Halaman Absensi HRD/super admin — lima tab:
 * - Monitoring: roster harian per shift (siapa sudah/belum absen, cuti,
 *   libur, tanpa jadwal) dari /api/hris/attendance/daily-roster.
 * - Rekap: tabel absensi per periode absensi (cut-off) + filter + export +
 *   kalender per-karyawan; HRD bisa menambah/mengoreksi absensi sebelum closing.
 * - Periode: pengaturan cut-off & closing, daftar periode, closing/buka ulang.
 * - Pengajuan Absensi: absen manual/koreksi dari ESS (EPIC-065), pemegang `approve` menyetujui/menolak.
 * - Perjalanan Dinas: pengajuan dinas karyawan (EPIC-062), pemegang `approve` bisa memutus dan mencatat.
 * Absen dilakukan karyawan via ESS (/dashboard/me/absensi) dengan selfie +
 * GPS — halaman ini murni monitoring & rekap utk HRD.
 */
export function AttendancePage() {
  // Tab keputusan HR (Pengajuan Absensi EPIC-065, Perjalanan Dinas EPIC-062) mengikuti aksi
  // `approve` pada daun Absensi dari server (EPIC-067 R14), bukan role. Gagal tutup: selama
  // memuat atau bila error, tab tidak tampil. Server tetap menolak aksi di luar hak itu.
  const { data: access } = useHrApproveAccess();
  const hrd = access?.attendance === true;
  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-xl sm:text-2xl font-bold text-gray-900">Absensi & Timesheet</h1>
        <p className="text-sm text-gray-500 mt-1">
          Monitoring kehadiran per shift dan rekap absensi karyawan
        </p>
      </div>

      <AttendanceGapSummaryCard />

      <Tabs defaultValue="monitoring" className="w-full flex-col">
        <TabsList className={`grid h-9 w-full max-w-3xl ${hrd ? "grid-cols-5" : "grid-cols-3"}`}>
          <TabsTrigger value="monitoring">Monitoring</TabsTrigger>
          <TabsTrigger value="rekap">Rekap</TabsTrigger>
          <TabsTrigger value="periode">Periode Absensi</TabsTrigger>
          {hrd && <TabsTrigger value="pengajuan">Pengajuan Absensi</TabsTrigger>}
          {hrd && <TabsTrigger value="dinas">Perjalanan Dinas</TabsTrigger>}
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
        {hrd && (
          <TabsContent value="pengajuan" className="mt-4">
            <AttendanceRequestsTab />
          </TabsContent>
        )}
        {hrd && (
          <TabsContent value="dinas" className="mt-4">
            <BusinessTripsTab />
          </TabsContent>
        )}
      </Tabs>
    </div>
  );
}
