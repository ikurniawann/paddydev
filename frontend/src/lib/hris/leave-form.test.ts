import { describe, expect, it } from "vitest";
import { applySingleDay, effectiveLeaveType, leaveFormNote } from "./leave-form";
import type { LeaveQuota } from "@/features/hris/leaves/types";

const quota: LeaveQuota = {
  employee_id: "e", month: "2031-03", position_level: "Supervisor", leader_eligible: true,
  types: [
    { leave_type: "personal", label: "Izin Pribadi", quota: 3, used: 1, remaining: 2, available: true },
    { leave_type: "leader", label: "Izin Leader", quota: 1, used: 1, remaining: 0, available: true },
  ],
};

describe("leaveFormNote", () => {
  it("Izin Pribadi: sisa kuota + memotong 1 hari THP, tanpa angka rupiah", () => {
    const note = leaveFormNote("personal", quota)!;
    expect(note).toContain("Sisa kuota Izin Pribadi bulan 2031-03: 2 dari 3 hari");
    expect(note).toContain("Memotong 1 hari THP");
    expect(note).not.toMatch(/Rp/);
  });
  it("Izin Leader habis: tidak memotong gaji, kuota habis", () => {
    const note = leaveFormNote("leader", quota)!;
    expect(note).toContain("Sisa kuota Izin Leader bulan 2031-03: 0 dari 1 hari");
    expect(note).toContain("Tidak memotong gaji");
  });
  it("intern: Izin Pribadi tidak memotong THP (deducts_thp=false), kuota tetap", () => {
    const intern: LeaveQuota = {
      ...quota,
      employment_status: "intern",
      types: [{ ...quota.types[0], deducts_thp: false }, quota.types[1]],
    };
    const note = leaveFormNote("personal", intern)!;
    expect(note).toContain("Sisa kuota Izin Pribadi bulan 2031-03: 2 dari 3 hari");
    expect(note).toContain("Tidak memotong THP");
    expect(note).not.toContain("Memotong 1 hari THP");
  });
  it("non-intern dengan deducts_thp=true tetap memotong 1 hari THP", () => {
    const worker: LeaveQuota = { ...quota, types: [{ ...quota.types[0], deducts_thp: true }, quota.types[1]] };
    expect(leaveFormNote("personal", worker)).toContain("Memotong 1 hari THP");
  });
  it("jenis tanpa kuota: tidak ada catatan", () => {
    expect(leaveFormNote("annual", quota)).toBeNull();
    expect(leaveFormNote("personal", null)).toBeNull();
  });
});

describe("applySingleDay", () => {
  it("jenis satu hari menyamakan tanggal selesai dengan tanggal mulai", () => {
    expect(applySingleDay("personal", "2031-03-04", "2031-03-06")).toEqual({ start: "2031-03-04", end: "2031-03-04" });
    expect(applySingleDay("annual", "2031-03-04", "2031-03-06")).toEqual({ start: "2031-03-04", end: "2031-03-06" });
  });
});

describe("effectiveLeaveType", () => {
  const withLeader = [{ value: "annual" }, { value: "personal" }, { value: "leader" }];
  const withoutLeader = [{ value: "annual" }, { value: "personal" }];
  it("tetap bila jenis ada di opsi", () => {
    expect(effectiveLeaveType("leader", withLeader)).toBe("leader");
    expect(effectiveLeaveType("personal", withoutLeader)).toBe("personal");
  });
  it("jatuh ke annual bila jenis tidak ada di opsi (Izin Leader hilang)", () => {
    expect(effectiveLeaveType("leader", withoutLeader)).toBe("annual");
    expect(effectiveLeaveType("menstrual", withLeader)).toBe("annual");
  });
});
