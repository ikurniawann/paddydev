import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { EmployeeAuditTab } from "./employee-audit-tab";

vi.mock("@tanstack/react-query", () => ({
  useQuery: () => ({
    isLoading: false,
    data: {
      data: [
        { id: "1", created_at: "2026-10-21T03:00:00Z", actor_name: "Sri Wardani", action: "employee.resign",
          before: { employment_status: "contract", is_active: true },
          after: { employment_status: "resigned", is_active: false, end_date: "2026-10-31", reason: "Pindah kota" }, reason: null },
        { id: "2", created_at: "2026-10-20T03:00:00Z", actor_name: "Sri Wardani", action: "employee.update",
          before: { full_name: "Feti" }, after: { full_name: "Feti Puspitasari", department_id: "x-uuid" }, reason: null },
      ],
      meta: { total: 2, totalPages: 1 },
    },
  }),
}));
afterEach(cleanup);

it("menampilkan riwayat dengan label bahasa user, tanpa nama field database", () => {
  render(<EmployeeAuditTab employeeId="e1" />);
  expect(screen.getAllByText("Resign").length).toBe(2); // judul aksi + status Resign
  expect(screen.getByText("Data diubah")).toBeTruthy();
  expect(screen.getByText("Nama Lengkap")).toBeTruthy();
  expect(screen.getByText("Feti Puspitasari")).toBeTruthy();
  expect(screen.getByText("31/10/2026")).toBeTruthy();
  expect(screen.getByText("Kontrak (PKWT)")).toBeTruthy();
  expect(screen.queryByText("full_name")).toBeNull();
  expect(screen.queryByText("x-uuid")).toBeNull();
  expect(screen.getAllByText(/Sri Wardani/).length).toBe(2);
});
