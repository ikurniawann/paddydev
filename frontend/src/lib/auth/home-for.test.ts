import { describe, expect, it } from "vitest";
import { homeFor } from "./home-for";

describe("homeFor", () => {
  it("HR tanpa Rekrutmen/Analytics ke daftar karyawan", () => {
    expect(homeFor("hrd", ["hris.kepegawaian", "hris.kepegawaian.users", "hris.insights.reports"])).toBe(
      "/dashboard/employees"
    );
  });
  it("HR dengan Rekrutmen tetap ke dashboard rekrutmen", () => {
    expect(homeFor("hrd", ["hris.recruitment.candidates"])).toBe("/dashboard/rekrutmen");
  });
  it("peran lain tidak berubah", () => {
    expect(homeFor("finance_staff", [])).toBe("/dashboard/hris/payroll");
    expect(homeFor("pos", [])).toBe("/dashboard/pos");
    expect(homeFor("marketing", [])).toBe("/dashboard/rekrutmen");
  });
});
