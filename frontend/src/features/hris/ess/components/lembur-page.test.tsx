import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { EssOvertimeDayTypeHint } from "../types";

const submit = vi.fn();
const toastError = vi.fn();
let hint: EssOvertimeDayTypeHint | null = null;

vi.mock("sonner", () => ({ toast: { error: (m: string) => toastError(m), success: vi.fn() } }));
vi.mock("../queries", () => ({
  useEssMe: () => ({ data: { employee: { id: "e1" } }, isLoading: false }),
  useMyOvertime: () => ({ data: [] }),
  useOvertimeDayType: (date: string) => ({ data: date ? (hint ?? undefined) : undefined }),
}));
vi.mock("../mutations", () => ({
  useSubmitOvertime: () => ({ mutate: submit, isPending: false }),
  useDecideOvertime: () => ({ mutate: vi.fn(), isPending: false, variables: undefined }),
}));
const upload = vi.fn();
vi.mock("../api", () => ({ uploadOvertimeAttachment: (f: File) => upload(f) }));

import { EssLemburPage } from "./lembur-page";

const PATH = "overtime-attachments/e1/1-abcdefabcdefabcd.pdf";
beforeEach(() => upload.mockResolvedValue(PATH));

function fill(date: string) {
  fireEvent.click(screen.getByRole("button", { name: /Input Lembur/ }));
  fireEvent.change(screen.getByLabelText("Tanggal"), { target: { value: date } });
  fireEvent.change(screen.getByLabelText("Jam Mulai"), { target: { value: "17:00" } });
  fireEvent.change(screen.getByLabelText("Jam Selesai"), { target: { value: "19:00" } });
}

async function attach() {
  const file = new File(["x"], "bukti.pdf", { type: "application/pdf" });
  fireEvent.change(screen.getByLabelText("Pilih file dokumentasi lembur"), { target: { files: [file] } });
  await screen.findByText(/Dokumentasi 1 \(PDF\)/);
}

afterEach(() => {
  cleanup();
  submit.mockReset();
  upload.mockReset();
  toastError.mockReset();
  hint = null;
  vi.useRealTimers();
});

describe("ESS Input Lembur (EPIC-066)", () => {
  it("menolak lembur yang belum selesai dan tanpa dokumentasi; menampilkan jam dibayar", () => {
    vi.setSystemTime(new Date("2026-10-21T18:00:00+07:00"));
    render(<EssLemburPage />);
    fireEvent.click(screen.getByRole("button", { name: /Input Lembur/ }));
    fireEvent.change(screen.getByLabelText("Tanggal"), { target: { value: "2026-10-21" } });
    fireEvent.change(screen.getByLabelText("Jam Mulai"), { target: { value: "17:00" } });
    fireEvent.change(screen.getByLabelText("Jam Selesai"), { target: { value: "20:00" } });
    fireEvent.change(screen.getByLabelText("Istirahat (menit)"), { target: { value: "30" } });
    expect(screen.getByText(/2,5 jam dibayar/)).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Kirim" }));
    expect(toastError).toHaveBeenCalledWith("Lembur baru bisa diinput setelah selesai (21-10-2026 20:00 WIB)");
    vi.setSystemTime(new Date("2026-10-22T08:00:00+07:00"));
    fireEvent.click(screen.getByRole("button", { name: "Kirim" }));
    expect(toastError).toHaveBeenLastCalledWith("Lampirkan minimal 1 dokumentasi lembur");
    expect(submit).not.toHaveBeenCalled();
  });

  it("menolak istirahat di luar 0–720 menit dan durasi dibayar di bawah 1 jam", () => {
    render(<EssLemburPage />);
    fill("2020-01-06");
    fireEvent.change(screen.getByLabelText("Istirahat (menit)"), { target: { value: "900" } });
    fireEvent.click(screen.getByRole("button", { name: "Kirim" }));
    expect(toastError).toHaveBeenLastCalledWith("Istirahat 0–720 menit");
    fireEvent.change(screen.getByLabelText("Istirahat (menit)"), { target: { value: "90" } });
    fireEvent.click(screen.getByRole("button", { name: "Kirim" }));
    expect(toastError).toHaveBeenLastCalledWith("Durasi lembur minimal 1 jam setelah dikurangi istirahat");
    expect(submit).not.toHaveBeenCalled();
  });

  it("mengirim lembur selesai dengan istirahat dan dokumentasi tanpa keterangan", async () => {
    render(<EssLemburPage />);
    fill("2020-01-06");
    fireEvent.change(screen.getByLabelText("Istirahat (menit)"), { target: { value: "30" } });
    await attach();
    fireEvent.click(screen.getByRole("button", { name: "Kirim" }));
    expect(toastError).not.toHaveBeenCalled();
    expect(submit).toHaveBeenCalledWith(
      expect.objectContaining({
        date: "2020-01-06",
        break_minutes: "30",
        reason: "",
        attachments: ["overtime-attachments/e1/1-abcdefabcdefabcd.pdf"],
      }),
      expect.anything()
    );
  });

  it("Kirim nonaktif selama dokumentasi diunggah", async () => {
    let finish: (path: string) => void = () => {};
    upload.mockImplementation(() => new Promise<string>((resolve) => { finish = resolve; }));
    render(<EssLemburPage />);
    fill("2020-01-06");
    const file = new File(["x"], "bukti.pdf", { type: "application/pdf" });
    fireEvent.change(screen.getByLabelText("Pilih file dokumentasi lembur"), { target: { files: [file] } });
    const kirim = screen.getByRole("button", { name: "Kirim" }) as HTMLButtonElement;
    await waitFor(() => expect(kirim.disabled).toBe(true));
    fireEvent.click(kirim);
    expect(submit).not.toHaveBeenCalled();
    finish(PATH);
    await screen.findByText(/Dokumentasi 1 \(PDF\)/);
    expect((screen.getByRole("button", { name: "Kirim" }) as HTMLButtonElement).disabled).toBe(false);
  });

  it("unggahan yang selesai setelah form dikirim tidak masuk ke form baru", async () => {
    let finish: (path: string) => void = () => {};
    let sent: { onSuccess: () => void } | undefined;
    submit.mockImplementation((_v, opts) => { sent = opts; });
    render(<EssLemburPage />);
    fill("2020-01-06");
    await attach();
    fireEvent.click(screen.getByRole("button", { name: "Kirim" }));
    expect(submit).toHaveBeenCalledTimes(1);
    // Unggahan kedua dimulai saat pengiriman masih berjalan, selesai setelah form direset.
    upload.mockImplementation(() => new Promise<string>((resolve) => { finish = resolve; }));
    const file = new File(["x"], "kedua.pdf", { type: "application/pdf" });
    fireEvent.change(screen.getByLabelText("Pilih file dokumentasi lembur"), { target: { files: [file] } });
    act(() => sent!.onSuccess());
    await act(async () => finish("overtime-attachments/e1/2-abcdefabcdefabcd.pdf"));
    fireEvent.click(screen.getByRole("button", { name: /Input Lembur/ }));
    expect(screen.queryByText(/Dokumentasi \d \(PDF\)/)).toBeNull();
    expect((screen.getByRole("button", { name: "Kirim" }) as HTMLButtonElement).disabled).toBe(false);
  });

  it("dokumentasi dapat dihapus", async () => {
    render(<EssLemburPage />);
    fill("2020-01-06");
    await attach();
    fireEvent.click(screen.getByRole("button", { name: "Hapus dokumentasi 1" }));
    expect(screen.queryByText(/Dokumentasi 1 \(PDF\)/)).toBeNull();
  });
});

describe("EssLemburPage — jenis lembur (EPIC-063)", () => {
  it("jenis dikunci ke saran jadwal dan ikut terkirim", async () => {
    hint = { date: "2020-01-04", day_type: "rest_day", label: "Weekend / Libur" };
    render(<EssLemburPage />);
    fill("2020-01-04");
    await attach();
    const select = screen.getByLabelText("Jenis lembur") as HTMLSelectElement;
    expect(select.disabled).toBe(true);
    expect(select.value).toBe("rest_day");
    expect(screen.getByText("Sesuai jadwal Anda: Weekend / Libur")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Kirim" }));
    expect(submit).toHaveBeenCalledWith(
      expect.objectContaining({ date: "2020-01-04", start_time: "17:00", day_type: "rest_day" }),
      expect.anything()
    );
  });

  it("tanpa jadwal karyawan memilih sendiri", async () => {
    hint = { date: "2020-01-05", day_type: null, label: null };
    render(<EssLemburPage />);
    fill("2020-01-05");
    await attach();
    const select = screen.getByLabelText("Jenis lembur") as HTMLSelectElement;
    expect(select.disabled).toBe(false);
    fireEvent.change(select, { target: { value: "rest_day" } });
    fireEvent.click(screen.getByRole("button", { name: "Kirim" }));
    expect(submit).toHaveBeenCalledWith(expect.objectContaining({ day_type: "rest_day" }), expect.anything());
  });
});
