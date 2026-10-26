import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { LeaveBalanceView } from "../types";
import { LeaveBalancesPage } from "./leave-balances-page";

const hooks = vi.hoisted(() => ({
  useLeaveBalances: vi.fn(),
  useLeaveBalanceAccess: vi.fn(),
}));
vi.mock("../queries", () => hooks);
vi.mock("@/hooks/use-debounced-value", () => ({ useDebouncedValue: <T,>(v: T) => v }));
vi.mock("@/features/hris/placements/queries", () => ({
  useStoreBranches: () => ({
    data: [
      { id: "b1", name: "PPJA", code: "PPJA" },
      { id: "b2", name: "PVJ", code: "PVJ" },
    ],
  }),
}));
vi.mock("@/components/ui/combobox", () => ({
  Combobox: ({ options, value, onChange }: { options: { value: string; label: string }[]; value?: string; onChange: (v: string) => void }) => (
    <select aria-label="Cabang" value={value} onChange={(e) => onChange(e.target.value)}>
      <option value="">Semua cabang</option>
      {options.map((o) => (
        <option key={o.value} value={o.value}>
          {o.label}
        </option>
      ))}
    </select>
  ),
}));
vi.mock("./leave-settings-card", () => ({
  LeaveSettingsCard: ({ canUpdate }: { canUpdate: boolean }) => <div>kartu-pengaturan-{canUpdate ? "ubah" : "baca"}</div>,
}));
vi.mock("./leave-carry-reviews-card", () => ({
  LeaveCarryReviewsCard: (p: { year: number; canUpdate: boolean; canCorrect: boolean }) => (
    <div>{`kartu-tinjauan-${p.year}-${p.canUpdate ? "ubah" : "baca"}-${p.canCorrect ? "koreksi" : "tanpa-koreksi"}`}</div>
  ),
}));
vi.mock("./leave-balance-upload-dialog", () => ({
  LeaveBalanceUploadDialog: (p: { year: number; branchId: string; branchName: string }) => (
    <div>{`dialog-import-${p.year}-${p.branchId || "semua"}-${p.branchName}`}</div>
  ),
}));
vi.mock("./leave-balance-edit-dialog", () => ({
  LeaveBalanceEditDialog: ({ balance }: { balance: LeaveBalanceView }) => <div>dialog-ubah-{balance.full_name}</div>,
}));

const row = (over: Partial<LeaveBalanceView>): LeaveBalanceView => ({
  employee_id: "e1",
  nip: "2025001",
  full_name: "Ana Lestari",
  branch_name: "PPJA",
  year: 2026,
  annual_leave_total: 12,
  annual_leave_adjustment: 0,
  annual_leave_used: 3,
  annual_leave_remaining: 9,
  carry_over_days: 2,
  carry_over_used: 0.5,
  carry_over_remaining: 1.5,
  carry_over_expires_on: "2027-03-31",
  carry_over_active: true,
  carry_over_source: "auto",
  available: 10.5,
  updated_at: null,
  exists: true,
  ...over,
});

beforeEach(() => {
  vi.useFakeTimers({ toFake: ["Date"] });
  vi.setSystemTime(new Date("2026-10-26T03:00:00Z"));
  hooks.useLeaveBalances.mockReset();
  hooks.useLeaveBalanceAccess.mockReset();
  hooks.useLeaveBalances.mockReturnValue({
    data: [row({}), row({ employee_id: "e2", nip: "2025002", full_name: "Budi", exists: false, annual_leave_used: 0, annual_leave_remaining: 9.5, annual_leave_total: 9.5, carry_over_days: 0, carry_over_used: 0, carry_over_remaining: 0, carry_over_expires_on: null, carry_over_active: false, carry_over_source: "manual", available: 9.5 })],
    isLoading: false,
    isError: false,
  });
  hooks.useLeaveBalanceAccess.mockReturnValue({ data: { read: true, update: true, import: true, export: true } });
});
afterEach(() => {
  cleanup();
  vi.useRealTimers();
});

describe("LeaveBalancesPage", () => {
  it("memuat tahun berjalan semua cabang dan menampilkan baris", () => {
    render(<LeaveBalancesPage />);
    expect(hooks.useLeaveBalances).toHaveBeenLastCalledWith({ year: 2026, branch_id: "", q: "" });
    expect(screen.getByText("Ana Lestari")).toBeTruthy();
    expect(screen.getByText("2025002")).toBeTruthy();
    expect(screen.getAllByText("10,5").length).toBeGreaterThan(0);
  });

  it("filter cabang, tahun, dan pencarian diteruskan ke query", () => {
    render(<LeaveBalancesPage />);
    fireEvent.change(screen.getByLabelText("Cabang"), { target: { value: "b2" } });
    expect(hooks.useLeaveBalances).toHaveBeenLastCalledWith({ year: 2026, branch_id: "b2", q: "" });
    fireEvent.change(screen.getByLabelText("Tahun"), { target: { value: "2027" } });
    expect(hooks.useLeaveBalances).toHaveBeenLastCalledWith({ year: 2027, branch_id: "b2", q: "" });
    fireEvent.change(screen.getByLabelText("Cari karyawan"), { target: { value: " ana " } });
    expect(hooks.useLeaveBalances).toHaveBeenLastCalledWith({ year: 2027, branch_id: "b2", q: "ana" });
  });

  it("kolom Sumber menandai sisa bawaan Otomatis atau Manual, dan tanda hanya bila ada sisa bawaan", () => {
    render(<LeaveBalancesPage />);
    const header = screen.getByRole("columnheader", { name: "Sumber" });
    expect(header).toBeTruthy();
    const idx = screen.getAllByRole("columnheader").indexOf(header);
    const rows = screen.getAllByRole("row");
    expect(within(rows[1]).getAllByRole("cell")[idx].textContent).toBe("Otomatis");
    expect(within(rows[2]).getAllByRole("cell")[idx].textContent).toBe("Manual");
  });

  it("kartu tinjauan tampil untuk akun read, mengikuti tahun terpilih, dan hak koreksi hanya tahun berjalan/depan", () => {
    render(<LeaveBalancesPage />);
    expect(screen.getByText("kartu-tinjauan-2026-ubah-koreksi")).toBeTruthy();
    fireEvent.change(screen.getByLabelText("Tahun"), { target: { value: "2025" } });
    expect(screen.getByText("kartu-tinjauan-2025-ubah-tanpa-koreksi")).toBeTruthy();
    cleanup();
    hooks.useLeaveBalanceAccess.mockReturnValue({ data: { read: true, update: false, import: false, export: false } });
    render(<LeaveBalancesPage />);
    expect(screen.getByText("kartu-tinjauan-2026-baca-tanpa-koreksi")).toBeTruthy();
    cleanup();
    hooks.useLeaveBalanceAccess.mockReturnValue({ data: undefined });
    render(<LeaveBalancesPage />);
    expect(screen.queryByText(/^kartu-tinjauan/)).toBeNull();
  });

  it("baris tanpa data tersimpan diberi tanda jatah default", () => {
    render(<LeaveBalancesPage />);
    expect(screen.getAllByText("Default")).toHaveLength(1);
  });

  it("tombol Ubah per baris membuka dialog hanya untuk tahun berjalan/depan dan akun update", () => {
    render(<LeaveBalancesPage />);
    fireEvent.click(screen.getAllByRole("button", { name: "Ubah" })[0]);
    expect(screen.getByText("dialog-ubah-Ana Lestari")).toBeTruthy();
    cleanup();
    hooks.useLeaveBalanceAccess.mockReturnValue({ data: { read: true, update: false, import: false, export: false } });
    render(<LeaveBalancesPage />);
    expect(screen.queryByRole("button", { name: "Ubah" })).toBeNull();
    expect(screen.getByText("kartu-pengaturan-baca")).toBeTruthy();
    fireEvent.change(screen.getByLabelText("Tahun"), { target: { value: "2025" } });
  });

  it("tahun lalu hanya dilihat", () => {
    render(<LeaveBalancesPage />);
    fireEvent.change(screen.getByLabelText("Tahun"), { target: { value: "2025" } });
    expect(screen.queryByRole("button", { name: "Ubah" })).toBeNull();
  });

  it("kartu pengaturan memakai hak update; tempat tombol Import tersedia untuk akun import", () => {
    render(<LeaveBalancesPage />);
    expect(screen.getByText("kartu-pengaturan-ubah")).toBeTruthy();
    expect(screen.getByTestId("leave-balance-import-slot")).toBeTruthy();
    cleanup();
    hooks.useLeaveBalanceAccess.mockReturnValue({ data: { read: true, update: true, import: false, export: false } });
    render(<LeaveBalancesPage />);
    expect(screen.queryByTestId("leave-balance-import-slot")).toBeNull();
  });

  it("tombol Import membuka dialog dengan tahun dan cabang terpilih; template mengikuti filter dan hak export", () => {
    render(<LeaveBalancesPage />);
    const tpl = screen.getByRole("link", { name: "Unduh template" }) as HTMLAnchorElement;
    expect(tpl.getAttribute("href")).toBe("/api/hris/leave-balances/template?year=2026");
    fireEvent.change(screen.getByLabelText("Cabang"), { target: { value: "b2" } });
    fireEvent.change(screen.getByLabelText("Tahun"), { target: { value: "2027" } });
    expect(screen.getByRole("link", { name: "Unduh template" }).getAttribute("href")).toBe(
      "/api/hris/leave-balances/template?year=2027&branch_id=b2"
    );
    expect(screen.queryByText(/^dialog-import-/)).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Import Excel" }));
    expect(screen.getByText("dialog-import-2027-b2-PVJ")).toBeTruthy();
    cleanup();
    hooks.useLeaveBalanceAccess.mockReturnValue({ data: { read: true, update: true, import: true, export: false } });
    render(<LeaveBalancesPage />);
    expect(screen.queryByRole("link", { name: "Unduh template" })).toBeNull();
    expect(screen.getByRole("button", { name: "Import Excel" })).toBeTruthy();
  });

  it("template tampil dengan hak export saja, tanpa tombol Import", () => {
    hooks.useLeaveBalanceAccess.mockReturnValue({ data: { read: true, update: false, import: false, export: true } });
    render(<LeaveBalancesPage />);
    expect(screen.getByTestId("leave-balance-import-slot")).toBeTruthy();
    expect(screen.getByRole("link", { name: "Unduh template" })).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Import Excel" })).toBeNull();
  });

  it("menampilkan galat dan keadaan kosong", () => {
    hooks.useLeaveBalances.mockReturnValue({ data: undefined, isLoading: false, isError: true, error: new Error("Forbidden") });
    render(<LeaveBalancesPage />);
    expect(screen.getByText("Forbidden")).toBeTruthy();
    cleanup();
    hooks.useLeaveBalances.mockReturnValue({ data: [], isLoading: false, isError: false });
    render(<LeaveBalancesPage />);
    expect(screen.getByText("Tidak ada karyawan yang cocok.")).toBeTruthy();
  });
});
