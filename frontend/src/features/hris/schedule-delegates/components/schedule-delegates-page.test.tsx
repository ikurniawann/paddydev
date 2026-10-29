import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { ScheduleDelegate } from "../types";
import { ScheduleDelegatesPage } from "./schedule-delegates-page";

const api = vi.hoisted(() => ({
  fetchScheduleDelegates: vi.fn(),
  fetchDelegateSuggest: vi.fn(),
  saveScheduleDelegate: vi.fn(),
  revokeScheduleDelegate: vi.fn(),
}));
vi.mock("../api", () => api);
vi.mock("./delegate-form-dialog", () => ({
  DelegateFormDialog: ({ open, delegate }: { open: boolean; delegate: ScheduleDelegate | null }) =>
    open ? <div role="dialog">{delegate ? `Ubah ${delegate.full_name}` : "Tambah pengatur"}</div> : null,
}));
vi.mock("sonner", () => ({ toast: { error: vi.fn(), success: vi.fn() } }));

const b = (code: string) => ({ id: `id-${code}`, code, name: `Paddy ${code}` });
const base = {
  has_login: true,
  is_active: true,
  updated_at: "2026-10-29T03:00:00Z",
  updated_by_name: "HR Admin",
};
const rows: ScheduleDelegate[] = [
  {
    ...base,
    employee_id: "e1",
    nip: "1001",
    full_name: "Ani Wijaya",
    position_title: "Area Manager",
    can_place: true,
    direct_reports: 5,
    branches: [b("PVJ"), b("KPO"), b("PIK"), b("PAI"), b("PSM")],
  },
  {
    ...base,
    employee_id: "e2",
    nip: "1002",
    full_name: "Budi Santoso",
    position_title: "Shopkeeper",
    can_place: false,
    direct_reports: 0,
    is_active: false,
    branches: [b("PVJ")],
  },
];

function renderPage() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={client}>
      <ScheduleDelegatesPage />
    </QueryClientProvider>
  );
  return client;
}

beforeEach(() => {
  Object.values(api).forEach((fn) => fn.mockReset());
  api.fetchScheduleDelegates.mockResolvedValue(rows);
});
afterEach(cleanup);

describe("ScheduleDelegatesPage (EPIC-071)", () => {
  it("menampilkan chip toko (maks 3 lalu +n), penempatan, bawahan, dan badge nonaktif", async () => {
    renderPage();
    const ani = (await screen.findByText("Ani Wijaya")).closest("tr")!;
    expect(within(ani).getByText("1001")).toBeTruthy();
    expect(within(ani).getByText("Area Manager")).toBeTruthy();
    for (const code of ["PVJ", "KPO", "PIK"]) expect(within(ani).getByText(code)).toBeTruthy();
    expect(within(ani).queryByText("PAI")).toBeNull();
    expect(within(ani).getByText("+2")).toBeTruthy();
    expect(within(ani).getByText("Ya")).toBeTruthy();
    expect(within(ani).getByText("5")).toBeTruthy();
    expect(within(ani).queryByText("Nonaktif")).toBeNull();

    const budi = screen.getByText("Budi Santoso").closest("tr")!;
    expect(within(budi).getByText("Tidak")).toBeTruthy();
    expect(within(budi).getByText("Nonaktif")).toBeTruthy();
  });

  it("daftar kosong menampilkan petunjuk", async () => {
    api.fetchScheduleDelegates.mockResolvedValue([]);
    renderPage();
    expect(
      await screen.findByText("Belum ada pengatur. Tambahkan karyawan yang boleh mengatur jadwal tokonya.")
    ).toBeTruthy();
  });

  it("Cabut meminta konfirmasi lalu memanggil DELETE dengan id karyawan", async () => {
    api.revokeScheduleDelegate.mockResolvedValue({ revoked: true });
    renderPage();
    const ani = (await screen.findByText("Ani Wijaya")).closest("tr")!;
    fireEvent.click(within(ani).getByRole("button", { name: "Cabut" }));
    expect(api.revokeScheduleDelegate).not.toHaveBeenCalled();
    fireEvent.click(await screen.findByRole("button", { name: "Ya, cabut" }));
    await waitFor(() => expect(api.revokeScheduleDelegate).toHaveBeenCalledWith("e1"));
  });

  it("Ubah dan Tambah membuka dialog", async () => {
    renderPage();
    const ani = (await screen.findByText("Ani Wijaya")).closest("tr")!;
    fireEvent.click(within(ani).getByRole("button", { name: "Ubah" }));
    expect(screen.getByRole("dialog").textContent).toBe("Ubah Ani Wijaya");
  });
});
