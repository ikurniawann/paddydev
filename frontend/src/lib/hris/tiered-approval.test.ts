import { describe, expect, it } from "vitest";
import { SPV_APPROVED_LABEL, spvApprovedNote, spvStageDone } from "./tiered-approval";

describe("tahap SPV (EPIC-092 R7)", () => {
  it("pending + spv_approved_at: tahap SPV selesai, menunggu Manager", () => {
    const row = { status: "pending", spv_approved_at: "2026-11-05T03:00:00Z", spv_approved_name: "SPV Toko" };
    expect(spvStageDone(row)).toBe(true);
    expect(SPV_APPROVED_LABEL).toBe("Disetujui SPV — menunggu Manager");
    expect(spvApprovedNote(row)).toBe("Disetujui SPV oleh SPV Toko");
  });
  it("belum disetujui SPV atau sudah diputus: bukan tahap Manager", () => {
    expect(spvStageDone({ status: "pending", spv_approved_at: null })).toBe(false);
    expect(spvStageDone({ status: "approved", spv_approved_at: "2026-11-05T03:00:00Z" })).toBe(false);
    expect(spvApprovedNote({})).toBeNull();
  });
  it("catatan tetap ada setelah diputus, untuk Detail", () => {
    expect(spvApprovedNote({ spv_approved_at: "x", spv_approved_name: "HR (mewakili SPV Toko)" }))
      .toBe("Disetujui SPV oleh HR (mewakili SPV Toko)");
  });
});
