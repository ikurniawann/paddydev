import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { AttendanceRequestDialog } from "./attendance-request-dialog";

const mutate = vi.fn();
const toastError = vi.fn();
vi.mock("sonner", () => ({ toast: { error: (m: string) => toastError(m), success: vi.fn() } }));
vi.mock("../mutations", () => ({ useCreateAttendanceRequest: () => ({ mutate, isPending: false }) }));
vi.mock("@/lib/dates", async (orig) => ({ ...(await orig<typeof import("@/lib/dates")>()), todayWib: () => "2026-10-05" }));

afterEach(() => {
  cleanup();
  mutate.mockReset();
  toastError.mockReset();
});

describe("AttendanceRequestDialog", () => {
  it("diisi dari absen terlewat dan hanya mengirim jam yang diisi", () => {
    render(
      <AttendanceRequestDialog
        prefill={{ date: "2026-10-03", hint: "Tercatat masuk 08:05 tanpa absen pulang — isi jam pulang yang benar." }}
        onClose={() => {}}
      />
    );
    expect((screen.getByLabelText("Tanggal") as HTMLInputElement).value).toBe("2026-10-03");
    expect(screen.getByText(/Tercatat masuk 08:05/)).toBeTruthy();
    fireEvent.change(screen.getByLabelText("Jam pulang"), { target: { value: "17:00" } });
    fireEvent.change(screen.getByLabelText("Keterangan"), { target: { value: "  Lupa absen pulang, HP mati " } });
    fireEvent.click(screen.getByRole("button", { name: "Kirim pengajuan" }));
    expect(mutate).toHaveBeenCalledWith(
      { date: "2026-10-03", clock_in: null, clock_out: "17:00", reason: "Lupa absen pulang, HP mati" },
      expect.anything()
    );
  });

  it("menolak tanggal masa depan dan keterangan pendek sebelum mengirim", () => {
    render(<AttendanceRequestDialog prefill={{ date: "2026-10-06" }} onClose={() => {}} />);
    fireEvent.change(screen.getByLabelText("Jam masuk"), { target: { value: "08:00" } });
    fireEvent.change(screen.getByLabelText("Keterangan"), { target: { value: "Internet mati" } });
    fireEvent.click(screen.getByRole("button", { name: "Kirim pengajuan" }));
    expect(toastError).toHaveBeenLastCalledWith("Tanggal pengajuan tidak boleh di masa depan");
    fireEvent.change(screen.getByLabelText("Tanggal"), { target: { value: "2026-10-02" } });
    fireEvent.change(screen.getByLabelText("Keterangan"), { target: { value: "Lupa" } });
    fireEvent.click(screen.getByRole("button", { name: "Kirim pengajuan" }));
    expect(toastError).toHaveBeenLastCalledWith("Keterangan wajib diisi (minimal 5 karakter)");
    expect(mutate).not.toHaveBeenCalled();
  });

  it("menolak keterangan lebih dari 1000 karakter dengan pesan jelas", () => {
    render(<AttendanceRequestDialog prefill={{ date: "2026-10-03" }} onClose={() => {}} />);
    fireEvent.change(screen.getByLabelText("Jam masuk"), { target: { value: "08:00" } });
    // jsdom tidak menerapkan maxLength pada nilai yang diset programatik, seperti tempel paksa
    fireEvent.change(screen.getByLabelText("Keterangan"), { target: { value: "😀".repeat(501) } });
    fireEvent.click(screen.getByRole("button", { name: "Kirim pengajuan" }));
    expect(toastError).toHaveBeenLastCalledWith("Keterangan maksimal 1000 karakter");
    expect(mutate).not.toHaveBeenCalled();
  });

  it("menolak lampiran bukan gambar dan lampiran di atas 5 MB tanpa mengirim", () => {
    render(<AttendanceRequestDialog prefill={{ date: "2026-10-03" }} onClose={() => {}} />);
    const input = screen.getByLabelText(/Foto bukti/) as HTMLInputElement;
    fireEvent.change(input, { target: { files: [new File(["x"], "a.pdf", { type: "application/pdf" })] } });
    expect(toastError).toHaveBeenLastCalledWith("Lampiran harus berupa gambar JPG/PNG/WebP");
    const big = new File(["x"], "besar.jpg", { type: "image/jpeg" });
    Object.defineProperty(big, "size", { value: 5 * 1024 * 1024 + 1 });
    fireEvent.change(input, { target: { files: [big] } });
    expect(toastError).toHaveBeenLastCalledWith("Ukuran lampiran maksimal 5 MB");
    expect(screen.queryByText(/besar\.jpg/)).toBeNull();
  });
});
