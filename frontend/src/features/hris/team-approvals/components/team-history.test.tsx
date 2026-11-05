import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { TeamHistory } from "../types";
import { TeamHistoryPanel } from "./team-history";

let history: { data?: TeamHistory; isLoading?: boolean; isError?: boolean } = {};
vi.mock("../queries", () => ({ useTeamHistory: () => history }));

const emp = { id: "e1", full_name: "Anggota A", nip: "N01" };
const base = {
  employee_id: "e1", leave_type: "marriage", total_days: "1.00", reason: "Acara keluarga",
  attachment_url: "leave-attachments/e1/s.png", attachment_return_reason: null, rejection_reason: null,
  decided_by_name: null, manager_approved_name: null, attachment_reviewed_name: null,
  created_at: "2026-11-01T01:00:00Z", employee: emp,
};

afterEach(() => {
  cleanup();
  history = {};
});

// EPIC-090 R2: riwayat semua status dengan Detail baca-saja.
describe("TeamHistoryPanel", () => {
  it("menampilkan rentang, status per tahap, dan Detail dengan catatan keputusan", () => {
    history = {
      data: {
        period: { from: "2026-10-01", to: "2026-11-30", label: "1 Okt – 30 Nov 2026" },
        leaves: [
          { ...base, id: "l1", start_date: "2026-11-20", end_date: "2026-11-20", status: "pending",
            attachment_review: "awaiting", manager_approved_name: "Atasan A" },
          { ...base, id: "l2", start_date: "2026-11-10", end_date: "2026-11-10", status: "approved",
            attachment_review: "verified", decided_by_name: "Atasan A", attachment_reviewed_name: "HR B" },
          { ...base, id: "l3", start_date: "2026-10-12", end_date: "2026-10-12", status: "rejected",
            attachment_review: null, decided_by_name: "Atasan A", rejection_reason: "Stok opname" },
        ],
        loans: [
          { id: "k1", employee_id: "e1", loan_type: "kasbon", principal_amount: "1000000.00", monthly_installment: "500000.00",
            tenor_months: 2, status: "pending", purpose: "Biaya sekolah anak", rejection_reason: null,
            manager_approver_name: "Manager Area", attachments: ["loan-attachments/e1/1-aaaaaaaaaaaaaaaa.pdf"],
            collateral_attachments: [], created_at: "2026-11-04T03:00:00Z", employee: emp },
          { id: "k3", employee_id: "e1", loan_type: "kasbon", principal_amount: "500000.00", monthly_installment: "500000.00",
            tenor_months: 1, status: "pending", purpose: "Berobat", rejection_reason: null,
            manager_approver_name: "Manager Area", supervisor_approver_name: "SPV Toko", supervisor_approved: false,
            attachments: [], collateral_attachments: [], created_at: "2026-11-04T02:00:00Z", employee: emp },
          { id: "k2", employee_id: "e1", loan_type: "loan", principal_amount: "3000000.00", monthly_installment: "1000000.00",
            tenor_months: 3, status: "rejected", purpose: "Renovasi", rejection_reason: "Masa kerja kurang",
            manager_approver_name: null, attachments: [], collateral_attachments: [], created_at: "2026-10-04T03:00:00Z", employee: emp },
        ],
        overtime: [
          { id: "o1", employee_id: "e1", date: "2026-11-03", start_time: "18:00:00", end_time: "23:00:00", hours: "5.00",
            break_minutes: 0, attachments: [], day_type: "regular", reason: "Closing stok akhir bulan", source: "employee",
            status: "approved", rejection_reason: null, decided_by_name: "Atasan A", created_at: "2026-11-04T01:00:00Z", employee: emp },
        ],
      },
    };
    render(<TeamHistoryPanel />);
    expect(screen.getByText((_, el) => el?.tagName === "P" && /sejak 1 Okt 2026/.test(el.textContent ?? ""))).toBeTruthy();
    expect(screen.getByText("Menunggu verifikasi HR")).toBeTruthy();
    expect(screen.getAllByText("Disetujui").length).toBe(2);

    fireEvent.click(screen.getAllByRole("button", { name: "Detail cuti Anggota A" })[1]);
    const dialog = screen.getByRole("dialog");
    expect(within(dialog).getByText("Disetujui oleh Atasan A")).toBeTruthy();
    expect(within(dialog).getByText("Lampiran diverifikasi HR: HR B")).toBeTruthy();
    expect(within(dialog).queryByRole("button", { name: "Setujui" })).toBeNull();
    fireEvent.keyDown(document.activeElement ?? document.body, { key: "Escape" });

    fireEvent.click(screen.getAllByRole("button", { name: "Detail cuti Anggota A" })[2]);
    expect(within(screen.getByRole("dialog")).getByText("Ditolak oleh Atasan A: Stok opname")).toBeTruthy();
    fireEvent.keyDown(document.activeElement ?? document.body, { key: "Escape" });

    fireEvent.click(screen.getByRole("button", { name: "Detail lembur Anggota A" }));
    const ot = screen.getByRole("dialog");
    expect(within(ot).getByText("Closing stok akhir bulan")).toBeTruthy();
    expect(within(ot).getByText("Disetujui oleh Atasan A")).toBeTruthy();
    fireEvent.keyDown(document.activeElement ?? document.body, { key: "Escape" });

    // EPIC-091: kasbon/pinjaman tim, Manager tetap penyetuju.
    expect(screen.getByText("Menunggu Manager: Manager Area")).toBeTruthy();
    expect(screen.getByText("Menunggu SPV: SPV Toko")).toBeTruthy();
    expect(screen.getAllByText("Ditolak").length).toBe(2);
    fireEvent.click(screen.getAllByRole("button", { name: "Detail kasbon Anggota A" })[0]);
    const loan = screen.getByRole("dialog");
    expect(within(loan).getByText("Biaya sekolah anak")).toBeTruthy();
    expect(within(loan).getByText(/2x/)).toBeTruthy();
    expect(within(loan).getByRole("button", { name: /^Dokumen 1/ })).toBeTruthy();
    expect(within(loan).queryByRole("button", { name: "Setujui" })).toBeNull();
    fireEvent.keyDown(document.activeElement ?? document.body, { key: "Escape" });
    fireEvent.click(screen.getByRole("button", { name: "Detail pinjaman karyawan Anggota A" }));
    expect(within(screen.getByRole("dialog")).getByText("Alasan penolakan: Masa kerja kurang")).toBeTruthy();
  });

  it("kosong dan gagal dimuat dibedakan", () => {
    history = { data: { period: { from: "2026-10-01", to: "2026-11-30", label: "1 Okt – 30 Nov 2026" }, leaves: [], overtime: [], loans: [] } };
    render(<TeamHistoryPanel />);
    expect(screen.getByText(/Belum ada cuti atau lembur tim/)).toBeTruthy();
    cleanup();
    history = { isError: true };
    render(<TeamHistoryPanel />);
    expect(screen.getByRole("alert").textContent).toMatch(/Gagal memuat riwayat/);
  });
});

// EPIC-092 R7: cuti berjenjang — tahap SPV yang sudah lewat terlihat di Riwayat & Detail.
describe("TeamHistoryPanel — tahap SPV (EPIC-092)", () => {
  it("pending yang sudah disetujui SPV: lencana menunggu Manager dan nama SPV di Detail", () => {
    history = {
      data: {
        period: { from: "2026-10-01", to: "2026-11-30", label: "1 Okt – 30 Nov 2026" },
        leaves: [{ ...base, id: "l9", start_date: "2026-11-25", end_date: "2026-11-25", status: "pending",
          attachment_review: null, spv_approved_at: "2026-11-05T03:00:00Z", spv_approved_name: "SPV Toko" }],
        loans: [], overtime: [],
      },
    };
    render(<TeamHistoryPanel />);
    expect(screen.getByText("Disetujui SPV — menunggu Manager")).toBeTruthy();
    fireEvent.click(screen.getAllByRole("button", { name: /Detail/ })[0]);
    expect(within(screen.getByRole("dialog")).getByText("Disetujui SPV oleh SPV Toko")).toBeTruthy();
  });
});
