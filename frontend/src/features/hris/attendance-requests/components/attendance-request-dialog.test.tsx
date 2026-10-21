import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { AttendanceRequestDialog } from "./attendance-request-dialog";

const mutate = vi.fn();
const toastError = vi.fn();
const toastSuccess = vi.fn();
let pending = false;
vi.mock("sonner", () => ({ toast: { error: (m: string) => toastError(m), success: (m: string) => toastSuccess(m) } }));
vi.mock("../mutations", () => ({ useCreateAttendanceRequest: () => ({ mutate, isPending: pending }) }));
vi.mock("@/lib/dates", async (orig) => ({ ...(await orig<typeof import("@/lib/dates")>()), todayWib: () => "2026-10-05" }));

afterEach(() => {
  cleanup();
  mutate.mockReset();
  toastError.mockReset();
  toastSuccess.mockReset();
  pending = false;
  vi.unstubAllGlobals();
});

/** Isi minimum yang valid agar tombol kirim mencapai mutate. */
function fillValid() {
  fireEvent.change(screen.getByLabelText("Jam masuk"), { target: { value: "08:00" } });
  fireEvent.change(screen.getByLabelText("Keterangan"), { target: { value: "Internet toko mati" } });
}

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
  it("menolak GIF (di luar JPG/PNG/WebP) tanpa mengirim", () => {
    render(<AttendanceRequestDialog prefill={{ date: "2026-10-03" }} onClose={() => {}} />);
    const input = screen.getByLabelText(/Foto bukti/) as HTMLInputElement;
    fireEvent.change(input, { target: { files: [new File(["x"], "anim.gif", { type: "image/gif" })] } });
    expect(toastError).toHaveBeenLastCalledWith("Lampiran harus berupa gambar JPG/PNG/WebP");
    expect(screen.queryByText(/anim\.gif/)).toBeNull();
    expect(mutate).not.toHaveBeenCalled();
  });

  it("foto berhasil dibaca dikirim sebagai photo di payload", async () => {
    render(<AttendanceRequestDialog prefill={{ date: "2026-10-03" }} onClose={() => {}} />);
    fillValid();
    const input = screen.getByLabelText(/Foto bukti/) as HTMLInputElement;
    fireEvent.change(input, { target: { files: [new File(["abc"], "bukti.png", { type: "image/png" })] } });
    await screen.findByText(/bukti\.png/);
    fireEvent.click(screen.getByRole("button", { name: "Kirim pengajuan" }));
    expect(mutate).toHaveBeenCalledWith(
      expect.objectContaining({ date: "2026-10-03", photo: expect.stringMatching(/^data:image\/png;base64,/) }),
      expect.anything()
    );
  });

  it("gagal membaca foto: tanpa chip terlampir, ada pesan, dan foto tidak terkirim", () => {
    class FailingReader {
      result: string | null = null;
      onload: (() => void) | null = null;
      onerror: (() => void) | null = null;
      onloadend: (() => void) | null = null;
      readAsDataURL() {
        queueMicrotask(() => {
          this.onerror?.();
          this.onloadend?.();
        });
      }
    }
    vi.stubGlobal("FileReader", FailingReader);
    render(<AttendanceRequestDialog prefill={{ date: "2026-10-03" }} onClose={() => {}} />);
    fillValid();
    const input = screen.getByLabelText(/Foto bukti/) as HTMLInputElement;
    fireEvent.change(input, { target: { files: [new File(["abc"], "rusak.png", { type: "image/png" })] } });
    return waitFor(() => expect(toastError).toHaveBeenLastCalledWith("Gagal membaca foto bukti")).then(() => {
      expect(screen.queryByText(/rusak\.png/)).toBeNull();
      fireEvent.click(screen.getByRole("button", { name: "Kirim pengajuan" }));
      expect(mutate.mock.calls[0][0]).not.toHaveProperty("photo");
    });
  });

  it("tombol kirim dinonaktifkan selama foto sedang dibaca", async () => {
    let finish: (() => void) | null = null;
    class SlowReader {
      result: string | null = null;
      onload: (() => void) | null = null;
      onerror: (() => void) | null = null;
      readAsDataURL() {
        finish = () => {
          this.result = "data:image/png;base64,AAAA";
          this.onload?.();
        };
      }
    }
    vi.stubGlobal("FileReader", SlowReader);
    render(<AttendanceRequestDialog prefill={{ date: "2026-10-03" }} onClose={() => {}} />);
    fillValid();
    const input = screen.getByLabelText(/Foto bukti/) as HTMLInputElement;
    fireEvent.change(input, { target: { files: [new File(["abc"], "lambat.png", { type: "image/png" })] } });
    const submit = screen.getByRole("button", { name: "Kirim pengajuan" }) as HTMLButtonElement;
    expect(submit.disabled).toBe(true);
    finish!();
    await waitFor(() => expect(submit.disabled).toBe(false));
    expect(screen.getByText(/lambat\.png/)).toBeTruthy();
  });

  it("berhasil: toast sukses dari server lalu dialog ditutup", () => {
    const onClose = vi.fn();
    mutate.mockImplementation((_payload, opts) => opts.onSuccess({ message: "Pengajuan absensi terkirim — menunggu persetujuan HR" }));
    render(<AttendanceRequestDialog prefill={{ date: "2026-10-03" }} onClose={onClose} />);
    fillValid();
    fireEvent.click(screen.getByRole("button", { name: "Kirim pengajuan" }));
    expect(toastSuccess).toHaveBeenCalledWith("Pengajuan absensi terkirim — menunggu persetujuan HR");
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it("gagal: pesan server tampil apa adanya dan dialog tetap terbuka", () => {
    const onClose = vi.fn();
    mutate.mockImplementation((_payload, opts) => opts.onError(new Error("Masih ada pengajuan absensi yang menunggu untuk tanggal ini")));
    render(<AttendanceRequestDialog prefill={{ date: "2026-10-03" }} onClose={onClose} />);
    fillValid();
    fireEvent.click(screen.getByRole("button", { name: "Kirim pengajuan" }));
    expect(toastError).toHaveBeenLastCalledWith("Masih ada pengajuan absensi yang menunggu untuk tanggal ini");
    expect(onClose).not.toHaveBeenCalled();
  });

  it("selama mengirim: Batal dan kirim dinonaktifkan, Escape tidak menutup dialog", () => {
    pending = true;
    const onClose = vi.fn();
    render(<AttendanceRequestDialog prefill={{ date: "2026-10-03" }} onClose={onClose} />);
    expect((screen.getByRole("button", { name: "Batal" }) as HTMLButtonElement).disabled).toBe(true);
    expect((screen.getByRole("button", { name: "Kirim pengajuan" }) as HTMLButtonElement).disabled).toBe(true);
    fireEvent.keyDown(document.activeElement ?? document.body, { key: "Escape" });
    fireEvent.click(screen.getByRole("button", { name: "Close" }));
    expect(onClose).not.toHaveBeenCalled();
  });
});
