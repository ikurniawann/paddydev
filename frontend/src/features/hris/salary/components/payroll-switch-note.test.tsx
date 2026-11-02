import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { EmployeeSalary } from "../types";
import { EditSalaryPage } from "./edit-salary-page";
import { NewSalaryPage } from "./new-salary-page";

let settings: Record<string, unknown> | null = null;

vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: vi.fn() }),
  useSearchParams: () => new URLSearchParams(),
}));
vi.mock("sonner", () => ({ toast: { error: vi.fn(), success: vi.fn() } }));
vi.mock("@/features/hris/payroll", () => ({
  usePayrollEntities: () => ({ data: [], isLoading: false }),
  usePayrollSettings: () => ({ data: settings ? { settings, tax_config: null } : undefined }),
}));
const salary = {
  id: "s1", employee_id: "e1", base_salary: 5_000_000, fixed_allowance: 0, transport_allowance: 0, meal_allowance: 0,
  ptkp_status: "TK/0", is_taxable: true, is_active: true, effective_date: "2026-10-01", company_id: "c1",
  employee: { id: "e1", full_name: "Budi", nip: "100" },
} as EmployeeSalary;
vi.mock("../queries", () => ({
  useSalaryEmployees: () => ({ data: [] }),
  useSalary: () => ({ data: salary, isLoading: false, isError: false }),
}));
vi.mock("../mutations", () => ({
  useCreateSalary: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useUpdateSalary: () => ({ mutateAsync: vi.fn(), isPending: false }),
}));

const NOTE = "Dinonaktifkan di Pengaturan Payroll";
const params = Object.assign(Promise.resolve({ id: "s1" }), { status: "fulfilled", value: { id: "s1" } }) as Promise<{ id: string }>;
const pages = [
  ["NewSalaryPage", () => <NewSalaryPage />],
  ["EditSalaryPage", () => <EditSalaryPage params={params} />],
] as const;

beforeEach(() => {
  settings = null;
});
afterEach(cleanup);

describe.each(pages)("%s catatan saklar payroll (EPIC-082 R7)", (_name, page) => {
  it("tanpa catatan bila semua saklar menyala atau pengaturan belum termuat", () => {
    render(page());
    expect(screen.queryByText(NOTE)).toBeNull();
    cleanup();
    settings = { pph21_enabled: true, bpjs_tk_enabled: true, bpjs_kes_enabled: true };
    render(page());
    expect(screen.queryByText(NOTE)).toBeNull();
  });

  it("satu catatan per saklar yang mati", () => {
    settings = { pph21_enabled: false, bpjs_tk_enabled: true, bpjs_kes_enabled: true };
    render(page());
    expect(screen.getAllByText(NOTE)).toHaveLength(1);
    cleanup();
    settings = { pph21_enabled: false, bpjs_tk_enabled: false, bpjs_kes_enabled: false };
    render(page());
    expect(screen.getAllByText(NOTE)).toHaveLength(3);
  });
});
