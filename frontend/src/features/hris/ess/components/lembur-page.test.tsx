import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { OVERTIME_TOO_LATE } from "@/lib/hris/overtime-rules";
import type { EssOvertimeDayTypeHint } from "../types";
import { EssLemburPage } from "./lembur-page";

const submit = vi.fn();
const toastError = vi.fn();
let hint: EssOvertimeDayTypeHint | null = null;

vi.mock("sonner", () => ({ toast: { error: (m: string) => toastError(m), success: vi.fn() } }));
vi.mock("../queries", () => ({
  useEssMe: () => ({ data: { employee: { id: "me1" } }, isLoading: false }),
  useMyOvertime: () => ({ data: [] }),
  useOvertimeDayType: (date: string) => ({ data: date ? (hint ?? undefined) : undefined }),
}));
vi.mock("../mutations", () => ({
  useSubmitOvertime: () => ({ mutate: submit, isPending: false }),
  useDecideOvertime: () => ({ mutate: vi.fn(), isPending: false, variables: undefined }),
}));

function fill(date: string) {
  fireEvent.click(screen.getByRole("button", { name: /Ajukan Lembur/ }));
  fireEvent.change(screen.getByLabelText("Tanggal"), { target: { value: date } });
  fireEvent.change(screen.getByLabelText("Jam Mulai"), { target: { value: "17:00" } });
  fireEvent.change(screen.getByLabelText("Jam Selesai"), { target: { value: "19:00" } });
  fireEvent.change(screen.getByLabelText(/Alasan/), { target: { value: "Closing stok bulanan" } });
}

afterEach(() => {
  cleanup();
  submit.mockReset();
  toastError.mockReset();
  hint = null;
});

describe("EssLemburPage — EPIC-063", () => {
  it("menolak jam mulai yang sudah lewat sebelum mengirim", () => {
    render(<EssLemburPage />);
    fill("2020-01-06");
    fireEvent.click(screen.getByRole("button", { name: "Kirim Pengajuan" }));
    expect(toastError).toHaveBeenCalledWith(OVERTIME_TOO_LATE);
    expect(submit).not.toHaveBeenCalled();
  });

  it("jenis dikunci ke saran jadwal dan ikut terkirim", () => {
    hint = { date: "2099-01-04", day_type: "rest_day", label: "Weekend / Libur" };
    render(<EssLemburPage />);
    fill("2099-01-04");
    const select = screen.getByLabelText("Jenis lembur") as HTMLSelectElement;
    expect(select.disabled).toBe(true);
    expect(select.value).toBe("rest_day");
    expect(screen.getByText("Sesuai jadwal Anda: Weekend / Libur")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Kirim Pengajuan" }));
    expect(submit).toHaveBeenCalledWith(
      expect.objectContaining({ date: "2099-01-04", start_time: "17:00", day_type: "rest_day" }),
      expect.anything()
    );
  });

  it("tanpa jadwal karyawan memilih sendiri", () => {
    hint = { date: "2099-01-05", day_type: null, label: null };
    render(<EssLemburPage />);
    fill("2099-01-05");
    const select = screen.getByLabelText("Jenis lembur") as HTMLSelectElement;
    expect(select.disabled).toBe(false);
    fireEvent.change(select, { target: { value: "rest_day" } });
    fireEvent.click(screen.getByRole("button", { name: "Kirim Pengajuan" }));
    expect(submit).toHaveBeenCalledWith(expect.objectContaining({ day_type: "rest_day" }), expect.anything());
  });
});
