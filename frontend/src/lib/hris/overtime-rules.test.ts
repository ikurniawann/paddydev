import { describe, it, expect } from "vitest";
import {
  overtimeAttachmentHref,
  overtimeTimeChanges,
  canDecideOvertime,
  decidedAs,
  OVERTIME_DAY_TYPE_OPTIONS,
  OVERTIME_TOO_LATE,
  overtimeDayTypeLabel,
  overtimeStartsInPast,
  type OvertimeDecisionActor,
  type OvertimeDecisionRequest,
} from "./overtime-rules";

const EMPLOYEE = "emp-1";
const MANAGER = "mgr-1";
const HR = "hr-1";
const OTHER = "emp-2";

function employeeRequest(
  overrides: Partial<OvertimeDecisionRequest> = {}
): OvertimeDecisionRequest {
  return {
    employee_id: EMPLOYEE,
    requested_by: EMPLOYEE,
    source: "employee",
    reporting_to: MANAGER,
    ...overrides,
  };
}

function companyAssignment(
  overrides: Partial<OvertimeDecisionRequest> = {}
): OvertimeDecisionRequest {
  return {
    employee_id: EMPLOYEE,
    requested_by: HR,
    source: "company",
    reporting_to: MANAGER,
    ...overrides,
  };
}

const hrActor: OvertimeDecisionActor = { employeeId: HR, isHr: true };
const managerActor: OvertimeDecisionActor = { employeeId: MANAGER, isHr: false };
const selfActor: OvertimeDecisionActor = { employeeId: EMPLOYEE, isHr: false };
const otherActor: OvertimeDecisionActor = { employeeId: OTHER, isHr: false };

describe("canDecideOvertime — pengajuan karyawan", () => {
  it("HR dapat approve/reject pengajuan orang lain", () => {
    expect(canDecideOvertime(hrActor, employeeRequest(), "approve").allowed).toBe(true);
    expect(canDecideOvertime(hrActor, employeeRequest(), "reject").allowed).toBe(true);
  });

  it("atasan langsung dapat approve", () => {
    expect(canDecideOvertime(managerActor, employeeRequest(), "approve").allowed).toBe(true);
  });

  it("rekan biasa TIDAK dapat memutuskan", () => {
    const result = canDecideOvertime(otherActor, employeeRequest(), "approve");
    expect(result.allowed).toBe(false);
  });

  it("pengaju TIDAK bisa approve pengajuannya sendiri — bahkan bila HR (regression)", () => {
    const hrSelf: OvertimeDecisionActor = { employeeId: EMPLOYEE, isHr: true };
    const result = canDecideOvertime(
      hrSelf,
      employeeRequest({ requested_by: EMPLOYEE }),
      "approve"
    );
    expect(result.allowed).toBe(false);
    expect(result.reason).toMatch(/sendiri/);
  });

  it("pengaju dapat membatalkan pengajuannya yang pending", () => {
    expect(canDecideOvertime(selfActor, employeeRequest(), "cancel").allowed).toBe(true);
  });

  it("orang lain tidak bisa membatalkan pengajuan karyawan", () => {
    expect(canDecideOvertime(otherActor, employeeRequest(), "cancel").allowed).toBe(false);
  });
});

describe("canDecideOvertime — penugasan perusahaan", () => {
  it("hanya karyawan yang ditugaskan yang bisa konfirmasi/menolak", () => {
    expect(canDecideOvertime(selfActor, companyAssignment(), "approve").allowed).toBe(true);
    expect(canDecideOvertime(selfActor, companyAssignment(), "reject").allowed).toBe(true);
  });

  it("HR pembuat TIDAK bisa mengonfirmasi atas nama karyawan", () => {
    expect(canDecideOvertime(hrActor, companyAssignment(), "approve").allowed).toBe(false);
  });

  it("HR dapat membatalkan penugasan yang belum dikonfirmasi", () => {
    const otherHr: OvertimeDecisionActor = { employeeId: "hr-2", isHr: true };
    expect(canDecideOvertime(otherHr, companyAssignment(), "cancel").allowed).toBe(true);
  });
});

describe("canDecideOvertime — akun tanpa record karyawan", () => {
  it("HR tanpa employeeId tetap bisa memutuskan pengajuan karyawan", () => {
    const hrNoEmployee: OvertimeDecisionActor = { employeeId: null, isHr: true };
    expect(canDecideOvertime(hrNoEmployee, employeeRequest(), "approve").allowed).toBe(true);
  });

  it("non-HR tanpa employeeId tidak bisa apa-apa", () => {
    const anon: OvertimeDecisionActor = { employeeId: null, isHr: false };
    expect(canDecideOvertime(anon, employeeRequest(), "approve").allowed).toBe(false);
    expect(canDecideOvertime(anon, employeeRequest(), "cancel").allowed).toBe(false);
    expect(canDecideOvertime(anon, companyAssignment(), "approve").allowed).toBe(false);
  });
});

describe("canDecideOvertime — pesan penolakan selaras Go (EPIC-063)", () => {
  it("bukan atasan/HRD → pesan yang sama dengan Go domain.CanDecideOvertime", () => {
    expect(canDecideOvertime(otherActor, employeeRequest(), "approve")).toEqual({
      allowed: false,
      reason: "Hanya atasan langsung atau HRD yang bisa memproses pengajuan ini",
    });
  });
});

describe("overtimeStartsInPast — MOM AP-02 (EPIC-063 R3)", () => {
  const now = new Date("2026-10-05T03:00:30Z"); // Senin 10:00:30 WIB

  it("menit berjalan masih boleh, sebelumnya sudah lewat", () => {
    expect(overtimeStartsInPast("2026-10-05", "10:00", now)).toBe(false);
    expect(overtimeStartsInPast("2026-10-05", "09:59", now)).toBe(true);
    expect(overtimeStartsInPast("2026-10-04", "23:00", now)).toBe(true);
    expect(overtimeStartsInPast("2026-10-06", "00:00", now)).toBe(false);
  });

  it("memakai jam WIB, bukan tanggal UTC", () => {
    const late = new Date("2026-10-05T16:45:00Z"); // 23:45 WIB
    expect(overtimeStartsInPast("2026-10-06", "00:15", late)).toBe(false);
    expect(overtimeStartsInPast("2026-10-05", "23:30", late)).toBe(true);
  });

  it("format tidak dikenal diserahkan ke validasi input", () => {
    expect(overtimeStartsInPast("05-10-2026", "10:00", now)).toBe(false);
    expect(overtimeStartsInPast("2026-02-30", "10:00", now)).toBe(false); // bukan tanggal kalender
    expect(overtimeStartsInPast("2026-10-05", "25:00", now)).toBe(false);
  });

  it("pesan penolakan sama dengan Go", () => {
    expect(OVERTIME_TOO_LATE).toBe("Lembur wajib diajukan sebelum dimulai — jam mulai sudah lewat");
  });
});

describe("decidedAs — Q-APV-3 (EPIC-063 R1/R2)", () => {
  it("atasan langsung lebih dulu, HR sebagai cadangan", () => {
    expect(decidedAs(managerActor, EMPLOYEE, MANAGER)).toBe("manager");
    expect(decidedAs(hrActor, EMPLOYEE, MANAGER)).toBe("hr");
    expect(decidedAs({ employeeId: null, isHr: true }, EMPLOYEE, null)).toBe("hr");
    expect(decidedAs(otherActor, EMPLOYEE, MANAGER)).toBeNull();
  });

  it("tidak ada yang memutus pengajuannya sendiri", () => {
    expect(decidedAs({ employeeId: EMPLOYEE, isHr: true }, EMPLOYEE, MANAGER)).toBeNull();
    expect(decidedAs(selfActor, EMPLOYEE, EMPLOYEE)).toBeNull();
  });

  it("atasan sekaligus HR → manager", () => {
    expect(decidedAs({ employeeId: MANAGER, isHr: true }, EMPLOYEE, MANAGER)).toBe("manager");
  });
});

describe("overtimeDayTypeLabel — K-13", () => {
  it("Weekday / Weekend-Libur", () => {
    expect(overtimeDayTypeLabel("regular")).toBe("Weekday");
    expect(overtimeDayTypeLabel("rest_day")).toBe("Weekend / Libur");
    expect(OVERTIME_DAY_TYPE_OPTIONS).toEqual([
      { value: "regular", label: "Weekday" },
      { value: "rest_day", label: "Weekend / Libur" },
    ]);
  });
});

describe("overtimeAttachmentHref & overtimeTimeChanges (EPIC-066)", () => {
  it("meng-encode tiap segmen path", () => {
    expect(overtimeAttachmentHref("overtime-attachments/e1/a b.pdf")).toBe(
      "/api/hris/overtime/attachment/overtime-attachments/e1/a%20b.pdf");
  });
  const stored = { start_time: "17:00:00", end_time: "21:00:00", break_minutes: 0 };
  it("tanpa perubahan -> undefined", () => {
    expect(overtimeTimeChanges(stored, { start_time: "17:00", end_time: "21:00", break_minutes: 0 })).toBeUndefined();
  });
  it("hanya kolom yang berbeda", () => {
    expect(overtimeTimeChanges(stored, { start_time: "17:00", end_time: "20:00", break_minutes: 0 })).toEqual({ end_time: "20:00" });
    expect(overtimeTimeChanges(stored, { start_time: "18:00", end_time: "21:00", break_minutes: 30 })).toEqual({ start_time: "18:00", break_minutes: 30 });
  });
  it("istirahat tersimpan string/null dibandingkan sebagai angka", () => {
    expect(overtimeTimeChanges({ ...stored, break_minutes: "30" as unknown as number }, { start_time: "17:00", end_time: "21:00", break_minutes: 30 })).toBeUndefined();
    expect(overtimeTimeChanges({ ...stored, break_minutes: undefined }, { start_time: "17:00", end_time: "21:00", break_minutes: 0 })).toBeUndefined();
  });
});
