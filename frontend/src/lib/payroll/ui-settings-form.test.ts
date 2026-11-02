import { describe, it, expect } from "vitest";
import {
  DEFAULT_LATE_TIERS,
  SETTINGS_KEYS,
  TAX_KEYS,
  formStateToPayload,
  isSwitchOn,
  parseLateTiers,
  rowToFormState,
} from "./ui-settings-form";

describe("rowToFormState", () => {
  it("stringifies known keys and blanks missing ones", () => {
    const state = rowToFormState({ bpjs_kes_employee: 1, payroll_day: "25", extra: 9 }, [
      "bpjs_kes_employee",
      "payroll_day",
      "tapera_employee",
    ]);
    expect(state).toEqual({ bpjs_kes_employee: "1", payroll_day: "25", tapera_employee: "" });
  });

  it("handles a missing row", () => {
    expect(rowToFormState(null, ["a"])).toEqual({ a: "" });
  });
});

describe("formStateToPayload", () => {
  it("drops empty values, converts numbers, keeps the late mode text", () => {
    expect(
      formStateToPayload({
        bpjs_kes_employee: "1.5",
        tapera_employee: "",
        late_deduction_mode: "per_minute",
        payroll_day: "abc",
      })
    ).toEqual({ bpjs_kes_employee: 1.5, late_deduction_mode: "per_minute" });
  });
});

describe("saklar komponen (EPIC-082)", () => {
  it("mengirim false (tidak dilewati) dan true sebagai boolean", () => {
    expect(
      formStateToPayload({ pph21_enabled: "false", bpjs_tk_enabled: "true", bpjs_kes_enabled: "" })
    ).toEqual({ pph21_enabled: false, bpjs_tk_enabled: true });
  });

  it("memuat nilai false dari baris server", () => {
    const state = rowToFormState({ pph21_enabled: false, bpjs_kes_enabled: true }, SETTINGS_KEYS);
    expect(state.pph21_enabled).toBe("false");
    expect(state.bpjs_kes_enabled).toBe("true");
    expect(state.bpjs_tk_enabled).toBe("");
  });

  it("isSwitchOn: kosong dianggap menyala, hanya 'false' yang mati", () => {
    expect(isSwitchOn({}, "pph21_enabled")).toBe(true);
    expect(isSwitchOn({ pph21_enabled: "" }, "pph21_enabled")).toBe(true);
    expect(isSwitchOn({ pph21_enabled: "false" }, "pph21_enabled")).toBe(false);
  });
});

describe("key lists", () => {
  it("include the late deduction settings and tax brackets", () => {
    expect(SETTINGS_KEYS).toContain("late_deduction_mode");
    expect(SETTINGS_KEYS).toContain("late_deduction_amount");
    expect(TAX_KEYS).toContain("ptkp_k_3");
    expect(TAX_KEYS).toContain("jabatan_expense_max");
  });
});

describe("EPIC-055 settings", () => {
  it("sends toggles as booleans, tiers as JSON and PPh21 options as text", () => {
    const payload = formStateToPayload({
      overtime_requires_clockout: "false",
      personal_leave_deduction: "true",
      late_tier_rules: JSON.stringify([{ min: 3, max: 5, days: 1, basis: "base" }]),
      pph21_method: "ter",
      pph21_bpjs_basis: "employer",
      late_min_minutes: "15",
    });
    expect(payload).toEqual({
      overtime_requires_clockout: false,
      personal_leave_deduction: true,
      late_tier_rules: [{ min: 3, max: 5, days: 1, basis: "base" }],
      pph21_method: "ter",
      pph21_bpjs_basis: "employer",
      late_min_minutes: 15,
    });
  });

  it("reads jsonb tiers from the row and falls back to the Odoo tiers", () => {
    const state = rowToFormState({ late_tier_rules: [{ min: 4, days: 1, basis: "thp" }] }, ["late_tier_rules"]);
    expect(parseLateTiers(state.late_tier_rules)).toEqual([{ min: 4, days: 1, basis: "thp" }]);
    expect(parseLateTiers("bukan json")).toEqual(DEFAULT_LATE_TIERS);
  });
});
