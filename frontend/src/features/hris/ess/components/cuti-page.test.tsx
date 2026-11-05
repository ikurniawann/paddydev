import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { EssLeaveBalance, EssLeaveRow, EssMe } from "../types";
import { EssCutiPage } from "./cuti-page";

let me: EssMe | undefined;
let myLeaves: EssLeaveRow[] = [];
vi.mock("../queries", () => ({
  useEssMe: () => ({ data: me, isLoading: false }),
  useMyLeaves: () => ({ data: myLeaves }),
}));
const resubmit = vi.fn();
vi.mock("../mutations", () => ({
  useResubmitLeaveAttachment: () => ({ mutate: resubmit, isPending: false }),
}));
vi.mock("@/features/hris/leaves/components/leave-attachment-field", () => ({
  LeaveAttachmentField: ({ onChange }: { onChange: (p: string) => void }) => (
    <button onClick={() => onChange("leave-attachments/e1/baru.png")}>pilih-file</button>
  ),
}));
vi.mock("sonner", () => ({ toast: { error: vi.fn(), success: vi.fn() } }));
vi.mock("./cuti-request-dialog", () => ({ CutiRequestDialog: () => null }));

const balance: EssLeaveBalance = {
  year: 2026,
  annual_leave_total: 12,
  annual_leave_adjustment: 1,
  annual_leave_used: 4,
  annual_leave_remaining: 9,
  carry_over_days: 2,
  carry_over_used: 0,
  carry_over_remaining: 2,
  carry_over_expires_on: "2026-12-31",
  carry_over_active: true,
  available: 11,
};

const employee = { id: "e1", full_name: "Ayu", nip: "N1", position_title: null, department_name: null };

afterEach(() => {
  cleanup();
  myLeaves = [];
  resubmit.mockReset();
});

const row: EssLeaveRow = {
  id: "l1", leave_type: "birthday", start_date: "2026-11-10", end_date: "2026-11-10", total_days: 1,
  status: "pending", reason: "Ulang tahun", rejection_reason: null,
};

describe("EssCutiPage", () => {
  it("headlines the available balance with the breakdown", () => {
    me = { employee, leave_balance: balance };
    render(<EssCutiPage />);
    expect(screen.getByText("11")).toBeTruthy();
    expect(screen.getByText(/hari tersedia/)).toBeTruthy();
    expect(screen.getByText("Penyesuaian")).toBeTruthy();
    expect(screen.getByText("Sisa bawaan")).toBeTruthy();
    expect(screen.getByText(/Kuota Cuti Tahunan 2026/)).toBeTruthy();
  });

  it("shows an empty state instead of an invented 12 days when there is no balance", () => {
    me = { employee, leave_balance: null };
    render(<EssCutiPage />);
    expect(screen.getByText(/Kuota cuti tahunan Anda belum tersedia/)).toBeTruthy();
    expect(screen.queryByText("12")).toBeNull();
    expect(screen.queryByText(/hari tersedia/)).toBeNull();
  });
});

describe("EssCutiPage: verifikasi lampiran (EPIC-088)", () => {
  it("dikembalikan: alasan tampil dan unggah ulang mengirim path baru", () => {
    me = { employee, leave_balance: balance };
    myLeaves = [{ ...row, attachment_review: "returned", attachment_return_reason: "Foto KTP buram" }];
    render(<EssCutiPage />);
    expect(screen.getByText("Lampiran dikembalikan")).toBeTruthy();
    expect(screen.getByText(/Foto KTP buram/)).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Unggah ulang lampiran" }));
    const send = screen.getByRole("button", { name: "Kirim lampiran" });
    expect((send as HTMLButtonElement).disabled).toBe(true);
    fireEvent.click(screen.getByText("pilih-file"));
    fireEvent.click(send);
    expect(resubmit).toHaveBeenCalledWith({ id: "l1", path: "leave-attachments/e1/baru.png" }, expect.anything());
  });

  it("menunggu HR: badge tanpa tombol unggah ulang", () => {
    me = { employee, leave_balance: balance };
    myLeaves = [{ ...row, attachment_review: "awaiting" }];
    render(<EssCutiPage />);
    expect(screen.getByText("Menunggu verifikasi HR")).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Unggah ulang lampiran" })).toBeNull();
  });
});

describe("EssCutiPage — tahap SPV (EPIC-092 R7)", () => {
  it("pengajuan yang sudah disetujui SPV berlabel menunggu Manager", () => {
    me = { employee, leave_balance: balance };
    myLeaves = [{ ...row, spv_approved_at: "2026-11-05T03:00:00Z", spv_approved_name: "SPV Toko" }];
    render(<EssCutiPage />);
    expect(screen.getByText("Disetujui SPV — menunggu Manager")).toBeTruthy();
  });
});
