"use client";

import { CalendarDaysIcon } from "@heroicons/react/24/outline";
import { AttendanceCalendar } from "@/components/hris/AttendanceCalendar";
import { EssAttendanceRequests } from "@/features/hris/attendance-requests";
import { TherapistAttendanceDay, TherapistAttendanceLegend } from "@/features/spa/components/therapist-attendance-day";
import { useEssMe } from "../queries";
import { EssClockPanel } from "./ess-clock-panel";
import { EssLoading, EssNotLinked } from "./ess-states";

/**
 * ESS → Absensi (/dashboard/me/absensi): shift hari ini, clock-in/out
 * dengan selfie + GPS, absen terlewat & pengajuan absen manual/koreksi
 * (EPIC-065), dan kalender absensi milik sendiri.
 */

export function EssAbsensiPage() {
  const { data: me, isLoading } = useEssMe();

  if (isLoading) return <EssLoading />;
  if (!me?.employee) return <EssNotLinked feature="Absensi" />;

  const shift = me.today_shift;

  return (
    <div className="space-y-6">
      <div className="rounded-2xl border border-gray-200/70 bg-gradient-to-r from-green-50 to-white p-6 shadow-sm">
        <div className="flex flex-col gap-1">
          <h1 className="text-2xl font-bold text-gray-900">Absensi</h1>
          <p className="flex items-center gap-1.5 text-sm text-gray-600">
            <CalendarDaysIcon className="h-4 w-4 text-green-600" />
            {shift ? (
              <>
                Shift hari ini: <b>{shift.name}</b> · {shift.start_time?.slice(0, 5)}–
                {shift.end_time?.slice(0, 5)} · toleransi {shift.late_tolerance_minutes} mnt
              </>
            ) : me.has_schedule ? (
              <>Hari ini jadwal Anda <b>libur</b> — absen tetap bisa, tercatat di luar jadwal.</>
            ) : (
              <>Jadwal shift Anda belum diatur HRD — absen tercatat tanpa penilaian terlambat.</>
            )}
          </p>
        </div>
        <div className="mt-4">
          <EssClockPanel />
        </div>
      </div>

      {/* EPIC-065 (MOM AB-04/AB-06): absen terlewat + pengajuan absen manual/koreksi ke HR. */}
      <EssAttendanceRequests />

      {/* Terapis spa: hari dengan jadwal treatment ditandai "Spa" (shift tetap shift). */}
      <AttendanceCalendar employeeId="me" DayExtra={TherapistAttendanceDay} legendExtra={<TherapistAttendanceLegend />} />
    </div>
  );
}
