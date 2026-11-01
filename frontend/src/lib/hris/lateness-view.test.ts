import { describe, expect, it } from "vitest";
import { formatLateMinutes, LATENESS_CODES, LATENESS_POLICY, LATENESS_TONE, nextLevelHint } from "./lateness-view";

describe("lateness-view (EPIC-077)", () => {
  it("menulis petunjuk tingkat berikutnya", () => {
    expect(nextLevelHint({ code: "sp1", label: "Surat Peringatan 1", at: 3, remaining: 1 })).toBe(
      "1 kali terlambat lagi → Surat Peringatan 1"
    );
    expect(nextLevelHint(null)).toBeNull();
  });

  it("memformat menit terlambat", () => {
    expect(formatLateMinutes(45)).toBe("45 mnt");
    expect(formatLateMinutes(65)).toBe("1 jam 5 mnt");
    expect(formatLateMinutes(120)).toBe("2 jam");
    expect(formatLateMinutes(-3)).toBe("0 mnt");
  });

  it("memuat ketentuan perusahaan apa adanya", () => {
    expect(LATENESS_POLICY.levels).toHaveLength(4);
    expect(LATENESS_POLICY.levels[0]).toContain("0 – 2x");
    expect(LATENESS_POLICY.steps.join(" ")).toContain("check-in");
  });

  it("punya warna untuk setiap tingkat", () => {
    for (const code of LATENESS_CODES) expect(LATENESS_TONE[code].badge).toBeTruthy();
  });
});
