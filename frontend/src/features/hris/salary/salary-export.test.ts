import { describe, expect, it } from "vitest";
import { matchesSalarySearch, salaryExportRows, sortSalaries } from "./salary-export";
import type { EmployeeSalary } from "./types";

const row = (name: string, nip: string, over: Partial<EmployeeSalary> = {}): EmployeeSalary => ({
  id: nip, employee_id: nip, base_salary: 3_000_000, fixed_allowance: 0, transport_allowance: 200_000,
  meal_allowance: 300_000, position_allowance: 500_000, health_allowance: 100_000, ptkp_status: "TK/0",
  is_taxable: true, is_active: true, effective_date: "2026-10-01", company_id: "c1",
  employee: { id: nip, full_name: name, nip, position: { title: "Staff" }, department: { name: "Toko" } }, ...over,
});

describe("salary-export (EPIC-075 R3–R5)", () => {
  it("sortSalaries: nama A–Z tanpa peduli huruf besar, lalu NIP", () => {
    const out = sortSalaries([row("budi", "3"), row("Andi", "2"), row("Citra", "1"), row("Andi", "1")]);
    expect(out.map((r) => `${r.employee?.full_name}#${r.employee?.nip}`)).toEqual(["Andi#1", "Andi#2", "budi#3", "Citra#1"]);
  });

  it("matchesSalarySearch: nama atau NIP, tidak peka huruf besar", () => {
    const r = row("Andi Pratama", "202501138");
    expect(matchesSalarySearch(r, "AND")).toBe(true);
    expect(matchesSalarySearch(r, "202501")).toBe(true);
    expect(matchesSalarySearch(r, "  ")).toBe(true);
    expect(matchesSalarySearch(r, "citra")).toBe(false);
  });

  it("salaryExportRows: header R5 dan nominal berupa angka", () => {
    const rows = salaryExportRows([row("Andi", "1")], "PT Paddy");
    expect(rows[0]).toEqual(["NIP", "Nama", "Jabatan", "Departemen", "Entitas", "Gaji Pokok", "Tunjangan Tetap",
      "Transport", "Makan", "Tunjangan Jabatan", "Tunjangan Kesehatan", "Status PTKP", "Efektif"]);
    expect(rows[1]).toEqual(["1", "Andi", "Staff", "Toko", "PT Paddy", 3_000_000, 0, 200_000, 300_000, 500_000, 100_000,
      "TK/0", "2026-10-01"]);
  });
});
