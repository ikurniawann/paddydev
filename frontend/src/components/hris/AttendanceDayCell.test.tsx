import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import type { CalendarAttendance } from "@/features/hris/attendance/types";
import type { HolidayRow } from "@/lib/hris/holidays";
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

// EPIC-080 R4/R6: penanda checkout otomatis dan jadwal dua shift.
describe("kalender: checkout otomatis dan dua shift (EPIC-080)", () => {
  const auto = (v: boolean): CalendarAttendance => ({ ...attendance(false), clock_out_auto: v });
  const twoShift = {
    day_of_week: 1, shift_id: "s1", effective_from: "2026-01-01", effective_to: null, shift_name: "PAGI4 + SORE4",
    start_time: "08:00:00", end_time: "21:00:00", is_overnight: false, time_label: "08.00–12.00, 17.00–21.00",
  };

  it("sel kalender menandai Checkout otomatis", () => {
    render(
      <AttendanceDayCell day={3} isToday={false} isSunday={false} attendance={auto(true)} scheduledShift={null}
        isDayOff={false} holidays={[]} onClick={() => {}} />
    );
    expect(screen.getByText("Checkout otomatis")).toBeTruthy();
    cleanup();
    render(
      <AttendanceDayCell day={3} isToday={false} isSunday={false} attendance={auto(false)} scheduledShift={null}
        isDayOff={false} holidays={[]} onClick={() => {}} />
    );
    expect(screen.queryByText("Checkout otomatis")).toBeNull();
  });

  it("detail hari menjelaskan Checkout otomatis", () => {
    render(
      <AttendanceDayDialog
        info={{ date: "2026-10-03", attendance: auto(true), scheduledShift: null, isDayOff: false, holidays: [] }}
        hasSchedule={false}
        onClose={() => {}}
      />
    );
    expect(screen.getByText(/Checkout otomatis · Jam pulang diisi sistem/)).toBeTruthy();
  });

  it("sel kalender dua shift menampilkan kedua rentang jam", () => {
    render(
      <AttendanceDayCell day={5} isToday={false} isSunday={false} scheduledShift={twoShift}
        isDayOff={false} holidays={[]} onClick={() => {}} />
    );
    expect(screen.getByText("PAGI4 + SORE4")).toBeTruthy();
    expect(screen.getByText("08.00–12.00, 17.00–21.00")).toBeTruthy();
  });

  it("detail hari dua shift menulis nama dan kedua rentang jam", () => {
    render(
      <AttendanceDayDialog
        info={{ date: "2026-10-05", scheduledShift: twoShift, isDayOff: false, holidays: [] }}
        hasSchedule
        onClose={() => {}}
      />
    );
    expect(screen.getByText("PAGI4 + SORE4 · 08.00–12.00, 17.00–21.00")).toBeTruthy();
  });
});

// EPIC-079 R1-R3: tanggal merah hanya meliburkan Non-Shift (cuti bersama hanya bila ditetapkan libur).
const natal: HolidayRow = { holiday_date: "2026-12-25", name: "Hari Natal", type: "nasional", deducts_leave: false };
const cutiBersama = (isDayOff: boolean): HolidayRow => ({
  holiday_date: "2026-12-24", name: "Cuti Bersama Natal", type: "cuti_bersama", deducts_leave: true, is_day_off: isDayOff,
});

function cell(holidays: HolidayRow[], scheduleType: "shift" | "non_shift") {
  return render(
    <AttendanceDayCell day={25} isToday={false} isSunday={false} scheduledShift={null} isDayOff={false}
      holidays={holidays} scheduleType={scheduleType} onClick={() => {}} />
  );
}

const dialog = (holidays: HolidayRow[], scheduleType: "shift" | "non_shift") =>
  render(
    <AttendanceDayDialog
      info={{ date: "2026-12-25", scheduledShift: null, isDayOff: false, holidays, scheduleType }}
      hasSchedule={false}
      onClose={() => {}}
    />
  );

describe("kalender: tanggal merah mengikuti tipe jadwal (EPIC-079)", () => {
  it("sel Non-Shift merah di libur nasional", () => {
    const { container } = cell([natal], "non_shift");
    expect(container.firstElementChild?.className).toContain("bg-red-50");
    expect(screen.getByText("Hari Natal").className).toContain("text-red-700");
  });

  it("sel staf shift tidak merah, nama libur hanya label netral", () => {
    const { container } = cell([natal], "shift");
    expect(container.firstElementChild?.className).not.toContain("bg-red-50");
    expect(screen.getByText("Hari Natal").className).not.toContain("red");
  });

  it("sel Non-Shift: cuti bersama biasa tidak merah, yang ditetapkan libur merah", () => {
    const biasa = cell([cutiBersama(false)], "non_shift");
    expect(biasa.container.firstElementChild?.className).not.toContain("bg-red-50");
    cleanup();
    const libur = cell([cutiBersama(true)], "non_shift");
    expect(libur.container.firstElementChild?.className).toContain("bg-red-50");
  });

  it("detail hari Non-Shift: kotak Hari Libur, cuti bersama libur memotong jatah", () => {
    dialog([natal, cutiBersama(true)], "non_shift");
    expect(screen.getByText("Hari Libur")).toBeTruthy();
    expect(screen.getByText("Hari Natal")).toBeTruthy();
    expect(screen.getByText("(memotong jatah cuti)")).toBeTruthy();
  });

  it("detail hari staf shift: tanpa kotak Hari Libur dan tanpa 'memotong jatah cuti'", () => {
    dialog([natal, cutiBersama(true)], "shift");
    expect(screen.queryByText("Hari Libur")).toBeNull();
    expect(screen.queryByText(/memotong jatah cuti/)).toBeNull();
    expect(screen.getByText("Tanggal Merah")).toBeTruthy();
    expect(screen.getByText("Hari Natal")).toBeTruthy();
  });

  it("detail hari Non-Shift: cuti bersama biasa hanya label, tanpa 'memotong jatah cuti'", () => {
    dialog([cutiBersama(false)], "non_shift");
    expect(screen.queryByText("Hari Libur")).toBeNull();
    expect(screen.queryByText(/memotong jatah cuti/)).toBeNull();
    expect(screen.getByText("Cuti Bersama Natal")).toBeTruthy();
  });
});
