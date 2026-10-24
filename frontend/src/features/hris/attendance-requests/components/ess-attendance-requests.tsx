"use client";

import { useState } from "react";
import { Button } from "@/components/ui/button";
import { todayWib } from "@/lib/dates";
import { AttendanceGapsCard } from "./attendance-gaps-card";
import { AttendanceRequestDialog, type AttendanceRequestPrefill } from "./attendance-request-dialog";
import { MyAttendanceRequests } from "./my-attendance-requests";

/** ESS → Absensi (EPIC-065): absen terlewat, pengajuan absen manual/koreksi, dan statusnya. */
export function EssAttendanceRequests() {
  const [prefill, setPrefill] = useState<AttendanceRequestPrefill | null>(null);
  return (
    <div className="min-w-0 space-y-3">
      <AttendanceGapsCard onRequest={setPrefill} />
      <div className="flex justify-end">
        <Button variant="outline" onClick={() => setPrefill({ date: todayWib() })}>
          Ajukan absen manual / koreksi
        </Button>
      </div>
      <MyAttendanceRequests />
      {prefill && <AttendanceRequestDialog prefill={prefill} onClose={() => setPrefill(null)} />}
    </div>
  );
}
