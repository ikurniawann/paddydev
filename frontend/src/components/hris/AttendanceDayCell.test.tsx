import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import type { CalendarAttendance } from "@/features/hris/attendance/types";
import { AttendanceDayCell } from "./AttendanceDayCell";
import { AttendanceDayDialog } from "./AttendanceDayDialog";

afterEach(cleanup);

const attendance = (isShort: boolean): CalendarAttendance => ({
  id: "a1", date: "2026-10-03", clock_in: "2026-10-03T02:00:00Z", clock_out: "2026-10-03T10:59:00Z",
  work_hours: 7.98, status: "present", is_late: false, is_short_hours: isShort, min_work_hours: isShort ? "9.00" : null,
});

describe("kalender ESS: label kurang jam (AB-05)", () => {
  it("sel kalender menandai baris kurang jam", () => {
    render(
      <AttendanceDayCell day={3} isToday={false} isSunday={false} attendance={attendance(true)} scheduledShift={null}
        isDayOff={false} holidays={[]} onClick={() => {}} />
    );
    expect(screen.getByText("Kurang jam").getAttribute("title")).toBe("Hadir kurang dari 9 jam (jam minimum toko)");
  });

  it("tanpa label bila tidak kurang jam", () => {
    render(
      <AttendanceDayCell day={3} isToday={false} isSunday={false} attendance={attendance(false)} scheduledShift={null}
        isDayOff={false} holidays={[]} onClick={() => {}} />
    );
    expect(screen.queryByText("Kurang jam")).toBeNull();
  });

  it("detail hari menuliskan keterangan lengkap", () => {
    render(
      <AttendanceDayDialog
        info={{ date: "2026-10-03", attendance: attendance(true), scheduledShift: null, isDayOff: false, holidays: [] }}
        hasSchedule={false}
        onClose={() => {}}
      />
    );
    expect(
      screen.getByText("Kurang jam / pulang lebih cepat · Hadir kurang dari 9 jam (jam minimum toko)")
    ).toBeTruthy();
  });
});
