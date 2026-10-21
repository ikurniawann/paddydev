import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { EmployeeWithoutManager } from "../types";
import { NoManagerCard } from "./no-manager-card";

let rows: EmployeeWithoutManager[] | undefined;

vi.mock("../queries", () => ({ useEmployeesWithoutManager: () => ({ data: rows }) }));

afterEach(() => {
  cleanup();
  rows = undefined;
});

describe("NoManagerCard", () => {
  it("tidak tampil bila semua karyawan punya atasan", () => {
    rows = [];
    const { container } = render(<NoManagerCard />);
    expect(container.innerHTML).toBe("");
  });

  it("menampilkan jumlah dan daftar dengan alasannya", () => {
    rows = [
      { id: "a1", nip: "N1", full_name: "Retail Ops", job_title: "Retail Operation Manager", department: "Retail", reason: "self" },
      { id: "b2", nip: null, full_name: "Crew Baru", job_title: null, department: null, reason: "empty" },
    ];
    render(<NoManagerCard />);
    fireEvent.click(screen.getByRole("button", { name: /2 karyawan aktif tanpa atasan langsung yang valid/ }));
    expect(screen.getByText("Atasan = diri sendiri")).toBeTruthy();
    expect(screen.getByText("Atasan belum diisi")).toBeTruthy();
    expect(screen.getByRole("link", { name: "Retail Ops" }).getAttribute("href")).toBe("/dashboard/employees/a1");
  });
});
