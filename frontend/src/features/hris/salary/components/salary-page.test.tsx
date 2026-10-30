import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { EmployeeSalary } from "../types";
import { SalaryPage } from "./salary-page";

const download = vi.fn();
vi.mock("../salary-export", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../salary-export")>()),
  downloadSalaryXlsx: (...args: unknown[]) => download(...args),
}));
vi.mock("next/navigation", () => ({ useRouter: () => ({ push: vi.fn() }) }));
const row = (name: string, nip: string): EmployeeSalary => ({
  id: nip, employee_id: nip, base_salary: 1, fixed_allowance: 0, transport_allowance: 0, meal_allowance: 0,
  ptkp_status: "TK/0", is_taxable: true, is_active: true, effective_date: "2026-10-01", company_id: "c1",
  employee: { id: nip, full_name: name, nip },
});
vi.mock("../queries", () => ({
  useSalaryList: () => ({ data: [row("Citra", "300"), row("andi", "100"), row("Budi", "200")], isLoading: false }),
}));
vi.mock("@/features/hris/payroll", () => ({
  EntityTabs: () => null,
  useEntityParam: () => ({ entity: "c1", setEntity: vi.fn(), entities: [{ id: "c1", name: "PT Paddy" }], loading: false }),
}));

afterEach(() => {
  cleanup();
  download.mockReset();
});

const names = () => screen.getAllByRole("row").slice(1).map((r) => within(r).getAllByRole("cell")[0].textContent);

describe("SalaryPage (EPIC-075)", () => {
  it("urut nama A–Z, cari nama/NIP, export baris yang tampil", () => {
    render(<SalaryPage />);
    expect(names()).toEqual(["andi100", "Budi200", "Citra300"]);
    fireEvent.change(screen.getByPlaceholderText("Cari nama atau NIP"), { target: { value: "200" } });
    expect(names()).toEqual(["Budi200"]);
    fireEvent.click(screen.getByRole("button", { name: /Export Excel/ }));
    expect(download).toHaveBeenCalledTimes(1);
    const [rows, entityName] = download.mock.calls[0];
    expect((rows as EmployeeSalary[]).map((r) => r.employee?.full_name)).toEqual(["Budi"]);
    expect(entityName).toBe("PT Paddy");
  });

  it("pencarian tanpa hasil", () => {
    render(<SalaryPage />);
    fireEvent.change(screen.getByPlaceholderText("Cari nama atau NIP"), { target: { value: "zzz" } });
    expect(screen.getByText("Tidak ada karyawan yang cocok")).toBeTruthy();
  });
});
