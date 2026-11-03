import { describe, expect, it } from "vitest";
import {
  applySingleDay,
  earliestStartDate,
  effectiveLeaveType,
  leaveAttachmentError,
  leaveFormNote,
  leaveRuleHint,
} from "./leave-form";
import { leaveTypeInfo } from "./leave-types";
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

describe("aturan TnC 03-11 di form (EPIC-084)", () => {
  const info = (code: string) => leaveTypeInfo(code)!;

  it("hint per jenis", () => {
    expect(leaveRuleHint(info("annual"))).toBe("Diajukan paling lambat H-3");
    expect(leaveRuleHint(info("pilgrimage"))).toBe(
      "Diajukan paling lambat H-30 · maksimal 40 hari kalender · lewat 14 hari tidak dibayar bila masa kerja < 1 tahun"
    );
    expect(leaveRuleHint(info("sick"))).toBe("Diajukan pada hari sakit");
    expect(leaveRuleHint(info("leader"))).toBe("1x per bulan, hangus bila tidak dipakai, tanpa potong gaji");
    expect(leaveRuleHint(info("birthday"))).toBe(
      "Diajukan paling lambat H-3 · 1 hari dalam bulan ulang tahun · wajib lampiran (mis. foto KTP)"
    );
    expect(leaveRuleHint(info("marriage"))).toBe("Diajukan paling lambat H-3 · maksimal 3 hari kerja");
    expect(leaveRuleHint(info("miscarriage"))).toBe(
      "Maksimal 45 hari kalender · upah penuh bila masa kerja ≥ 1 tahun · wajib lampiran (surat dokter)"
    );
    expect(leaveRuleHint(info("personal"))).toBe("Hanya bila belum punya hak cuti atau cuti tahunan habis");
  });

  it("form HR untuk karyawan lain tanpa aturan H-n / hari H", () => {
    expect(leaveRuleHint(info("annual"), { forOthers: true })).toBeNull();
    expect(leaveRuleHint(info("sick"), { forOthers: true })).toBeNull();
    expect(leaveRuleHint(info("marriage"), { forOthers: true })).toBe("Maksimal 3 hari kerja");
  });

  it("tanggal mulai paling awal", () => {
    expect(earliestStartDate(info("annual"), "2026-11-03")).toBe("2026-11-06");
    expect(earliestStartDate(info("pilgrimage"), "2026-11-03")).toBe("2026-12-03");
    expect(earliestStartDate(info("sick"), "2026-11-03")).toBe("2026-11-03");
    expect(earliestStartDate(info("bereavement"), "2026-11-03")).toBeNull();
  });

  it("lampiran wajib", () => {
    expect(leaveAttachmentError("birthday", false)).toBe("Lampiran wajib untuk Birthday Leave (Cuti Ulang Tahun)");
    expect(leaveAttachmentError("birthday", true)).toBeNull();
    expect(leaveAttachmentError("annual", false)).toBeNull();
  });
});
