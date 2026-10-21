import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import type { Employee } from "@/types/hris";
import { EmployeeProfileHeader } from "./employee-profile-header";

vi.mock("next/navigation", () => ({ useRouter: () => ({ push: vi.fn() }) }));
afterEach(cleanup);

const emp = (over: Partial<Employee>) =>
  ({ id: "e1", full_name: "Rina", nip: "N1", employment_status: "resigned", is_active: false, join_date: "2025-01-10", end_date: "2026-06-30", ...over }) as Employee;

const props = { canManageUsers: false, onCreateAccount: () => {}, onResetPassword: () => {} };

it("tombol Aktifkan kembali tampil untuk karyawan nonaktif bila onRehire ada", () => {
  const onRehire = vi.fn();
  render(<EmployeeProfileHeader {...props} employee={emp({})} onRehire={onRehire} />);
  fireEvent.click(screen.getByRole("button", { name: "Aktifkan kembali" }));
  expect(onRehire).toHaveBeenCalled();
});

it("tidak tampil untuk karyawan aktif", () => {
  render(<EmployeeProfileHeader {...props} employee={emp({ is_active: true, employment_status: "permanent", end_date: null })} onRehire={() => {}} />);
  expect(screen.queryByRole("button", { name: "Aktifkan kembali" })).toBeNull();
});

it("tidak tampil tanpa onRehire", () => {
  render(<EmployeeProfileHeader {...props} employee={emp({})} />);
  expect(screen.queryByRole("button", { name: "Aktifkan kembali" })).toBeNull();
});
