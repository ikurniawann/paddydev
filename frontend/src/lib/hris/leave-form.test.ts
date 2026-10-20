import { describe, expect, it } from "vitest";
import { applySingleDay, leaveFormNote } from "./leave-form";
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
