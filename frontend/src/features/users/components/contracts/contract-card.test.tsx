import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import type { EmployeeContractRow } from "../../api";
import { ContractCard } from "./contract-card";

afterEach(cleanup);

const contract = {
  id: "c1", employee_id: "e1", contract_number: "001/PKWT/X/2026", contract_type: "pkwt", status: "active",
  start_date: "2026-01-01", end_date: "2027-01-01", probation_end_date: null, sequence: 1,
  position_title: null, department_name: null, work_location: null, base_salary: "4800000.00",
  signed_at: null, signed_document_url: null, kemnaker_registered_at: null, compensation_amount: null,
  compensation_paid_at: null, terminated_reason: null, notes: null, created_by_name: null,
  created_at: "2026-01-01T00:00:00Z",
} as unknown as EmployeeContractRow;
const noop = () => {};
const props = { contract, actionPending: false, deletePending: false, onAction: noop, onEdit: noop,
  onAdmin: noop, onRenew: noop, onTerminate: noop, onDelete: noop };

it("tanpa akses Penggajian: tidak ada gaji pokok dan tombol PDF", () => {
  render(<ContractCard {...props} canViewSalary={false} />);
  expect(screen.queryByText("Gaji pokok")).toBeNull();
  expect(screen.queryByTitle("Unduh PDF surat perjanjian kerja")).toBeNull();
});

it("dengan akses Penggajian: gaji pokok dan PDF tampil", () => {
  render(<ContractCard {...props} canViewSalary />);
  expect(screen.getByText("Gaji pokok")).toBeTruthy();
  expect(screen.getByTitle("Unduh PDF surat perjanjian kerja")).toBeTruthy();
});
