import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { AttendanceGapSummary } from "../types";
import { AttendanceGapSummaryCard } from "./attendance-gap-summary-card";

const summaryFor = vi.fn();
let state: { data?: AttendanceGapSummary; isLoading: boolean; isError: boolean; error?: Error | null } = {
  isLoading: false,
  isError: false,
};
vi.mock("../queries", () => ({
  useAttendanceGapSummary: (date?: string) => {
    summaryFor(date);
    return state;
  },
}));

function summary(over: Partial<AttendanceGapSummary> = {}): AttendanceGapSummary {
  return {
    date: "2026-10-25",
    missing: 5,
    no_clock_out: 2,
    unscheduled: 3,
    period_closed: false,
    employees: [
      { employee_id: "e1", name: "Budi Santoso", nip: "N01", department: "Dapur", kind: "missing", clock_in: null },
      { employee_id: "e2", name: "Sari Dewi", nip: null, department: null, kind: "no_clock_out", clock_in: "08:05" },
    ],
    ...over,
  };
}

afterEach(() => {
  cleanup();
  summaryFor.mockReset();
  state = { isLoading: false, isError: false };
});

describe("AttendanceGapSummaryCard", () => {
  it("menampilkan jumlah belum absen, belum absen pulang, dan karyawan tanpa jadwal", () => {
    state = { data: summary(), isLoading: false, isError: false };
    render(<AttendanceGapSummaryCard />);
    expect(screen.getByText("Absen terlewat kemarin")).toBeTruthy();
    expect(screen.getByText("5 belum absen")).toBeTruthy();
    expect(screen.getByText("2 belum absen pulang")).toBeTruthy();
    expect(screen.getByText("3 karyawan tanpa jadwal (tidak dihitung)")).toBeTruthy();
    expect(screen.queryByText("Periode sudah ditutup")).toBeNull();
  });

  it("Lihat daftar membuka nama, NIP, jenis, dan jam masuk", () => {
    state = { data: summary(), isLoading: false, isError: false };
    render(<AttendanceGapSummaryCard />);
    expect(screen.queryByText("Budi Santoso")).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Lihat daftar" }));
    expect(screen.getByText("Budi Santoso")).toBeTruthy();
    expect(screen.getByText(/N01/)).toBeTruthy();
    expect(screen.getByText("Belum absen")).toBeTruthy();
    expect(screen.getByText("Sari Dewi")).toBeTruthy();
    expect(screen.getByText("Belum absen pulang (masuk 08:05)")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Sembunyikan daftar" }));
    expect(screen.queryByText("Budi Santoso")).toBeNull();
  });

  it("tanpa absen terlewat menampilkan Tidak ada absen terlewat dan tanpa tombol daftar", () => {
    state = {
      data: summary({ missing: 0, no_clock_out: 0, unscheduled: 0, employees: [] }),
      isLoading: false,
      isError: false,
    };
    render(<AttendanceGapSummaryCard />);
    expect(screen.getByText("Tidak ada absen terlewat")).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Lihat daftar" })).toBeNull();
    expect(screen.queryByText(/tanpa jadwal/)).toBeNull();
  });

  it("menyebut periode yang sudah ditutup", () => {
    state = { data: summary({ period_closed: true }), isLoading: false, isError: false };
    render(<AttendanceGapSummaryCard />);
    expect(screen.getByText("Periode sudah ditutup")).toBeTruthy();
  });

  it("status error menampilkan pesan tanpa crash", () => {
    state = { isLoading: false, isError: true, error: new Error("Tanggal tidak valid") };
    render(<AttendanceGapSummaryCard />);
    expect(screen.getByText("Tanggal tidak valid")).toBeTruthy();
  });

  it("status memuat menampilkan teks memuat", () => {
    state = { isLoading: true, isError: false };
    render(<AttendanceGapSummaryCard />);
    expect(screen.getByText("Memuat ringkasan…")).toBeTruthy();
  });

  it("input tanggal (default kemarin) mengubah query", () => {
    vi.useFakeTimers({ toFake: ["Date"] });
    vi.setSystemTime(new Date("2026-10-26T03:00:00Z"));
    try {
      state = { data: summary(), isLoading: false, isError: false };
      render(<AttendanceGapSummaryCard />);
      expect(summaryFor).toHaveBeenLastCalledWith("2026-10-25");
      const input = screen.getByLabelText("Tanggal absen terlewat") as HTMLInputElement;
      expect(input.value).toBe("2026-10-25");
      fireEvent.change(input, { target: { value: "2026-10-20" } });
      expect(summaryFor).toHaveBeenLastCalledWith("2026-10-20");
      expect(screen.getByText("Absen terlewat 20 Okt 2026")).toBeTruthy();
    } finally {
      vi.useRealTimers();
    }
  });

  it("input tanggal dibatasi 62 hari ke belakang sampai kemarin (WIB)", () => {
    vi.useFakeTimers({ toFake: ["Date"] });
    // 26 Okt 2026 01.00 WIB (25 Okt UTC): hari ini WIB tetap 26 Okt.
    vi.setSystemTime(new Date("2026-10-25T18:00:00Z"));
    try {
      state = { data: summary(), isLoading: false, isError: false };
      render(<AttendanceGapSummaryCard />);
      const input = screen.getByLabelText("Tanggal absen terlewat") as HTMLInputElement;
      expect(input.getAttribute("max")).toBe("2026-10-25");
      expect(input.getAttribute("min")).toBe("2026-08-25");
    } finally {
      vi.useRealTimers();
    }
  });
});
