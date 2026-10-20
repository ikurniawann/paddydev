import { describe, expect, it } from "vitest";
import {
  LEAVE_TYPES,
  SELECTABLE_LEAVE_CODES,
  leaveTypeExportLabel,
  leaveTypeInfo,
  leaveTypeLabel,
  leaveTypeOptions,
  leaveTypeShort,
} from "./leave-types";

describe("katalog jenis cuti (EPIC-058)", () => {
  it("hanya jenis yang berlaku yang bisa diajukan, urut form", () => {
    expect([...SELECTABLE_LEAVE_CODES]).toEqual([
      "annual", "sick", "personal", "leader", "maternity", "paternity", "pilgrimage", "marriage", "bereavement",
    ]);
    expect(LEAVE_TYPES).toHaveLength(12);
  });

  it("jenis yang dihapus tetap punya label riwayat", () => {
    expect(leaveTypeLabel("emergency")).toBe("Izin Darurat");
    expect(leaveTypeLabel("unpaid")).toBe("Izin Tanpa Gaji");
    expect(leaveTypeLabel("menstrual")).toBe("Cuti Haid");
    expect(leaveTypeInfo("menstrual")?.selectable).toBe(false);
  });

  it("label pendek & ekspor sama dengan katalog Go", () => {
    expect(leaveTypeShort("annual")).toBe("Tahunan");
    expect(leaveTypeExportLabel("sick")).toBe("Cuti Sakit");
    expect(leaveTypeExportLabel("emergency")).toBe("Cuti Darurat");
    expect(leaveTypeLabel("lainnya")).toBe("lainnya");
  });

  it("aturan Izin Leader & Izin Pribadi", () => {
    expect(leaveTypeInfo("leader")).toMatchObject({ monthlyQuota: 1, singleDay: true, leaderOnly: true });
    expect(leaveTypeInfo("personal")).toMatchObject({ monthlyQuota: 3, singleDay: true, deductsThpDay: true });
  });

  it("opsi form menyembunyikan Izin Leader untuk non-SPV", () => {
    const staff = leaveTypeOptions({ leaderEligible: false }).map((o) => o.value);
    const spv = leaveTypeOptions({ leaderEligible: true }).map((o) => o.value);
    expect(staff).not.toContain("leader");
    expect(spv).toContain("leader");
    expect(spv).not.toContain("emergency");
    expect(spv).not.toContain("unpaid");
    expect(spv).not.toContain("menstrual");
  });
});
