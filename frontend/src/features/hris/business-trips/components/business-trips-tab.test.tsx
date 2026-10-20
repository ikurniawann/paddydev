import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { BusinessTrip } from "../types";
import { BusinessTripsTab } from "./business-trips-tab";

const decide = vi.fn();
const create = vi.fn();
const rows: BusinessTrip[] = [];

vi.mock("sonner", () => ({ toast: { error: vi.fn(), success: vi.fn() } }));
vi.mock("@/features/hris/ess/queries", () => ({
  useEssMe: () => ({ data: { employee: { id: "me1" } } }),
}));
vi.mock("../queries", () => ({ useBusinessTrips: () => ({ data: rows }) }));
vi.mock("../mutations", () => ({
  useDecideBusinessTrip: () => ({ mutate: decide, isPending: false, variables: undefined }),
  useCreateBusinessTrip: () => ({ mutate: create, isPending: false }),
}));
vi.mock("@/features/hris/attendance/queries", () => ({
  useActiveEmployees: () => ({ data: [{ id: "e9", full_name: "Sari Dewi", nip: "N09" }] }),
}));
vi.mock("@/components/ui/combobox", () => ({
  Combobox: ({ options, onChange }: { options: { value: string; label: string }[]; onChange: (v: string) => void }) => (
    <div>
      {options.map((o) => (
        <button key={o.value} type="button" onClick={() => onChange(o.value)}>
          {o.label}
        </button>
      ))}
    </div>
  ),
}));
vi.mock("@/components/ui/select", () => ({
  Select: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  SelectTrigger: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  SelectValue: () => null,
  SelectContent: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  SelectItem: ({ children }: { children: React.ReactNode }) => <span>{children}</span>,
}));

function trip(over: Partial<BusinessTrip>): BusinessTrip {
  return {
    id: "t1", employee_id: "e1", start_date: "2099-03-02", end_date: "2099-03-04", destination: "Gudang Cikarang",
    purpose: "Stock opname", status: "pending", requested_by: "e1", decided_by: null, decided_by_name: null,
    decided_at: null, rejection_reason: null, created_at: "2099-03-01T00:00:00Z",
    employee: { id: "e1", full_name: "Budi Santoso", nip: "N01", reporting_to: null }, ...over,
  };
}

afterEach(() => {
  cleanup();
  decide.mockReset();
  create.mockReset();
  rows.length = 0;
});

describe("BusinessTripsTab", () => {
  it("HRD melihat Setujui/Tolak hanya saat menunggu dan Batalkan saat menunggu atau disetujui", () => {
    rows.push(
      trip({ id: "p", status: "pending" }),
      trip({ id: "a", status: "approved" }),
      trip({ id: "r", status: "rejected" }),
      trip({ id: "c", status: "cancelled" })
    );
    render(<BusinessTripsTab />);
    expect(screen.getAllByRole("button", { name: "Setujui" })).toHaveLength(1);
    expect(screen.getAllByRole("button", { name: "Tolak" })).toHaveLength(1);
    expect(screen.getAllByRole("button", { name: "Batalkan" })).toHaveLength(2);
  });

  it("menyembunyikan Setujui/Tolak pada pengajuan HRD sendiri tetapi menyisakan Batalkan", () => {
    rows.push(trip({ id: "own", employee_id: "me1", status: "pending" }));
    render(<BusinessTripsTab />);
    expect(screen.queryByRole("button", { name: "Setujui" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Tolak" })).toBeNull();
    expect(screen.getByRole("button", { name: "Batalkan" })).toBeTruthy();
  });

  it("membatalkan yang sudah disetujui: konfirmasi menyebut absensi tidak dikembalikan otomatis", () => {
    rows.push(trip({ id: "a", status: "approved" }));
    render(<BusinessTripsTab />);
    fireEvent.click(screen.getByRole("button", { name: "Batalkan" }));
    expect(screen.getByText(/tidak dikembalikan otomatis/)).toBeTruthy();
    expect(decide).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "Batalkan pengajuan" }));
    expect(decide).toHaveBeenCalledWith({ id: "a", action: "cancel", rejectionReason: undefined }, expect.anything());
  });

  it("HRD mencatat dinas untuk karyawan lain", () => {
    render(<BusinessTripsTab />);
    fireEvent.click(screen.getByRole("button", { name: /Catat Perjalanan Dinas/ }));
    fireEvent.click(screen.getByRole("button", { name: "Sari Dewi (N09)" }));
    for (const [label, value] of Object.entries({
      "Tanggal mulai": "2099-03-02", "Tanggal selesai": "2099-03-03", Tujuan: "Toko Bandung", Keperluan: "Audit stok",
    })) {
      fireEvent.change(screen.getByLabelText(label), { target: { value } });
    }
    fireEvent.click(screen.getByRole("button", { name: "Simpan" }));
    expect(create).toHaveBeenCalledWith(
      { start_date: "2099-03-02", end_date: "2099-03-03", destination: "Toko Bandung", purpose: "Audit stok", employee_id: "e9" },
      expect.anything()
    );
  });
});
