import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { TeamApprovals, TeamLeaveRequest, TeamOvertimeRequest } from "../types";
import { EssPersetujuanTimPage } from "./persetujuan-tim-page";

const decide = vi.fn();
const toastError = vi.fn();
const toastSuccess = vi.fn();
let queue: TeamApprovals | undefined = { leaves: [], overtime: [] };
let queueError = false;
let tripsError = false;
let loansError = false;
let loans: { id: string }[] = [];
let trips: { id: string }[] = [];

vi.mock("sonner", () => ({
  toast: { error: (m: string) => toastError(m), success: (m: string) => toastSuccess(m) },
}));
vi.mock("@/features/hris/ess/queries", () => ({
  useEssMe: () => ({ data: { employee: { id: "boss" } }, isLoading: false }),
  useTeamLoanApprovals: () => ({ data: loans, isError: loansError }),
}));
vi.mock("@/features/hris/business-trips", () => ({
  TeamTripApprovals: () => (trips.length > 0 ? <p>Antrean Perjalanan Dinas</p> : null),
  useBusinessTrips: () => ({ data: trips, isError: tripsError }),
}));
vi.mock("../queries", () => ({
  useTeamApprovals: () => ({ data: queue, isLoading: false, isError: queueError }),
}));
vi.mock("../mutations", () => ({
  useDecideTeamRequest: () => ({ mutate: decide, isPending: false, variables: undefined }),
}));

const leave: TeamLeaveRequest = {
  id: "l1", employee_id: "e1", leave_type: "annual", start_date: "2031-03-04", end_date: "2031-03-05",
  total_days: "2.00", reason: "Acara keluarga", attachment_url: null, created_at: "2031-03-01T00:00:00Z",
  employee: { id: "e1", full_name: "Anggota A", nip: "N01" },
};
const overtime: TeamOvertimeRequest = {
  id: "o1", employee_id: "e2", date: "2026-10-06", start_time: "17:00:00", end_time: "19:30:00", hours: "2.50",
  day_type: "rest_day", break_minutes: 0, attachments: ["overtime-attachments/e2/1-aaaaaaaaaaaaaaaa.pdf"],
  reason: "Stock opname", created_at: "2026-10-05T00:00:00Z",
  employee: { id: "e2", full_name: "Anggota B", nip: "N02" },
};

afterEach(() => {
  cleanup();
  decide.mockReset();
  toastError.mockReset();
  toastSuccess.mockReset();
  queue = { leaves: [], overtime: [] };
  queueError = tripsError = loansError = false;
  loans = [];
  trips = [];
});

describe("EssPersetujuanTimPage", () => {
  it("menampilkan cuti & lembur tim dan menyetujui cuti langsung", () => {
    queue = { leaves: [leave], overtime: [overtime] };
    decide.mockImplementation((_v, opts) => opts.onSuccess());
    render(<EssPersetujuanTimPage />);
    expect(screen.getByText(/Anggota A · Cuti Tahunan/)).toBeTruthy();
    expect(screen.getByText(/17:00–19:30 \(2,5 jam\) · Weekend \/ Libur/)).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Setujui cuti Anggota A" }));
    expect(decide).toHaveBeenCalledWith(
      { kind: "leave", id: "l1", action: "approve", rejectionReason: undefined },
      expect.anything()
    );
    expect(toastSuccess).toHaveBeenCalledWith("Cuti disetujui");
  });

  it("lembur: ringkasan menampilkan istirahat & dokumentasi; Setujui tanpa perubahan tidak mengirim jam (EPIC-066)", () => {
    queue = { leaves: [], overtime: [overtime] };
    decide.mockImplementation((_v, opts) => opts.onSuccess());
    render(<EssPersetujuanTimPage />);
    expect(screen.getByText(/istirahat 0 menit · 1 dokumentasi/)).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Setujui lembur Anggota B" }));
    expect(decide).not.toHaveBeenCalled(); // membuka dialog dulu
    fireEvent.click(screen.getByRole("button", { name: "Setujui" }));
    expect(decide).toHaveBeenCalledWith(
      { kind: "overtime", id: "o1", action: "approve", times: undefined },
      expect.anything()
    );
    expect(toastSuccess).toHaveBeenCalledWith("Lembur disetujui");
  });

  it("lembur: hanya mengirim kolom yang berbeda dari nilai tersimpan (ubah jam selesai saja)", () => {
    queue = { leaves: [], overtime: [overtime] };
    render(<EssPersetujuanTimPage />);
    fireEvent.click(screen.getByRole("button", { name: "Setujui lembur Anggota B" }));
    fireEvent.change(screen.getByLabelText("Jam Selesai"), { target: { value: "19:00" } });
    fireEvent.click(screen.getByRole("button", { name: "Setujui" }));
    expect(decide).toHaveBeenCalledWith(
      { kind: "overtime", id: "o1", action: "approve", times: { end_time: "19:00" } },
      expect.anything()
    );
  });

  it("menolak lembur wajib alasan lewat dialog", () => {
    queue = { leaves: [], overtime: [overtime] };
    render(<EssPersetujuanTimPage />);
    fireEvent.click(screen.getByRole("button", { name: "Tolak lembur Anggota B" }));
    fireEvent.click(screen.getByRole("button", { name: "Tolak pengajuan" }));
    expect(toastError).toHaveBeenCalledWith("Alasan penolakan wajib diisi");
    expect(decide).not.toHaveBeenCalled();
    fireEvent.change(screen.getByLabelText("Alasan penolakan"), { target: { value: "  Tidak ada stok masuk " } });
    fireEvent.click(screen.getByRole("button", { name: "Tolak pengajuan" }));
    expect(decide).toHaveBeenCalledWith(
      { kind: "overtime", id: "o1", action: "reject", rejectionReason: "Tidak ada stok masuk" },
      expect.anything()
    );
  });

  it("menampilkan pesan error server apa adanya", () => {
    queue = { leaves: [leave], overtime: [] };
    decide.mockImplementation((_v, opts) => opts.onError(new Error("Tidak bisa memutuskan pengajuan cuti sendiri")));
    render(<EssPersetujuanTimPage />);
    fireEvent.click(screen.getByRole("button", { name: "Setujui cuti Anggota A" }));
    expect(toastError).toHaveBeenCalledWith("Tidak bisa memutuskan pengajuan cuti sendiri");
  });

  it("Perjalanan Dinas memakai antrean bersama, kasbon ditautkan ke Pinjaman", () => {
    loans = [{ id: "k1" }, { id: "k2" }];
    trips = [{ id: "t1" }];
    render(<EssPersetujuanTimPage />);
    expect(screen.getByText("Antrean Perjalanan Dinas")).toBeTruthy();
    const link = screen.getByRole("link", { name: /2 kasbon tim menunggu persetujuan Anda/ });
    expect(link.getAttribute("href")).toBe("/dashboard/me/pinjaman");
    expect(screen.queryByText("Tidak ada pengajuan tim yang menunggu persetujuan Anda.")).toBeNull();
  });

  it("menampilkan keadaan kosong", () => {
    render(<EssPersetujuanTimPage />);
    expect(screen.getByText("Tidak ada pengajuan tim yang menunggu persetujuan Anda.")).toBeTruthy();
  });

  const LOAD_ERROR = "Gagal memuat antrean persetujuan. Coba muat ulang.";
  const EMPTY = "Tidak ada pengajuan tim yang menunggu persetujuan Anda.";

  it("antrean gagal dimuat menampilkan pesan error, bukan keadaan kosong", () => {
    queue = undefined;
    queueError = true;
    render(<EssPersetujuanTimPage />);
    expect(screen.getByText(LOAD_ERROR)).toBeTruthy();
    expect(screen.queryByText(EMPTY)).toBeNull();
  });

  it("antrean Perjalanan Dinas atau kasbon gagal dimuat juga bukan keadaan kosong", () => {
    tripsError = true;
    const { unmount } = render(<EssPersetujuanTimPage />);
    expect(screen.getByText(LOAD_ERROR)).toBeTruthy();
    expect(screen.queryByText(EMPTY)).toBeNull();
    unmount();
    tripsError = false;
    loansError = true;
    render(<EssPersetujuanTimPage />);
    expect(screen.getByText(LOAD_ERROR)).toBeTruthy();
    expect(screen.queryByText(EMPTY)).toBeNull();
  });

  it("tanpa error, keadaan kosong tampil dan pesan error tidak", () => {
    render(<EssPersetujuanTimPage />);
    expect(screen.getByText(EMPTY)).toBeTruthy();
    expect(screen.queryByText(LOAD_ERROR)).toBeNull();
  });

  it("cuti dengan lampiran menampilkan pratinjau berkas privat; tanpa lampiran tidak (EPIC-072)", () => {
    queue = {
      leaves: [
        { ...leave, attachment_url: "leave-attachments/e1/surat dokter.png" },
        { ...leave, id: "l2", employee: { id: "e3", full_name: "Anggota C", nip: "N03" }, attachment_url: null },
      ],
      overtime: [],
    };
    render(<EssPersetujuanTimPage />);
    const buttons = screen.getAllByRole("button", { name: "Lampiran 1 cuti Anggota A" });
    expect(buttons).toHaveLength(1);
    fireEvent.click(buttons[0]);
    expect(screen.getByAltText("Lampiran 1").getAttribute("src")).toBe(
      "/api/hris/leaves/attachment/leave-attachments/e1/surat%20dokter.png");
  });

  it("Detail cuti: alasan utuh + lampiran, lalu Setujui dari dialog (EPIC-072)", () => {
    const reason = "Acara keluarga di luar kota.\nBerangkat pagi, kembali lusa malam.";
    queue = { leaves: [{ ...leave, reason, attachment_url: "leave-attachments/e1/x.png" }], overtime: [] };
    decide.mockImplementation((_v, opts) => opts.onSuccess());
    render(<EssPersetujuanTimPage />);
    fireEvent.click(screen.getByRole("button", { name: "Detail cuti Anggota A" }));
    const dialog = screen.getByRole("dialog");
    expect(within(dialog).getByText((_, el) => el?.tagName === "P" && el.textContent === reason)).toBeTruthy();
    expect(within(dialog).getByRole("button", { name: "Lampiran 1" })).toBeTruthy();
    expect(within(dialog).getByText("2 hari")).toBeTruthy();
    fireEvent.click(within(dialog).getByRole("button", { name: "Setujui" }));
    expect(decide).toHaveBeenCalledWith(
      { kind: "leave", id: "l1", action: "approve", rejectionReason: undefined },
      expect.anything()
    );
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("Detail lembur: alasan + dokumentasi; Tolak membuka dialog alasan dengan ringkasan pengajuan (EPIC-072)", () => {
    queue = { leaves: [], overtime: [overtime] };
    render(<EssPersetujuanTimPage />);
    fireEvent.click(screen.getByRole("button", { name: "Detail lembur Anggota B" }));
    const dialog = screen.getByRole("dialog");
    expect(within(dialog).getByText("Stock opname")).toBeTruthy();
    expect(within(dialog).getByRole("button", { name: "Dokumentasi 1" })).toBeTruthy();
    fireEvent.click(within(dialog).getByRole("button", { name: "Tolak" }));
    expect(screen.getByLabelText("Alasan penolakan")).toBeTruthy();
    expect(screen.getByText(/Alasan pengajuan: Stock opname/)).toBeTruthy();
  });

  it("Setujui lembur menampilkan alasan pengajuan di dialog koreksi (EPIC-072 R3)", () => {
    queue = { leaves: [], overtime: [overtime] };
    render(<EssPersetujuanTimPage />);
    fireEvent.click(screen.getByRole("button", { name: "Setujui lembur Anggota B" }));
    expect(within(screen.getByRole("dialog")).getByText("Stock opname")).toBeTruthy();
  });

  it("Detail tertutup bila pengajuan sudah diputus penyetuju lain (data antrean terbaru) (EPIC-072 sisa)", () => {
    queue = { leaves: [leave], overtime: [] };
    const { rerender } = render(<EssPersetujuanTimPage />);
    fireEvent.click(screen.getByRole("button", { name: "Detail cuti Anggota A" }));
    expect(screen.getByRole("dialog")).toBeTruthy();
    queue = { leaves: [], overtime: [] };
    rerender(<EssPersetujuanTimPage />);
    expect(screen.queryByRole("dialog")).toBeNull();
  });
});

